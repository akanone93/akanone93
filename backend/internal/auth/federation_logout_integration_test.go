// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// logoutEventURI is the event a back-channel logout token carries
// (Back-Channel Logout 1.0 §2.4).
const logoutEventURI = "http://schemas.openid.net/event/backchannel-logout"

// logoutToken signs a logout token for the organisation's provider. The
// defaults are a valid token for the client issued now. An override replaces
// a claim; an override with a nil value removes it.
func (f *federated) logoutToken(t *testing.T, overrides map[string]any) string {
	t.Helper()
	now := f.clock.now()
	claims := map[string]any{
		"iss": f.idp.srv.URL, "aud": "studio", "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
		"jti": newTokenID(t), "events": map[string]any{logoutEventURI: map[string]any{}},
	}
	for k, v := range overrides {
		if v == nil {
			delete(claims, k)
			continue
		}
		claims[k] = v
	}
	return f.idp.sign(t, claims)
}

// newTokenID returns a fresh token identifier, so that tokens are not
// replays of each other.
func newTokenID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// sessionEnded reports whether a session secret no longer authenticates.
func (f *federated) sessionEnded(secret string) bool {
	_, err := f.svc.AuthenticateSession(context.Background(), secret)
	return err != nil
}

// Verifies: GOV-004, SEC-100.
// A federated sign-in records the provider's session identifier (sid) and
// the authentication context (acr, amr) against the session it created, and
// audits them, so that a later back-channel logout can find the session.
func TestFederatedSignInRecordsAuthenticationContext_GOV_004(t *testing.T) {
	t.Parallel()
	f := newFederated(t)
	f.createProvider(t)
	session := f.mustSignIn(t, "sub-bob", "bob@example.com", map[string]any{
		"sid": "sid-1", "acr": "urn:example:loa2", "amr": []string{"pwd", "otp"},
	})
	var sid, acr, sessionID string
	var amr []string
	if err := f.db.Pool().QueryRow(context.Background(),
		`SELECT sid, acr, amr, COALESCE(session_id::text, '') FROM federated_sign_ins WHERE user_id = $1`, f.bobID).
		Scan(&sid, &acr, &amr, &sessionID); err != nil {
		t.Fatalf("the federated sign-in is not recorded: %v", err)
	}
	if sid != "sid-1" || acr != "urn:example:loa2" || len(amr) != 2 || amr[0] != "pwd" || amr[1] != "otp" {
		t.Errorf("recorded sid=%q acr=%q amr=%v", sid, acr, amr)
	}
	if sessionID != session.ID {
		t.Errorf("the sign-in is recorded against session %q; want %q", sessionID, session.ID)
	}
	var detail string
	for _, e := range auditEntries(t, f.fixture, "") {
		if e.Action == audit.FederatedSignIn {
			detail = e.Detail
		}
	}
	if detail != "provider=acme-sso acr=urn:example:loa2 amr=pwd,otp" {
		t.Errorf("audit detail = %q", detail)
	}
}

// Verifies: GOV-004, SEC-100.
// A valid back-channel logout token ends the sessions that signed in with the
// named provider session and nothing else, and its replay is refused. A
// token with a nonce, a missing or wrong events claim, the wrong audience, an
// expired or future issue time, or no identifier is refused, and ends nothing.
func TestBackChannelLogout_GOV_004(t *testing.T) {
	t.Parallel()
	f := newFederated(t)
	f.createProvider(t)
	ctx := context.Background()
	ended := f.mustSignIn(t, "sub-bob", "bob@example.com", map[string]any{"sid": "sid-1"})
	kept := f.mustSignIn(t, "sub-bob", "bob@example.com", map[string]any{"sid": "sid-2"})

	for name, overrides := range map[string]map[string]any{
		"a nonce":           {"nonce": "n-1"},
		"no events":         {"events": nil},
		"another event":     {"events": map[string]any{"http://schemas.openid.net/event/other": map[string]any{}}},
		"another audience":  {"aud": "another-client"},
		"an expired token":  {"exp": f.clock.now().Add(-time.Hour).Unix()},
		"a future issue":    {"iat": f.clock.now().Add(time.Hour).Unix()},
		"no identifier":     {"jti": nil},
		"no subject or sid": {"sid": nil, "sub": nil},
	} {
		_, err := f.svc.BackChannelLogout(ctx, f.provider.ID, f.logoutToken(t, withSid(overrides, "sid-1")))
		if !errors.Is(err, auth.ErrLogoutToken) {
			t.Errorf("%s: %v; want a refused logout token", name, err)
		}
	}
	if f.sessionEnded(ended.Secret) {
		t.Error("a refused logout token ended a session")
	}

	token := f.logoutToken(t, map[string]any{"sid": "sid-1"})
	if n, err := f.svc.BackChannelLogout(ctx, f.provider.ID, token); err != nil || n != 1 {
		t.Fatalf("BackChannelLogout = %d, %v; want one session ended", n, err)
	}
	if !f.sessionEnded(ended.Secret) {
		t.Error("the named session still authenticates")
	}
	if f.sessionEnded(kept.Secret) {
		t.Error("a session the token did not name was ended")
	}
	if _, err := f.svc.BackChannelLogout(ctx, f.provider.ID, token); !errors.Is(err, auth.ErrLogoutReplayed) {
		t.Errorf("a replayed logout token: %v", err)
	}
	if !hasAudit(auditEntries(t, f.fixture, ""), audit.FederatedSignOut) {
		t.Error("no user.federated_sign_out entry in the installation's chain")
	}
}

// withSid returns the overrides with the session identifier set to sid,
// unless the overrides name one, so that a refusal is not caused by a missing
// session.
func withSid(overrides map[string]any, sid string) map[string]any {
	out := map[string]any{"sid": sid}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

// Verifies: GOV-004, SEC-100.
// A logout token that names only a subject ends every federated session of
// that person through the provider. A password session of the same person is
// not a federated session and survives.
func TestBackChannelLogoutBySubject_GOV_004(t *testing.T) {
	t.Parallel()
	f := newFederated(t)
	f.createProvider(t)
	ctx := context.Background()
	first := f.mustSignIn(t, "sub-bob", "bob@example.com", map[string]any{"sid": "sid-1"})
	second := f.mustSignIn(t, "sub-bob", "bob@example.com", map[string]any{"sid": "sid-2"})
	passwordSession, _, err := f.svc.StartPasswordLogin(ctx, "bob@example.com", password)
	if err != nil {
		t.Fatalf("StartPasswordLogin: %v", err)
	}
	n, err := f.svc.BackChannelLogout(ctx, f.provider.ID, f.logoutToken(t, map[string]any{"sid": nil, "sub": "sub-bob"}))
	if err != nil || n != 2 {
		t.Fatalf("BackChannelLogout by subject = %d, %v; want both federated sessions ended", n, err)
	}
	if !f.sessionEnded(first.Secret) || !f.sessionEnded(second.Secret) {
		t.Error("a federated session survived a logout by subject")
	}
	if f.sessionEnded(passwordSession.Secret) {
		t.Error("a password session was ended by a federated logout")
	}
}

// Verifies: GOV-004, SEC-100.
// A provider's token that belongs to another sign-in creates no session. A
// person with a confirmed second factor who signs in through the provider is
// given a challenge, not a session.
func TestFederatedSignInNeedsTheProvider_GOV_004(t *testing.T) {
	t.Parallel()
	f := newFederated(t)
	f.createProvider(t)
	ctx := context.Background()
	login, err := f.svc.StartProviderLogin(ctx, f.provider.ID)
	if err != nil {
		t.Fatalf("StartProviderLogin: %v", err)
	}
	authCode := f.idp.authorize(t, login, "sub-bob", "bob@example.com", true, "a-nonce-of-another-sign-in")
	if _, _, err := f.svc.CompleteOIDCLogin(ctx, login.State, authCode); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("a token for another sign-in: %v", err)
	}
	f.enrol(t, f.signIn(t, "bob@example.com"))
	session, challenge, err := f.federatedSignIn(t, "sub-bob", "bob@example.com", true, map[string]any{"sid": "sid-3"})
	if err != nil || session.Secret != "" || challenge.Secret == "" {
		t.Errorf("a second factor was not asked for: %+v, %+v, %v", session, challenge, err)
	}
}
