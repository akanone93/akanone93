// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package receipttest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"sync"
	"testing"
)

// Signer is a test ES256 signer holding a fresh P-256 key. It satisfies
// receipt.ES256Signer and is safe for concurrent use.
type Signer struct {
	key *ecdsa.PrivateKey

	mu    sync.Mutex
	roles []string
	fail  error
}

// NewSigner returns a signer with a new key.
func NewSigner(t testing.TB) *Signer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &Signer{key: key}
}

// Public returns the verification key of the requests it signs.
func (s *Signer) Public() *ecdsa.PublicKey { return &s.key.PublicKey }

// FailWith makes every later signature fail with err; nil restores signing.
func (s *Signer) FailWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = err
}

// Roles returns the roles it was asked to sign for, in order.
func (s *Signer) Roles() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.roles...)
}

// SignES256 signs a SHA-256 digest and returns r‖s.
func (s *Signer) SignES256(_ context.Context, role string, digest []byte) ([]byte, error) {
	s.mu.Lock()
	s.roles = append(s.roles, role)
	fail := s.fail
	s.mu.Unlock()
	if fail != nil {
		return nil, fail
	}
	if len(digest) != 32 {
		return nil, errors.New("receipttest: the digest is not a SHA-256 value")
	}
	r, ss, err := ecdsa.Sign(rand.Reader, s.key, digest)
	if err != nil {
		return nil, err //nolint:wrapcheck // a test helper
	}
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	ss.FillBytes(out[32:])
	return out, nil
}
