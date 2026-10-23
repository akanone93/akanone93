// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/crewjam/saml/xmlenc"
	dsig "github.com/russellhaering/goxmldsig"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/ee/backend/saml"
)

// The fixed values of the tests' world.
const (
	idpEntity = "https://idp.example.com/metadata"
	spEntity  = "https://plux.example.com/saml/metadata"
	acsURL    = "https://plux.example.com/saml/acs"
	requestID = "id-request-1"
)

// clockNow is the clock of every test: a Tuesday noon.
var clockNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// testIdP is an identity provider that signs with a key made for the test.
type testIdP struct {
	key  *rsa.PrivateKey
	cert *x509.Certificate
}

// newIdP makes an identity provider whose certificate is valid around
// clockNow.
func newIdP(t testing.TB) testIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "test identity provider"},
		NotBefore: clockNow.Add(-24 * time.Hour), NotAfter: clockNow.Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return testIdP{key: key, cert: cert}
}

// pem returns the provider's certificate as the descriptor stores it.
func (i testIdP) pem() string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: i.cert.Raw}))
}

// sign signs el as an enveloped signature with the given method.
func (i testIdP) sign(t testing.TB, el *etree.Element, method string) *etree.Element {
	t.Helper()
	ctx, err := dsig.NewSigningContext(i.key, [][]byte{i.cert.Raw})
	if err != nil {
		t.Fatal(err)
	}
	ctx.Canonicalizer = dsig.MakeC14N10ExclusiveCanonicalizerWithPrefixList("")
	if err := ctx.SetSignatureMethod(method); err != nil {
		t.Fatal(err)
	}
	signed, err := ctx.SignEnveloped(el)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// spKey is the service provider's key pair, made once: generating a key of
// the size the product uses takes a moment.
var spKey = sync.OnceValues(func() (keyPEM, certPEM []byte) {
	key, cert, err := saml.Provider{}.GenerateServiceKey(rand.Reader, clockNow, spEntity)
	if err != nil {
		panic(err)
	}
	return key, cert
})

// provider is the SAML provider under test.
func provider() saml.Provider { return saml.Provider{} }

// config is the stored configuration of a provider trusting idp.
func config(idp testIdP) identity.SAMLConfig {
	key, cert := spKey()
	return identity.SAMLConfig{
		EntityID: spEntity, ACSURL: acsURL, AllowIdPInitiated: true,
		IdP:              identity.IdPDescriptor{EntityID: idpEntity, SSOURL: "https://idp.example.com/sso", SigningCertificates: []string{idp.pem()}},
		DecryptionKeyPEM: key, DecryptionCertPEM: cert,
	}
}

// spec describes a response; the zero value of a field is what a well-behaved
// provider sends, filled in by build.
type spec struct {
	// What the assertion says.
	assertionID, issuer, audience, recipient, inResponseTo string
	nameID, nameIDFormat                                   string
	notBefore, notOnOrAfter                                time.Time
	attributes                                             [][2]string
	// What the response says.
	destination, status string
	// How it is protected.
	signAssertion, signResponse, encrypt bool
	method                               string
	// mutate changes the assertion before it is signed.
	mutate func(assertion *etree.Element)
	// tamper changes the response after everything is signed.
	tamper func(response *etree.Element)
}

// defaults fills the fields a test did not set.
func (s spec) defaults() spec {
	fill := func(p *string, v string) {
		if *p == "" {
			*p = v
		}
	}
	fill(&s.assertionID, "_assertion-1")
	fill(&s.issuer, idpEntity)
	fill(&s.audience, spEntity)
	fill(&s.recipient, acsURL)
	fill(&s.nameID, "alice-subject")
	fill(&s.destination, acsURL)
	fill(&s.status, "urn:oasis:names:tc:SAML:2.0:status:Success")
	fill(&s.method, dsig.RSASHA256SignatureMethod)
	if s.notBefore.IsZero() {
		s.notBefore = clockNow.Add(-time.Minute)
	}
	if s.notOnOrAfter.IsZero() {
		s.notOnOrAfter = clockNow.Add(5 * time.Minute)
	}
	if s.attributes == nil {
		s.attributes = [][2]string{
			{"email", "alice@example.com"},
			{"displayName", "Alice Example"},
			{"groups", "engineering"},
			{"groups", "admins"},
		}
	}
	return s
}

func stamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

// assertion builds the assertion element.
func (s spec) assertion() *etree.Element {
	a := etree.NewElement("saml:Assertion")
	a.CreateAttr("xmlns:saml", "urn:oasis:names:tc:SAML:2.0:assertion")
	a.CreateAttr("ID", s.assertionID)
	a.CreateAttr("Version", "2.0")
	a.CreateAttr("IssueInstant", stamp(clockNow))
	a.CreateElement("saml:Issuer").SetText(s.issuer)
	subject := a.CreateElement("saml:Subject")
	name := subject.CreateElement("saml:NameID")
	if s.nameIDFormat != "" {
		name.CreateAttr("Format", s.nameIDFormat)
	}
	name.SetText(s.nameID)
	confirmation := subject.CreateElement("saml:SubjectConfirmation")
	confirmation.CreateAttr("Method", "urn:oasis:names:tc:SAML:2.0:cm:bearer")
	data := confirmation.CreateElement("saml:SubjectConfirmationData")
	data.CreateAttr("NotOnOrAfter", stamp(s.notOnOrAfter))
	data.CreateAttr("Recipient", s.recipient)
	if s.inResponseTo != "" {
		data.CreateAttr("InResponseTo", s.inResponseTo)
	}
	cond := a.CreateElement("saml:Conditions")
	cond.CreateAttr("NotBefore", stamp(s.notBefore))
	cond.CreateAttr("NotOnOrAfter", stamp(s.notOnOrAfter))
	cond.CreateElement("saml:AudienceRestriction").CreateElement("saml:Audience").SetText(s.audience)
	authn := a.CreateElement("saml:AuthnStatement")
	authn.CreateAttr("AuthnInstant", stamp(clockNow))
	authn.CreateAttr("SessionIndex", "session-9")
	authn.CreateElement("saml:AuthnContext").CreateElement("saml:AuthnContextClassRef").SetText("urn:oasis:names:tc:SAML:2.0:ac:classes:X509")
	statement := a.CreateElement("saml:AttributeStatement")
	for _, kv := range s.attributes {
		attr := statement.CreateElement("saml:Attribute")
		attr.CreateAttr("Name", kv[0])
		attr.CreateElement("saml:AttributeValue").SetText(kv[1])
	}
	return a
}

// response builds the response element around an assertion.
func (s spec) response(assertion *etree.Element) *etree.Element {
	r := etree.NewElement("samlp:Response")
	r.CreateAttr("xmlns:samlp", "urn:oasis:names:tc:SAML:2.0:protocol")
	r.CreateAttr("xmlns:saml", "urn:oasis:names:tc:SAML:2.0:assertion")
	r.CreateAttr("ID", "_response-1")
	r.CreateAttr("Version", "2.0")
	r.CreateAttr("IssueInstant", stamp(clockNow))
	r.CreateAttr("Destination", s.destination)
	if s.inResponseTo != "" {
		r.CreateAttr("InResponseTo", s.inResponseTo)
	}
	r.CreateElement("saml:Issuer").SetText(s.issuer)
	r.CreateElement("samlp:Status").CreateElement("samlp:StatusCode").CreateAttr("Value", s.status)
	r.AddChild(assertion)
	return r
}

// encrypted wraps an assertion for the service provider's certificate with
// the given key transport.
func encrypted(t testing.TB, assertion *etree.Element, transport xmlenc.RSA) *etree.Element {
	t.Helper()
	_, certPEM := spKey()
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	doc := etree.NewDocument()
	doc.SetRoot(assertion)
	plain, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	data, err := transport.Encrypt(cert, plain, nil)
	if err != nil {
		t.Fatal(err)
	}
	data.CreateAttr("Type", "http://www.w3.org/2001/04/xmlenc#Element")
	holder := etree.NewElement("saml:EncryptedAssertion")
	holder.AddChild(data)
	return holder
}

// build returns the base64 response a provider would post.
func (s spec) build(t testing.TB, idp testIdP) string {
	t.Helper()
	s = s.defaults()
	assertion := s.assertion()
	if s.mutate != nil {
		s.mutate(assertion)
	}
	inner := assertion
	if s.signAssertion {
		inner = idp.sign(t, assertion, s.method)
	}
	if s.encrypt {
		inner = encrypted(t, inner, xmlenc.OAEP())
	}
	response := s.response(inner)
	if s.signResponse {
		response = idp.sign(t, response, s.method)
	}
	if s.tamper != nil {
		s.tamper(response)
	}
	doc := etree.NewDocument()
	doc.SetRoot(response)
	raw, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// memoryReplay is a replay cache in memory.
type memoryReplay struct {
	mu   sync.Mutex
	seen map[string]time.Duration
	err  error
}

// Seen records key and reports whether it was recorded before.
func (m *memoryReplay) Seen(_ context.Context, key string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return false, m.err
	}
	if m.seen == nil {
		m.seen = map[string]time.Duration{}
	}
	_, ok := m.seen[key]
	m.seen[key] = ttl
	return ok, nil
}

// licence is a provider with a fixed status.
type licence edition.Status

// Status returns the fixed status.
func (l licence) Status() edition.Status { return edition.Status(l) }

// licensed returns a context whose licence lists SAML.
func licensed() context.Context {
	return edition.WithProvider(context.Background(), licence{State: edition.StateValid, Features: []edition.Feature{edition.FeatureSAML}})
}

// verify posts a response to the provider the way the core does.
func verify(ctx context.Context, cfg identity.SAMLConfig, response, request string, cache identity.ReplayCache) (identity.Identity, error) {
	return saml.Provider{}.Verify(ctx, identity.SAMLVerify{
		Config: cfg, SAMLResponse: response, RequestID: request, Now: clockNow, ClockSkew: 30 * time.Second, Replay: cache,
	})
}
