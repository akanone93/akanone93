// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// The test identity provider of Plux Bank (SEC-026). It is the customer's
// identity provider in the reference set-up: the bank's login asks it for the
// token a signed-in user holds, and it publishes the keys the Plux server
// verifies that token with before it exchanges it. Environments name it with
// SetUserIdentityProvider: the issuer and the keys URL below, the audience
// idpAudience.
const (
	// idpPath is where the provider's endpoints are mounted.
	idpPath = "/idp"
	// idpAudience is the `aud` of the tokens it issues: the audience the
	// environment's provider registration accepts.
	idpAudience = "plux"
	// idpTokenTTL is how long a token lives.
	idpTokenTTL = time.Hour
	// idpDefaultIssuer is the issuer when the API is not told where it
	// listens, as in unit tests.
	idpDefaultIssuer = "https://refapi.invalid" + idpPath
)

// identityProvider issues ES256 tokens and publishes their key.
type identityProvider struct {
	key    *ecdsa.PrivateKey
	kid    string
	issuer string
	now    func() time.Time
}

// newIdentityProvider generates the provider's key. publicURL is where the
// API is reachable, without a trailing slash; empty gives a placeholder.
func newIdentityProvider(publicURL string) (*identityProvider, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating the identity provider's key: %w", err)
	}
	pub, err := key.PublicKey.Bytes()
	if err != nil {
		return nil, fmt.Errorf("encoding the identity provider's key: %w", err)
	}
	sum := sha256.Sum256(pub)
	issuer := idpDefaultIssuer
	if publicURL != "" {
		issuer = publicURL + idpPath
	}
	return &identityProvider{key: key, kid: hex.EncodeToString(sum[:8]), issuer: issuer, now: time.Now}, nil
}

// idpClaims are the claims of a user's token.
type idpClaims struct {
	Iss  string `json:"iss"`
	Aud  string `json:"aud"`
	Sub  string `json:"sub"`
	Name string `json:"name"`
	IAT  int64  `json:"iat"`
	EXP  int64  `json:"exp"`
}

// issue signs a token for the user sub.
func (p *identityProvider) issue(sub, name string) (string, error) {
	now := p.now()
	return signES256(p.key, jwsHeader{Typ: "JWT", Kid: p.kid}, idpClaims{
		Iss: p.issuer, Aud: idpAudience, Sub: sub, Name: name, IAT: now.Unix(), EXP: now.Add(idpTokenTTL).Unix(),
	})
}

// keys serves the provider's key set.
func (p *identityProvider) keys(w http.ResponseWriter, _ *http.Request) {
	k, err := publicJWK(&p.key.PublicKey, p.kid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "the key could not be published")
		return
	}
	w.Header().Set("Content-Type", "application/jwk-set+json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_ = json.NewEncoder(w).Encode(jwkSet{Keys: []jwk{k}})
}
