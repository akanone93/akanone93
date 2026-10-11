// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devtoken

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/nightCode42/plux3/backend/internal/signing"
)

// Compact signs JWTs in compact form with the token keys of a
// signing.TokenSigner: the access tokens of this package, and the
// assertions and user tokens the gateway issues (SEC-031, SEC-026). The
// zero value is ready to use; it is safe for concurrent use and must not
// be copied after first use.
type Compact struct {
	mu sync.Mutex
	// kids remembers the key identifier that signs for each class. The
	// signature's own key identifier is checked against it, so a
	// rotation shows up as a mismatch that clears the entry.
	kids map[signing.TokenClass]string
}

// Sign builds a JWT with the given typ header and payload, signed with the
// newest key of the class. The key identifier is written into the header
// before signing, because the header is part of what is signed, and the
// identifier the signer reports is checked against it afterwards. When a
// rotation makes them differ the keys are read again and signing is
// retried once; a token is never returned with a kid that is not its
// signer's.
func (c *Compact) Sign(ctx context.Context, signer signing.TokenSigner, class signing.TokenClass, typ string, payload []byte) (string, error) {
	if signer == nil {
		return "", errors.New("devtoken: a signer is required")
	}
	for attempt := 0; attempt < 2; attempt++ {
		token, ok, err := c.sign(ctx, signer, class, typ, payload)
		if err != nil {
			return "", err
		}
		if ok {
			return token, nil
		}
	}
	return "", errors.New("devtoken: the signing key changed twice while signing a token")
}

// sign builds and signs one token. It reports false, with the cached
// key identifier cleared, when the signer used another key than the one
// named in the header.
func (c *Compact) sign(ctx context.Context, signer signing.TokenSigner, class signing.TokenClass, typ string, payload []byte) (string, bool, error) {
	kid, err := c.keyID(ctx, signer, class)
	if err != nil {
		return "", false, err
	}
	header, err := json.Marshal(struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
		Kid string `json:"kid"`
	}{Alg: "ES256", Typ: typ, Kid: kid})
	if err != nil {
		return "", false, fmt.Errorf("devtoken: encode the header: %w", err)
	}
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	sig, signedBy, err := signer.SignToken(ctx, class, []byte(input))
	if err != nil {
		return "", false, fmt.Errorf("devtoken: sign: %w", err)
	}
	if signedBy != kid {
		c.mu.Lock()
		delete(c.kids, class)
		c.mu.Unlock()
		return "", false, nil
	}
	if len(sig) != signing.TokenSignatureSize {
		return "", false, fmt.Errorf("devtoken: the signer returned a %d-byte signature", len(sig))
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), true, nil
}

// keyID returns the identifier of the newest key of a class, reading the
// signer's keys when none is remembered.
func (c *Compact) keyID(ctx context.Context, signer signing.TokenSigner, class signing.TokenClass) (string, error) {
	c.mu.Lock()
	kid, ok := c.kids[class]
	c.mu.Unlock()
	if ok {
		return kid, nil
	}
	keys, err := signer.TokenKeys(ctx)
	if err != nil {
		return "", fmt.Errorf("devtoken: list the token keys: %w", err)
	}
	for _, k := range keys {
		if k.Class == class {
			kid = k.ID
		}
	}
	if kid == "" {
		return "", fmt.Errorf("devtoken: the signer has no %s token key", class)
	}
	c.mu.Lock()
	if c.kids == nil {
		c.kids = map[signing.TokenClass]string{}
	}
	c.kids[class] = kid
	c.mu.Unlock()
	return kid, nil
}
