// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml_test

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// Verifies: GOV-004.
// Start makes an AuthnRequest for the service provider, dated by the clock
// the core passes in, and sends it to the provider's endpoint with the relay
// state; the request's identifier is what the response must answer.
func TestStart_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	start, err := provider().Start(licensed(), config(idp), "plux_state_abc-DEF_123", clockNow)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	u, err := url.Parse(start.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "idp.example.com" || u.Path != "/sso" {
		t.Errorf("URL = %s", start.URL)
	}
	if u.Query().Get("RelayState") != "plux_state_abc-DEF_123" {
		t.Errorf("RelayState = %q", u.Query().Get("RelayState"))
	}
	compressed, err := base64.StdEncoding.DecodeString(u.Query().Get("SAMLRequest"))
	if err != nil {
		t.Fatal(err)
	}
	inflated, err := io.ReadAll(flate.NewReader(bytes.NewReader(compressed)))
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		ID           string `xml:"ID,attr"`
		IssueInstant string `xml:"IssueInstant,attr"`
		Destination  string `xml:"Destination,attr"`
		ACS          string `xml:"AssertionConsumerServiceURL,attr"`
		Issuer       string `xml:"Issuer"`
	}
	if err := xml.Unmarshal(inflated, &req); err != nil {
		t.Fatalf("AuthnRequest: %v\n%s", err, inflated)
	}
	if req.ID != start.RequestID || req.ACS != acsURL || req.Issuer != spEntity || req.Destination != "https://idp.example.com/sso" {
		t.Errorf("AuthnRequest = %+v, want ID %q", req, start.RequestID)
	}
	if req.IssueInstant != "2026-10-20T12:00:00Z" {
		t.Errorf("IssueInstant = %q, want the injected clock", req.IssueInstant)
	}
	other, err := provider().Start(licensed(), config(idp), "state", clockNow)
	if err != nil || other.RequestID == start.RequestID {
		t.Errorf("two sign-ins share a request identifier: %v", err)
	}
}

// Verifies: GOV-004.
// A relay state that is not safe in a URL is escaped, and an address that is
// not https is not sent to.
func TestStart_Refusals_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	start, err := provider().Start(licensed(), config(idp), "a b&c=d", clockNow)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start.URL)
	if got := u.Query().Get("RelayState"); got != "a b&c=d" || len(u.Query()) != 2 {
		t.Errorf("RelayState = %q in %s", got, start.URL)
	}
	for name, mutate := range map[string]func(*identity.SAMLConfig){
		"an http endpoint":      func(c *identity.SAMLConfig) { c.IdP.SSOURL = "http://idp.example.com/sso" },
		"no endpoint":           func(c *identity.SAMLConfig) { c.IdP.SSOURL = "" },
		"an http ACS":           func(c *identity.SAMLConfig) { c.ACSURL = "http://plux.example.com/acs" },
		"no service identifier": func(c *identity.SAMLConfig) { c.EntityID = "" },
	} {
		cfg := config(idp)
		mutate(&cfg)
		if _, err := provider().Start(licensed(), cfg, "state", clockNow); err == nil {
			t.Errorf("%s: Start succeeded", name)
		}
	}
}

// idpMetadata is a provider's metadata document with the given pieces.
func idpMetadata(entity, location, certs string) string {
	return `<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" xmlns:ds="http://www.w3.org/2000/09/xmldsig#" entityID="` + entity + `">
  <md:IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">` + certs + `
    <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="https://idp.example.com/post"/>
    <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="` + location + `"/>
  </md:IDPSSODescriptor>
</md:EntityDescriptor>`
}

// keyDescriptor is a KeyDescriptor for a certificate.
func keyDescriptor(use string, cert *x509.Certificate) string {
	b64 := base64.StdEncoding.EncodeToString(cert.Raw)
	wrapped := b64[:40] + "\n      " + b64[40:]
	attr := ""
	if use != "" {
		attr = ` use="` + use + `"`
	}
	return `<md:KeyDescriptor` + attr + `><ds:KeyInfo><ds:X509Data><ds:X509Certificate>` + wrapped + `</ds:X509Certificate></ds:X509Data></ds:KeyInfo></md:KeyDescriptor>`
}

// Verifies: GOV-004.
// An identity provider's metadata gives its entity, its redirect endpoint and
// its signing certificates, and nothing a provider uses to encrypt.
func TestImportMetadata_GOV_004(t *testing.T) {
	t.Parallel()
	signing, unused, encryption := newIdP(t), newIdP(t), newIdP(t)
	doc := idpMetadata(idpEntity, "https://idp.example.com/sso",
		keyDescriptor("signing", signing.cert)+keyDescriptor("", unused.cert)+keyDescriptor("encryption", encryption.cert))
	d, err := provider().ImportMetadata(licensed(), []byte(doc))
	if err != nil {
		t.Fatalf("ImportMetadata: %v", err)
	}
	if d.EntityID != idpEntity || d.SSOURL != "https://idp.example.com/sso" {
		t.Errorf("descriptor = %+v", d)
	}
	if len(d.SigningCertificates) != 2 || d.SigningCertificates[0] != signing.pem() || d.SigningCertificates[1] != unused.pem() {
		t.Errorf("certificates = %v", d.SigningCertificates)
	}
	if _, err := verify(licensed(), identity.SAMLConfig{EntityID: spEntity, ACSURL: acsURL, IdP: d}, signedAssertion().build(t, signing), requestID, &memoryReplay{}); err != nil {
		t.Errorf("a response verified against the imported descriptor: %v", err)
	}
}

// Verifies: GOV-004.
// Metadata that cannot be trusted structurally is refused: it is not
// well-formed, describes no single provider, has no usable certificate or
// sends people to an address that is not https.
func TestImportMetadata_Refusals_GOV_004(t *testing.T) {
	t.Parallel()
	good := newIdP(t)
	signing := keyDescriptor("signing", good.cert)
	for name, doc := range map[string]string{
		"not xml":            "this is not metadata",
		"truncated":          idpMetadata(idpEntity, "https://idp.example.com/sso", signing)[:120],
		"no entity":          idpMetadata("", "https://idp.example.com/sso", signing),
		"an http endpoint":   idpMetadata(idpEntity, "http://idp.example.com/sso", signing),
		"no certificate":     idpMetadata(idpEntity, "https://idp.example.com/sso", ""),
		"encryption only":    idpMetadata(idpEntity, "https://idp.example.com/sso", keyDescriptor("encryption", good.cert)),
		"a broken cert":      idpMetadata(idpEntity, "https://idp.example.com/sso", `<md:KeyDescriptor use="signing"><ds:KeyInfo><ds:X509Data><ds:X509Certificate>AAAA</ds:X509Certificate></ds:X509Data></ds:KeyInfo></md:KeyDescriptor>`),
		"no redirect":        strings.ReplaceAll(idpMetadata(idpEntity, "https://idp.example.com/sso", signing), "HTTP-Redirect", "SOAP"),
		"another protocol":   strings.ReplaceAll(idpMetadata(idpEntity, "https://idp.example.com/sso", signing), "SAML:2.0:protocol", "SAML:1.1:protocol"),
		"not an entity":      `<md:EntitiesDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata"/>`,
		"a service provider": `<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="x"><md:SPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol"/></md:EntityDescriptor>`,
	} {
		if d, err := provider().ImportMetadata(licensed(), []byte(doc)); err == nil {
			t.Errorf("%s: accepted %+v", name, d)
		}
	}
}

// Verifies: GOV-004.
// The service provider's metadata names the entity, the HTTP-POST consumer
// service and the certificate to encrypt to; it carries no private key, no
// expiry that depends on the clock, and is the same every time.
func TestServiceProviderMetadata_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	cfg := config(idp)
	doc, err := provider().ServiceProviderMetadata(licensed(), cfg, spEntity)
	if err != nil {
		t.Fatalf("ServiceProviderMetadata: %v", err)
	}
	again, _ := provider().ServiceProviderMetadata(licensed(), cfg, spEntity)
	if !bytes.Equal(doc, again) {
		t.Error("the metadata differs between calls")
	}
	text := string(doc)
	block, _ := pem.Decode(cfg.DecryptionCertPEM)
	for _, want := range []string{`entityID="` + spEntity + `"`, `Location="` + acsURL + `"`, "HTTP-POST", base64.StdEncoding.EncodeToString(block.Bytes), `WantAssertionsSigned="true"`} {
		if !strings.Contains(text, want) {
			t.Errorf("the metadata lacks %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"PRIVATE", "validUntil", "HTTP-Artifact"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("the metadata contains %q", unwanted)
		}
	}
	cfg.DecryptionCertPEM = nil
	if _, err := provider().ServiceProviderMetadata(licensed(), cfg, spEntity); err == nil {
		t.Error("metadata without a certificate was made")
	}
}

// Verifies: GOV-004.
// The generated service key is a PKCS#8 RSA key with a self-signed
// certificate valid from the moment given, for the entity given.
func TestGenerateServiceKey_GOV_004(t *testing.T) {
	t.Parallel()
	keyPEM, certPEM, err := provider().GenerateServiceKey(rand.Reader, clockNow, spEntity)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := pem.Decode(keyPEM)
	cb, _ := pem.Decode(certPEM)
	if kb == nil || kb.Type != "PRIVATE KEY" || cb == nil || cb.Type != "CERTIFICATE" {
		t.Fatalf("PEM blocks: %v %v", kb, cb)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(kb.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.N.BitLen() != 3072 {
		t.Fatalf("key = %T", parsed)
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !cert.NotBefore.Equal(clockNow) || cert.NotAfter.Sub(clockNow) < 9*365*24*time.Hour || len(cert.URIs) != 1 || cert.URIs[0].String() != spEntity {
		t.Errorf("certificate = %v to %v, URIs %v", cert.NotBefore, cert.NotAfter, cert.URIs)
	}
	if err := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); err != nil {
		t.Errorf("not self-signed: %v", err)
	}
	if pub, ok := cert.PublicKey.(*rsa.PublicKey); !ok || !pub.Equal(&key.PublicKey) {
		t.Error("the certificate is not for the key")
	}
}
