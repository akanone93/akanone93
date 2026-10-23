// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/devtoken"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// sign builds a token of the class and typ over the claims.
func sign(t *testing.T, signer signing.TokenSigner, class signing.TokenClass, typ string, c devsession.Claims) string {
	t.Helper()
	payload, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var compact devtoken.Compact
	token, err := compact.Sign(context.Background(), signer, class, typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// keys lists the signer's keys.
func keys(t *testing.T, signer signing.TokenSigner) []signing.TokenKey {
	t.Helper()
	k, err := signer.TokenKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// claims are the claims of a well-formed token.
var claims = devsession.Claims{Session: "s", Subject: "u", App: "a", Environment: "e", Expiry: 1_800_000_000, Nonce: "bm9uY2U"}

// Verifies: DEV-002.
// A pairing token signed with a development token key verifies and returns
// its claims.
func TestVerifyPairing_RoundTrip_DEV_002(t *testing.T) {
	t.Parallel()
	signer := devicetest.NewTokenSigner(t)
	token := sign(t, signer, signing.TokenDevelopment, devsession.PairingType, claims)
	got, err := devsession.VerifyPairing(token, keys(t, signer))
	if err != nil {
		t.Fatal(err)
	}
	if got != claims {
		t.Errorf("claims = %+v, want %+v", got, claims)
	}
	header, _, _ := strings.Cut(token, ".")
	raw, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil || !strings.Contains(string(raw), `"typ":"plux-pair+jwt"`) || !strings.Contains(string(raw), `"alg":"ES256"`) {
		t.Errorf("header = %s (%v)", raw, err)
	}
}

// Verifies: DEV-002, DEV-003.
// A token that is altered, signed by another key, signed with the
// production key, of another type or malformed is refused.
func TestVerifyPairing_Refusals_DEV_002(t *testing.T) {
	t.Parallel()
	signer := devicetest.NewTokenSigner(t)
	good := sign(t, signer, signing.TokenDevelopment, devsession.PairingType, claims)
	parts := strings.Split(good, ".")
	other := claims
	other.Session = "another"
	forgedPayload, err := json.Marshal(other)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"altered payload":  parts[0] + "." + base64.RawURLEncoding.EncodeToString(forgedPayload) + "." + parts[2],
		"another signer":   sign(t, devicetest.NewTokenSigner(t), signing.TokenDevelopment, devsession.PairingType, claims),
		"production key":   sign(t, signer, signing.TokenProduction, devsession.PairingType, claims),
		"another type":     sign(t, signer, signing.TokenDevelopment, "at+jwt", claims),
		"two parts":        parts[0] + "." + parts[1],
		"empty":            "",
		"unsigned":         parts[0] + "." + parts[1] + ".",
		"garbage":          "a.b.c",
		"truncated sig":    parts[0] + "." + parts[1] + "." + parts[2][:20],
		"no session claim": sign(t, signer, signing.TokenDevelopment, devsession.PairingType, devsession.Claims{Expiry: 1}),
	}
	for name, token := range cases {
		_, err := devsession.VerifyPairing(token, keys(t, signer))
		if code, _ := plxerr.CodeOf(err); code != plxerr.PermissionDenied {
			t.Errorf("%s: %v, want PermissionDenied", name, err)
		}
	}
}

// Verifies: DEV-002.
// The short code is eight base32 characters derived from the token.
func TestCode_DEV_002(t *testing.T) {
	t.Parallel()
	a, b := devsession.Code("token-a"), devsession.Code("token-b")
	if len(a) != 8 || strings.Trim(a, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567") != "" || a == b || a != devsession.Code("token-a") {
		t.Errorf("codes %q and %q", a, b)
	}
}

// Verifies: DEV-002.
// A session is live while it is not ended, within its lifetime and not idle.
func TestSessionLive_DEV_002(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	s := devsession.Session{CreatedAt: start, ExpiresAt: start.Add(8 * time.Hour), LastActivityAt: start.Add(time.Hour)}
	idle := 30 * time.Minute
	for name, tc := range map[string]struct {
		s    devsession.Session
		at   time.Time
		want bool
	}{
		"active":             {s, start.Add(time.Hour + 29*time.Minute), true},
		"idle exactly":       {s, start.Add(time.Hour + 30*time.Minute), false},
		"idle":               {s, start.Add(2 * time.Hour), false},
		"at the expiry":      {devsession.Session{ExpiresAt: start, LastActivityAt: start}, start, false},
		"ended":              {devsession.Session{ExpiresAt: start.Add(time.Hour), LastActivityAt: start, EndedAt: start}, start.Add(time.Minute), false},
		"active before end":  {devsession.Session{ExpiresAt: start.Add(time.Hour), LastActivityAt: start}, start.Add(time.Minute), true},
		"lifetime over busy": {devsession.Session{ExpiresAt: start.Add(time.Hour), LastActivityAt: start.Add(59 * time.Minute)}, start.Add(time.Hour), false},
	} {
		if got := tc.s.Live(tc.at, idle); got != tc.want {
			t.Errorf("%s: live = %v, want %v", name, got, tc.want)
		}
	}
}
