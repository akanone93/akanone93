// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// The RFC 8693 token types the exchange accepts and returns.
const (
	TokenTypeJWT         = "urn:ietf:params:oauth:token-type:jwt"          //nolint:gosec // G101: an RFC 8693 token type URN, not a credential
	TokenTypeAccessToken = "urn:ietf:params:oauth:token-type:access_token" //nolint:gosec // G101: an RFC 8693 token type URN, not a credential
	TokenTypeIDToken     = "urn:ietf:params:oauth:token-type:id_token"     //nolint:gosec // G101: an RFC 8693 token type URN, not a credential
)

const (
	// maxSubjectToken bounds the host's token.
	maxSubjectToken = 8 << 10
	// maxJWKS bounds a key set an identity provider publishes.
	maxJWKS = 256 << 10
	// jwksTTL is how long a replica reuses a key set it read.
	jwksTTL = 10 * time.Minute
	// jwksRefetchGap is the shortest interval between two reads of one key
	// set, so a token with an unknown key cannot make the server hammer the
	// provider.
	jwksRefetchGap = time.Minute
	// idpLeeway is the clock skew tolerated between the provider and this
	// server.
	idpLeeway = time.Minute
	// maxForwardedClaims bounds the claims one exchange forwards.
	maxForwardedBytes = 2 << 10
)

// idpAlgorithms are the signature algorithms an identity provider's token
// may use. HS256 and none are not among them: the provider's keys are
// public.
var idpAlgorithms = []jose.SignatureAlgorithm{jose.RS256, jose.PS256, jose.ES256, jose.ES384, jose.EdDSA}

// Exchanger turns the host's user token into the Plux user token (RFC
// 8693, SEC-026). It verifies the host's token against the customer's
// identity provider and nothing of the device's request.
type Exchanger struct {
	// Providers returns an environment's identity provider.
	Providers func(ctx context.Context, organizationID, environmentID string) (IdentityProvider, error)
	// Upstreams checks that the audience names a registered upstream.
	Upstreams func(ctx context.Context, organizationID, environmentID, name string) (Upstream, error)
	// Production says whether an environment is a production one.
	Production func(ctx context.Context, environmentID string) (bool, error)
	// Lifetime returns how long a user token lives for a device; the
	// exchange never outlives the host's own token.
	Lifetime func(ctx context.Context, d device.Identity) (time.Duration, error)
	// Tokens issues the Plux user token.
	Tokens *UserTokens
	// HTTP fetches key sets; it must be the SSRF-safe client (SEC-105).
	HTTP *http.Client
	// Now is the clock; nil uses time.Now.
	Now func() time.Time

	mu   sync.Mutex
	sets map[string]*keySet
}

// keySet is one provider's key set as this replica last read it.
type keySet struct {
	keys    []jose.JSONWebKey
	fetched time.Time
	until   time.Time
}

// Exchanged is the result of an exchange.
type Exchanged struct {
	Token     string
	ExpiresAt time.Time
}

func (x *Exchanger) now() time.Time {
	if x.Now != nil {
		return x.Now()
	}
	return time.Now()
}

// Exchange verifies subjectToken and issues a user token for the device
// and the upstream named by audience.
func (x *Exchanger) Exchange(ctx context.Context, d device.Identity, subjectToken, subjectType, audience string) (Exchanged, error) {
	switch subjectType {
	case TokenTypeJWT, TokenTypeAccessToken, TokenTypeIDToken:
	default:
		return Exchanged{}, plxerr.New(plxerr.InvalidFormat, "the subject token type %q is not supported", subjectType)
	}
	if subjectToken == "" {
		return Exchanged{}, plxerr.New(plxerr.MissingProperty, "the subject token is required")
	}
	idp, err := x.Providers(ctx, d.OrganizationID, d.EnvironmentID)
	if err != nil {
		return Exchanged{}, err
	}
	if _, err := x.Upstreams(ctx, d.OrganizationID, d.EnvironmentID, audience); err != nil {
		return Exchanged{}, err
	}
	claims, expires, err := x.verify(ctx, idp, subjectToken)
	if err != nil {
		return Exchanged{}, err
	}
	lifetime, err := x.Lifetime(ctx, d)
	if err != nil {
		return Exchanged{}, err
	}
	if until := x.now().Add(lifetime); until.Before(expires) {
		expires = until
	}
	sub, _ := claims["sub"].(string)
	user := User{Subject: sub, Issuer: idp.Issuer, Claims: forwarded(claims, idp.ForwardClaims)}
	production, err := x.Production(ctx, d.EnvironmentID)
	if err != nil {
		return Exchanged{}, err
	}
	token, err := x.Tokens.Issue(ctx, BoundTo{
		DeviceID: d.DeviceID, AppID: d.AppID, EnvironmentID: d.EnvironmentID, Upstream: audience, Production: production,
	}, user, expires)
	if err != nil {
		return Exchanged{}, err
	}
	return Exchanged{Token: token, ExpiresAt: expires.UTC().Truncate(time.Second)}, nil
}

// verify checks the host's token against the provider and returns its
// claims and expiry.
func (x *Exchanger) verify(ctx context.Context, idp IdentityProvider, token string) (map[string]any, time.Time, error) {
	if len(token) > maxSubjectToken {
		return nil, time.Time{}, userInvalid("size")
	}
	jws, err := jose.ParseSigned(token, idpAlgorithms)
	if err != nil || len(jws.Signatures) != 1 {
		return nil, time.Time{}, userInvalid("format")
	}
	h := jws.Signatures[0].Header
	if h.JSONWebKey != nil {
		return nil, time.Time{}, userInvalid("header parameters")
	}
	key, err := x.key(ctx, idp.JWKSURL, h.KeyID, h.Algorithm)
	if err != nil {
		return nil, time.Time{}, err
	}
	payload, err := jws.Verify(key)
	if err != nil {
		return nil, time.Time{}, userInvalid("signature")
	}
	var claims map[string]any
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	if err := dec.Decode(&claims); err != nil {
		return nil, time.Time{}, userInvalid("claims")
	}
	expires, err := x.checkClaims(claims, idp)
	return claims, expires, err
}

// checkClaims checks issuer, audience, subject and time.
func (x *Exchanger) checkClaims(claims map[string]any, idp IdentityProvider) (time.Time, error) {
	if iss, _ := claims["iss"].(string); iss != idp.Issuer {
		return time.Time{}, userInvalid("iss")
	}
	if !audienceMatches(claims["aud"], idp.Audiences) {
		return time.Time{}, userInvalid("aud")
	}
	if sub, _ := claims["sub"].(string); sub == "" || len(sub) > 256 {
		return time.Time{}, userInvalid("sub")
	}
	now := x.now()
	exp, ok := numericDate(claims["exp"])
	if !ok || !exp.Add(idpLeeway).After(now) {
		return time.Time{}, userInvalid("exp")
	}
	if nbf, ok := numericDate(claims["nbf"]); claims["nbf"] != nil && (!ok || nbf.After(now.Add(idpLeeway))) {
		return time.Time{}, userInvalid("nbf")
	}
	if iat, ok := numericDate(claims["iat"]); claims["iat"] != nil && (!ok || iat.After(now.Add(idpLeeway))) {
		return time.Time{}, userInvalid("iat")
	}
	return exp, nil
}

// numericDate reads a NumericDate claim.
func numericDate(v any) (time.Time, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return time.Time{}, false
	}
	f, err := n.Float64()
	if err != nil || f < 0 || f > 1e11 {
		return time.Time{}, false
	}
	return time.Unix(int64(f), 0), true
}

// audienceMatches reports whether an `aud` claim, a string or a list of
// strings, names one of the accepted audiences.
func audienceMatches(claim any, accepted []string) bool {
	switch a := claim.(type) {
	case string:
		return slices.Contains(accepted, a)
	case []any:
		for _, v := range a {
			if s, ok := v.(string); ok && slices.Contains(accepted, s) {
				return true
			}
		}
	}
	return false
}

// forwarded picks the claims an environment forwards: scalars and lists of
// strings, within a size bound.
func forwarded(claims map[string]any, names []string) map[string]any {
	out := map[string]any{}
	size := 0
	for _, name := range names {
		if name == "sub" {
			continue
		}
		v, ok := claims[name]
		if !ok || !simpleClaim(v) {
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil || size+len(raw)+len(name) > maxForwardedBytes {
			continue
		}
		size += len(raw) + len(name)
		out[name] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// simpleClaim reports whether a claim is a string, number or boolean, or a
// list of strings.
func simpleClaim(v any) bool {
	switch c := v.(type) {
	case string, bool, json.Number:
		return true
	case []any:
		for _, e := range c {
			if _, ok := e.(string); !ok {
				return false
			}
		}
		return true
	}
	return false
}

// key finds the provider's key for a token: by key identifier, or, for a
// provider that publishes one key, that key. A key identifier this replica
// does not know makes it read the set again, at most once a minute.
func (x *Exchanger) key(ctx context.Context, url, kid, alg string) (any, error) {
	set, err := x.keys(ctx, url, false)
	if err != nil {
		return nil, err
	}
	if k, ok := pick(set.keys, kid, alg); ok {
		return k, nil
	}
	set, err = x.keys(ctx, url, true)
	if err != nil {
		return nil, err
	}
	if k, ok := pick(set.keys, kid, alg); ok {
		return k, nil
	}
	return nil, userInvalid("kid")
}

// pick selects a key of a set.
func pick(keys []jose.JSONWebKey, kid, alg string) (any, bool) {
	var found []jose.JSONWebKey
	for _, k := range keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		if k.Algorithm != "" && alg != "" && k.Algorithm != alg {
			continue
		}
		if kid != "" && k.KeyID != kid {
			continue
		}
		found = append(found, k)
	}
	if len(found) != 1 {
		return nil, false
	}
	return found[0].Key, true
}

// keys returns a provider's key set, reading it when the replica has none,
// when it is old, or, if refresh is set, when the last read is not too
// recent.
func (x *Exchanger) keys(ctx context.Context, url string, refresh bool) (*keySet, error) {
	now := x.now()
	x.mu.Lock()
	set := x.sets[url]
	x.mu.Unlock()
	if set != nil && now.Before(set.until) && (!refresh || now.Sub(set.fetched) < jwksRefetchGap) {
		return set, nil
	}
	fetched, err := x.fetch(ctx, url)
	if err != nil {
		if set != nil {
			return set, nil
		}
		return nil, err
	}
	fetched.fetched, fetched.until = now, now.Add(jwksTTL)
	x.mu.Lock()
	if x.sets == nil || len(x.sets) >= maxConnections {
		x.sets = map[string]*keySet{}
	}
	x.sets[url] = fetched
	x.mu.Unlock()
	return fetched, nil
}

// fetch reads a key set through the SSRF-safe client.
func (x *Exchanger) fetch(ctx context.Context, url string) (*keySet, error) {
	if x.HTTP == nil {
		return nil, errors.New("gateway: the exchange needs an HTTP client")
	}
	ctx, cancel := context.WithTimeout(ctx, tokenTimeout)
	defer cancel()
	resp, err := httpx.Get(ctx, x.HTTP, url)
	if err != nil {
		return nil, plxerr.Wrap(plxerr.UpstreamUnavailable, err, "the identity provider's keys could not be read")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, plxerr.New(plxerr.UpstreamUnavailable, "the identity provider's keys could not be read (status %d)", resp.StatusCode)
	}
	body, err := httpx.ReadAll(resp.Body, maxJWKS)
	if err != nil {
		return nil, plxerr.Wrap(plxerr.UpstreamUnavailable, err, "the identity provider's key set is too large or cut short")
	}
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(body, &set); err != nil || len(set.Keys) == 0 {
		return nil, plxerr.New(plxerr.UpstreamUnavailable, "the identity provider published no usable keys")
	}
	public := set.Keys[:0]
	for _, k := range set.Keys {
		if k.Valid() && k.IsPublic() {
			public = append(public, k)
		}
	}
	if len(public) == 0 {
		return nil, plxerr.New(plxerr.UpstreamUnavailable, "the identity provider published no usable public keys")
	}
	return &keySet{keys: public}, nil
}
