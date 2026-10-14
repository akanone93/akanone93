// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const fnInput = `{"amount":"125.50","payee":"DE89370400440532013000"}`

var fnTx = FunctionTransaction{Name: "payments.transfer", Version: 3}

// invocationChallenge issues the challenge of fnTx for fnInput.
func (f *fixture) invocationChallenge(ctx context.Context) Challenge {
	f.t.Helper()
	sum := sha256.Sum256([]byte(fnInput))
	c, err := f.sca.CreateInvocation(ctx, f.dev, fnTx, sum[:], []byte(`{"summary":"Transfer 125.50 EUR"}`), 5*time.Minute)
	if err != nil {
		f.t.Fatalf("CreateInvocation: %v", err)
	}
	return c
}

// Verifies: FN-020, SEC-027, SEC-028.
// A function invocation is confirmed with a signature over the challenge
// and a summary that names the function, its version and the hash of the
// input; the same input verifies, and the challenge is single-use.
func TestInvocationChallengeIsBoundToTheInput_SEC_027(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := t.Context()
	c := f.invocationChallenge(ctx)
	sum := sha256.Sum256([]byte(fnInput))
	want := `{"function":"payments.transfer","inputSha256":"` + base64.RawURLEncoding.EncodeToString(sum[:]) + `","summary":"Transfer 125.50 EUR","version":"3"}`
	if string(c.Summary) != want {
		t.Fatalf("summary = %s, want %s", c.Summary, want)
	}
	sig := f.signTransaction(c, c.Summary)
	if err := f.sca.VerifyInvocation(ctx, f.dev, fnTx, []byte(fnInput), c.ID, sig, 5*time.Minute); err != nil {
		t.Fatalf("a confirmed invocation: %v", err)
	}
	if err := f.sca.VerifyInvocation(ctx, f.dev, fnTx, []byte(fnInput), c.ID, sig, 5*time.Minute); code(err) != plxerr.SCAChallengeInvalid {
		t.Errorf("a second use of the challenge: %v", err)
	}
}

// Verifies: FN-020, SEC-027, SEC-028.
// Every way a function invocation can fail its confirmation is refused
// with the gateway's codes, and a challenge for another input is spent.
func TestInvocationConfirmationRefusals_SEC_027(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		do   func(ctx context.Context, f *fixture) error
		want plxerr.Code
	}{
		{"another input", func(ctx context.Context, f *fixture) error {
			c := f.invocationChallenge(ctx)
			return f.sca.VerifyInvocation(ctx, f.dev, fnTx, []byte(`{"amount":"9999.00","payee":"DE89370400440532013000"}`), c.ID, f.signTransaction(c, c.Summary), time.Minute)
		}, plxerr.SCADynamicLinkingMismatch},
		{"another version", func(ctx context.Context, f *fixture) error {
			c := f.invocationChallenge(ctx)
			return f.sca.VerifyInvocation(ctx, f.dev, FunctionTransaction{Name: fnTx.Name, Version: 4}, []byte(fnInput), c.ID, f.signTransaction(c, c.Summary), time.Minute)
		}, plxerr.SCAChallengeInvalid},
		{"another function", func(ctx context.Context, f *fixture) error {
			c := f.invocationChallenge(ctx)
			return f.sca.VerifyInvocation(ctx, f.dev, FunctionTransaction{Name: "payments.refund", Version: 3}, []byte(fnInput), c.ID, f.signTransaction(c, c.Summary), time.Minute)
		}, plxerr.SCAChallengeInvalid},
		{"another device", func(ctx context.Context, f *fixture) error {
			c := f.invocationChallenge(ctx)
			other := f.dev
			other.DeviceID = "dev-2"
			return f.sca.VerifyInvocation(ctx, other, fnTx, []byte(fnInput), c.ID, f.signTransaction(c, c.Summary), time.Minute)
		}, plxerr.SCAChallengeInvalid},
		{"a signature over another summary", func(ctx context.Context, f *fixture) error {
			c := f.invocationChallenge(ctx)
			return f.sca.VerifyInvocation(ctx, f.dev, fnTx, []byte(fnInput), c.ID, f.signTransaction(c, []byte(`{"summary":"Transfer 1 EUR"}`)), time.Minute)
		}, plxerr.SCASignatureInvalid},
		{"a short signature", func(ctx context.Context, f *fixture) error {
			c := f.invocationChallenge(ctx)
			return f.sca.VerifyInvocation(ctx, f.dev, fnTx, []byte(fnInput), c.ID, []byte("short"), time.Minute)
		}, plxerr.SCASignatureInvalid},
		{"no challenge", func(ctx context.Context, f *fixture) error {
			return f.sca.VerifyInvocation(ctx, f.dev, fnTx, []byte(fnInput), "", nil, time.Minute)
		}, plxerr.SCAChallengeInvalid},
		{"an expired challenge", func(ctx context.Context, f *fixture) error {
			c := f.invocationChallenge(ctx)
			f.clock.Advance(6 * time.Minute)
			return f.sca.VerifyInvocation(ctx, f.dev, fnTx, []byte(fnInput), c.ID, f.signTransaction(c, c.Summary), time.Minute)
		}, plxerr.SCAChallengeInvalid},
		{"a transaction challenge of an upstream", func(ctx context.Context, f *fixture) error {
			c := f.challenge() //nolint:contextcheck // the fixture's helper uses its own background context.
			return f.sca.VerifyInvocation(ctx, f.dev, fnTx, []byte(fnInput), c.ID, f.signTransaction(c, c.Summary), time.Minute)
		}, plxerr.SCAChallengeInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			if got := code(tc.do(t.Context(), f)); got != tc.want {
				t.Errorf("code = %d, want %d", got, tc.want)
			}
		})
	}
}

// Verifies: FN-020, SEC-027.
// The summary a device sends is a canonical object of strings that does
// not name the members the server sets, and the input hash is a SHA-256.
func TestInvocationChallengeRequest_SEC_027(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	sum := sha256.Sum256([]byte(fnInput))
	for name, summary := range map[string]string{
		"names the function":   `{"function":"other.fn"}`,
		"names the version":    `{"version":"9"}`,
		"names the input hash": `{"inputSha256":"AAAA"}`,
		"not canonical":        `{"b":"1", "a":"2"}`,
		"not an object":        `["a"]`,
		"a number member":      `{"amount":1}`,
		"not JSON":             `nope`,
	} {
		if _, err := f.sca.CreateInvocation(ctx, f.dev, fnTx, sum[:], []byte(summary), time.Minute); code(err) != plxerr.InvalidFormat {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := f.sca.CreateInvocation(ctx, f.dev, fnTx, sum[:5], []byte(`{}`), time.Minute); code(err) != plxerr.InvalidFormat {
		t.Errorf("a short input hash: %v", err)
	}
	delete(f.keys, f.dev.DeviceID)
	if _, err := f.sca.CreateInvocation(ctx, f.dev, fnTx, sum[:], []byte(`{}`), time.Minute); code(err) != plxerr.PreconditionFailed {
		t.Errorf("a device with no SCA key: %v", err)
	}
	if upstreamName.MatchString(functionScope) {
		t.Error("the function scope can be taken for an upstream name")
	}
}
