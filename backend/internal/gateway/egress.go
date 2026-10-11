// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const (
	// connectionTTL is how long a replica keeps the connections it built
	// for an upstream, so a rotated client certificate takes effect.
	connectionTTL = 5 * time.Minute
	// tokenSkew is how long before its expiry an OAuth token stops being
	// used, so that it does not expire in flight.
	tokenSkew = 30 * time.Second
	// defaultTokenLifetime applies when a token endpoint names none.
	defaultTokenLifetime = 5 * time.Minute
	// maxTokenResponse bounds what a token endpoint may send.
	maxTokenResponse = 64 << 10
	// tokenTimeout bounds one request to a token endpoint.
	tokenTimeout = 10 * time.Second
	// maxConnections bounds the memory: past it, the cache starts over.
	maxConnections = 1024
)

// SecretReader opens an environment secret for the gateway. The value
// never leaves the process: it keys a connection or authenticates a call
// and is not logged (SEC-106, SEC-092).
type SecretReader interface {
	OpenSecret(ctx context.Context, organizationID, environmentID, key string) ([]byte, error)
}

// Egress holds the pooled connections to the upstreams, one HTTP/2-capable
// transport each, and the credentials that authenticate them (SEC-031).
// Every connection is checked against the upstream's address policy at the
// moment it is made, with the address the name resolved to, so a name that
// resolves to a private address later is refused too (SEC-105).
type Egress struct {
	secrets SecretReader
	now     func() time.Time
	// roots are the certificate authorities upstreams are verified with,
	// for an installation whose customers run a private CA; nil uses the
	// system's.
	roots *x509.CertPool

	mu      sync.Mutex
	entries map[string]*connection
}

// NewEgress returns the connection manager. now is the clock; nil uses
// time.Now.
func NewEgress(secrets SecretReader, now func() time.Time) *Egress {
	if now == nil {
		now = time.Now
	}
	return &Egress{secrets: secrets, now: now}
}

// WithRoots returns the connection manager verifying upstreams with the
// given authorities instead of the system's. Call it before first use.
func (e *Egress) WithRoots(roots *x509.CertPool) *Egress {
	e.roots = roots
	return e
}

// connection is the state kept for one upstream.
type connection struct {
	version   int64
	built     time.Time
	transport *http.Transport
	// header and value are the static credential, when there is one.
	header, value string

	tokenMu sync.Mutex
	token   string
	expires time.Time
}

// policyFor is the address policy of an upstream: private ranges are
// refused unless it allows them, and link-local, metadata, multicast and
// reserved ranges are refused always.
func policyFor(u Upstream) httpx.Policy {
	if !u.AllowPrivate {
		return httpx.Policy{}
	}
	return httpx.Policy{
		AllowLoopback: true,
		Allow: []netip.Prefix{
			netip.MustParsePrefix("10.0.0.0/8"),
			netip.MustParsePrefix("172.16.0.0/12"),
			netip.MustParsePrefix("192.168.0.0/16"),
			netip.MustParsePrefix("fc00::/7"),
		},
	}
}

// connectionFor returns the connection of an upstream, building it when
// there is none, when the upstream changed or when it is old.
func (e *Egress) connectionFor(ctx context.Context, u Upstream) (*connection, error) {
	now := e.now()
	e.mu.Lock()
	c := e.entries[u.ID]
	e.mu.Unlock()
	if c != nil && c.version == u.Version && now.Sub(c.built) < connectionTTL {
		return c, nil
	}
	built, err := e.build(ctx, u)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	if e.entries == nil || len(e.entries) >= maxConnections {
		for _, old := range e.entries {
			old.transport.CloseIdleConnections()
		}
		e.entries = map[string]*connection{}
	}
	if old := e.entries[u.ID]; old != nil {
		old.transport.CloseIdleConnections()
	}
	e.entries[u.ID] = built
	e.mu.Unlock()
	return built, nil
}

// build makes the connection of an upstream, reading the secrets its
// authentication names.
func (e *Egress) build(ctx context.Context, u Upstream) (*connection, error) {
	opts := httpx.TransportOptions{Policy: policyFor(u), MaxIdleConnsPerHost: 64}
	if e.roots != nil {
		opts.TLS = &tls.Config{RootCAs: e.roots, MinVersion: tls.VersionTLS12}
	}
	c := &connection{version: u.Version, built: e.now()}
	switch u.Auth.Kind {
	case AuthMTLS:
		pem, err := e.secret(ctx, u, u.Auth.MTLSSecret)
		if err != nil {
			return nil, err
		}
		cert, err := tls.X509KeyPair(pem, pem)
		if err != nil {
			return nil, plxerr.New(plxerr.GatewayUpstreamFailed, "the client certificate secret of the upstream is not a PEM certificate chain with its private key")
		}
		if opts.TLS == nil {
			opts.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		opts.TLS.Certificates = []tls.Certificate{cert}
	case AuthAPIKey:
		key, err := e.secret(ctx, u, u.Auth.KeySecret)
		if err != nil {
			return nil, err
		}
		c.header, c.value = u.Auth.Header, strings.TrimSpace(string(key))
	}
	c.transport = httpx.NewTransport(opts)
	return c, nil
}

// secret opens a secret of the upstream's environment. A failure is the
// upstream's, not the device's, and says only that the secret could not be
// read.
func (e *Egress) secret(ctx context.Context, u Upstream, key string) ([]byte, error) {
	if e.secrets == nil {
		return nil, plxerr.New(plxerr.GatewayUpstreamFailed, "the gateway cannot read the credentials of the upstream")
	}
	v, err := e.secrets.OpenSecret(ctx, u.OrganizationID, u.EnvironmentID, key)
	if err != nil {
		return nil, plxerr.Wrap(plxerr.GatewayUpstreamFailed, err, "the credentials of the upstream could not be read")
	}
	return v, nil
}

// Transport returns the pooled transport of an upstream.
func (e *Egress) Transport(ctx context.Context, u Upstream) (http.RoundTripper, error) {
	c, err := e.connectionFor(ctx, u)
	if err != nil {
		return nil, err
	}
	return c.transport, nil
}

// Credential returns the header that authenticates a call to the
// upstream: a static key, or an OAuth 2.0 bearer token, fetched once and
// reused until it expires. It reports false for an upstream whose
// authentication is a client certificate or only the assertion.
func (e *Egress) Credential(ctx context.Context, u Upstream) (name, value string, ok bool, err error) {
	c, err := e.connectionFor(ctx, u)
	if err != nil {
		return "", "", false, err
	}
	switch u.Auth.Kind {
	case AuthAPIKey:
		return c.header, c.value, true, nil
	case AuthOAuth:
		token, err := e.bearer(ctx, c, u)
		if err != nil {
			return "", "", false, err
		}
		return "Authorization", "Bearer " + token, true, nil
	}
	return "", "", false, nil
}

// bearer returns a valid OAuth access token, fetching one when the cached
// token is missing or about to expire. Calls for one upstream wait for a
// fetch in progress rather than starting their own.
func (e *Egress) bearer(ctx context.Context, c *connection, u Upstream) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token != "" && e.now().Add(tokenSkew).Before(c.expires) {
		return c.token, nil
	}
	secret, err := e.secret(ctx, u, u.Auth.ClientSecret)
	if err != nil {
		return "", err
	}
	token, lifetime, err := fetchToken(ctx, c.transport, u.Auth, strings.TrimSpace(string(secret)))
	if err != nil {
		return "", err
	}
	c.token, c.expires = token, e.now().Add(lifetime)
	return token, nil
}

// fetchToken runs the client credentials grant (RFC 6749 section 4.4).
func fetchToken(ctx context.Context, rt http.RoundTripper, a Auth, secret string) (string, time.Duration, error) {
	failed := func(why string) (string, time.Duration, error) {
		return "", 0, plxerr.New(plxerr.GatewayUpstreamFailed, "the token endpoint of the upstream %s", why)
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	if len(a.Scopes) > 0 {
		form.Set("scope", strings.Join(a.Scopes, " "))
	}
	ctx, cancel := context.WithTimeout(ctx, tokenTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return failed("has an address that cannot be used")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(url.QueryEscape(a.ClientID), url.QueryEscape(secret))
	resp, err := (&http.Client{Transport: rt, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		return failed("could not be reached")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return failed(fmt.Sprintf("refused the grant (status %d)", resp.StatusCode))
	}
	body, err := httpx.ReadAll(resp.Body, maxTokenResponse)
	if err != nil {
		return failed("sent an answer that is too large or cut short")
	}
	var out struct {
		AccessToken string          `json:"access_token"`
		TokenType   string          `json:"token_type"`
		ExpiresIn   json.RawMessage `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" || !strings.EqualFold(out.TokenType, "bearer") {
		return failed("did not return a bearer token")
	}
	return out.AccessToken, tokenLifetime(out.ExpiresIn), nil
}

// tokenLifetime reads expires_in, which is a number of seconds (some
// servers send it as a string).
func tokenLifetime(raw json.RawMessage) time.Duration {
	var seconds float64
	if err := json.Unmarshal(raw, &seconds); err != nil {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return defaultTokenLifetime
		}
		if err := json.Unmarshal([]byte(text), &seconds); err != nil {
			return defaultTokenLifetime
		}
	}
	if seconds <= 0 || seconds > 24*3600 {
		return defaultTokenLifetime
	}
	return time.Duration(seconds * float64(time.Second))
}

// Blocked reports whether an error from the transport is the address
// policy refusing a destination.
func Blocked(err error) bool { return errors.Is(err, httpx.ErrBlocked) }
