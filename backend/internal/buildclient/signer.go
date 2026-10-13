// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package buildclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/signing"
)

// DefaultKeyRef is the key reference of the function-artifact key
// (signing.keys.functionArtifact, FN-009, ADR-0063).
const DefaultKeyRef = "function-artifact"

// Signer signs artifact statements with one key of a signing backend. It
// implements function.ArtifactSigner and is safe for concurrent use when the
// backend is.
type Signer struct {
	signer signing.Signer
	keyRef string
}

// NewSigner returns a Signer for the key reference.
func NewSigner(signer signing.Signer, keyRef string) (*Signer, error) {
	if signer == nil || keyRef == "" {
		return nil, errors.New("buildclient.NewSigner: a signer and a key reference are required")
	}
	return &Signer{signer: signer, keyRef: keyRef}, nil
}

// SignStatement implements function.ArtifactSigner.
func (s *Signer) SignStatement(ctx context.Context, statement []byte) ([]byte, string, error) {
	signature, keyID, err := s.signer.Sign(ctx, s.keyRef, statement)
	if err != nil {
		return nil, "", fmt.Errorf("buildclient.Sign: %w", err)
	}
	return signature, keyID, nil
}
