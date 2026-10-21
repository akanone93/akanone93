// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package governance_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/ext/governance"
)

// signedEnvelope signs a report the way the core does and returns the
// envelope with the public key.
func signedEnvelope(t *testing.T, r governance.Report) (governance.Envelope, map[string]ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	statement, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, governance.PAE(governance.PayloadType, statement))
	return governance.NewEnvelope(statement, "k1", sig), map[string]ed25519.PublicKey{"k1": pub}
}

// sample is a small report.
func sample() governance.Report {
	return governance.Report{
		Version: governance.ReportVersion, OrganizationID: "org-1",
		From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Changes: []governance.Change{{ID: "c1", Subject: "release:1", ContentHash: strings.Repeat("ab", 32), Environment: "production"}},
	}
}

// Verifies: GOV-026.
// The pre-authentication encoding is DSSE's: the type and the payload, each
// with its length, so a signature binds both.
func TestPAE_GOV_026(t *testing.T) {
	t.Parallel()
	got := string(governance.PAE("http://example.com/HelloWorld", []byte("hello world")))
	if want := "DSSEv1 29 http://example.com/HelloWorld 11 hello world"; got != want {
		t.Errorf("PAE = %q, want %q", got, want)
	}
}

// Verifies: GOV-026.
// An envelope the audit key signed opens to the report it carries, through
// its JSON form.
func TestEnvelope_RoundTrip_GOV_026(t *testing.T) {
	t.Parallel()
	env, keys := signedEnvelope(t, sample())
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := governance.ParseEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	report, err := parsed.Open(keys)
	if err != nil {
		t.Fatal(err)
	}
	if report.OrganizationID != "org-1" || len(report.Changes) != 1 || report.Changes[0].Subject != "release:1" {
		t.Errorf("report = %+v", report)
	}
}

// Verifies: GOV-026, SEC-122.
// A changed byte of the payload or of a signature, another payload type, an
// untrusted or wrong key, no signature, a payload that is not base64 and a
// report version the verifier does not know are all refused, and the
// signature is checked before the payload is parsed.
func TestEnvelope_Refusals_GOV_026(t *testing.T) {
	t.Parallel()
	env, keys := signedEnvelope(t, sample())
	payload, _ := env.Statement()
	flip := func(b []byte) []byte { c := append([]byte(nil), b...); c[len(c)/2] ^= 0x80; return c }
	sig, _ := base64.StdEncoding.DecodeString(env.Signatures[0].Sig)
	wrong, _, _ := ed25519.GenerateKey(nil)

	cases := map[string]struct {
		mutate func(*governance.Envelope)
		keys   map[string]ed25519.PublicKey
		want   error
	}{
		"a changed payload":              {func(e *governance.Envelope) { e.Payload = base64.StdEncoding.EncodeToString(flip(payload)) }, keys, governance.ErrSignature},
		"a changed signature":            {func(e *governance.Envelope) { e.Signatures[0].Sig = base64.StdEncoding.EncodeToString(flip(sig)) }, keys, governance.ErrSignature},
		"another type":                   {func(e *governance.Envelope) { e.PayloadType = "text/plain" }, keys, governance.ErrMalformed},
		"no signature":                   {func(e *governance.Envelope) { e.Signatures = nil }, keys, governance.ErrSignature},
		"a signature that is not base64": {func(e *governance.Envelope) { e.Signatures[0].Sig = "***" }, keys, governance.ErrSignature},
		"an untrusted key id":            {func(*governance.Envelope) {}, map[string]ed25519.PublicKey{"k2": keys["k1"]}, governance.ErrSignature},
		"the wrong key":                  {func(*governance.Envelope) {}, map[string]ed25519.PublicKey{"k1": wrong}, governance.ErrSignature},
		"a short key":                    {func(*governance.Envelope) {}, map[string]ed25519.PublicKey{"k1": keys["k1"][:8]}, governance.ErrSignature},
		"no keys":                        {func(*governance.Envelope) {}, nil, governance.ErrSignature},
		"a payload that is not base64":   {func(e *governance.Envelope) { e.Payload = "***" }, keys, governance.ErrMalformed},
	}
	for name, c := range cases {
		e := env
		e.Signatures = append([]governance.Signature(nil), env.Signatures...)
		c.mutate(&e)
		if _, err := e.Open(c.keys); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}

	future := sample()
	future.Version = governance.ReportVersion + 1
	newer, newerKeys := signedEnvelope(t, future)
	if _, err := newer.Open(newerKeys); !errors.Is(err, governance.ErrMalformed) {
		t.Errorf("a newer report version: %v, want ErrMalformed", err)
	}
	if _, err := governance.ParseEnvelope([]byte("{")); !errors.Is(err, governance.ErrMalformed) {
		t.Errorf("a truncated envelope: %v, want ErrMalformed", err)
	}
}

// Verifies: GOV-026.
// Another signature in the envelope does not hide a bad one, and any one trusted
// signature is enough.
func TestEnvelope_AnyTrustedSignature_GOV_026(t *testing.T) {
	t.Parallel()
	env, keys := signedEnvelope(t, sample())
	env.Signatures = append([]governance.Signature{{KeyID: "unknown", Sig: "AAAA"}}, env.Signatures...)
	if _, err := env.Open(keys); err != nil {
		t.Errorf("a trusted signature after an unknown one: %v", err)
	}
}

// Verifies: GOV-025.
// The community calendar accepts no window and holds nothing, and the
// defaults of Extensions fill only what is empty.
func TestNoFreezesAndExtensions_GOV_025(t *testing.T) {
	t.Parallel()
	var cal governance.FreezeCalendar = governance.NoFreezes{}
	w := governance.Window{Name: "w", TimeZone: "UTC", Start: "2026-05-01T10:00", Duration: time.Hour}
	if err := cal.Validate(t.Context(), w); !errors.Is(err, governance.ErrUnavailable) {
		t.Errorf("Validate = %v, want ErrUnavailable", err)
	}
	if _, held, err := cal.InForce(t.Context(), []governance.Window{w}, "production", time.Now()); held || err != nil {
		t.Errorf("InForce = %v, %v", held, err)
	}
	ext := governance.Extensions{}.OrDefault()
	if _, ok := ext.Freezes.(governance.NoFreezes); !ok || ext.Reports != nil {
		t.Errorf("defaults = %+v", ext)
	}
}
