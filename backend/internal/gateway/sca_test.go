// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

const (
	summaryJSON = `{"amount":"125.50","currency":"EUR","payee":"DE89370400440532013000"}`
	payment     = `{"amount":125.5,"currency":"EUR","to":{"iban":"DE89370400440532013000"},"memo":"rent"}`
)

// transfer returns the call that carries a signed transaction.
func transfer(c Challenge, sig []byte) Request {
	return Request{
		Upstream: "bank", Operation: "transfer", Method: "POST", Target: "/transfers",
		Header: http.Header{"Content-Type": {"application/json"}}, ChallengeID: c.ID, Signature: sig,
	}
}

// challenge creates a challenge for the summaryJSON and returns it.
func (f *fixture) challenge(sum string) Challenge {
	f.t.Helper()
	c, err := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "transfer", []byte(sum))
	if err != nil {
		f.t.Fatalf("CreateChallenge: %v", err)
	}
	return c
}

// Verifies: SEC-027, SEC-028, SEC-032.
// A transaction whose signature, challenge and linked fields all check out
// is forwarded with an assertion that carries the verified proof; the
// device's signature is not.
func TestTransactionIsVerifiedAndForwarded(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	c := f.challenge(summaryJSON)
	if want := f.clock.Now().Add(5 * time.Minute).Truncate(time.Second); !c.ExpiresAt.Equal(want) {
		t.Errorf("expires at %v, want %v (sca.challengeLifetime)", c.ExpiresAt, want)
	}
	sig := f.signTransaction(c, "bank", "transfer", []byte(summaryJSON))
	if _, err := f.call(transfer(c, sig), []byte(payment)); err != nil {
		t.Fatalf("a valid transaction: %v", err)
	}
	got := f.next()
	if string(got.Body) != payment {
		t.Errorf("the upstream received %q", got.Body)
	}
	claims := f.claims(got.Header.Get(AssertionHeader), AssertionType)
	sca, ok := claims["sca"].(map[string]any)
	if !ok || sca["cid"] != c.ID {
		t.Fatalf("assertion sca = %v", claims["sca"])
	}
	sum := sha256.Sum256([]byte(summaryJSON))
	if sca["sh"] != base64.RawURLEncoding.EncodeToString(sum[:]) {
		t.Errorf("summary hash %v", sca["sh"])
	}
	for name, values := range got.Header {
		for _, v := range values {
			if v == base64.StdEncoding.EncodeToString(sig) || v == base64.RawURLEncoding.EncodeToString(sig) {
				t.Errorf("the signature reached the upstream in header %s", name)
			}
		}
	}
	// The same amount written another way is the same amount.
	c2 := f.challenge(summaryJSON)
	sig2 := f.signTransaction(c2, "bank", "transfer", []byte(summaryJSON))
	body := `{"amount":"125.500","currency":"EUR","to":{"iban":"DE89370400440532013000"}}`
	if _, err := f.call(transfer(c2, sig2), []byte(body)); err != nil {
		t.Errorf("125.500 for 125.50: %v", err)
	}
}

// Verifies: SEC-027, SEC-028, SEC-032.
// Every way a transaction can fail verification is refused before the
// upstream hears of it.
func TestTransactionRefusals(t *testing.T) {
	t.Parallel()
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		do   func(f *fixture) error
		want plxerr.Code
	}{
		{"valid", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			_, err := f.call(transfer(c, f.signTransaction(c, "bank", "transfer", []byte(summaryJSON))), []byte(payment))
			return err
		}, 0},
		{"another amount", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			body := `{"amount":9999,"currency":"EUR","to":{"iban":"DE89370400440532013000"}}`
			_, err := f.call(transfer(c, f.signTransaction(c, "bank", "transfer", []byte(summaryJSON))), []byte(body))
			return err
		}, plxerr.SCADynamicLinkingMismatch},
		{"another payee", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			body := `{"amount":125.5,"currency":"EUR","to":{"iban":"DE00000000000000000000"}}`
			_, err := f.call(transfer(c, f.signTransaction(c, "bank", "transfer", []byte(summaryJSON))), []byte(body))
			return err
		}, plxerr.SCADynamicLinkingMismatch},
		{"another currency", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			body := `{"amount":125.5,"currency":"USD","to":{"iban":"DE89370400440532013000"}}`
			_, err := f.call(transfer(c, f.signTransaction(c, "bank", "transfer", []byte(summaryJSON))), []byte(body))
			return err
		}, plxerr.SCADynamicLinkingMismatch},
		{"a field the mapping does not find", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			_, err := f.call(transfer(c, f.signTransaction(c, "bank", "transfer", []byte(summaryJSON))), []byte(`{"amount":125.5,"currency":"EUR"}`))
			return err
		}, plxerr.SCADynamicLinkingMismatch},
		{"duplicate members smuggle a second amount", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			body := `{"amount":125.5,"amount":9999,"currency":"EUR","to":{"iban":"DE89370400440532013000"}}`
			_, err := f.call(transfer(c, f.signTransaction(c, "bank", "transfer", []byte(summaryJSON))), []byte(body))
			return err
		}, plxerr.SCADynamicLinkingMismatch},
		{"a body that is not JSON", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			_, err := f.call(transfer(c, f.signTransaction(c, "bank", "transfer", []byte(summaryJSON))), []byte("amount=1"))
			return err
		}, plxerr.SCADynamicLinkingMismatch},
		{"signed for another summaryJSON", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			sig := f.signTransaction(c, "bank", "transfer", []byte(`{"amount":"1.00","currency":"EUR","payee":"x"}`))
			_, err := f.call(transfer(c, sig), []byte(payment))
			return err
		}, plxerr.SCASignatureInvalid},
		{"signed for another operation", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			_, err := f.call(transfer(c, f.signTransaction(c, "bank", "balance", []byte(summaryJSON))), []byte(payment))
			return err
		}, plxerr.SCASignatureInvalid},
		{"signed by another key", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			f.scaKey = other
			_, err := f.call(transfer(c, f.signTransaction(c, "bank", "transfer", []byte(summaryJSON))), []byte(payment))
			return err
		}, plxerr.SCASignatureInvalid},
		{"a signature that is not DER", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			_, err := f.call(transfer(c, make([]byte, 64)), []byte(payment))
			return err
		}, plxerr.SCASignatureInvalid},
		{"no signature", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			_, err := f.gw.Begin(context.Background(), f.dev, transfer(c, nil))
			return err
		}, plxerr.SCASignatureInvalid},
		{"no challenge", func(f *fixture) error {
			_, err := f.gw.Begin(context.Background(), f.dev, transfer(Challenge{}, []byte{1}))
			return err
		}, plxerr.SCAChallengeInvalid},
		{"an unknown challenge", func(f *fixture) error {
			_, err := f.call(transfer(Challenge{ID: "unknown"}, []byte{1}), []byte(payment))
			return err
		}, plxerr.SCAChallengeInvalid},
		{"a challenge of another device", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			f.dev.DeviceID = "dev-2"
			f.keys["dev-2"] = f.keys["dev-1"]
			_, err := f.call(transfer(c, f.signTransaction(c, "bank", "transfer", []byte(summaryJSON))), []byte(payment))
			return err
		}, plxerr.SCAChallengeInvalid},
		{"an expired challenge", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			f.clock.Advance(6 * time.Minute)
			_, err := f.call(transfer(c, f.signTransaction(c, "bank", "transfer", []byte(summaryJSON))), []byte(payment))
			return err
		}, plxerr.SCAChallengeInvalid},
		{"a replayed challenge", func(f *fixture) error {
			c := f.challenge(summaryJSON)
			req := transfer(c, f.signTransaction(c, "bank", "transfer", []byte(summaryJSON)))
			if _, err := f.call(req, []byte(payment)); err != nil {
				return err
			}
			f.next()
			_, err := f.call(req, []byte(payment))
			return err
		}, plxerr.SCAChallengeInvalid},
		{"a device with no SCA key", func(f *fixture) error {
			delete(f.keys, "dev-1")
			_, err := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "transfer", []byte(summaryJSON))
			return err
		}, plxerr.PreconditionFailed},
		{"a summary that is not canonical", func(f *fixture) error {
			_, err := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "transfer", []byte(`{"currency":"EUR","amount":"1","payee":"x"}`))
			return err
		}, plxerr.InvalidFormat},
		{"a summary with an unmapped member", func(f *fixture) error {
			_, err := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "transfer", []byte(`{"amount":"1","currency":"EUR","note":"x","payee":"x"}`))
			return err
		}, plxerr.InvalidFormat},
		{"an amount that is not a number", func(f *fixture) error {
			_, err := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "transfer", []byte(`{"amount":"lots","currency":"EUR","payee":"x"}`))
			return err
		}, plxerr.InvalidFormat},
		{"a challenge for an operation that is not a transaction", func(f *fixture) error {
			_, err := f.gw.CreateChallenge(context.Background(), f.dev, "bank", "balance", []byte(summaryJSON))
			return err
		}, plxerr.GatewayDestinationRefused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			err := tt.do(f)
			if tt.want == 0 {
				if err != nil {
					t.Fatalf("error = %v", err)
				}
				f.next()
				return
			}
			if code(err) != tt.want {
				t.Fatalf("error = %v (code %d), want code %d", err, code(err), tt.want)
			}
			f.none()
		})
	}
}

// Verifies: SEC-027, SEC-032.
// The challenge lifetime is the registry's: a tightened sca.challengeLifetime
// shortens it.
func TestChallengeLifetimeComesFromTheRegistry(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.limits = mustTighten(t, f.limits, limits.ScaChallengeLifetime, 30_000)
	c := f.challenge(summaryJSON)
	if want := f.clock.Now().Add(30 * time.Second); !c.ExpiresAt.Equal(want) {
		t.Errorf("expires at %v, want %v", c.ExpiresAt, want)
	}
}

// Verifies: SEC-027, SEC-032.
// The wire format of the signed message is fixed: a device built against
// this vector signs what the server verifies.
func TestSigningInputVector(t *testing.T) {
	t.Parallel()
	got := SigningInput("c1", "bank", "transfer", []byte{1, 2, 3}, []byte(`{"amount":"1"}`))
	const want = "706c75782d7363612d763100" + // "plux-sca-v1" 0x00
		"00000002" + "6331" + // challenge id
		"00000004" + "62616e6b" + // upstream
		"00000008" + "7472616e73666572" + // operation
		"00000003" + "010203" + // challenge
		"0000000e" + "7b22616d6f756e74223a2231227d" // summaryJSON
	if hex.EncodeToString(got) != want {
		t.Errorf("signing input = %x\nwant          %s", got, want)
	}
}
