// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// agreementJWK returns a new P-256 public key as a JWK.
func agreementJWK(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(jose.JSONWebKey{Key: &key.PublicKey})
	if err != nil {
		t.Fatal(err)
	}
	return key, raw
}

// Verifies: SEC-053.
func TestParseAgreementKey(t *testing.T) {
	t.Parallel()
	if got, err := parseAgreementKey(nil); err != nil || got != nil {
		t.Errorf("no key: %q, %v", got, err)
	}
	_, raw := agreementJWK(t)
	got, err := parseAgreementKey(raw)
	if err != nil || len(got) == 0 {
		t.Fatalf("a valid key: %q, %v", got, err)
	}
	other, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p384, err := json.Marshal(jose.JSONWebKey{Key: &other.PublicKey})
	if err != nil {
		t.Fatal(err)
	}
	private, err := json.Marshal(jose.JSONWebKey{Key: other})
	if err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string][]byte{"not JSON": []byte("x"), "a P-384 key": p384, "a private key": private} {
		if _, err := parseAgreementKey(bad); err == nil {
			t.Errorf("%s was accepted", name)
		} else if c, _ := plxerr.CodeOf(err); c != plxerr.InvalidFormat {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Verifies: SEC-053.
// A device gets a content key only when it holds the assurance level the
// settings ask for and registered a key-agreement key.
func TestRecipientOf(t *testing.T) {
	t.Parallel()
	key, jwk := agreementJWK(t)
	canonical, err := parseAgreementKey(jwk)
	if err != nil {
		t.Fatal(err)
	}
	strict := mustSettings(t, settings.Strict) // confidentialMinAssurance is AL2
	row := func(level string, agreement []byte, revoked bool) dbgen.Device {
		return dbgen.Device{AssuranceLevel: level, AgreementPublicKey: agreement, RevokedAt: pgtype.Timestamptz{Valid: revoked}}
	}
	for _, c := range []struct {
		name string
		row  dbgen.Device
		want plxerr.Code
	}{
		{"at the level", row("AL2", canonical, false), 0},
		{"above the level", row("AL3", canonical, false), 0},
		{"below the level", row("AL1", canonical, false), plxerr.ConfidentialKeyUnavailable},
		{"no level", row("", canonical, false), plxerr.ConfidentialKeyUnavailable},
		{"no key-agreement key", row("AL2", nil, false), plxerr.ConfidentialKeyUnavailable},
		{"revoked", row("AL2", canonical, true), plxerr.DeviceRevoked},
	} {
		got, err := recipientOf(c.row, strict)
		if c.want == 0 {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
				continue
			}
			want, _ := key.PublicKey.ECDH()
			if !got.Equal(want) {
				t.Errorf("%s: another key", c.name)
			}
			continue
		}
		if code, _ := plxerr.CodeOf(err); code != c.want {
			t.Errorf("%s: got %v, want %s", c.name, err, c.want)
		}
	}
}
