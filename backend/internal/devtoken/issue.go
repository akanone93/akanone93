// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devtoken

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/nightCode42/plux3/backend/internal/signing"
)

// Issuer mints device access tokens. It is safe for concurrent use and
// must not be copied after first use.
type Issuer struct {
	// Signer signs the tokens; the api role passes signing.TokenOnly.
	Signer signing.TokenSigner
	// Issuer is the iss claim.
	Issuer string
	// Audience is the aud claim.
	Audience string
	// Lifetime is how long a token lives: more than zero and at most
	// MaxLifetime, in whole seconds.
	Lifetime time.Duration
	// Now is the clock; nil uses the wall clock.
	Now func() time.Time
	// Random supplies the token identifiers; nil uses the system source.
	Random io.Reader

	jws Compact
}

// Issue mints a token for the claims and returns it with its expiry.
// The signing key is the newest of the class (production if c.Production,
// development otherwise); its identifier is written into the header
// before signing, because the header is part of what is signed, and the
// identifier the signer reports is checked against it afterwards. When a
// rotation makes them differ the keys are read again and signing is
// retried once; a token is never returned with a kid that is not its
// signer's.
func (i *Issuer) Issue(ctx context.Context, c Claims) (string, time.Time, error) {
	if err := i.validate(c); err != nil {
		return "", time.Time{}, err
	}
	class := signing.TokenDevelopment
	if c.Production {
		class = signing.TokenProduction
	}
	jti, err := i.newID()
	if err != nil {
		return "", time.Time{}, err
	}
	now := i.now()
	iat := now.Unix()
	exp := iat + int64(i.lifetime(c)/time.Second)
	payload, err := json.Marshal(wire{
		Iss: i.Issuer, Aud: i.Audience, Sub: c.DeviceID, ClientID: c.AppID,
		Env: c.Environment, Org: c.OrganizationID, HB: c.HostBuild, AL: c.Assurance, Cnf: wireCnf{JKT: c.JKT},
		IAT: iat, EXP: exp, JTI: jti,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("devtoken: encode the claims: %w", err)
	}
	token, err := i.jws.Sign(ctx, i.Signer, class, tokenType, payload)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, time.Unix(exp, 0).UTC(), nil
}

// validate checks the issuer's configuration and the claims.
func (i *Issuer) validate(c Claims) error {
	switch {
	case i.Signer == nil:
		return errors.New("devtoken: the issuer needs a signer")
	case i.Issuer == "" || i.Audience == "":
		return errors.New("devtoken: the issuer needs an issuer and an audience")
	case i.lifetime(c) < time.Second || i.lifetime(c) > MaxLifetime:
		return fmt.Errorf("devtoken: the lifetime must be between one second and %s", MaxLifetime)
	case c.DeviceID == "" || c.AppID == "" || c.Environment == "" || c.OrganizationID == "" || c.JKT == "":
		return errors.New("devtoken: the claims need a device, an app, an environment, an organisation and a key thumbprint")
	case !validAssurance(c.Assurance):
		return errors.New("devtoken: the assurance level is not AL0 to AL3")
	}
	return nil
}

// lifetime is how long the token for c lives.
func (i *Issuer) lifetime(c Claims) time.Duration {
	if c.Lifetime > 0 {
		return c.Lifetime
	}
	return i.Lifetime
}

// now reads the clock.
func (i *Issuer) now() time.Time {
	if i.Now != nil {
		return i.Now()
	}
	return time.Now()
}

// newID returns a fresh token identifier: sixteen random bytes in
// unpadded base64url.
func (i *Issuer) newID() (string, error) {
	src := i.Random
	if src == nil {
		src = rand.Reader
	}
	buf := make([]byte, jtiBytes)
	if _, err := io.ReadFull(src, buf); err != nil {
		return "", fmt.Errorf("devtoken: draw a token identifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
