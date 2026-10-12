// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// The bank trusts one thing about its caller: the Plux-Assertion the Plux
// gateway adds to every request it forwards (ADR-0056). It is an ES256 JWT of
// the type below, signed with a key of the server's published key set; it
// says which device calls, which user the identity provider vouched for and,
// for a transaction, that the user confirmed it with the device's SCA key. The
// bank never sees a device credential, a user token or a signature, and it
// accepts a request that carries no assertion from nobody.
const (
	// assertionHeader names the header that carries the assertion.
	assertionHeader = "Plux-Assertion"
	// assertionType is the `typ` of an assertion: the server's keys also sign
	// access tokens and user tokens, whose type differs.
	assertionType = "plux-assertion+jwt"
	// assertionLeeway is the clock skew tolerated.
	assertionLeeway = 30 * time.Second
	// keysTTL is how long a key set is reused; an unknown `kid` reads it
	// again, at most once per keysRefetchGap.
	keysTTL         = 10 * time.Minute
	keysRefetchGap  = 10 * time.Second
	maxKeySetBytes  = 64 << 10
	maxAssertionLen = 8 << 10
)

// GatewayTrust says which gateway the bank trusts.
type GatewayTrust struct {
	// JWKSURL is where the Plux server publishes the keys that verify
	// assertions: /.well-known/plux/gateway-jwks/{production,development}.
	JWKSURL string
	// Issuer is the `iss` of an assertion: the Plux server's public base URL.
	Issuer string
	// Audience is the `aud` of an assertion: the audience the upstream was
	// registered with.
	Audience string
	// Client reads the key set; nil uses http.DefaultClient.
	Client *http.Client
}

// assertion is the part of an assertion the bank uses.
type assertion struct {
	Device string
	JTI    string
	Level  string
	// User is the verified end user, or nil for a call without one.
	User *assertedUser
	// SCA is the proof the user confirmed this transaction, or nil.
	SCA *assertedSCA
}

type assertedUser struct {
	Subject string
	Issuer  string
}

type assertedSCA struct {
	ChallengeID string
	SummaryHash string
}

// wireAssertion is the JSON form of the claims.
type wireAssertion struct {
	Iss string `json:"iss"`
	Aud string `json:"aud"`
	Sub string `json:"sub"`
	AL  string `json:"al"`
	Usr *struct {
		Sub string `json:"sub"`
		Iss string `json:"iss"`
	} `json:"usr"`
	SCA *struct {
		CID string `json:"cid"`
		SH  string `json:"sh"`
	} `json:"sca"`
	RH  string `json:"rh"`
	IAT int64  `json:"iat"`
	EXP int64  `json:"exp"`
	JTI string `json:"jti"`
}

// verifier checks assertions.
type verifier struct {
	trust GatewayTrust
	now   func() time.Time

	mu      sync.Mutex
	keys    map[string]*ecdsa.PublicKey
	fetched time.Time
}

func newVerifier(t GatewayTrust) *verifier {
	return &verifier{trust: t, now: time.Now}
}

var errNotTrusted = errors.New("the request carries no valid assertion")

// verify checks the assertion of r. The body, when the request has one, is
// read to check the request hash and put back.
func (v *verifier) verify(ctx context.Context, r *http.Request) (*assertion, error) {
	if v.trust.JWKSURL == "" || v.trust.Issuer == "" || v.trust.Audience == "" {
		return nil, errors.New("no gateway is trusted")
	}
	raw := r.Header.Get(assertionHeader)
	if raw == "" || len(raw) > maxAssertionLen {
		return nil, errNotTrusted
	}
	jws, err := parseJWS(raw)
	if err != nil || jws.header.Typ != assertionType || jws.header.Kid == "" {
		return nil, errNotTrusted
	}
	key, err := v.key(ctx, jws.header.Kid)
	if err != nil || !jws.verify(key) {
		return nil, errNotTrusted
	}
	var w wireAssertion
	if err := json.Unmarshal(jws.payload, &w); err != nil {
		return nil, errNotTrusted
	}
	now := v.now()
	switch {
	case w.Iss != v.trust.Issuer, w.Aud != v.trust.Audience, w.Sub == "", w.JTI == "":
		return nil, errNotTrusted
	case now.After(time.Unix(w.EXP, 0).Add(assertionLeeway)), time.Unix(w.IAT, 0).After(now.Add(assertionLeeway)):
		return nil, errNotTrusted
	}
	if err := checkRequestHash(r, w.RH); err != nil {
		return nil, err
	}
	out := &assertion{Device: w.Sub, JTI: w.JTI, Level: w.AL}
	if w.Usr != nil && w.Usr.Sub != "" {
		out.User = &assertedUser{Subject: w.Usr.Sub, Issuer: w.Usr.Iss}
	}
	if w.SCA != nil && w.SCA.CID != "" {
		out.SCA = &assertedSCA{ChallengeID: w.SCA.CID, SummaryHash: w.SCA.SH}
	}
	return out, nil
}

// checkRequestHash binds the assertion to the request it came with: the hash
// of the method, the path with its query and the hash of the body, which is
// empty for a body the gateway streamed rather than read.
func checkRequestHash(r *http.Request, want string) error {
	var body []byte
	if r.Body != nil && r.Body != http.NoBody && r.Method != http.MethodGet {
		var err error
		if body, err = io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBody)); err != nil {
			return fmt.Errorf("reading the body: %w", err)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	uri := r.URL.RequestURI()
	if want == requestHash(r.Method, uri, nil) {
		return nil
	}
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		if want == requestHash(r.Method, uri, sum[:]) {
			return nil
		}
	}
	return errNotTrusted
}

// requestHash is the gateway's hash of a request: the unpadded base64url
// SHA-256 of the method, the path and query and the body's hash, each
// preceded by its length as four big-endian bytes.
func requestHash(method, uri string, bodySHA256 []byte) string {
	h := sha256.New()
	for _, f := range [][]byte{[]byte(method), []byte(uri), bodySHA256} {
		h.Write(binary.BigEndian.AppendUint32(nil, uint32(len(f)))) //nolint:gosec // G115: a request's fields are bounded
		h.Write(f)
	}
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// key finds the key a token names, reading the set when it is old or the
// key is unknown.
func (v *verifier) key(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	v.mu.Lock()
	k, ok := v.keys[kid]
	age := v.now().Sub(v.fetched)
	v.mu.Unlock()
	if ok && age < keysTTL {
		return k, nil
	}
	if !ok && v.keys != nil && age < keysRefetchGap {
		return nil, errNotTrusted
	}
	set, err := v.fetch(ctx)
	if err != nil {
		return nil, err
	}
	v.mu.Lock()
	v.keys, v.fetched = set, v.now()
	k, ok = set[kid]
	v.mu.Unlock()
	if !ok {
		return nil, errNotTrusted
	}
	return k, nil
}

// fetch reads the gateway's key set.
func (v *verifier) fetch(ctx context.Context) (map[string]*ecdsa.PublicKey, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.trust.JWKSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building the key request: %w", err)
	}
	client := v.trust.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reading the gateway's keys: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the gateway's keys answered %d", res.StatusCode)
	}
	var set jwkSet
	if err := json.NewDecoder(io.LimitReader(res.Body, maxKeySetBytes)).Decode(&set); err != nil {
		return nil, fmt.Errorf("the gateway's keys are malformed: %w", err)
	}
	out := map[string]*ecdsa.PublicKey{}
	for _, k := range set.Keys {
		if pub, err := k.key(); err == nil && k.Kid != "" {
			out[k.Kid] = pub
		}
	}
	return out, nil
}
