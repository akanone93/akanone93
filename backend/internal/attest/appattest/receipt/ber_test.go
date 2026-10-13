// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package receipt

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest/receipt/receipttest"
)

// unhex decodes hexadecimal text; spaces are ignored.
func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Verifies: SEC-007.
// The normaliser turns BER into definite-length DER: indefinite lengths,
// long forms of short lengths and constructed OCTET STRINGs.
func TestNormaliseBER_SEC_007(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, in, want string }{
		{"already DER", "30 03 02 01 05", "30 03 02 01 05"},
		{"indefinite sequence", "30 80 02 01 05 00 00", "30 03 02 01 05"},
		{"long form of a short length", "30 81 03 02 01 05", "30 03 02 01 05"},
		{"constructed octet string", "24 80 04 02 01 02 04 03 03 04 05 00 00", "04 05 01 02 03 04 05"},
		{"nested constructed octet string", "24 80 24 80 04 01 aa 00 00 04 01 bb 00 00", "04 02 aa bb"},
		{"nested indefinite", "30 80 30 80 02 01 05 00 00 00 00", "30 05 30 03 02 01 05"},
		{"context tag, indefinite", "a0 80 02 01 01 00 00", "a0 03 02 01 01"},
		{"high tag number", "bf 8a 00 03 02 01 01", "bf 8a 00 03 02 01 01"},
	} {
		got, err := normaliseBER(unhex(t, c.in), 1<<16)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if want := unhex(t, c.want); !bytes.Equal(got, want) {
			t.Errorf("%s: got %x, want %x", c.name, got, want)
		}
	}
}

// Verifies: SEC-007.
// The normaliser refuses malformed and hostile input.
func TestNormaliseBERRefuses_SEC_007(t *testing.T) {
	t.Parallel()
	deep := bytes.Repeat([]byte{0x30, 0x80}, maxDepth+2)
	for _, c := range []struct {
		name string
		in   []byte
	}{
		{"empty", nil},
		{"truncated header", []byte{0x30}},
		{"truncated body", []byte{0x30, 0x05, 0x02, 0x01}},
		{"trailing data", unhex(t, "300302010500")},
		{"indefinite primitive", unhex(t, "02800000")},
		{"unterminated", unhex(t, "3080020105")},
		{"stray end of contents", unhex(t, "300200 00")},
		{"reserved length", unhex(t, "30ff")},
		{"length of five octets", unhex(t, "30850000000001 00")},
		{"non-minimal high tag", unhex(t, "1f8001 00")},
		{"too deep", deep},
		{"octet string holding an integer", unhex(t, "2480020105 0000")},
		{"length beyond input", unhex(t, "3084ffffffff")},
	} {
		if got, err := normaliseBER(c.in, 1<<16); err == nil {
			t.Errorf("%s: accepted, got %x", c.name, got)
		}
	}
	if _, err := normaliseBER(unhex(t, "3003020105"), 4); err == nil {
		t.Error("input beyond the size bound was accepted")
	}
}

// fuzzFixture returns seeds: receipts in both encodings and the fixed
// pieces around them.
func fuzzFixture(t testing.TB) (*Verifier, Expect, [][]byte) {
	t.Helper()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	a := receipttest.NewAuthority(t, now)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var seeds [][]byte
	for _, enc := range []receipttest.Encoding{receipttest.DER, receipttest.BERIndefinite} {
		for _, form := range []receipttest.KeyForm{receipttest.KeyPoint, receipttest.KeySPKI, receipttest.KeyCertificate} {
			seeds = append(seeds, a.Issue(t, receipttest.IssueOptions{AppID: "TEAMID.com.example.app", Key: &key.PublicKey, KeyForm: form, Risk: 1, Encoding: enc}))
		}
	}
	return &Verifier{Roots: a.Roots(), Now: func() time.Time { return now }}, Expect{AppID: "TEAMID.com.example.app", Key: &key.PublicKey}, seeds
}

// Verifies: SEC-007.
// FuzzParseReceipt feeds arbitrary bytes to the whole pipeline (BER
// normalisation, CMS parsing, signature and chain checks, payload decoding).
// It must never panic, must accept nothing it did not verify, and the
// normaliser must be idempotent.
func FuzzParseReceipt(f *testing.F) {
	v, want, seeds := fuzzFixture(f)
	for _, s := range seeds {
		f.Add(s)
		if len(s) > 8 {
			f.Add(s[:len(s)/2])
			mutated := bytes.Clone(s)
			mutated[len(mutated)/2] ^= 0xff
			f.Add(mutated)
		}
	}
	f.Add([]byte{})
	f.Add([]byte{0x30, 0x80, 0x00, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		if der, err := normaliseBER(data, 1<<20); err == nil {
			again, err := normaliseBER(der, 1<<20)
			if err != nil || !bytes.Equal(der, again) {
				t.Fatalf("normalisation is not idempotent: %x -> %x -> %x (%v)", data, der, again, err)
			}
			_, _ = parseSignedData(der)
			_, _ = parsePayload(der)
		}
		if got, err := v.Verify(data, want); err == nil {
			// Only an input that differs from a seed in insignificant ways
			// can verify; it must then describe a valid receipt.
			if got.RiskMetric < 0 || !got.Expiration.After(got.NotBefore) {
				t.Fatalf("a verified receipt is inconsistent: %+v", got)
			}
		}
	})
}
