// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// readFile reads a file or fails the test.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // a path the test built
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A Connect call posts JSON with the protocol header and the bearer token,
// decodes the answer, and turns a Connect error into an error with its code.
func TestConnectClientCall(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Connect-Protocol-Version") != "1" || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad protocol", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/plux.v1.X/Ok":
			var echo any
			_ = json.NewDecoder(r.Body).Decode(&echo)
			_ = json.NewEncoder(w).Encode(map[string]any{"echo": echo, "auth": r.Header.Get("Authorization")})
		case "/plux.v1.X/Denied":
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"code":"permission_denied","message":"PLX-6200 step-up required"}`)
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	t.Cleanup(srv.Close)
	c := &ConnectClient{BaseURL: srv.URL + "/", Token: "tok", HTTP: srv.Client()}
	var resp map[string]any
	if err := c.Call(t.Context(), "/plux.v1.X/Ok", map[string]any{"a": 1}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp["auth"] != "Bearer tok" || resp["echo"].(map[string]any)["a"] != float64(1) {
		t.Errorf("resp = %v", resp)
	}
	if err := (&ConnectClient{BaseURL: srv.URL, HTTP: srv.Client()}).Call(t.Context(), "/plux.v1.X/Ok", map[string]any{}, nil); err != nil {
		t.Errorf("no answer wanted: %v", err)
	}
	if err := c.Call(t.Context(), "/plux.v1.X/Denied", map[string]any{}, &resp); err == nil || !strings.Contains(err.Error(), "permission_denied") || !strings.Contains(err.Error(), "PLX-6200") {
		t.Errorf("err = %v", err)
	}
	if err := c.Call(t.Context(), "/plux.v1.X/Other", map[string]any{}, &resp); err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("err = %v", err)
	}
	if err := c.Call(t.Context(), "/plux.v1.X/Ok", map[string]any{"bad": make(chan int)}, &resp); err == nil {
		t.Error("an unencodable request must be an error")
	}
}

// SCIM resources are posted with the SCIM media type and the token; any
// other status than created is an error.
func TestHTTPSCIMClientPost(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/scim+json" || r.Header.Get("Authorization") != "Bearer scim-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/scim/v2/Conflict" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"u-1"}`)
	}))
	t.Cleanup(srv.Close)
	c := &HTTPSCIMClient{BaseURL: srv.URL, HTTP: srv.Client()}
	got, err := c.Post(t.Context(), "scim-token", "/scim/v2/Users", map[string]any{"userName": "a"})
	if err != nil || got["id"] != "u-1" {
		t.Errorf("Post = %v, %v", got, err)
	}
	if _, err := c.Post(t.Context(), "wrong", "/scim/v2/Users", nil); err == nil {
		t.Error("a refused token must be an error")
	}
	if _, err := c.Post(t.Context(), "scim-token", "/scim/v2/Conflict", nil); err == nil || !strings.Contains(err.Error(), "409") {
		t.Errorf("err = %v", err)
	}
}

// tlsClient trusts one CA.
func tlsClient(t *testing.T, caPEM string) *tls.Config {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(caPEM)) {
		t.Fatal("no certificate in the CA PEM")
	}
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

// The sink accepts RFC 5425 frames over TLS from a client that trusts its CA,
// keeps the messages, and drops a connection that sends a malformed frame.
func TestSinkReceivesOctetCountedFrames(t *testing.T) {
	t.Parallel()
	sink, err := NewSink("127.0.0.1:0", []string{"127.0.0.1"}, time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	conn, err := dialSink(t, sink)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"<14>1 2026-01-01T00:00:00Z host plux - - - first", "<14>1 2026-01-01T00:00:01Z host plux - - - second"} {
		if _, err := fmt.Fprintf(conn, "%d %s", len(m), m); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	got, err := sink.WaitFor(ctx, "second")
	if err != nil || !strings.HasSuffix(got, "second") {
		t.Fatalf("WaitFor = %q, %v", got, err)
	}
	if n := len(sink.Messages()); n != 2 {
		t.Errorf("%d messages, want 2", n)
	}
	_ = conn.Close()

	bad, err := dialSink(t, sink)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(bad, "not-a-length garbage")
	_ = bad.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := bad.Read(make([]byte, 1)); err == nil {
		t.Error("the sink must close a connection that sends a malformed frame")
	}
	short, cancelShort := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancelShort()
	if _, err := sink.WaitFor(short, "never sent"); err == nil {
		t.Error("WaitFor must fail when the message never comes")
	}
}

// The sink's certificate chains only to its own CA: a client that trusts the
// system roots refuses it.
func TestSinkCertificateIsPrivate(t *testing.T) {
	t.Parallel()
	sink, err := NewSink("127.0.0.1:0", []string{"127.0.0.1"}, time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	dialer := &tls.Dialer{Config: &tls.Config{MinVersion: tls.VersionTLS12}}
	if conn, err := dialer.DialContext(t.Context(), "tcp", sink.Address()); err == nil {
		_ = conn.Close()
		t.Error("a client without the walkthrough CA must refuse the sink")
	}
}

// pkcePair returns a verifier and its S256 challenge.
func pkcePair() (string, string) {
	verifier := "verifier-" + strings.Repeat("v", 40)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

// The provider's whole flow: the sign-in URL, a code, the token endpoint with
// PKCE and the client secret, an ID token signed by the published key; a code
// works once, and a wrong verifier or secret is refused.
func TestIDPSignInFlow(t *testing.T) {
	t.Parallel()
	port := freePort(t)
	p, err := NewIDP("127.0.0.1:"+port, "127.0.0.1:"+port, []string{"127.0.0.1"}, "client-1", "s3cret", time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsClient(t, p.CAPEM())}, Timeout: 10 * time.Second}

	var meta map[string]any
	getJSON(t, client, p.Issuer()+"/.well-known/openid-configuration", &meta)
	if meta["issuer"] != p.Issuer() || meta["token_endpoint"] != p.Issuer()+"/token" {
		t.Errorf("discovery = %v", meta)
	}

	verifier, challenge := pkcePair()
	login := p.Issuer() + "/authorize?" + url.Values{
		"client_id": {"client-1"}, "state": {"st"}, "nonce": {"n-1"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}.Encode()
	code, err := p.Authorize(login, "sub-alice", "alice@walkthrough.example")
	if err != nil {
		t.Fatal(err)
	}
	form := func(code, verifier, secret string) url.Values {
		return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "client_id": {"client-1"}, "client_secret": {secret}}
	}
	if status, _ := postForm(t, client, p.Issuer()+"/token", form(code, "wrong-verifier", "s3cret")); status != http.StatusBadRequest {
		t.Errorf("a wrong verifier: status %d", status)
	}
	code, _ = p.Authorize(login, "sub-alice", "alice@walkthrough.example")
	if status, _ := postForm(t, client, p.Issuer()+"/token", form(code, verifier, "wrong-secret")); status != http.StatusBadRequest {
		t.Errorf("a wrong secret: status %d", status)
	}
	code, _ = p.Authorize(login, "sub-alice", "alice@walkthrough.example")
	status, body := postForm(t, client, p.Issuer()+"/token", form(code, verifier, "s3cret"))
	if status != http.StatusOK {
		t.Fatalf("token: status %d: %s", status, body)
	}
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal([]byte(body), &tok); err != nil {
		t.Fatal(err)
	}
	claims := verifyIDToken(t, client, p, tok.IDToken)
	if claims["sub"] != "sub-alice" || claims["nonce"] != "n-1" || claims["aud"] != "client-1" || claims["iss"] != p.Issuer() || claims["email_verified"] != true {
		t.Errorf("claims = %v", claims)
	}
	if status, _ := postForm(t, client, p.Issuer()+"/token", form(code, verifier, "s3cret")); status != http.StatusBadRequest {
		t.Errorf("a code used twice: status %d", status)
	}
}

// Authorize refuses a URL that is not this provider's PKCE sign-in request.
func TestIDPAuthorizeRefusesForeignRequests(t *testing.T) {
	t.Parallel()
	port := freePort(t)
	p, err := NewIDP("127.0.0.1:"+port, "127.0.0.1:"+port, []string{"127.0.0.1"}, "client-1", "s", time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	_, challenge := pkcePair()
	for name, u := range map[string]string{
		"other host":     "https://evil.example/authorize?client_id=client-1&state=s&code_challenge=" + challenge + "&code_challenge_method=S256",
		"other client":   p.Issuer() + "/authorize?client_id=other&state=s&code_challenge=" + challenge + "&code_challenge_method=S256",
		"no PKCE":        p.Issuer() + "/authorize?client_id=client-1&state=s",
		"plain PKCE":     p.Issuer() + "/authorize?client_id=client-1&state=s&code_challenge=x&code_challenge_method=plain",
		"no state":       p.Issuer() + "/authorize?client_id=client-1&code_challenge=" + challenge + "&code_challenge_method=S256",
		"not a URL":      "::",
		"other endpoint": p.Issuer() + "/token?client_id=client-1",
	} {
		if _, err := p.Authorize(u, "s", "e@x"); err == nil {
			t.Errorf("%s: Authorize must refuse %q", name, u)
		}
	}
}

// The authorisation endpoint is what a separate program that plays the browser
// (the WebAuthn driver) visits: login_hint names the person, the answer is a
// redirect to the registered URL with the code and the state, and the code
// redeems like any other. A request without a person, for another redirect URL
// or for another client is refused.
func TestIDPAuthorizeEndpoint(t *testing.T) {
	t.Parallel()
	port := freePort(t)
	p, err := NewIDP("127.0.0.1:"+port, "127.0.0.1:"+port, []string{"127.0.0.1"}, "client-1", "s3cret", time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	client := &http.Client{
		Transport:     &http.Transport{TLSClientConfig: tlsClient(t, p.CAPEM())},
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	verifier, challenge := pkcePair()
	request := func(hint, redirect, clientID string) *http.Response {
		q := url.Values{
			"client_id": {clientID}, "state": {"st-1"}, "nonce": {"n-1"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"},
			"login_hint": {hint}, "redirect_uri": {redirect},
		}
		res, err := client.Get(p.Issuer() + "/authorize?" + q.Encode())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		return res
	}
	res := request("bob@walkthrough.example", p.Issuer()+"/callback", "client-1")
	back, err := res.Location()
	if res.StatusCode != http.StatusFound || err != nil || back.Query().Get("state") != "st-1" || back.Query().Get("code") == "" {
		t.Fatalf("authorize: status %d, location %v, %v", res.StatusCode, back, err)
	}
	status, body := postForm(t, client, p.Issuer()+"/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {back.Query().Get("code")}, "code_verifier": {verifier},
		"client_id": {"client-1"}, "client_secret": {"s3cret"},
	})
	if status != http.StatusOK {
		t.Fatalf("token: status %d: %s", status, body)
	}
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal([]byte(body), &tok); err != nil {
		t.Fatal(err)
	}
	claims := verifyIDToken(t, client, p, tok.IDToken)
	if claims["sub"] != "sub-bob" || claims["email"] != "bob@walkthrough.example" || claims["nonce"] != "n-1" {
		t.Errorf("claims = %v", claims)
	}
	for name, r := range map[string]*http.Response{
		"no person":        request("", "", "client-1"),
		"no address":       request("bob", "", "client-1"),
		"another redirect": request("bob@walkthrough.example", "https://evil.example/callback", "client-1"),
		"another client":   request("bob@walkthrough.example", "", "other"),
		"empty local part": request("@walkthrough.example", "", "client-1"),
	} {
		if r.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, r.StatusCode)
		}
	}
}

// freePort returns a TCP port that was free a moment ago.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// getJSON fetches and decodes a JSON document.
func getJSON(t *testing.T, c *http.Client, u string, out any) {
	t.Helper()
	res, err := c.Get(u) //nolint:noctx // a test against a loopback server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}

// postForm posts a form and returns the status and body.
func postForm(t *testing.T, c *http.Client, u string, v url.Values) (int, string) {
	t.Helper()
	res, err := c.PostForm(u, v) //nolint:noctx // a test against a loopback server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

// verifyIDToken checks an RS256 token against the provider's published key
// and returns its claims.
func verifyIDToken(t *testing.T, c *http.Client, p *IDP, token string) map[string]any {
	t.Helper()
	var set struct {
		Keys []struct{ N, E string } `json:"keys"`
	}
	getJSON(t, c, p.Issuer()+"/jwks", &set)
	n, _ := base64.RawURLEncoding.DecodeString(set.Keys[0].N)
	e, _ := base64.RawURLEncoding.DecodeString(set.Keys[0].E)
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token = %q", token)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("the ID token's signature does not verify: %v", err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}
