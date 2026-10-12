// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package keyattest

import (
	"context"
	"crypto/elliptic"
	"errors"
	"math/big"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/attest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

func TestParseRevocations(t *testing.T) {
	// Verifies: SEC-002.
	r, err := ParseRevocations([]byte(`{"entries":{
		"c35747a084470c3135aeefe2b8d40921":{"status":"REVOKED","reason":"KEY_COMPROMISE"},
		"0A":{"status":"SUSPENDED","reason":"SOFTWARE_FLAW"}}}`))
	if err != nil {
		t.Fatalf("ParseRevocations: %v", err)
	}
	big1, _ := new(big.Int).SetString("c35747a084470c3135aeefe2b8d40921", 16)
	tests := []struct {
		serial *big.Int
		want   string
		listed bool
	}{
		{big1, "REVOKED", true},
		{big.NewInt(10), "SUSPENDED", true},
		{big.NewInt(11), "", false},
		{nil, "", false},
	}
	for _, tt := range tests {
		got, listed := r.status(tt.serial)
		if got != tt.want || listed != tt.listed {
			t.Errorf("status(%v) = %q, %v; want %q, %v", tt.serial, got, listed, tt.want, tt.listed)
		}
	}
	if r.Len() != 2 {
		t.Errorf("Len = %d, want 2", r.Len())
	}
}

func TestRevocationsNil(t *testing.T) {
	// Verifies: SEC-002.
	var r *Revocations
	if _, listed := r.status(big.NewInt(1)); listed || r.Len() != 0 {
		t.Error("nil list reports entries")
	}
}

func TestParseRevocationsMalformed(t *testing.T) {
	// Verifies: SEC-002.
	cases := map[string]string{
		"not JSON":         `{`,
		"no entries":       `{}`,
		"entries is array": `{"entries":[]}`,
		"serial not hex":   `{"entries":{"xyz":{"status":"REVOKED"}}}`,
		"empty serial":     `{"entries":{"":{"status":"REVOKED"}}}`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := ParseRevocations([]byte(in))
			if r != nil {
				t.Errorf("list = %+v", r)
			}
			wantCode(t, err, plxerr.AttestationFailed)
		})
	}
}

// fixedSource is a RevocationSource with a fixed answer.
type fixedSource struct {
	list *Revocations
	err  error
}

func (f fixedSource) Revocations(context.Context) (*Revocations, error) { return f.list, f.err }

// Verifies: SEC-002, SEC-009.
// A source supplies the list for every verification: a listed serial,
// revoked or suspended, refuses the chain, and a source that has no list
// makes it an outage of the provider, never an acceptance.
func TestVerifierSource(t *testing.T) {
	revoked, err := ParseRevocations([]byte(`{"entries":{"2":{"status":"REVOKED"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	suspended, err := ParseRevocations([]byte(`{"entries":{"03":{"status":"SUSPENDED"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	other, err := ParseRevocations([]byte(`{"entries":{"ffff":{"status":"REVOKED"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	validKD := encodeKD(validSpec())
	run := func(src RevocationSource) error {
		p := newPKI(t, nil)
		v := p.verifier(nil)
		v.Source = src
		_, err := v.Verify(context.Background(), p.chain(p.leaf(elliptic.P256(), validKD)), testChallenge, testPolicy())
		return err
	}
	if err := run(fixedSource{list: other}); err != nil {
		t.Errorf("a list without these serials: %v", err)
	}
	wantCode(t, run(fixedSource{list: revoked}), plxerr.AttestationFailed)
	wantCode(t, run(fixedSource{list: suspended}), plxerr.AttestationFailed)
	err = run(fixedSource{err: errors.New("no list")})
	wantCode(t, err, plxerr.AttestationUnavailable)
	if !errors.Is(err, attest.ErrProviderUnavailable) {
		t.Errorf("an unavailable list is not marked as the provider's outage: %v", err)
	}

	// The source wins over the fixed list.
	p := newPKI(t, nil)
	v := p.verifier(revoked)
	v.Source = fixedSource{list: other}
	if _, err := v.Verify(context.Background(), p.chain(p.leaf(elliptic.P256(), validKD)), testChallenge, testPolicy()); err != nil {
		t.Errorf("the fixed list applied beside a source: %v", err)
	}
}
