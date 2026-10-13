// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

const (
	// MaxGrantLifetime is the longest a grant is valid, whatever the
	// invocation's timeout (ADR-0064).
	MaxGrantLifetime = 60 * time.Second
	// GrantMargin is how long a grant outlives its invocation's timeout,
	// so that the runner's last host calls are still authorised.
	GrantMargin = 5 * time.Second

	grantSecretName = "function-grant"
	grantContext    = "plux-function-grant-v1\x00"
	grantFormat     = 1
	grantNonceBytes = 16
	grantKeyBytes   = 32
)

// Grant is the opaque token that authorises one invocation's host calls
// (ADR-0064). It is a credential: it never appears in a log record, an
// error or a trace, and formatting it prints a marker.
type Grant string

// String hides the token from fmt verbs.
func (Grant) String() string { return "grant:redacted" }

// GoString hides the token from %#v.
func (Grant) GoString() string { return "grant:redacted" }

// LogValue hides the token from structured logs.
func (Grant) LogValue() slog.Value { return slog.StringValue("grant:redacted") }

// GrantClaims is what a grant is scoped to: one app, environment,
// function and version, the module that version runs and the depth of the
// invocation within its call chain (FN-012, ADR-0064).
type GrantClaims struct {
	OrganizationID string
	AppID          string
	EnvironmentID  string
	Function       string
	Version        int32
	// ModuleSHA256 is the hex SHA-256 of the module the grant lets the
	// runner fetch.
	ModuleSHA256 string
	// Depth is 0 for a device's invocation and one more for each nested
	// call (FN-012).
	Depth     uint32
	ExpiresAt time.Time
	// Nonce makes every grant unique.
	Nonce [grantNonceBytes]byte
}

// KVScope is the key-value namespace the grant reaches: the grant's app,
// environment and function, never one the module names (FN-018).
func (c GrantClaims) KVScope() KVScope {
	return KVScope{OrganizationID: c.OrganizationID, AppID: c.AppID, EnvironmentID: c.EnvironmentID, Function: c.Function}
}

// Grants issues and verifies grants. It holds one secret and no state, so
// it is safe for concurrent use and every replica that holds the secret
// verifies every grant (FN-012).
type Grants struct {
	key    []byte
	now    func() time.Time
	random io.Reader
}

// NewGrants returns the grants of an installation. key is the installation
// secret, 32 bytes; a nil clock uses time.Now and a nil source of
// randomness uses crypto/rand.
func NewGrants(key []byte, now func() time.Time, random io.Reader) (*Grants, error) {
	if len(key) != grantKeyBytes {
		return nil, errors.New("function: the grant secret is 32 bytes")
	}
	if now == nil {
		now = time.Now
	}
	if random == nil {
		random = rand.Reader
	}
	return &Grants{key: bytes.Clone(key), now: now, random: random}, nil
}

// OpenGrants returns the grants of an installation, creating the
// installation secret "function-grant" on first use. It is stored sealed
// by the crypter and every replica reads the same one.
func OpenGrants(ctx context.Context, db *storage.DB, c signing.Crypter, now func() time.Time) (*Grants, error) {
	key, err := signing.InstallationKey(ctx, db, c, grantSecretName)
	if err != nil {
		return nil, fmt.Errorf("function: %w", err)
	}
	return NewGrants(key, now, nil)
}

// Issue returns a grant for the claims, valid for the invocation's timeout
// plus GrantMargin and never longer than MaxGrantLifetime. It ignores the
// ExpiresAt and Nonce the caller sets, and returns the claims it signed.
func (g *Grants) Issue(c GrantClaims, timeout time.Duration) (Grant, GrantClaims, error) {
	if timeout <= 0 {
		return "", GrantClaims{}, errors.New("function: a grant needs the invocation's timeout")
	}
	if c.AppID == "" || c.EnvironmentID == "" || c.Function == "" || c.OrganizationID == "" {
		return "", GrantClaims{}, errors.New("function: a grant needs an organisation, app, environment and function")
	}
	life := min(timeout+GrantMargin, MaxGrantLifetime)
	c.ExpiresAt = g.now().Add(life).UTC().Truncate(time.Millisecond)
	if _, err := io.ReadFull(g.random, c.Nonce[:]); err != nil {
		return "", GrantClaims{}, fmt.Errorf("function: draw a grant nonce: %w", err)
	}
	payload := encodeClaims(c)
	return Grant(base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(g.mac(payload))), c, nil
}

// Verify checks a grant's authenticity and expiry and returns its claims.
// The payload is decoded only after the MAC verified, in constant time.
// Every failure is the same generic refusal, so a caller learns nothing
// about why a grant was refused.
func (g *Grants) Verify(token Grant) (GrantClaims, error) {
	refused := plxerr.New(plxerr.AuthenticationRequired, "the grant is not valid")
	body, sig, ok := strings.Cut(string(token), ".")
	if !ok || len(token) > maxGrantSize {
		return GrantClaims{}, refused
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return GrantClaims{}, refused
	}
	given, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(given, g.mac(payload)) {
		return GrantClaims{}, refused
	}
	c, err := decodeClaims(payload)
	if err != nil || !g.now().Before(c.ExpiresAt) {
		return GrantClaims{}, refused
	}
	return c, nil
}

// maxGrantSize bounds the token the verifier looks at.
const maxGrantSize = 2048

// mac authenticates a payload under the installation secret.
func (g *Grants) mac(payload []byte) []byte {
	h := hmac.New(sha256.New, g.key)
	h.Write([]byte(grantContext))
	h.Write(payload)
	return h.Sum(nil)
}

// encodeClaims is the canonical encoding that the MAC covers: a format
// byte, then each field in a fixed order, strings with a length prefix so
// that no field can run into the next.
func encodeClaims(c GrantClaims) []byte {
	b := []byte{grantFormat}
	for _, s := range []string{c.OrganizationID, c.AppID, c.EnvironmentID, c.Function, c.ModuleSHA256} {
		b = binary.AppendUvarint(b, uint64(len(s)))
		b = append(b, s...)
	}
	b = binary.AppendUvarint(b, uint64(c.Version)) //nolint:gosec // versions are positive
	b = binary.AppendUvarint(b, uint64(c.Depth))
	b = binary.AppendUvarint(b, uint64(c.ExpiresAt.UnixMilli())) //nolint:gosec // an expiry is after 1970
	return append(b, c.Nonce[:]...)
}

// decodeClaims reads what encodeClaims wrote, refusing anything else.
func decodeClaims(b []byte) (GrantClaims, error) {
	var c GrantClaims
	if len(b) == 0 || b[0] != grantFormat {
		return c, errors.New("function: unknown grant format")
	}
	r := bytes.NewReader(b[1:])
	for _, dst := range []*string{&c.OrganizationID, &c.AppID, &c.EnvironmentID, &c.Function, &c.ModuleSHA256} {
		n, err := binary.ReadUvarint(r)
		if err != nil || n > uint64(r.Len()) {
			return c, errors.New("function: malformed grant")
		}
		s := make([]byte, n)
		_, _ = io.ReadFull(r, s)
		*dst = string(s)
	}
	version, errV := binary.ReadUvarint(r)
	depth, errD := binary.ReadUvarint(r)
	expires, errE := binary.ReadUvarint(r)
	if errV != nil || errD != nil || errE != nil || version > 1<<31-1 || depth > 1<<31-1 || expires > 1<<62 || r.Len() != grantNonceBytes {
		return c, errors.New("function: malformed grant")
	}
	c.Version, c.Depth, c.ExpiresAt = int32(version), uint32(depth), time.UnixMilli(int64(expires)).UTC() //nolint:gosec // bounded above
	_, _ = io.ReadFull(r, c.Nonce[:])
	return c, nil
}
