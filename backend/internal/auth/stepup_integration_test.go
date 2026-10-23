// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// stepUpWorld is an organisation whose owner has a user-verifying security
// key, a browser session of the owner's and a token of the owner's.
type stepUpWorld struct {
	*fixture
	org     string
	session auth.Identity
	key     *authenticator
	admin   auth.Principal
	token   auth.Identity
	// tokenSecret is the bearer secret of token.
	tokenSecret string
}

func newStepUpWorld(t *testing.T) *stepUpWorld {
	t.Helper()
	ctx := context.Background()
	f := newFixtureWith(t, nil, failedSignIns, withWebAuthn)
	ada := f.person(t, "ada@example.com")
	w := &stepUpWorld{fixture: f, key: newAuthenticator(t, false)}
	w.key.verified = true
	w.org = f.organisation(t, "acme", map[string]auth.Role{ada: auth.RoleOwner})
	w.session = f.signIn(t, "ada@example.com")
	// The principal is resolved before the key exists: once it does, signing in
	// asks for it.
	var err error
	if w.admin, err = f.svc.Resolve(ctx, w.session, w.org); err != nil {
		t.Fatal(err)
	}
	w.admin.SecondFactor = true
	w.admin.Identity = w.admin.WithStepUp(0)
	factor, options, err := f.svc.BeginWebAuthnRegistration(ctx, w.session, "yubikey")
	if err != nil {
		t.Fatal(err)
	}
	cd, obj := w.key.register(t, options)
	if _, err := f.svc.FinishWebAuthnRegistration(ctx, w.session, factor.ID, cd, obj); err != nil {
		t.Fatal(err)
	}
	w.token, w.tokenSecret = w.mintToken(t)
	return w
}

// mintToken creates a personal access token of the owner's and returns the
// identity it authenticates as.
func (w *stepUpWorld) mintToken(t *testing.T) (auth.Identity, string) {
	t.Helper()
	ctx := context.Background()
	minted, err := w.svc.CreateAccessToken(ctx, w.admin, "cli", []string{"app.read"}, time.Hour)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	id, err := w.svc.AuthenticateToken(ctx, minted.Secret)
	if err != nil {
		t.Fatal(err)
	}
	return id, minted.Secret
}

// approve performs the browser's side of a CLI request: begin the ceremony
// and answer it with the key.
func (w *stepUpWorld) approve(t *testing.T, requestID string) error {
	t.Helper()
	ctx := context.Background()
	o, err := w.svc.BeginStepUp(ctx, w.session, requestID)
	if err != nil {
		return err
	}
	_, err = w.svc.CompleteStepUp(ctx, w.session, o.RequestID, w.key.assert(t, o.Options))
	return err
}

// jkt returns a thumbprint-shaped string; the service only compares them.
func jkt(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// Verifies: SEC-103, GOV-005.
// A CLI's step-up is requested with its token and key, approved by a key
// in the person's browser, collected once, and counts as a step-up for that
// token and that key only, for five minutes.
func TestCLIStepUpGrant_SEC_103(t *testing.T) {
	t.Parallel()
	w := newStepUpWorld(t)
	ctx := context.Background()
	key := jkt(t)

	if err := w.token.StepUpError(auth.StepUpWindow); code(err) != plxerr.StepUpRequired {
		t.Fatalf("a token that never stepped up: %v", err)
	}
	req, err := w.svc.CreateCLIStepUp(ctx, w.token, key)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := w.svc.PollCLIStepUp(ctx, w.token, req.RequestID, req.Secret, key); err != nil || got.Approved {
		t.Fatalf("a poll before the approval = %+v, %v", got, err)
	}
	if err := w.approve(t, req.RequestID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	got, err := w.svc.PollCLIStepUp(ctx, w.token, req.RequestID, req.Secret, key)
	if err != nil || !got.Approved || got.Grant == "" {
		t.Fatalf("the poll after the approval = %+v, %v", got, err)
	}
	if _, err := w.svc.PollCLIStepUp(ctx, w.token, req.RequestID, req.Secret, key); code(err) != plxerr.PreconditionFailed {
		t.Errorf("a second collection of the grant: %v", err)
	}

	stepped, err := w.svc.CheckStepUpGrant(ctx, w.token, got.Grant, key)
	if err != nil || stepped.StepUpError(auth.StepUpWindow) != nil {
		t.Fatalf("the grant with its token and key: %v, %v", err, stepped.StepUpError(auth.StepUpWindow))
	}
	w.clock.advance(auth.StepUpWindow + time.Second)
	old, err := w.svc.CheckStepUpGrant(ctx, w.token, got.Grant, key)
	if err != nil || code(old.StepUpError(auth.StepUpWindow)) != plxerr.StepUpExpired {
		t.Errorf("a grant after five minutes: %v, %v", err, old.StepUpError(auth.StepUpWindow))
	}
}

// Verifies: SEC-103.
// A grant presented with another DPoP key or another token adds nothing, a
// poll with another key or without the secret is refused, and the refusals
// are audited.
func TestCLIStepUpGrantIsBound_SEC_103(t *testing.T) {
	t.Parallel()
	w := newStepUpWorld(t)
	ctx := context.Background()
	key := jkt(t)
	req, err := w.svc.CreateCLIStepUp(ctx, w.token, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.PollCLIStepUp(ctx, w.token, req.RequestID, req.Secret, jkt(t)); code(err) != plxerr.PermissionDenied {
		t.Errorf("a poll with another key: %v", err)
	}
	if _, err := w.svc.PollCLIStepUp(ctx, w.token, req.RequestID, "plux_stp_guess", key); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a poll with a wrong secret: %v", err)
	}
	other, _ := w.mintToken(t)
	if _, err := w.svc.PollCLIStepUp(ctx, other, req.RequestID, req.Secret, key); code(err) != plxerr.PermissionDenied {
		t.Errorf("a poll with another token: %v", err)
	}
	if err := w.approve(t, req.RequestID); err != nil {
		t.Fatal(err)
	}
	got, err := w.svc.PollCLIStepUp(ctx, w.token, req.RequestID, req.Secret, key)
	if err != nil {
		t.Fatal(err)
	}
	withOtherKey, err := w.svc.CheckStepUpGrant(ctx, w.token, got.Grant, jkt(t))
	if err != nil || code(withOtherKey.StepUpError(auth.StepUpWindow)) != plxerr.StepUpRequired {
		t.Errorf("the grant with another key: %v, %v", err, withOtherKey.StepUpError(auth.StepUpWindow))
	}
	withOtherToken, err := w.svc.CheckStepUpGrant(ctx, other, got.Grant, key)
	if err != nil || code(withOtherToken.StepUpError(auth.StepUpWindow)) != plxerr.StepUpRequired {
		t.Errorf("the grant with another token: %v, %v", err, withOtherToken.StepUpError(auth.StepUpWindow))
	}
	unknown, err := w.svc.CheckStepUpGrant(ctx, w.token, "plux_sug_unknown", key)
	if err != nil || unknown.StepUpError(auth.StepUpWindow) == nil {
		t.Errorf("an unknown grant: %v, %v", err, unknown.StepUpError(auth.StepUpWindow))
	}
	var refused int
	err = w.db.InTx(ctx, storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'stepup.grant_refused'`).Scan(&refused)
	})
	if err != nil || refused != 4 {
		t.Errorf("stepup.grant_refused entries = %d, %v; want 4", refused, err)
	}
}

// Verifies: SEC-103.
// An assertion answers once, one that did not verify user presence with the
// user's verification is refused, a request of another person is not found,
// and a request accepts only auth.stepUpAttempts ceremonies.
func TestStepUpCeremonyIsBounded_SEC_103(t *testing.T) {
	t.Parallel()
	w := newStepUpWorld(t)
	ctx := context.Background()
	req, err := w.svc.CreateCLIStepUp(ctx, w.token, jkt(t))
	if err != nil {
		t.Fatal(err)
	}

	o, err := w.svc.BeginStepUp(ctx, w.session, req.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	unverified := *w.key
	unverified.verified = false
	if _, err := w.svc.CompleteStepUp(ctx, w.session, o.RequestID, unverified.assert(t, o.Options)); code(err) != plxerr.WebAuthnAssertionInvalid {
		t.Errorf("an assertion without user verification: %v", err)
	}
	o, err = w.svc.BeginStepUp(ctx, w.session, req.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	assertion := w.key.assert(t, o.Options)
	if _, err := w.svc.CompleteStepUp(ctx, w.session, o.RequestID, assertion); err != nil {
		t.Fatalf("a verified assertion: %v", err)
	}
	if _, err := w.svc.CompleteStepUp(ctx, w.session, o.RequestID, assertion); err == nil {
		t.Error("a replayed assertion was accepted")
	}

	fresh, err := w.svc.CreateCLIStepUp(ctx, w.token, jkt(t))
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err := w.svc.BeginStepUp(ctx, w.session, fresh.RequestID); err != nil {
			t.Fatalf("a ceremony within the bound: %v", err)
		}
	}
	if _, err := w.svc.BeginStepUp(ctx, w.session, fresh.RequestID); code(err) != plxerr.RateLimited {
		t.Errorf("a ceremony beyond the bound: %v", err)
	}
	if _, err := w.svc.BeginStepUp(ctx, w.session, fresh.RequestID); err == nil {
		t.Error("a closed request began another ceremony")
	}
}

// Verifies: SEC-103.
// A step-up of a browser session sets its step_up_at, which the session then
// carries into its identity; a token cannot perform one.
func TestSessionStepUp_SEC_103(t *testing.T) {
	t.Parallel()
	w := newStepUpWorld(t)
	ctx := context.Background()
	if _, err := w.svc.BeginStepUp(ctx, w.token, ""); code(err) != plxerr.PreconditionFailed {
		t.Errorf("a step-up from a token: %v", err)
	}
	o, err := w.svc.BeginStepUp(ctx, w.session, "")
	if err != nil {
		t.Fatal(err)
	}
	at, err := w.svc.CompleteStepUp(ctx, w.session, o.RequestID, w.key.assert(t, o.Options))
	if err != nil || at.IsZero() {
		t.Fatalf("CompleteStepUp = %v, %v", at, err)
	}
	var stepped bool
	if err := w.db.Pool().QueryRow(ctx, `SELECT step_up_at IS NOT NULL FROM sessions WHERE id = $1`, w.session.SessionID).Scan(&stepped); err != nil || !stepped {
		t.Errorf("step_up_at recorded = %v, %v", stepped, err)
	}
}

// Verifies: GOV-005.
// A session an administrator revokes is refused at once by another service
// instance that shares only the database, and so is a revoked token.
func TestRevocationReachesAnotherInstance_GOV_005(t *testing.T) {
	t.Parallel()
	w := newStepUpWorld(t)
	ctx := context.Background()
	crypter, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := auth.NewService(auth.Options{
		DB: w.db, Audit: w.log, Cache: cache.NewMemory(nil), Crypter: crypter, IDs: w.ids, Now: w.clock.now,
		WebAuthn: auth.WebAuthnConfig{RPID: "plux.example", RPName: "Plux", Origins: []string{"https://plux.example"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	bob := w.person(t, "bob@example.com")
	membership, _ := w.ids.New()
	err = w.db.InTx(ctx, storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO memberships (id, organization_id, user_id, role) VALUES ($1, $2, $3, 'viewer')`, membership, w.org, bob)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := w.svc.StartPasswordLogin(ctx, "bob@example.com", password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.AuthenticateSession(ctx, session.Secret); err != nil {
		t.Fatalf("a live session on the second instance: %v", err)
	}
	live, err := w.svc.AuthenticateSession(ctx, session.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.svc.RevokeSession(ctx, w.admin, live.SessionID); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, err := second.AuthenticateSession(ctx, session.Secret); code(err) != plxerr.SessionRevoked {
		t.Errorf("a revoked session on the second instance: %v", err)
	}

	if err := w.svc.RevokeSession(ctx, w.admin, w.token.ID); err != nil {
		t.Fatalf("RevokeSession of a token: %v", err)
	}
	if _, err := second.AuthenticateToken(ctx, w.tokenSecret); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("a revoked token on the second instance: %v", err)
	}
}
