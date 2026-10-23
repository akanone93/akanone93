// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/nightCode42/plux3/backend/edition/licensefile"
)

// GenerateKey creates a licence-signing key pair. rand is crypto/rand.Reader
// outside tests. The private key must stay offline (ADR-0084).
func GenerateKey(random io.Reader) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	if random == nil {
		random = rand.Reader
	}
	pub, priv, err := ed25519.GenerateKey(random)
	if err != nil {
		return nil, nil, fmt.Errorf("license: generate key: %w", err)
	}
	return pub, priv, nil
}

// Sign validates a payload, fills the default grace period, and returns the
// signed licence file. It refuses a payload the verifier would refuse, so a
// file that leaves the signing machine is one that parses.
func Sign(p licensefile.Payload, key ed25519.PrivateKey) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("license: the signing key must be %d bytes", ed25519.PrivateKeySize)
	}
	if p.GraceDays == nil {
		g := licensefile.DefaultGraceDays
		p.GraceDays = &g
	}
	if p.Features == nil {
		p.Features = []string{}
	}
	if p.Limits == nil {
		p.Limits = map[string]int64{}
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("license: %w", err)
	}
	canonical, err := licensefile.Canonical(p)
	if err != nil {
		return nil, fmt.Errorf("license: %w", err)
	}
	pub, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("license: the signing key has no Ed25519 public half")
	}
	sig := licensefile.Signature{
		KeyID: licensefile.KeyID(pub), Alg: licensefile.AlgEd25519,
		Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(key, canonical)),
	}
	out, err := licensefile.Marshal(p, []licensefile.Signature{sig})
	if err != nil {
		return nil, fmt.Errorf("license: encode the signed licence: %w", err)
	}
	return out, nil
}
