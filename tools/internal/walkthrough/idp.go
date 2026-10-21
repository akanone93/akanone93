// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// IDP is the OpenID Connect test provider of step 1. It serves discovery, the
// key set and the token endpoint over TLS; the "browser" is the walkthrough
// itself, which calls [IDP.Authorize] with the sign-in URL the server gave.
// Every person it signs in is whoever the walkthrough names: there is no
// password, because the provider exists only inside one run.
type IDP struct {
	issuer       string
	clientID     string
	clientSecret string
	caPEM        []byte
	key          *rsa.PrivateKey
	now          func() time.Time
	random       io.Reader
	srv          *http.Server
	wg           sync.WaitGroup

	mu    sync.Mutex
	codes map[string]idpGrant
}

// idpGrant is an issued authorisation code.
type idpGrant struct {
	subject, email, nonce, challenge string
}

// NewIDP serves the provider on listen (host:port). Its issuer is
// https://advertise, which must be an address the installation can reach and
// one of hosts, the names its certificate is valid for.
func NewIDP(listen, advertise string, hosts []string, clientID, clientSecret string, now func() time.Time, random io.Reader) (*IDP, error) {
	cfg, caPEM, err := serverTLS(hosts, now, random)
	if err != nil {
		return nil, fmt.Errorf("walkthrough.NewIDP: %w", err)
	}
	key, err := rsa.GenerateKey(random, 2048)
	if err != nil {
		return nil, fmt.Errorf("walkthrough.NewIDP: %w", err)
	}
	ln, err := tls.Listen("tcp", listen, cfg)
	if err != nil {
		return nil, fmt.Errorf("walkthrough.NewIDP: %w", err)
	}
	p := &IDP{
		issuer: "https://" + advertise, clientID: clientID, clientSecret: clientSecret, caPEM: caPEM,
		key: key, now: now, random: random, codes: map[string]idpGrant{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("GET /jwks", p.jwks)
	mux.HandleFunc("POST /token", p.token)
	p.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		_ = p.srv.Serve(ln)
	}()
	return p, nil
}

// Issuer is the provider's issuer URL.
func (p *IDP) Issuer() string { return p.issuer }

// CAPEM is the certificate authority the provider's certificate chains to.
func (p *IDP) CAPEM() string { return string(p.caPEM) }

// ClientID is the OAuth client the server is registered as.
func (p *IDP) ClientID() string { return p.clientID }

// ClientSecret is the client's secret; it only ever goes into the request that
// creates the identity provider, which the evidence records redacted.
func (p *IDP) ClientSecret() string { return p.clientSecret }

// Close stops the provider.
func (p *IDP) Close() error {
	err := p.srv.Close()
	p.wg.Wait()
	if err != nil {
		return fmt.Errorf("walkthrough.IDP.Close: %w", err)
	}
	return nil
}

// discovery serves the provider metadata.
func (p *IDP) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"issuer": p.issuer, "authorization_endpoint": p.issuer + "/authorize", "token_endpoint": p.issuer + "/token",
		"jwks_uri": p.issuer + "/jwks", "response_types_supported": []string{"code"},
		"subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported": []string{"S256"},
	})
}

// jwks serves the signing key.
func (p *IDP) jwks(w http.ResponseWriter, _ *http.Request) {
	pub := p.key.PublicKey
	writeJSON(w, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "walkthrough",
		"n": b64(pub.N.Bytes()), "e": b64(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

// Authorize plays the browser at the provider: it reads the sign-in URL the
// server returned and issues an authorisation code for the person, bound to
// the request's PKCE challenge and nonce. It returns the code.
func (p *IDP) Authorize(loginURL, subject, email string) (string, error) {
	u, err := url.Parse(loginURL)
	if err != nil || u.Scheme+"://"+u.Host+u.Path != p.issuer+"/authorize" {
		return "", fmt.Errorf("walkthrough.IDP.Authorize: %q is not this provider's sign-in URL", loginURL)
	}
	q := u.Query()
	if q.Get("client_id") != p.clientID || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("state") == "" {
		return "", fmt.Errorf("walkthrough.IDP.Authorize: the sign-in request is not a PKCE code flow for client %q", p.clientID)
	}
	raw := make([]byte, 24)
	if _, err := io.ReadFull(p.random, raw); err != nil {
		return "", fmt.Errorf("walkthrough.IDP.Authorize: %w", err)
	}
	code := b64(raw)
	p.mu.Lock()
	p.codes[code] = idpGrant{subject: subject, email: email, nonce: q.Get("nonce"), challenge: q.Get("code_challenge")}
	p.mu.Unlock()
	return code, nil
}

// token redeems a code: the client authenticates, the PKCE verifier must match
// the challenge, and a code works once.
func (p *IDP) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}
	id, secret, ok := r.BasicAuth()
	if !ok {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	p.mu.Lock()
	g, found := p.codes[r.PostForm.Get("code")]
	delete(p.codes, r.PostForm.Get("code"))
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if id != p.clientID || secret != p.clientSecret || !found || r.PostForm.Get("grant_type") != "authorization_code" || b64(sum[:]) != g.challenge {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	idToken, err := p.idToken(g)
	if err != nil {
		http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"access_token": "unused", "token_type": "Bearer", "expires_in": 300, "id_token": idToken})
}

// idToken signs the ID token of a grant with RS256.
func (p *IDP) idToken(g idpGrant) (string, error) {
	now := p.now()
	claims := map[string]any{
		"iss": p.issuer, "sub": g.subject, "aud": p.clientID, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"nonce": g.nonce, "email": g.email, "email_verified": true,
	}
	head, err := json.Marshal(map[string]string{"alg": "RS256", "kid": "walkthrough", "typ": "JWT"})
	if err != nil {
		return "", fmt.Errorf("walkthrough.IDP.idToken: %w", err)
	}
	body, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("walkthrough.IDP.idToken: %w", err)
	}
	signed := b64(head) + "." + b64(body)
	sum := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(p.random, p.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("walkthrough.IDP.idToken: %w", err)
	}
	return signed + "." + b64(sig), nil
}

// b64 is unpadded URL-safe base64.
func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// writeJSON answers with a JSON document.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
