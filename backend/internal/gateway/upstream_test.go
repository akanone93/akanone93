// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// valid is a registration that passes.
func valid() Upstream {
	return Upstream{
		Name: "bank", BaseURL: "https://api.bank.example/v1", Auth: Auth{Kind: AuthNone}, Methods: []string{"GET", "POST"},
		Operations: []Operation{{
			Name: "pay", Methods: []string{"POST"}, PathPrefix: "/payments",
			Transaction: &Transaction{Amount: "/amount", Currency: "/currency", Payee: "/payee"},
		}},
	}
}

// Verifies: SEC-031.
// A registration is refused for every setting that would weaken the
// gateway or make it ambiguous.
func TestUpstreamValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(*Upstream)
		want plxerr.Code // 0: accepted
	}{
		{"valid", func(*Upstream) {}, 0},
		{"name in capitals", func(u *Upstream) { u.Name = "Bank" }, plxerr.InvalidFormat},
		{"name that starts with a digit", func(u *Upstream) { u.Name = "1bank" }, plxerr.InvalidFormat},
		{"cleartext", func(u *Upstream) { u.BaseURL = "http://api.bank.example" }, plxerr.InvalidFormat},
		{"cleartext on a private network", func(u *Upstream) { u.BaseURL, u.AllowPrivate = "http://api.internal", true }, 0},
		{"credentials in the URL", func(u *Upstream) { u.BaseURL = "https://user:pw@api.bank.example" }, plxerr.InvalidFormat},
		{"a query in the URL", func(u *Upstream) { u.BaseURL = "https://api.bank.example/?key=1" }, plxerr.InvalidFormat},
		{"a fragment in the URL", func(u *Upstream) { u.BaseURL = "https://api.bank.example/#x" }, plxerr.InvalidFormat},
		{"no host", func(u *Upstream) { u.BaseURL = "https:///v1" }, plxerr.InvalidFormat},
		{"dot segments in the URL", func(u *Upstream) { u.BaseURL = "https://api.bank.example/a/../b" }, plxerr.GatewayDestinationRefused},
		{"a loopback literal", func(u *Upstream) { u.BaseURL = "https://127.0.0.1" }, plxerr.GatewayDestinationRefused},
		{"a metadata literal", func(u *Upstream) { u.BaseURL = "https://169.254.169.254/latest" }, plxerr.GatewayDestinationRefused},
		{"a private literal, allowed", func(u *Upstream) { u.BaseURL, u.AllowPrivate = "https://10.0.0.5", true }, 0},
		{"no methods", func(u *Upstream) { u.Methods = nil }, plxerr.MissingProperty},
		{"CONNECT", func(u *Upstream) { u.Methods = []string{"CONNECT"} }, plxerr.InvalidFormat},
		{"a method twice", func(u *Upstream) { u.Methods = []string{"GET", "GET"} }, plxerr.InvalidFormat},
		{"an unknown assurance level", func(u *Upstream) { u.Assurance = "AL9" }, plxerr.InvalidFormat},
		{"a negative timeout", func(u *Upstream) { u.Timeout = -1 }, plxerr.InvalidFormat},
		{"an operation twice", func(u *Upstream) { u.Operations = append(u.Operations, u.Operations[0]) }, plxerr.InvalidFormat},
		{"an operation method the upstream lacks", func(u *Upstream) { u.Operations[0].Methods = []string{"DELETE"} }, plxerr.InvalidFormat},
		{"a path prefix without a slash", func(u *Upstream) { u.Operations[0].PathPrefix = "payments" }, plxerr.InvalidFormat},
		{"a path prefix with dots", func(u *Upstream) { u.Operations[0].PathPrefix = "/a/../b" }, plxerr.GatewayDestinationRefused},
		{"a transaction that can GET", func(u *Upstream) { u.Operations[0].Methods = []string{"GET"} }, plxerr.InvalidFormat},
		{"a transaction with no payee", func(u *Upstream) { u.Operations[0].Transaction.Payee = "" }, plxerr.InvalidFormat},
		{"a transaction pointer that is no pointer", func(u *Upstream) { u.Operations[0].Transaction.Amount = "amount" }, plxerr.InvalidFormat},
		{"a plugin key in capitals", func(u *Upstream) { u.Operations[0].Plugins = []string{"Shop"} }, plxerr.InvalidFormat},
		{"mutual TLS without a secret", func(u *Upstream) { u.Auth = Auth{Kind: AuthMTLS} }, plxerr.InvalidFormat},
		{"mutual TLS", func(u *Upstream) { u.Auth = Auth{Kind: AuthMTLS, MTLSSecret: "bank_cert"} }, 0},
		{"OAuth with a cleartext token URL", func(u *Upstream) {
			u.Auth = Auth{Kind: AuthOAuth, TokenURL: "http://idp.bank.example/token", ClientID: "c", ClientSecretRef: "s"}
		}, plxerr.InvalidFormat},
		{"OAuth", func(u *Upstream) {
			u.Auth = Auth{Kind: AuthOAuth, TokenURL: "https://idp.bank.example/token", ClientID: "c", ClientSecretRef: "s", Scopes: []string{"pay"}}
		}, 0},
		{"OAuth with a scope with a space", func(u *Upstream) {
			u.Auth = Auth{Kind: AuthOAuth, TokenURL: "https://idp.bank.example/token", ClientID: "c", ClientSecretRef: "s", Scopes: []string{"a b"}}
		}, plxerr.InvalidFormat},
		{"an API key in the Host header", func(u *Upstream) { u.Auth = Auth{Kind: AuthAPIKey, Header: "Host", KeySecret: "k"} }, plxerr.InvalidFormat},
		{"an API key in a Plux header", func(u *Upstream) { u.Auth = Auth{Kind: AuthAPIKey, Header: "Plux-Assertion", KeySecret: "k"} }, plxerr.InvalidFormat},
		{"an API key", func(u *Upstream) { u.Auth = Auth{Kind: AuthAPIKey, Header: "X-Api-Key", KeySecret: "k"} }, 0},
		{"an unknown authentication", func(u *Upstream) { u.Auth.Kind = "basic" }, plxerr.InvalidFormat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u := valid()
			tt.edit(&u)
			err := u.Validate()
			if code(err) != tt.want {
				t.Errorf("Validate = %v (code %d), want code %d", err, code(err), tt.want)
			}
		})
	}
}

// Verifies: SEC-026.
// An identity provider needs an https key URL, an issuer and an audience.
func TestIdentityProviderValidation(t *testing.T) {
	t.Parallel()
	ok := IdentityProvider{Issuer: "https://login.example", JWKSURL: "https://login.example/keys", Audiences: []string{"app"}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a valid provider: %v", err)
	}
	for name, edit := range map[string]func(*IdentityProvider){
		"cleartext keys":   func(p *IdentityProvider) { p.JWKSURL = "http://login.example/keys" },
		"no issuer":        func(p *IdentityProvider) { p.Issuer = "" },
		"no audience":      func(p *IdentityProvider) { p.Audiences = nil },
		"an empty claim":   func(p *IdentityProvider) { p.ForwardClaims = []string{""} },
		"credentials":      func(p *IdentityProvider) { p.JWKSURL = "https://u:p@login.example/keys" },
		"no host":          func(p *IdentityProvider) { p.JWKSURL = "https:///keys" },
		"an audience line": func(p *IdentityProvider) { p.Audiences = []string{"a\nb"} },
	} {
		p := ok
		edit(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// pemPair returns a client certificate and key as one PEM document, and
// the CA that signed it.
func clientCertificate(t *testing.T) (secret []byte, ca *x509.CertPool) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test CA"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "plux-gateway"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(leafKey)
	secret = append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})...)
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return secret, pool
}

// Verifies: SEC-031.
// With mutual TLS the gateway presents the client certificate held in the
// upstream's secret, and the upstream is verified against the installation's
// roots; without the certificate the upstream refuses the connection.
func TestMutualTLS(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	secret, clientCA := clientCertificate(t)
	var subject string
	tlsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject = r.TLS.PeerCertificates[0].Subject.CommonName
		_, _ = w.Write([]byte("secure"))
	}))
	tlsServer.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCA, MinVersion: tls.VersionTLS12}
	tlsServer.StartTLS()
	t.Cleanup(tlsServer.Close)
	serverRoots := x509.NewCertPool()
	serverRoots.AddCert(tlsServer.Certificate())

	f.secrets["bank_cert"] = secret
	f.egress.WithRoots(serverRoots)
	u := f.ups["bank"]
	u.BaseURL, u.Operations = tlsServer.URL, nil
	u.Auth = Auth{Kind: AuthMTLS, MTLSSecret: "bank_cert"}
	f.ups["bank"] = u
	res, err := f.call(Request{Upstream: "bank", Method: "GET", Target: "/"}, nil)
	if err != nil || string(res.Body) != "secure" || subject != "plux-gateway" {
		t.Fatalf("mutual TLS call: %v %q %q", err, res.Body, subject)
	}

	u.Auth, u.Version = Auth{Kind: AuthNone}, 2
	f.ups["bank"] = u
	if _, err := f.call(Request{Upstream: "bank", Method: "GET", Target: "/"}, nil); code(err) != plxerr.GatewayUpstreamFailed {
		t.Errorf("no client certificate: %v", err)
	}

	f.secrets["bank_cert"] = []byte("not a certificate")
	u.Auth, u.Version = Auth{Kind: AuthMTLS, MTLSSecret: "bank_cert"}, 3
	f.ups["bank"] = u
	if _, err := f.call(Request{Upstream: "bank", Method: "GET", Target: "/"}, nil); code(err) != plxerr.GatewayUpstreamFailed {
		t.Errorf("a secret that is no certificate: %v", err)
	}
}

// Verifies: SEC-031.
// The upstream is reached over the connections built for its version: a
// change of the registration builds new ones.
func TestConnectionsFollowTheUpstreamVersion(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	u := f.ups["bank"]
	first, err := f.egress.Transport(t.Context(), u)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := f.egress.Transport(t.Context(), u)
	if first != again {
		t.Error("two calls for one upstream version did not share their connections")
	}
	u.Version++
	next, _ := f.egress.Transport(t.Context(), u)
	if next == first {
		t.Error("a new version kept the old connections")
	}
}
