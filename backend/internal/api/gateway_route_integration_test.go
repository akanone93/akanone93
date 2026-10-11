// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// routeUpstreams resolves one upstream for every device.
type routeUpstreams struct{ up gateway.Upstream }

func (r routeUpstreams) Resolve(_ context.Context, _, _, name string) (gateway.Upstream, error) {
	if name != r.up.Name {
		return gateway.Upstream{}, plxerr.New(plxerr.GatewayUpstreamUnknown, "no such upstream")
	}
	return r.up, nil
}

// routeFixture is the streaming route in front of an upstream, with the
// real device authentication of the world.
type routeFixture struct {
	w        *world
	dev      *testDevice
	srv      *httptest.Server
	upstream *httptest.Server
	seen     chan http.Header
}

func newRouteFixture(t *testing.T) *routeFixture {
	t.Helper()
	f := &routeFixture{w: newWorld(t), seen: make(chan http.Header, 8)}
	_, app, _ := f.w.appOnly(t)
	f.dev = f.w.registerDevelopment(t, app)
	f.dev.refresh(t)

	f.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.seen <- r.Header.Clone()
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "hello "+r.URL.Path) //nolint:gosec // G705: a test upstream echoing its path as plain text
	}))
	t.Cleanup(f.upstream.Close)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	up := gateway.Upstream{
		ID: "u", Name: "bank", BaseURL: f.upstream.URL + "/v1", Auth: gateway.Auth{Kind: gateway.AuthNone},
		AllowPrivate: true, Methods: []string{"GET"}, Version: 1,
	}
	gw, err := gateway.New(gateway.Options{
		Upstreams:  routeUpstreams{up},
		Egress:     gateway.NewEgress(nil, nil),
		Assertions: &gateway.Assertions{Signer: signing.TokenOnly(backend), Issuer: f.w.base},
		Limits:     func(context.Context, device.Identity) (limits.Set, error) { return limits.Defaults(), nil },
		Production: f.w.devices.EnvironmentIsProduction,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.srv = httptest.NewServer(api.GatewayRoute{Auth: f.w.deviceAuth, Gateway: gw})
	t.Cleanup(f.srv.Close)
	return f
}

// get sends a GET through the route with a proof for htm and htu (the
// path is the route's), adjusted by tweak.
func (f *routeFixture) get(t *testing.T, path string, tweak func(*devicetest.Proof, http.Header)) routeResponse {
	t.Helper()
	p := devicetest.Proof{Method: "GET", URL: f.w.base + strings.SplitN(path, "?", 2)[0], AccessToken: f.dev.token, Nonce: f.w.deviceAuth.Nonces.Current()}
	h := http.Header{"Authorization": {"DPoP " + f.dev.token}}
	if tweak != nil {
		tweak(&p, h)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header = h
	if req.Header.Get("DPoP") == "" {
		req.Header.Set("DPoP", devicetest.SignProof(t, f.dev.key, p))
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return routeResponse{StatusCode: res.StatusCode, Header: res.Header, Body: body}
}

// routeResponse is what the route answered, its body read.
type routeResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Verifies: SEC-030, SEC-021, SEC-029.
// The streaming route authenticates like every device call (token and
// proof bound to the request's own method and path, replay refused), then
// forwards with the assertion, and refuses what the gateway refuses.
func TestGatewayRoute(t *testing.T) {
	t.Parallel()
	f := newRouteFixture(t)

	res := f.get(t, "/gateway/v1/bank/accounts?x=1", nil)
	if res.StatusCode != http.StatusOK || string(res.Body) != "hello /v1/accounts" {
		t.Fatalf("a valid call: %d %q", res.StatusCode, res.Body)
	}
	if res.Header.Get("DPoP-Nonce") == "" {
		t.Error("the response carries no DPoP-Nonce")
	}
	seen := <-f.seen
	if seen.Get(gateway.AssertionHeader) == "" || seen.Get("Authorization") != "" || seen.Get("Dpop") != "" {
		t.Errorf("headers at the upstream: %v", seen)
	}

	// The same proof twice is a replay.
	proof := devicetest.SignProof(t, f.dev.key, devicetest.Proof{
		Method: "GET", URL: f.w.base + "/gateway/v1/bank/accounts", AccessToken: f.dev.token, Nonce: f.w.deviceAuth.Nonces.Current(),
	})
	use := func(p *devicetest.Proof, h http.Header) { h.Set("DPoP", proof) }
	if res := f.get(t, "/gateway/v1/bank/accounts", use); res.StatusCode != http.StatusOK {
		t.Fatalf("a fresh proof: %d", res.StatusCode)
	}
	<-f.seen
	if res := f.get(t, "/gateway/v1/bank/accounts", use); res.StatusCode != http.StatusUnauthorized || res.Header.Get("WWW-Authenticate") == "" {
		t.Errorf("a replayed proof: %d %q", res.StatusCode, res.Header.Get("WWW-Authenticate"))
	}

	for name, c := range map[string]struct {
		path  string
		tweak func(*devicetest.Proof, http.Header)
		want  int
	}{
		"a proof for another method": {"/gateway/v1/bank/accounts", func(p *devicetest.Proof, _ http.Header) { p.Method = "POST" }, http.StatusUnauthorized},
		"a proof for another path":   {"/gateway/v1/bank/accounts", func(p *devicetest.Proof, _ http.Header) { p.URL = f.w.base + "/gateway/v1/bank/other" }, http.StatusUnauthorized},
		"a proof with no nonce":      {"/gateway/v1/bank/accounts", func(p *devicetest.Proof, _ http.Header) { p.Nonce = "" }, http.StatusUnauthorized},
		"a proof an hour old":        {"/gateway/v1/bank/accounts", func(p *devicetest.Proof, _ http.Header) { p.IssuedAt = time.Now().Add(-time.Hour) }, http.StatusUnauthorized},
		"no credentials":             {"/gateway/v1/bank/accounts", func(_ *devicetest.Proof, h http.Header) { h.Del("Authorization"); h.Set("DPoP", "x") }, http.StatusUnauthorized},
		"an unknown upstream":        {"/gateway/v1/nope/accounts", nil, http.StatusNotFound},
		"dot segments":               {"/gateway/v1/bank/%2e%2e/admin", nil, http.StatusForbidden},
		"a plain call":               {"/gateway/v1/bank/accounts", nil, http.StatusOK},
	} {
		res := f.get(t, c.path, c.tweak)
		if res.StatusCode != c.want {
			t.Errorf("%s: status %d, want %d", name, res.StatusCode, c.want)
		}
		if c.want == http.StatusOK {
			<-f.seen
		}
	}
	select {
	case h := <-f.seen:
		t.Errorf("a refused call reached the upstream: %v", h)
	default:
	}
}
