// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package licensefile reads and verifies Plux licence files (GOV-030,
// ADR-0084). It holds the parts that are not secret: the payload, the
// signed envelope and the check of a signature against public keys, so the
// open-source CLI can inspect a licence. Deciding what a verified licence
// allows, and re-checking it over time, is enforcement and lives in the
// enterprise module (GOV-033).
//
// The envelope is the one the update metadata uses (ADR-0004): {"signed":
// <canonical JSON>, "signatures": [{"keyid", "alg", "sig"}]}, signed over the
// RFC 8785 canonical bytes of the signed part with Ed25519. The package
// builds it with the same canonicaliser and does not import the server-only
// updatemeta package, so the Apache-licensed CLI stays free of AGPL code; a
// test pins that both produce the same bytes.
package licensefile

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
)

// AlgEd25519 is the only signature algorithm of a licence.
const AlgEd25519 = "Ed25519"

// depth bounds the nesting of a licence file.
const depth = 8

// Signature is one signature over the signed part.
type Signature struct {
	KeyID string `json:"keyid"`
	Alg   string `json:"alg"`
	Sig   string `json:"sig"`
}

// document is the envelope on disk.
type document struct {
	Signed     json.RawMessage `json:"signed"`
	Signatures []Signature     `json:"signatures"`
}

// File is a parsed licence file. Parsing checks the shape only; Verify
// checks the signature.
type File struct {
	// Payload is the signed content.
	Payload Payload
	// Signed is the canonical JSON the signatures cover.
	Signed []byte
	// Signatures are the file's signatures.
	Signatures []Signature
}

// KeyID is the identifier of a public key: the first sixteen bytes of its
// SHA-256 in lower-case hexadecimal, as the update metadata derives it.
func KeyID(public ed25519.PublicKey) string {
	sum := sha256.Sum256(public)
	return hex.EncodeToString(sum[:16])
}

// Canonical returns the bytes a signature covers: the RFC 8785 form of the
// payload.
func Canonical(p Payload) ([]byte, error) {
	plain, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("licensefile: %w", err)
	}
	out, err := jcs.Canonicalize(plain, depth)
	if err != nil {
		return nil, fmt.Errorf("licensefile: %w", err)
	}
	return out, nil
}

// Marshal returns the licence file for a payload and its signatures, in
// canonical form, signatures ordered by key identifier.
func Marshal(p Payload, signatures []Signature) ([]byte, error) {
	canonical, err := Canonical(p)
	if err != nil {
		return nil, err
	}
	sigs := slices.Clone(signatures)
	if sigs == nil {
		sigs = []Signature{}
	}
	slices.SortFunc(sigs, func(a, b Signature) int {
		if a.KeyID != b.KeyID {
			return cmpString(a.KeyID, b.KeyID)
		}
		return cmpString(a.Alg, b.Alg)
	})
	plain, err := json.Marshal(document{Signed: canonical, Signatures: sigs})
	if err != nil {
		return nil, fmt.Errorf("licensefile: %w", err)
	}
	out, err := jcs.Canonicalize(plain, depth+1)
	if err != nil {
		return nil, fmt.Errorf("licensefile: %w", err)
	}
	return out, nil
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Parse reads a licence file and checks the shape of its payload. It does
// not check the signature: use Verify, and trust nothing in the result until
// it has passed.
func Parse(raw []byte) (File, error) {
	var d document
	if err := decodeStrict(raw, &d); err != nil {
		return File{}, err
	}
	if len(d.Signed) == 0 {
		return File{}, fmt.Errorf("%w: no signed part", ErrFormat)
	}
	canonical, err := jcs.Canonicalize(d.Signed, depth)
	if err != nil {
		return File{}, fmt.Errorf("%w: %w", ErrFormat, err)
	}
	var p Payload
	if err := decodeStrict(canonical, &p); err != nil {
		return File{}, err
	}
	for _, s := range d.Signatures {
		if s.KeyID == "" || s.Alg == "" || s.Sig == "" {
			return File{}, fmt.Errorf("%w: a signature is malformed", ErrFormat)
		}
	}
	if err := p.Validate(); err != nil {
		return File{}, err
	}
	return File{Payload: p, Signed: canonical, Signatures: d.Signatures}, nil
}

// Verify reports whether a key in keys signed the file: the signature must
// name the key's identifier and Ed25519 and verify over the signed bytes.
// Signatures by unknown keys count for nothing.
func (f File) Verify(keys KeySet) error {
	for _, s := range f.Signatures {
		pub, ok := keys.Lookup(s.KeyID)
		if !ok || s.Alg != AlgEd25519 {
			continue
		}
		sig, err := base64.StdEncoding.DecodeString(s.Sig)
		if err != nil {
			continue
		}
		if ed25519.Verify(pub, f.Signed, sig) {
			return nil
		}
	}
	return ErrSignature
}
