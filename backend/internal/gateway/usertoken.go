// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"crypto/elliptic"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/nightCode42/plux3/backend/internal/devtoken"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// maxUserTokenSize bounds the token the verifier looks at.
const maxUserTokenSize = 8 << 10

// UserTokens issues and verifies the Plux user token that
// ExchangeUserToken returns and the gateway turns into the verified user
// of an assertion (SEC-026). The token is bound to one device, one app
// and environment and one upstream, so it is of no use anywhere else.
type UserTokens struct {
	// Signer signs through the signing package (L-3).
	Signer signing.TokenSigner
	// Keys lists the public token keys to verify with; the gateway caches
	// what it returns.
	Keys func(ctx context.Context) ([]signing.TokenKey, error)
	// Issuer is the `iss` claim: the installation's public base URL.
	Issuer string
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
	// Random supplies identifiers; nil uses crypto/rand.
	Random io.Reader

	jws devtoken.Compact
}

// BoundTo is what a user token must be bound to.
type BoundTo struct {
	DeviceID, AppID, EnvironmentID string
	// Upstream is the name of the upstream the call goes to.
	Upstream   string
	Production bool
}

// wireUser is the JSON form of the payload.
type wireUser struct {
	Iss string         `json:"iss"`
	Aud string         `json:"aud"`
	Sub string         `json:"sub"`
	Did string         `json:"did"`
	App string         `json:"app"`
	Env string         `json:"env"`
	IDP string         `json:"idp"`
	UC  map[string]any `json:"uc,omitempty"`
	IAT *float64       `json:"iat,omitempty"`
	EXP *float64       `json:"exp,omitempty"`
	JTI string         `json:"jti,omitempty"`
}

func (t *UserTokens) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

// Issue signs a user token for a device and an upstream, valid until
// expires.
func (t *UserTokens) Issue(ctx context.Context, b BoundTo, u User, expires time.Time) (string, error) {
	if t.Signer == nil || t.Issuer == "" {
		return "", errors.New("gateway: the user token issuer needs a signer and an issuer")
	}
	jti, err := randomID(t.Random)
	if err != nil {
		return "", err
	}
	iat, exp := float64(t.now().Unix()), float64(expires.Unix())
	payload, err := json.Marshal(wireUser{
		Iss: t.Issuer, Aud: b.Upstream, Sub: u.Subject, Did: b.DeviceID, App: b.AppID, Env: b.EnvironmentID,
		IDP: u.Issuer, UC: u.Claims, IAT: &iat, EXP: &exp, JTI: jti,
	})
	if err != nil {
		return "", fmt.Errorf("gateway: encode a user token: %w", err)
	}
	return t.jws.Sign(ctx, t.Signer, classOf(b.Production), UserTokenType, payload) //nolint:wrapcheck // signing errors are descriptive
}

func userInvalid(check string) error {
	return plxerr.New(plxerr.UserTokenInvalid, "user token failed check %q", check)
}

// Verify checks a user token against what it must be bound to and returns
// the user. Every failed check is UserTokenInvalid and names the check,
// never the value.
func (t *UserTokens) Verify(ctx context.Context, token string, b BoundTo) (User, error) {
	if t.Keys == nil || t.Issuer == "" {
		return User{}, errors.New("gateway: the user token verifier needs keys and an issuer")
	}
	if len(token) > maxUserTokenSize {
		return User{}, userInvalid("size")
	}
	jws, err := jose.ParseSignedCompact(token, []jose.SignatureAlgorithm{jose.ES256})
	if err != nil || len(jws.Signatures) != 1 {
		return User{}, userInvalid("format")
	}
	h := jws.Signatures[0].Header
	if typ, _ := h.ExtraHeaders[jose.HeaderType].(string); typ != UserTokenType {
		return User{}, userInvalid("typ")
	}
	if h.KeyID == "" || h.JSONWebKey != nil {
		return User{}, userInvalid("kid")
	}
	keys, err := t.Keys(ctx)
	if err != nil {
		return User{}, fmt.Errorf("gateway: load the token keys: %w", err)
	}
	var key *signing.TokenKey
	for i := range keys {
		if keys[i].ID == h.KeyID {
			key = &keys[i]
		}
	}
	if key == nil || key.Public == nil || key.Public.Curve != elliptic.P256() || (key.Class == signing.TokenProduction) != b.Production {
		return User{}, userInvalid("kid")
	}
	payload, err := jws.Verify(key.Public)
	if err != nil {
		return User{}, userInvalid("signature")
	}
	var w wireUser
	if err := json.Unmarshal(payload, &w); err != nil {
		return User{}, userInvalid("claims")
	}
	switch {
	case w.Iss != t.Issuer:
		return User{}, userInvalid("iss")
	case w.Aud != b.Upstream:
		return User{}, userInvalid("aud")
	case w.Did != b.DeviceID, w.App != b.AppID, w.Env != b.EnvironmentID:
		return User{}, userInvalid("binding")
	case w.Sub == "":
		return User{}, userInvalid("sub")
	case w.EXP == nil || w.IAT == nil || math.Abs(*w.EXP) > 1e11 || math.Abs(*w.IAT) > 1e11:
		return User{}, userInvalid("exp")
	}
	now := t.now()
	if !time.Unix(int64(*w.EXP), 0).After(now) || time.Unix(int64(*w.IAT), 0).After(now.Add(time.Minute)) {
		return User{}, userInvalid("exp")
	}
	return User{Subject: w.Sub, Issuer: w.IDP, Claims: w.UC}, nil
}
