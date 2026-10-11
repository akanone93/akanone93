// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// Verifies: SEC-031.
// A call reaches the upstream under its base URL with the Plux assertion,
// and nothing of the device's credentials: the assertion names the device,
// app, environment and assurance level, is bound to the request and lives
// a minute.
func TestCallForwardsWithAssertion(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	r := Request{
		Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts/42?expand=history",
		Header: http.Header{
			"Authorization": {"DPoP device-token"}, "Dpop": {"proof"}, "Cookie": {"session=1"}, "Accept": {"application/json"},
			"X-Anything": {"else"}, "Idempotency-Key": {"k-1"},
		},
	}
	res, err := f.call(r, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.Status != 200 || string(res.Body) != `{"ok":true}` {
		t.Errorf("response = %d %s", res.Status, res.Body)
	}
	got := f.next()
	if got.Method != "GET" || got.URI != "/api/accounts/42?expand=history" {
		t.Errorf("the upstream saw %s %s", got.Method, got.URI)
	}
	for _, h := range []string{"Authorization", "Dpop", "Cookie", "X-Anything"} {
		if got.Header.Get(h) != "" {
			t.Errorf("the device's %s header reached the upstream", h)
		}
	}
	if got.Header.Get("Accept") != "application/json" || got.Header.Get("Idempotency-Key") != "k-1" {
		t.Errorf("allowed headers were dropped: %v", got.Header)
	}
	c := f.claims(got.Header.Get(AssertionHeader), AssertionType)
	want := map[string]string{"iss": testIssuer, "aud": f.ups["bank"].BaseURL, "sub": "dev-1", "app": "app-1", "env": "env-1", "org": "org-1", "al": "AL2"}
	for k, v := range want {
		if c[k] != v {
			t.Errorf("claim %s = %v, want %s", k, c[k], v)
		}
	}
	if c["exp"].(float64)-c["iat"].(float64) != 60 || c["jti"] == "" {
		t.Errorf("lifetime or jti wrong: %v", c)
	}
	if c["rh"] != RequestHash("GET", "/api/accounts/42?expand=history", nil) {
		t.Errorf("the request hash does not bind the request")
	}
	if _, ok := c["usr"]; ok {
		t.Error("an assertion with no user token names a user")
	}
}

// Verifies: SEC-031, SEC-105.
// The gateway refuses what it must not forward, and the upstream never
// hears of it.
func TestCallRefusals(t *testing.T) {
	t.Parallel()
	get := func(op, target string) Request {
		return Request{Upstream: "bank", Operation: op, Method: "GET", Target: target}
	}
	tests := []struct {
		name  string
		req   Request
		tweak func(*fixture)
		want  plxerr.Code
	}{
		{"unknown upstream", Request{Upstream: "nope", Method: "GET", Target: "/"}, nil, plxerr.GatewayUpstreamUnknown},
		{"method the upstream does not allow", Request{Upstream: "bank", Operation: "balance", Method: "DELETE", Target: "/accounts"}, nil, plxerr.GatewayDestinationRefused},
		{"method the operation does not allow", Request{Upstream: "bank", Operation: "balance", Method: "POST", Target: "/accounts"}, nil, plxerr.GatewayDestinationRefused},
		{"unknown operation", get("steal", "/accounts"), nil, plxerr.GatewayDestinationRefused},
		{"outside the operation's path", get("balance", "/admin"), nil, plxerr.GatewayDestinationRefused},
		{"prefix sibling", get("balance", "/accounts-admin"), nil, plxerr.GatewayDestinationRefused},
		{"dot segments", get("balance", "/accounts/../admin"), nil, plxerr.GatewayDestinationRefused},
		{"encoded dot segments", get("balance", "/accounts/%2e%2e/admin"), nil, plxerr.GatewayDestinationRefused},
		{"encoded slash", get("balance", "/accounts%2f..%2fadmin"), nil, plxerr.GatewayDestinationRefused},
		{"backslash", get("balance", `/accounts\..\admin`), nil, plxerr.GatewayDestinationRefused},
		{"empty segment", get("balance", "/accounts//1"), nil, plxerr.GatewayDestinationRefused},
		{"scheme-relative host", get("balance", "//evil.example/accounts"), nil, plxerr.GatewayDestinationRefused},
		{"absolute URL", get("balance", "https://evil.example/accounts"), nil, plxerr.GatewayDestinationRefused},
		{"fragment", get("balance", "/accounts#x"), nil, plxerr.GatewayDestinationRefused},
		{"control character", get("balance", "/accounts/\x00"), nil, plxerr.GatewayDestinationRefused},
		{"plugin without the capability", Request{Upstream: "bank", Operation: "plugin-only", Plugin: "other", Method: "GET", Target: "/x"}, nil, plxerr.GatewayDestinationRefused},
		{"assurance too low", get("balance", "/accounts"), func(f *fixture) { f.dev.Assurance = "AL0" }, plxerr.AssuranceInsufficient},
		{"forged assertion", Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts", Header: http.Header{"Plux-Assertion": {"x"}}}, nil, plxerr.GatewayDestinationRefused},
		{"signature in a header", Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts", Header: http.Header{"Plux-Transaction-Signature": {"x"}}}, nil, plxerr.TransactionRequiresGateway},
		{"signature for a plain operation", Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts", ChallengeID: "c", Signature: []byte{1}}, nil, plxerr.PreconditionFailed},
		{"private address the upstream does not allow", get("balance", "/accounts"), func(f *fixture) {
			u := f.ups["bank"]
			u.AllowPrivate = false
			u.BaseURL = strings.Replace(u.BaseURL, "http://", "https://", 1)
			f.ups["bank"] = u
		}, plxerr.GatewayDestinationRefused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			if tt.tweak != nil {
				tt.tweak(f)
			}
			_, err := f.call(tt.req, nil)
			if code(err) != tt.want {
				t.Fatalf("error = %v (code %d), want code %d", err, code(err), tt.want)
			}
			f.none()
		})
	}
}

// Verifies: SEC-105.
// A name that resolves to a private address is refused at the connection,
// not only when the URL is registered, so DNS rebinding cannot reach the
// internal network; an upstream that allows private addresses is reached.
func TestPrivateAddressRefusedAtConnect(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	u := f.ups["bank"]
	u.AllowPrivate = false
	f.ups["bank"] = u // http://127.0.0.1 is valid only for an upstream that allows private addresses
	if err := u.Validate(); code(err) != plxerr.InvalidFormat {
		t.Fatalf("registering an http upstream that forbids private addresses: %v", err)
	}
	r := Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts"}
	if _, err := f.call(r, nil); code(err) != plxerr.GatewayDestinationRefused {
		t.Errorf("loopback upstream, private addresses refused: %v", err)
	}
	f.none()
	u.AllowPrivate, u.Version = true, 2
	f.ups["bank"] = u
	if _, err := f.call(r, nil); err != nil {
		t.Errorf("loopback upstream, private addresses allowed: %v", err)
	}
	f.next()
}

// Verifies: SEC-105.
// Link-local and metadata addresses stay refused even for an upstream that
// allows private ones.
func TestMetadataAddressAlwaysRefused(t *testing.T) {
	t.Parallel()
	u := Upstream{AllowPrivate: true}
	policy := policyFor(u)
	for _, a := range []string{"169.254.169.254", "fe80::1", "224.0.0.1"} {
		if err := policy.Check(mustAddr(a)); err == nil {
			t.Errorf("%s is reachable", a)
		}
	}
	for _, a := range []string{"10.1.2.3", "192.168.0.9", "127.0.0.1"} {
		if err := policy.Check(mustAddr(a)); err != nil {
			t.Errorf("%s is refused for an upstream that allows private addresses: %v", a, err)
		}
	}
	if err := policyFor(Upstream{}).Check(mustAddr("10.1.2.3")); err == nil {
		t.Error("a private address is reachable by default")
	}
}

// Verifies: LIM-001, SEC-031.
// The rate, the request size, the response size and the timeout come from
// the limits registry.
func TestCallLimits(t *testing.T) {
	t.Parallel()
	t.Run("rate", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.limits = mustTighten(t, f.limits, limits.GatewayRequestsPerMinutePerDevice, 2)
		r := Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts"}
		for range 2 {
			if _, err := f.call(r, nil); err != nil {
				t.Fatalf("within the rate: %v", err)
			}
			f.next()
		}
		if _, err := f.call(r, nil); code(err) != plxerr.GatewayRateLimited {
			t.Errorf("over the rate: %v", err)
		}
		f.clock.Advance(2 * time.Minute)
		if _, err := f.call(r, nil); err != nil {
			t.Errorf("after the window: %v", err)
		}
	})
	t.Run("request size", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.limits = mustTighten(t, f.limits, limits.GatewayRequestBytes, 16)
		u := f.ups["bank"]
		u.Operations = nil
		f.ups["bank"] = u
		r := Request{Upstream: "bank", Method: "POST", Target: "/x"}
		if _, err := f.call(r, make([]byte, 17)); code(err) != plxerr.LimitExceeded {
			t.Errorf("a body over the limit: %v", err)
		}
		f.none()
		if _, err := f.call(r, make([]byte, 16)); err != nil {
			t.Errorf("a body at the limit: %v", err)
		}
	})
	t.Run("response size", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.limits = mustTighten(t, f.limits, limits.GatewayResponseBytes, 8)
		f.handler = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(make([]byte, 9)) }
		r := Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts"}
		if _, err := f.call(r, nil); code(err) != plxerr.GatewayUpstreamFailed {
			t.Errorf("a response over the limit: %v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		f.handler = func(http.ResponseWriter, *http.Request) { <-release }
		u := f.ups["bank"]
		u.Timeout = 50 * time.Millisecond
		f.ups["bank"] = u
		r := Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts"}
		if _, err := f.call(r, nil); code(err) != plxerr.GatewayUpstreamFailed {
			t.Errorf("a slow upstream: %v", err)
		}
	})
}

// Verifies: SEC-031.
// A redirect is returned to the device, not followed, so the assertion
// never travels to a host nobody registered.
func TestRedirectsAreNotFollowed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.handler = func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, "http://127.0.0.1:1/elsewhere", http.StatusFound)
	}
	res, err := f.call(Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts"}, nil)
	if err != nil || res.Status != http.StatusFound {
		t.Fatalf("call: %v %d", err, res.Status)
	}
	f.next()
	f.none()
}

// Verifies: SEC-031.
// An API key and an OAuth token authenticate the call; the OAuth token is
// fetched once and reused until it expires.
func TestUpstreamAuthentication(t *testing.T) {
	t.Parallel()
	t.Run("api key", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.secrets["bank_key"] = []byte("s3cret-key\n")
		u := f.ups["bank"]
		u.Auth = Auth{Kind: AuthAPIKey, Header: "X-Api-Key", KeySecret: "bank_key"}
		f.ups["bank"] = u
		if _, err := f.call(Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts"}, nil); err != nil {
			t.Fatal(err)
		}
		if got := f.next().Header.Get("X-Api-Key"); got != "s3cret-key" {
			t.Errorf("api key = %q", got)
		}
	})
	t.Run("oauth", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		tokens := 0
		idp := newTokenServer(t, func() (string, int) { tokens++; return "tok-" + string(rune('0'+tokens)), 120 })
		f.secrets["bank_secret"] = []byte("client-secret")
		u := f.ups["bank"]
		u.Auth = Auth{Kind: AuthOAuth, TokenURL: idp.URL, ClientID: "plux", ClientSecret: "bank_secret", Scopes: []string{"read", "pay"}}
		f.ups["bank"] = u
		r := Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts"}
		for i, want := range []string{"Bearer tok-1", "Bearer tok-1", "Bearer tok-2"} {
			if i == 2 {
				f.clock.Advance(100 * time.Second) // within 30 s of the expiry: fetched again
			}
			if _, err := f.call(r, nil); err != nil {
				t.Fatal(err)
			}
			if got := f.next().Header.Get("Authorization"); got != want {
				t.Errorf("call %d: Authorization = %q, want %q", i, got, want)
			}
		}
		if tokens != 2 {
			t.Errorf("the token endpoint was called %d times, want 2", tokens)
		}
	})
	t.Run("the secret cannot be read", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		u := f.ups["bank"]
		u.Auth = Auth{Kind: AuthAPIKey, Header: "X-Api-Key", KeySecret: "missing"}
		f.ups["bank"] = u
		_, err := f.call(Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts"}, nil)
		if code(err) != plxerr.GatewayUpstreamFailed {
			t.Errorf("call: %v", err)
		}
		f.none()
	})
}

// Verifies: SEC-080, SEC-007.
// The plugin named in a call must be one the operation lists, and the
// assurance an operation asks for is the highest of the upstream's and its
// own.
func TestOperationPolicy(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	r := Request{Upstream: "bank", Operation: "plugin-only", Plugin: "shop", Method: "GET", Target: "/x"}
	if _, err := f.call(r, nil); err != nil {
		t.Fatalf("the plugin that declared the capability: %v", err)
	}
	f.next()
	f.dev.Assurance = "AL1" // the upstream asks AL1, the transfer operation AL2
	body := []byte(`{}`)
	_, err := f.gw.Begin(t.Context(), f.dev, Request{Upstream: "bank", Operation: "transfer", Method: "POST", Target: "/transfers", ChallengeID: "c", Signature: []byte{1}})
	if code(err) != plxerr.AssuranceInsufficient {
		t.Errorf("AL1 for an AL2 operation: %v (body %s)", err, body)
	}
}

// Verifies: SEC-031.
// The published key set holds the keys of one class and verifies an
// assertion; it holds nothing private.
func TestJWKSVerifiesAssertions(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.call(Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts"}, nil); err != nil {
		t.Fatal(err)
	}
	assertion := f.next().Header.Get(AssertionHeader)
	keys, err := f.signer.TokenKeys(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := JWKS(keys, signing.TokenDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(raw, &set); err != nil || len(set.Keys) == 0 {
		t.Fatalf("key set %s: %v", raw, err)
	}
	jws, err := jose.ParseSignedCompact(assertion, []jose.SignatureAlgorithm{jose.ES256})
	if err != nil {
		t.Fatal(err)
	}
	found := set.Key(jws.Signatures[0].Header.KeyID)
	if len(found) != 1 || !found[0].IsPublic() {
		t.Fatalf("the assertion's key is not in the set")
	}
	if _, err := jws.Verify(found[0].Key); err != nil {
		t.Errorf("verify: %v", err)
	}
	if prod, _ := JWKS(keys, signing.TokenProduction); strings.Contains(string(prod), found[0].KeyID) {
		t.Error("a development key is in the production set")
	}
}

// Verifies: SEC-030, SEC-028.
// A call that names no operation, the load of a data source, is matched by
// its method and path: a path that belongs to a transaction still needs
// its confirmation, and a path no operation covers falls under the
// upstream's own rules.
func TestCallWithoutOperationIsMatchedByPath(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.call(Request{Upstream: "bank", Method: "GET", Target: "/accounts/1"}, nil); err != nil {
		t.Fatalf("a load under an operation's path: %v", err)
	}
	f.next()
	if _, err := f.call(Request{Upstream: "bank", Method: "POST", Target: "/transfers"}, []byte(payment)); code(err) != plxerr.SCAChallengeInvalid {
		t.Errorf("a transfer with no operation named: %v", err)
	}
	f.none()
	f.dev.Assurance = "AL1"
	if _, err := f.call(Request{Upstream: "bank", Method: "POST", Target: "/transfers"}, []byte(payment)); code(err) != plxerr.AssuranceInsufficient {
		t.Errorf("the transfer's assurance applies to the inferred operation: %v", err)
	}
	if _, err := f.call(Request{Upstream: "bank", Method: "POST", Target: "/elsewhere"}, []byte(`{}`)); err != nil {
		t.Errorf("a path no operation covers: %v", err)
	}
	f.next()
}
