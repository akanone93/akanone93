// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

const testIssuer = "https://plux.example"

// clock is a settable clock.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// seen is what an upstream received.
type seen struct {
	Method, URI string
	Header      http.Header
	Body        []byte
}

// fakeUpstreams resolves from a map.
type fakeUpstreams map[string]Upstream

func (f fakeUpstreams) Resolve(_ context.Context, _, _, name string) (Upstream, error) {
	u, ok := f[name]
	if !ok {
		return Upstream{}, plxerr.New(plxerr.GatewayUpstreamUnknown, "no such upstream")
	}
	return u, nil
}

// fakeSecrets serves secrets by key.
type fakeSecrets map[string][]byte

func (f fakeSecrets) OpenSecret(_ context.Context, _, _, key string) ([]byte, error) {
	v, ok := f[key]
	if !ok {
		return nil, plxerr.New(plxerr.ResourceNotFound, "no such secret")
	}
	return v, nil
}

// fakeKeys holds SCA keys by device.
type fakeKeys map[string]device.SCAKey

func (f fakeKeys) SCAKey(_ context.Context, d device.Identity) (device.SCAKey, error) {
	k, ok := f[d.DeviceID]
	if !ok {
		return device.SCAKey{}, plxerr.New(plxerr.ResourceNotFound, "no key")
	}
	return k, nil
}

// fixture is a gateway in front of an httptest upstream.
type fixture struct {
	t        *testing.T
	clock    *clock
	signer   *signing.File
	cache    *cache.Memory
	secrets  fakeSecrets
	keys     fakeKeys
	upstream *httptest.Server
	requests chan seen
	// handler answers the upstream's requests; tests replace it.
	handler http.HandlerFunc
	ups     fakeUpstreams
	limits  limits.Set
	gw      *Gateway
	users   *UserTokens
	sca     *SCA
	egress  *Egress
	dev     device.Identity
	// scaKey is the private half of the device's SCA key.
	scaKey *ecdsa.PrivateKey
}

// newFixture builds the gateway, an upstream called "bank" with a
// transaction operation "transfer" and a plain one "balance", and a device.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		t: t, clock: newClock(), secrets: fakeSecrets{}, keys: fakeKeys{}, requests: make(chan seen, 64),
		ups: fakeUpstreams{}, limits: limits.Defaults(),
		dev: device.Identity{DeviceID: "dev-1", OrganizationID: "org-1", AppID: "app-1", EnvironmentID: "env-1", Assurance: "AL2"},
	}
	var err error
	if f.signer, err = signing.NewFile(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	f.cache = cache.NewMemory(f.clock.Now)
	f.handler = func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"ok":true}`) }
	f.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.requests <- seen{Method: r.Method, URI: r.RequestURI, Header: r.Header.Clone(), Body: body}
		r.Body = io.NopCloser(bytes.NewReader(body))
		f.handler(w, r)
	}))
	t.Cleanup(f.upstream.Close)

	f.scaKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f.keys[f.dev.DeviceID] = device.SCAKey{Public: &f.scaKey.PublicKey, JKT: "jkt", Storage: device.KeyStorageStrongBox}

	f.ups["bank"] = Upstream{
		ID: "u1", OrganizationID: "org-1", AppID: "app-1", EnvironmentID: "env-1", Name: "bank", Version: 1,
		BaseURL: f.upstream.URL + "/api", Auth: Auth{Kind: AuthNone}, AllowPrivate: true,
		Methods: []string{"GET", "POST"}, Assurance: "AL1",
		Operations: []Operation{
			{Name: "balance", Methods: []string{"GET"}, PathPrefix: "/accounts"},
			{
				Name: "transfer", Methods: []string{"POST"}, PathPrefix: "/transfers", Assurance: "AL2",
				Transaction: &Transaction{Amount: "/amount", Currency: "/currency", Payee: "/to/iban"},
			},
			{Name: "plugin-only", Methods: []string{"GET"}, Plugins: []string{"shop"}},
		},
	}
	f.build()
	return f
}

// build (re)creates the gateway from the fixture's parts.
func (f *fixture) build() {
	f.t.Helper()
	tokens := signing.TokenOnly(f.signer)
	keys := (&tokenKeyCache{signer: tokens}).Keys
	f.egress = NewEgress(f.secrets, f.clock.Now)
	f.users = &UserTokens{Signer: tokens, Keys: keys, Issuer: testIssuer, Now: f.clock.Now}
	f.sca = &SCA{Cache: f.cache, Keys: f.keys, Now: f.clock.Now}
	gw, err := New(Options{
		Upstreams: f.ups, Egress: f.egress,
		Assertions: &Assertions{Signer: tokens, Issuer: testIssuer, Now: f.clock.Now},
		Users:      f.users, SCA: f.sca,
		Limits: func(context.Context, device.Identity) (limits.Set, error) { return f.limits, nil },
		Count: func(ctx context.Context, key string, ttl time.Duration) (int64, error) {
			return f.cache.Increment(ctx, key, ttl)
		},
		Production: func(context.Context, string) (bool, error) { return false, nil },
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.gw = gw
}

// tokenKeyCache lists the signer's keys without caching, for tests.
type tokenKeyCache struct{ signer signing.TokenSigner }

func (c *tokenKeyCache) Keys(ctx context.Context) ([]signing.TokenKey, error) {
	return c.signer.TokenKeys(ctx)
}

// call authorises and sends a call whose body fits in memory.
func (f *fixture) call(r Request, body []byte) (Response, error) {
	f.t.Helper()
	ctx := context.Background()
	p, err := f.gw.Begin(ctx, f.dev, r)
	if err != nil {
		return Response{}, err
	}
	plan, err := p.Finish(ctx, body)
	if err != nil {
		return Response{}, err
	}
	return f.gw.Do(ctx, plan)
}

// next returns what the upstream received next.
func (f *fixture) next() seen {
	f.t.Helper()
	select {
	case s := <-f.requests:
		return s
	case <-time.After(5 * time.Second):
		f.t.Fatal("the upstream received nothing")
		return seen{}
	}
}

// none asserts the upstream received nothing.
func (f *fixture) none() {
	f.t.Helper()
	select {
	case s := <-f.requests:
		f.t.Fatalf("the upstream received %s %s, want nothing", s.Method, s.URI)
	case <-time.After(50 * time.Millisecond):
	}
}

// claims verifies a compact JWT of the given typ with the signer's keys
// and returns its payload.
func (f *fixture) claims(token, typ string) map[string]any {
	f.t.Helper()
	jws, err := jose.ParseSignedCompact(token, []jose.SignatureAlgorithm{jose.ES256})
	if err != nil {
		f.t.Fatalf("parse: %v", err)
	}
	if got, _ := jws.Signatures[0].Header.ExtraHeaders[jose.HeaderType].(string); got != typ {
		f.t.Fatalf("typ = %q, want %q", got, typ)
	}
	keys, err := f.signer.TokenKeys(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	for _, k := range keys {
		if k.ID == jws.Signatures[0].Header.KeyID {
			payload, err := jws.Verify(k.Public)
			if err != nil {
				f.t.Fatalf("verify: %v", err)
			}
			var out map[string]any
			if err := json.Unmarshal(payload, &out); err != nil {
				f.t.Fatal(err)
			}
			return out
		}
	}
	f.t.Fatal("no key signed the token")
	return nil
}

// code returns the Plux code of an error.
func code(err error) plxerr.Code {
	c, _ := plxerr.CodeOf(err)
	return c
}

// signTransaction answers a challenge as the device's SCA key would.
func (f *fixture) signTransaction(c Challenge, upstream, operation string, summary []byte) []byte {
	f.t.Helper()
	digest := sha256.Sum256(SigningInput(c.ID, upstream, operation, c.Bytes, summary))
	sig, err := ecdsa.SignASN1(rand.Reader, f.scaKey, digest[:])
	if err != nil {
		f.t.Fatal(err)
	}
	return sig
}

// mustTighten lowers a limit.
func mustTighten(t *testing.T, s limits.Set, k limits.Key, v int64) limits.Set {
	t.Helper()
	out, err := s.Tighten(k, limits.ScopeApp, v)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// mustAddr parses an address.
func mustAddr(s string) netip.Addr {
	a, err := netip.ParseAddr(s)
	if err != nil {
		panic(err)
	}
	return a
}

// newTokenServer is an OAuth token endpoint: next returns the token and
// its lifetime in seconds. It checks the client authenticates with Basic.
func newTokenServer(t *testing.T, next func() (string, int)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		if !ok || id != "plux" || secret != "client-secret" || r.FormValue("grant_type") != "client_credentials" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		token, life := next()
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": life})
	}))
	t.Cleanup(srv.Close)
	return srv
}
