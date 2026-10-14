// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// idp is a test identity provider: a key set and a way to mint tokens.
type idp struct {
	t       *testing.T
	srv     *httptest.Server
	fetches atomic.Int32
	keys    []jose.JSONWebKey
	private map[string]any
}

const idpIssuer = "https://login.customer.example"

func newIdP(t *testing.T) *idp {
	t.Helper()
	p := &idp{t: t, private: map[string]any{}}
	p.addKey("ec1", jose.ES256)
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		p.fetches.Add(1)
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: p.keys})
	}))
	t.Cleanup(p.srv.Close)
	return p
}

// addKey publishes a new key under kid.
func (p *idp) addKey(kid string, alg jose.SignatureAlgorithm) {
	p.t.Helper()
	var private, public any
	if alg == jose.RS256 {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			p.t.Fatal(err)
		}
		private, public = k, &k.PublicKey
	} else {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			p.t.Fatal(err)
		}
		private, public = k, &k.PublicKey
	}
	p.private[kid] = private
	p.keys = append(p.keys, jose.JSONWebKey{Key: public, KeyID: kid, Algorithm: string(alg), Use: "sig"})
}

// token mints a token with the standard claims, which edit may change.
func (p *idp) token(kid string, alg jose.SignatureAlgorithm, now time.Time, edit func(map[string]any)) string {
	p.t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: alg, Key: jose.JSONWebKey{Key: p.private[kid], KeyID: kid}}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		p.t.Fatal(err)
	}
	claims := map[string]any{
		"iss": idpIssuer, "aud": "plux-app", "sub": "user-7", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"email": "ada@example.com", "roles": []string{"customer"}, "secret_blob": map[string]any{"a": 1},
	}
	if edit != nil {
		edit(claims)
	}
	out, err := jwt.Signed(signer).Claims(claims).Serialize() //nolint:misspell // the go-jose API name
	if err != nil {
		p.t.Fatal(err)
	}
	return out
}

// exchanger builds the exchange for the fixture and the provider.
func (f *fixture) exchanger(p *idp) *Exchanger {
	return &Exchanger{
		Providers: func(context.Context, string, string) (IdentityProvider, error) {
			return IdentityProvider{
				Issuer: idpIssuer, JWKSURL: p.srv.URL, Audiences: []string{"plux-app"}, ForwardClaims: []string{"email", "roles", "secret_blob", "sub", "missing"},
			}, nil
		},
		Upstreams: func(ctx context.Context, org, env, name string) (Upstream, error) {
			return f.ups.Resolve(ctx, org, env, name)
		},
		Production: func(context.Context, string) (bool, error) { return false, nil },
		Lifetime:   func(context.Context, device.Identity) (time.Duration, error) { return 5 * time.Minute, nil },
		Tokens:     f.users, HTTP: p.srv.Client(), Now: f.clock.Now,
	}
}

// Verifies: SEC-026.
// The host's user token is verified against the customer's key set and
// exchanged for a token bound to the device and the upstream; the gateway
// forwards the verified user, with the claims the environment names, in
// the assertion.
func TestExchangeUserTokenAndForwardUser(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	p := newIdP(t)
	x := f.exchanger(p)
	res, err := x.Exchange(t.Context(), f.dev, p.token("ec1", jose.ES256, f.clock.Now(), nil), TokenTypeJWT, "bank")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if want := f.clock.Now().Add(5 * time.Minute); !res.ExpiresAt.Equal(want) {
		t.Errorf("expires at %v, want %v: the token lives as long as the lifetime says", res.ExpiresAt, want)
	}
	if _, err := f.call(Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts", UserToken: res.Token}, nil); err != nil {
		t.Fatalf("call with the user token: %v", err)
	}
	claims := f.claims(f.next().Header.Get(AssertionHeader), AssertionType)
	usr, _ := claims["usr"].(map[string]any)
	if usr["sub"] != "user-7" || usr["iss"] != idpIssuer {
		t.Fatalf("usr = %v", usr)
	}
	forwarded, _ := usr["claims"].(map[string]any)
	if forwarded["email"] != "ada@example.com" || forwarded["roles"] == nil {
		t.Errorf("forwarded claims = %v", forwarded)
	}
	if _, ok := forwarded["secret_blob"]; ok {
		t.Error("a claim that is not a scalar or a list of strings was forwarded")
	}
	// The user token is for this device, this upstream and its lifetime.
	f.clock.Advance(6 * time.Minute)
	if _, err := f.gw.Begin(t.Context(), f.dev, Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts", UserToken: res.Token}); code(err) != plxerr.UserTokenInvalid {
		t.Errorf("an expired user token: %v", err)
	}
}

// Verifies: SEC-026.
// A user token is of no use to another device, another upstream, or as
// anything but what it is.
func TestUserTokenBinding(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	p := newIdP(t)
	res, err := f.exchanger(p).Exchange(t.Context(), f.dev, p.token("ec1", jose.ES256, f.clock.Now(), nil), TokenTypeJWT, "bank")
	if err != nil {
		t.Fatal(err)
	}
	other := Upstream{
		ID: "u2", OrganizationID: "org-1", AppID: "app-1", EnvironmentID: "env-1", Name: "other", Version: 1,
		BaseURL: f.ups["bank"].BaseURL, AllowPrivate: true, Methods: []string{"GET"},
	}
	f.ups["other"] = other
	stolen := f.dev
	stolen.DeviceID = "dev-9"
	for name, c := range map[string]struct {
		d   device.Identity
		req Request
	}{
		"another upstream": {f.dev, Request{Upstream: "other", Method: "GET", Target: "/x", UserToken: res.Token}},
		"another device":   {stolen, Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts", UserToken: res.Token}},
		"garbage":          {f.dev, Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts", UserToken: "a.b.c"}},
		"an access token":  {f.dev, Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts", UserToken: f.accessTokenLike()}},
	} {
		if _, err := f.gw.Begin(t.Context(), c.d, c.req); code(err) != plxerr.UserTokenInvalid {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// accessTokenLike returns a token signed with the same keys as an access
// token (typ at+jwt), to show that it is not taken for a user token.
func (f *fixture) accessTokenLike() string {
	f.t.Helper()
	var c compactForTest
	tok, err := c.sign(f.t.Context(), f.signer, "at+jwt", []byte(`{"iss":"`+testIssuer+`","aud":"bank","sub":"x"}`))
	if err != nil {
		f.t.Fatal(err)
	}
	return tok
}

// Verifies: SEC-026.
// The host's token is refused for every reason a token can be wrong.
func TestExchangeRefusals(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	p := newIdP(t)
	p.addKey("rsa1", jose.RS256)
	now := f.clock.Now()
	tests := []struct {
		name    string
		token   func() string
		subject string
		want    plxerr.Code
	}{
		{"valid, signed with RS256", func() string { return p.token("rsa1", jose.RS256, now, nil) }, TokenTypeJWT, 0},
		{"valid as an id token", func() string { return p.token("ec1", jose.ES256, now, nil) }, TokenTypeIDToken, 0},
		{"another issuer", func() string {
			return p.token("ec1", jose.ES256, now, func(c map[string]any) { c["iss"] = "https://evil.example" })
		}, TokenTypeJWT, plxerr.UserTokenInvalid},
		{"another audience", func() string {
			return p.token("ec1", jose.ES256, now, func(c map[string]any) { c["aud"] = "other-app" })
		}, TokenTypeJWT, plxerr.UserTokenInvalid},
		{"an audience list with ours", func() string {
			return p.token("ec1", jose.ES256, now, func(c map[string]any) { c["aud"] = []string{"x", "plux-app"} })
		}, TokenTypeJWT, 0},
		{"expired", func() string { return p.token("ec1", jose.ES256, now.Add(-3*time.Hour), nil) }, TokenTypeJWT, plxerr.UserTokenInvalid},
		{"not yet valid", func() string {
			return p.token("ec1", jose.ES256, now, func(c map[string]any) { c["nbf"] = now.Add(time.Hour).Unix() })
		}, TokenTypeJWT, plxerr.UserTokenInvalid},
		{"no subject", func() string { return p.token("ec1", jose.ES256, now, func(c map[string]any) { delete(c, "sub") }) }, TokenTypeJWT, plxerr.UserTokenInvalid},
		{"no expiry", func() string { return p.token("ec1", jose.ES256, now, func(c map[string]any) { delete(c, "exp") }) }, TokenTypeJWT, plxerr.UserTokenInvalid},
		{"a key the provider does not publish", func() string {
			q := newIdP(t)
			return q.token("ec1", jose.ES256, now, nil)
		}, TokenTypeJWT, plxerr.UserTokenInvalid},
		{"signed with a key under another identifier", func() string {
			q := newIdP(t)
			q.private["nope"], q.keys = p.private["ec1"], p.keys
			return q.token("nope", jose.ES256, now, nil)
		}, TokenTypeJWT, plxerr.UserTokenInvalid},
		{"not a JWT", func() string { return "opaque-access-token" }, TokenTypeAccessToken, plxerr.UserTokenInvalid},
		{"an unsupported token type", func() string { return p.token("ec1", jose.ES256, now, nil) }, "urn:ietf:params:oauth:token-type:saml2", plxerr.InvalidFormat},
		{"no token", func() string { return "" }, TokenTypeJWT, plxerr.MissingProperty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.exchanger(p).Exchange(t.Context(), f.dev, tt.token(), tt.subject, "bank")
			if code(err) != tt.want {
				t.Errorf("error = %v (code %d), want %d", err, code(err), tt.want)
			}
		})
	}
	if _, err := f.exchanger(p).Exchange(t.Context(), f.dev, p.token("ec1", jose.ES256, now, nil), TokenTypeJWT, "unknown"); code(err) != plxerr.GatewayUpstreamUnknown {
		t.Errorf("an audience that is no upstream: %v", err)
	}
}

// Verifies: SEC-026.
// A token signed with HMAC under a known public value is refused: the
// provider's keys are public.
func TestExchangeRefusesSymmetricAlgorithms(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	p := newIdP(t)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte("0123456789abcdef0123456789abcdef")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := jwt.Signed(signer).Claims(map[string]any{"iss": idpIssuer, "aud": "plux-app", "sub": "x", "exp": f.clock.Now().Add(time.Hour).Unix()}).Serialize() //nolint:misspell // the go-jose API name
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.exchanger(p).Exchange(t.Context(), f.dev, tok, TokenTypeJWT, "bank"); code(err) != plxerr.UserTokenInvalid {
		t.Errorf("an HS256 token: %v", err)
	}
}

// Verifies: SEC-026, SEC-105.
// The provider's key set is cached, read again for a key it does not
// know at most once a minute, and read through the SSRF-safe client.
func TestKeySetCaching(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	p := newIdP(t)
	x := f.exchanger(p)
	now := f.clock.Now()
	for range 3 {
		if _, err := x.Exchange(t.Context(), f.dev, p.token("ec1", jose.ES256, now, nil), TokenTypeJWT, "bank"); err != nil {
			t.Fatal(err)
		}
	}
	if n := p.fetches.Load(); n != 1 {
		t.Fatalf("the key set was read %d times for three tokens, want 1", n)
	}
	// A key published later is found by one more read, but a token with a
	// key that never appears cannot make the server read again within a
	// minute.
	p.addKey("ec2", jose.ES256)
	f.clock.Advance(2 * time.Minute)
	if _, err := x.Exchange(t.Context(), f.dev, p.token("ec2", jose.ES256, f.clock.Now(), nil), TokenTypeJWT, "bank"); err != nil {
		t.Fatalf("a rotated key: %v", err)
	}
	before := p.fetches.Load()
	for range 5 {
		p.private["ghost"] = p.private["ec1"]
		_, _ = x.Exchange(t.Context(), f.dev, p.token("ghost", jose.ES256, f.clock.Now(), nil), TokenTypeJWT, "bank")
	}
	if n := p.fetches.Load() - before; n > 1 {
		t.Errorf("an unknown key made the server read the key set %d times", n)
	}
}

// bound is what a verifier of the device's tokens binds a user token to.
func (f *fixture) bound(audience string) BoundTo {
	return BoundTo{DeviceID: f.dev.DeviceID, AppID: f.dev.AppID, EnvironmentID: f.dev.EnvironmentID, Upstream: audience}
}

// Verifies: SEC-026, FN-002.
// The audience function:<name> is accepted when a version of the function
// exists in the device's environment, with no upstream of that name; the
// token it gives is for that function and for nothing else.
func TestExchangeForFunctionAudience(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	p := newIdP(t)
	x := f.exchanger(p)
	var asked []string
	x.Functions = func(_ context.Context, d device.Identity, name string) error {
		if d.EnvironmentID != f.dev.EnvironmentID {
			t.Errorf("the function was looked up in environment %q, not the device's", d.EnvironmentID)
		}
		asked = append(asked, name)
		if name != "bank.transferFee" {
			return plxerr.New(plxerr.FunctionNotFound, "no function %s is deployed in this environment", name)
		}
		return nil
	}
	subject := p.token("ec1", jose.ES256, f.clock.Now(), nil)

	res, err := x.Exchange(t.Context(), f.dev, subject, TokenTypeJWT, FunctionAudience("bank.transferFee"))
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	user, err := f.users.Verify(t.Context(), res.Token, f.bound(FunctionAudience("bank.transferFee")))
	if err != nil {
		t.Fatalf("a function refused its own token: %v", err)
	}
	if user.Subject != "user-7" || user.Claims["email"] != "ada@example.com" {
		t.Errorf("user = %+v", user)
	}
	if len(asked) != 1 || asked[0] != "bank.transferFee" {
		t.Errorf("functions looked up = %v, want [bank.transferFee]", asked)
	}

	// The token is for this function only: not for another function, not for
	// the function's bare name and not for an upstream.
	for _, audience := range []string{FunctionAudience("bank.other"), "bank.transferFee", "bank"} {
		if _, err := f.users.Verify(t.Context(), res.Token, f.bound(audience)); code(err) != plxerr.UserTokenInvalid {
			t.Errorf("the token for the function was accepted for %q: %v", audience, err)
		}
	}
	// And the gateway does not forward it to an upstream.
	if _, err := f.gw.Begin(t.Context(), f.dev, Request{Upstream: "bank", Operation: "balance", Method: "GET", Target: "/accounts", UserToken: res.Token}); code(err) != plxerr.UserTokenInvalid {
		t.Errorf("an upstream call with a function's token: %v", err)
	}
}

// Verifies: SEC-026, FN-002.
// A function does not take the token for an upstream.
func TestFunctionRefusesUpstreamToken(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	p := newIdP(t)
	res, err := f.exchanger(p).Exchange(t.Context(), f.dev, p.token("ec1", jose.ES256, f.clock.Now(), nil), TokenTypeJWT, "bank")
	if err != nil {
		t.Fatal(err)
	}
	for _, audience := range []string{FunctionAudience("bank"), FunctionAudience("bank.transferFee")} {
		if _, err := f.users.Verify(t.Context(), res.Token, f.bound(audience)); code(err) != plxerr.UserTokenInvalid {
			t.Errorf("the token for the upstream was accepted for %q: %v", audience, err)
		}
	}
}

// Verifies: SEC-026, FN-002.
// The exchange for a function is refused when the function has no version in
// the environment, when the server runs no functions, and when the audience
// names none; an upstream audience is not looked up as a function.
func TestExchangeForFunctionAudienceRefusals(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	p := newIdP(t)
	subject := p.token("ec1", jose.ES256, f.clock.Now(), nil)

	withoutFunctions := f.exchanger(p)
	if _, err := withoutFunctions.Exchange(t.Context(), f.dev, subject, TokenTypeJWT, FunctionAudience("bank.transferFee")); code(err) != plxerr.PreconditionFailed {
		t.Errorf("a server without functions: %v", err)
	}

	x := f.exchanger(p)
	lookups := 0
	x.Functions = func(context.Context, device.Identity, string) error {
		lookups++
		return plxerr.New(plxerr.FunctionNotFound, "no such function")
	}
	if _, err := x.Exchange(t.Context(), f.dev, subject, TokenTypeJWT, FunctionAudience("bank.missing")); code(err) != plxerr.FunctionNotFound {
		t.Errorf("a function with no version: %v", err)
	}
	if _, err := x.Exchange(t.Context(), f.dev, subject, TokenTypeJWT, FunctionAudiencePrefix); code(err) != plxerr.InvalidFormat {
		t.Errorf("an audience without a name: %v", err)
	}
	if _, err := x.Exchange(t.Context(), f.dev, subject, TokenTypeJWT, "bank"); err != nil {
		t.Errorf("an upstream audience: %v", err)
	}
	if lookups != 1 {
		t.Errorf("functions looked up %d times, want once, for the audience that named one", lookups)
	}
}
