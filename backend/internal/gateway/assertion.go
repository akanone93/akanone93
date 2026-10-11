// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/nightCode42/plux3/backend/internal/devtoken"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

const (
	// AssertionHeader carries the assertion to the upstream (SEC-031).
	AssertionHeader = "Plux-Assertion"
	// AssertionType is the JOSE typ of an assertion. An upstream must
	// require it: the same keys also sign access tokens and user tokens,
	// whose typ differs.
	AssertionType = "plux-assertion+jwt"
	// AssertionLifetime is how long an assertion is valid (ADR-0056).
	AssertionLifetime = 60 * time.Second
	// UserTokenType is the JOSE typ of the token ExchangeUserToken issues.
	UserTokenType = "plux-user+jwt"

	jtiBytes = 16
)

// User is a verified end user as the upstream learns of them (SEC-026).
type User struct {
	// Subject is the user's `sub` at the customer's identity provider.
	Subject string `json:"sub"`
	// Issuer is the identity provider that vouched for them.
	Issuer string `json:"iss"`
	// Claims are the claims the environment forwards.
	Claims map[string]any `json:"claims,omitempty"`
}

// SCAProof is what the gateway verified about a transaction (SEC-032).
type SCAProof struct {
	// ChallengeID names the single-use challenge that was answered.
	ChallengeID string `json:"cid"`
	// SummaryHash is the unpadded base64url SHA-256 of the signed summary.
	SummaryHash string `json:"sh"`
}

// AssertionClaims are what the gateway tells an upstream about a call.
type AssertionClaims struct {
	// Audience is the upstream's audience.
	Audience                                       string
	DeviceID, AppID, EnvironmentID, OrganizationID string
	// Assurance is the device's assurance level, "AL0" to "AL3".
	Assurance string
	// Production selects the class of key that signs.
	Production bool
	User       *User
	SCA        *SCAProof
	// RequestHash binds the assertion to the request (RequestHash).
	RequestHash string
}

// Assertions issues the Plux-Assertion JWT. It is safe for concurrent use.
type Assertions struct {
	// Signer signs through the signing package; the gateway never holds a
	// private key (L-3).
	Signer signing.TokenSigner
	// Issuer is the `iss` claim: the installation's public base URL.
	Issuer string
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
	// Random supplies identifiers; nil uses crypto/rand.
	Random io.Reader

	jws devtoken.Compact
}

// wireAssertion is the JSON form of the payload.
type wireAssertion struct {
	Iss string    `json:"iss"`
	Aud string    `json:"aud"`
	Sub string    `json:"sub"`
	App string    `json:"app"`
	Env string    `json:"env"`
	Org string    `json:"org"`
	AL  string    `json:"al"`
	Usr *User     `json:"usr,omitempty"`
	SCA *SCAProof `json:"sca,omitempty"`
	RH  string    `json:"rh,omitempty"`
	IAT int64     `json:"iat"`
	EXP int64     `json:"exp"`
	JTI string    `json:"jti"`
}

// Issue signs an assertion for a call.
func (a *Assertions) Issue(ctx context.Context, c AssertionClaims) (string, error) {
	if a.Signer == nil || a.Issuer == "" {
		return "", errors.New("gateway: the assertion issuer needs a signer and an issuer")
	}
	jti, err := randomID(a.Random)
	if err != nil {
		return "", err
	}
	now := time.Now()
	if a.Now != nil {
		now = a.Now()
	}
	payload, err := json.Marshal(wireAssertion{
		Iss: a.Issuer, Aud: c.Audience, Sub: c.DeviceID, App: c.AppID, Env: c.EnvironmentID, Org: c.OrganizationID,
		AL: c.Assurance, Usr: c.User, SCA: c.SCA, RH: c.RequestHash,
		IAT: now.Unix(), EXP: now.Add(AssertionLifetime).Unix(), JTI: jti,
	})
	if err != nil {
		return "", fmt.Errorf("gateway: encode an assertion: %w", err)
	}
	return a.jws.Sign(ctx, a.Signer, classOf(c.Production), AssertionType, payload) //nolint:wrapcheck // signing errors are descriptive
}

// classOf is the key class of an environment.
func classOf(production bool) signing.TokenClass {
	if production {
		return signing.TokenProduction
	}
	return signing.TokenDevelopment
}

// randomID returns a fresh identifier: sixteen random bytes in unpadded
// base64url.
func randomID(src io.Reader) (string, error) {
	if src == nil {
		src = rand.Reader
	}
	buf := make([]byte, jtiBytes)
	if _, err := io.ReadFull(src, buf); err != nil {
		return "", fmt.Errorf("gateway: draw an identifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// RequestHash binds an assertion to the request it accompanies: the
// unpadded base64url SHA-256 of the method, the path and query as
// forwarded, and the SHA-256 of the body, each field preceded by its
// length as four big-endian bytes. A body that was streamed rather than
// read has no hash, and its field is empty.
func RequestHash(method, pathAndQuery string, bodySHA256 []byte) string {
	h := sha256.New()
	for _, f := range [][]byte{[]byte(method), []byte(pathAndQuery), bodySHA256} {
		h.Write([]byte{byte(len(f) >> 24), byte(len(f) >> 16), byte(len(f) >> 8), byte(len(f))})
		h.Write(f)
	}
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
