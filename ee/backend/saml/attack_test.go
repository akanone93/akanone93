// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml_test

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/crewjam/saml/xmlenc"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// refusedWith asserts that Verify refuses response as an invalid one, and
// that the reason it gives mentions want.
func refusedWith(t *testing.T, name string, idp testIdP, response, want string) {
	t.Helper()
	who, err := verify(licensed(), config(idp), response, requestID, &memoryReplay{})
	if !errors.Is(err, identity.ErrInvalidResponse) {
		t.Errorf("%s: accepted %+v, err = %v", name, who, err)
		return
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("%s: refused for the wrong reason: %v (want %q)", name, err, want)
	}
}

// evilAssertion is an assertion for someone else, which the attacker did not
// get signed.
func evilAssertion() *etree.Element {
	s := answering(spec{nameID: "mallory", assertionID: "_evil"}).defaults()
	return s.assertion()
}

// Verifies: GOV-004, SEC-103.
// XML signature wrapping: a signed assertion with an injected unsigned one
// beside it is refused, wherever the injected assertion sits, so that a
// reader taking the first assertion and a verifier taking the signed one
// cannot be made to disagree.
func TestVerify_SignatureWrapping_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	for name, inject := range map[string]func(response *etree.Element){
		"the injected assertion first": func(r *etree.Element) { r.InsertChildAt(r.FindElement("Status").Index()+1, evilAssertion()) },
		"the injected assertion last":  func(r *etree.Element) { r.AddChild(evilAssertion()) },
		"the signed assertion hidden and the injected one in view": func(r *etree.Element) {
			signed := r.FindElement("Assertion")
			r.RemoveChild(signed)
			wrapper := r.CreateElement("samlp:Extensions")
			wrapper.AddChild(signed)
			r.AddChild(evilAssertion())
		},
	} {
		s := signedAssertion()
		s.tamper = inject
		refusedWith(t, name, idp, s.build(t, idp), "assertion")
	}
}

// Verifies: GOV-004, SEC-103.
// Content changed after signing, a signature moved from one assertion to
// another, and a signature made by a key the provider's descriptor does not
// list are all refused.
func TestVerify_ForgedSignatures_GOV_004(t *testing.T) {
	t.Parallel()
	idp, attacker := newIdP(t), newIdP(t)
	changed := signedAssertion()
	changed.tamper = func(r *etree.Element) { r.FindElement(".//NameID").SetText("mallory") }
	refusedWith(t, "a subject changed after signing", idp, changed.build(t, idp), "signature")

	moved := signedAssertion()
	moved.tamper = func(r *etree.Element) {
		signed := r.FindElement("Assertion")
		evil := evilAssertion()
		evil.RemoveAttr("ID")
		evil.CreateAttr("ID", signed.SelectAttrValue("ID", ""))
		evil.AddChild(signed.FindElement("Signature").Copy())
		r.RemoveChild(signed)
		r.AddChild(evil)
	}
	refusedWith(t, "a signature moved onto another assertion", idp, moved.build(t, idp), "signature")

	forged := signedAssertion()
	refusedWith(t, "signed by a key nobody trusts", idp, forged.build(t, attacker), "signature")
}

// Verifies: GOV-004.
// A comment that splits a name in two, which a signature over the
// canonical form does not cover, is refused: the document is not read.
func TestVerify_CommentInjection_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	s := signedAssertion()
	s.nameID = "admin@example.com.evil.example"
	s.tamper = func(r *etree.Element) {
		name := r.FindElement(".//NameID")
		name.SetText("admin@example.com")
		name.CreateComment("")
		name.CreateCharData(".evil.example")
	}
	refusedWith(t, "a comment in the subject", idp, s.build(t, idp), "comment")
}

// Verifies: GOV-004.
// Documents that are not the response of an identity provider are refused
// without being read as one.
func TestVerify_Malformed_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	encode := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	for name, response := range map[string]string{
		"empty":               "",
		"not base64":          "!!!not base64!!!",
		"not xml":             encode("this is not xml"),
		"another document":    encode(`<root xmlns="urn:oasis:names:tc:SAML:2.0:protocol"/>`),
		"no assertion":        encode(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" Version="2.0"><samlp:Status><samlp:StatusCode Value="urn:oasis:names:tc:SAML:2.0:status:Success"/></samlp:Status></samlp:Response>`),
		"a repeated ID":       encode(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" ID="a" ID="b" Version="2.0"/>`),
		"a doctype":           encode(`<!DOCTYPE r [<!ENTITY x "y">]><samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" Version="2.0">&x;</samlp:Response>`),
		"truncated":           encode(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" Version="2.0"><samlp:Sta`),
		"deeply nested":       encode(strings.Repeat("<a>", 100000) + strings.Repeat("</a>", 100000)),
		"wrong root and deep": encode(strings.Repeat("<samlp:Response xmlns:samlp=\"urn:oasis:names:tc:SAML:2.0:protocol\">", 100)),
	} {
		cache := &memoryReplay{}
		if _, err := verify(licensed(), config(idp), response, requestID, cache); !errors.Is(err, identity.ErrInvalidResponse) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Verifies: GOV-004.
// A wrapped content key is opened only when it is wrapped with OAEP: PKCS#1
// v1.5 is a decryption oracle, and a key the service provider does not hold
// opens nothing.
func TestVerify_EncryptionAttacks_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	s := answering(spec{}).defaults()
	response := s.response(encrypted(t, idp.sign(t, s.assertion(), s.method), xmlenc.PKCS1v15()))
	doc := etree.NewDocument()
	doc.SetRoot(response)
	raw, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	refusedWith(t, "a key wrapped with PKCS#1 v1.5", idp, base64.StdEncoding.EncodeToString(raw), "key transport")

	cfg := config(idp)
	otherKey, _, err := provider().GenerateServiceKey(rand.Reader, clockNow, spEntity)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DecryptionKeyPEM = otherKey
	_, err = verify(licensed(), cfg, answering(spec{signAssertion: true, encrypt: true}).build(t, idp), requestID, &memoryReplay{})
	if !errors.Is(err, identity.ErrInvalidResponse) {
		t.Errorf("encrypted to another key: %v", err)
	}
}
