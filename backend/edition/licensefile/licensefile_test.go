// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package licensefile_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/edition/licensefile"
	"github.com/nightCode42/plux3/backend/internal/updatemeta"
)

func key(b byte) ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, ed25519.SeedSize))
}

func pub(k ed25519.PrivateKey) ed25519.PublicKey {
	p, _ := k.Public().(ed25519.PublicKey)
	return p
}

func payload() licensefile.Payload {
	grace := 30
	return licensefile.Payload{
		LicenseID: "LIC-1", Organization: "Acme AG", Edition: licensefile.Edition,
		IssuedAt: "2026-01-01T00:00:00Z", NotBefore: "2026-01-01T00:00:00Z", ExpiresAt: "2027-01-01T00:00:00Z",
		GraceDays: &grace, Features: []string{"saml"}, Limits: map[string]int64{"page.nodes": 100},
	}
}

func sign(t *testing.T, p licensefile.Payload, k ed25519.PrivateKey) []byte {
	t.Helper()
	canonical, err := licensefile.Canonical(p)
	if err != nil {
		t.Fatal(err)
	}
	sig := licensefile.Signature{KeyID: licensefile.KeyID(pub(k)), Alg: licensefile.AlgEd25519, Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(k, canonical))}
	raw, err := licensefile.Marshal(p, []licensefile.Signature{sig})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Verifies: GOV-030.
// A licence file is the update metadata's envelope: the update metadata's
// own parser reads it, its signature verifies over the bytes that package
// canonicalises, and both packages marshal the same file byte for byte.
func TestEnvelopeIsTheUpdateMetadataEnvelope_GOV_030(t *testing.T) {
	t.Parallel()
	k := key(1)
	raw := sign(t, payload(), k)

	doc, err := updatemeta.ParseDocument(raw)
	if err != nil {
		t.Fatalf("updatemeta cannot read the licence: %v", err)
	}
	f, err := licensefile.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(doc.Signed, f.Signed) {
		t.Errorf("signed parts differ:\n%s\n%s", doc.Signed, f.Signed)
	}
	if got, err := doc.Bytes(); err != nil || !bytes.Equal(got, raw) {
		t.Errorf("updatemeta re-marshals the file differently: %v", err)
	}
	sigBytes, _ := base64.StdEncoding.DecodeString(doc.Signatures[0].Sig)
	if !ed25519.Verify(pub(k), doc.Signed, sigBytes) {
		t.Error("the signature does not verify over updatemeta's canonical bytes")
	}
	if doc.Signatures[0].KeyID != updatemeta.KeyID(pub(k)) || licensefile.KeyID(pub(k)) != updatemeta.KeyID(pub(k)) {
		t.Error("key identifiers differ from the update metadata's")
	}
	if doc.Signatures[0].Alg != updatemeta.AlgEd25519 {
		t.Errorf("alg = %s", doc.Signatures[0].Alg)
	}
}

// Verifies: GOV-030.
func TestVerify_GOV_030(t *testing.T) {
	t.Parallel()
	k := key(1)
	f, err := licensefile.Parse(sign(t, payload(), k))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Verify(licensefile.NewKeySet(pub(k))); err != nil {
		t.Errorf("a good signature: %v", err)
	}
	for name, keys := range map[string]licensefile.KeySet{
		"another key":   licensefile.NewKeySet(pub(key(2))),
		"no keys":       {},
		"compiled keys": licensefile.TrustedKeys(),
	} {
		if err := f.Verify(keys); !errors.Is(err, licensefile.ErrSignature) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// A signature that names a trusted key but does not verify, or names
	// another algorithm, counts for nothing.
	for _, mutate := range []func(*licensefile.File){
		func(f *licensefile.File) { f.Signatures[0].Alg = "ES256" },
		func(f *licensefile.File) { f.Signatures[0].Sig = "!!!" },
		func(f *licensefile.File) { f.Signed = append([]byte(nil), f.Signed...); f.Signed[5] ^= 1 },
	} {
		g := f
		g.Signatures = append([]licensefile.Signature(nil), f.Signatures...)
		mutate(&g)
		if err := g.Verify(licensefile.NewKeySet(pub(k))); !errors.Is(err, licensefile.ErrSignature) {
			t.Errorf("mutated file verified: %v", err)
		}
	}
}

// Verifies: GOV-030, LIM-001.
func TestPayloadValidation_GOV_030(t *testing.T) {
	t.Parallel()
	mod := func(f func(*licensefile.Payload)) licensefile.Payload { p := payload(); f(&p); return p }
	neg, huge := -1, 366
	tests := map[string]struct {
		p    licensefile.Payload
		want error
	}{
		"valid":                {payload(), nil},
		"no grace":             {mod(func(p *licensefile.Payload) { p.GraceDays = nil }), nil},
		"bad id":               {mod(func(p *licensefile.Payload) { p.LicenseID = "" }), licensefile.ErrFormat},
		"no organisation":      {mod(func(p *licensefile.Payload) { p.Organization = "" }), licensefile.ErrFormat},
		"other edition":        {mod(func(p *licensefile.Payload) { p.Edition = "community" }), licensefile.ErrFormat},
		"bad installation":     {mod(func(p *licensefile.Payload) { p.InstallationID = "a b" }), licensefile.ErrFormat},
		"bad date":             {mod(func(p *licensefile.Payload) { p.ExpiresAt = "next year" }), licensefile.ErrFormat},
		"bad issued date":      {mod(func(p *licensefile.Payload) { p.IssuedAt = "x" }), licensefile.ErrFormat},
		"bad notBefore date":   {mod(func(p *licensefile.Payload) { p.NotBefore = "x" }), licensefile.ErrFormat},
		"expiry before start":  {mod(func(p *licensefile.Payload) { p.ExpiresAt = p.NotBefore }), licensefile.ErrFormat},
		"start before issue":   {mod(func(p *licensefile.Payload) { p.IssuedAt = "2026-06-01T00:00:00Z" }), licensefile.ErrFormat},
		"negative grace":       {mod(func(p *licensefile.Payload) { p.GraceDays = &neg }), licensefile.ErrFormat},
		"excessive grace":      {mod(func(p *licensefile.Payload) { p.GraceDays = &huge }), licensefile.ErrFormat},
		"bad feature":          {mod(func(p *licensefile.Payload) { p.Features = []string{"SAML!"} }), licensefile.ErrFormat},
		"repeated feature":     {mod(func(p *licensefile.Payload) { p.Features = []string{"saml", "saml"} }), licensefile.ErrFormat},
		"unknown limit":        {mod(func(p *licensefile.Payload) { p.Limits = map[string]int64{"nope": 1} }), licensefile.ErrLimitKey},
		"zero limit":           {mod(func(p *licensefile.Payload) { p.Limits = map[string]int64{"page.nodes": 0} }), licensefile.ErrFormat},
		"limit above register": {mod(func(p *licensefile.Payload) { p.Limits = map[string]int64{"page.nodes": 1 << 40} }), licensefile.ErrFormat},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := tc.p.Validate()
			if !errors.Is(err, tc.want) || (tc.want == nil) != (err == nil) {
				t.Errorf("Validate = %v, want %v", err, tc.want)
			}
		})
	}
	if payload().Grace() != 30 || mod(func(p *licensefile.Payload) { p.GraceDays = nil }).Grace() != licensefile.DefaultGraceDays {
		t.Error("Grace")
	}
	if _, _, _, err := payload().Window(); err != nil {
		t.Error(err)
	}
}

// Verifies: GOV-030.
func TestParseRefusesMalformedFiles_GOV_030(t *testing.T) {
	t.Parallel()
	good := string(sign(t, payload(), key(1)))
	tests := map[string]string{
		"not json":           "licence",
		"trailing data":      good + "{}",
		"no signed part":     `{"signatures":[]}`,
		"unknown field":      strings.Replace(good, `{"signatures"`, `{"extra":1,"signatures"`, 1),
		"unknown payload":    strings.Replace(good, `"organization"`, `"surprise":1,"organization"`, 1),
		"empty signature":    `{"signed":{},"signatures":[{"keyid":"","alg":"","sig":""}]}`,
		"payload not object": `{"signed":[1],"signatures":[]}`,
		"too deep":           `{"signed":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":{"a":1}}}}}}}}},"signatures":[]}`,
		"invalid payload":    strings.Replace(good, `"enterprise"`, `"community"`, 1),
	}
	for name, raw := range tests {
		if _, err := licensefile.Parse([]byte(raw)); !errors.Is(err, licensefile.ErrFormat) && !errors.Is(err, licensefile.ErrLimitKey) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := licensefile.Parse([]byte(good)); err != nil {
		t.Errorf("the good file: %v", err)
	}
}

// Verifies: GOV-030.
func TestMarshalOrdersSignaturesAndKeys_GOV_030(t *testing.T) {
	t.Parallel()
	a, b := key(1), key(2)
	p := payload()
	canonical, _ := licensefile.Canonical(p)
	mk := func(k ed25519.PrivateKey) licensefile.Signature {
		return licensefile.Signature{KeyID: licensefile.KeyID(pub(k)), Alg: licensefile.AlgEd25519, Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(k, canonical))}
	}
	x, err := licensefile.Marshal(p, []licensefile.Signature{mk(a), mk(b)})
	if err != nil {
		t.Fatal(err)
	}
	y, err := licensefile.Marshal(p, []licensefile.Signature{mk(b), mk(a)})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(x, y) {
		t.Error("the file depends on the order of its signatures")
	}
	if _, err := licensefile.Marshal(p, nil); err != nil {
		t.Error(err)
	}
	// Either of two keys is enough.
	f, _ := licensefile.Parse(x)
	if err := f.Verify(licensefile.NewKeySet(pub(b))); err != nil {
		t.Errorf("the second key: %v", err)
	}
}

// Verifies: GOV-030.
func TestKeys_GOV_030(t *testing.T) {
	t.Parallel()
	k := pub(key(1))
	b64 := base64.StdEncoding.EncodeToString(k)
	got, err := licensefile.ParsePublicKey(b64)
	if err != nil || !bytes.Equal(got, k) {
		t.Fatalf("ParsePublicKey: %v", err)
	}
	for _, bad := range []string{"", "!!", base64.StdEncoding.EncodeToString(k[:16])} {
		if _, err := licensefile.ParsePublicKey(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	set := licensefile.TrustedKeysWith(k)
	if _, ok := set.Lookup(licensefile.KeyID(k)); !ok || set.Len() != licensefile.TrustedKeys().Len()+1 {
		t.Error("TrustedKeysWith")
	}
	if _, ok := (licensefile.KeySet{}).Lookup("x"); ok {
		t.Error("the zero KeySet trusts a key")
	}
	// The production keys are pinned: the slot holds exactly the key of the
	// maintainer's signing ceremony (2026-10-22), by its key ID.
	prod, err := licensefile.ParsePublicKey("92YBSPIJxFIct4/HhUXiaf1T6nVJEzGHAE15V1hRKus=")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := licensefile.TrustedKeys().Lookup(licensefile.KeyID(prod)); !ok || licensefile.TrustedKeys().Len() != 1 {
		t.Errorf("the production keys are not exactly the pinned key %s", licensefile.KeyID(prod))
	}
}
