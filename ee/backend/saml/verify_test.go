// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// answering makes s the answer to the sign-in that request requestID started.
func answering(s spec) spec {
	s.inResponseTo = requestID
	return s
}

// signedAssertion is the response most identity providers send: the
// assertion signed, answering our request.
func signedAssertion() spec { return answering(spec{signAssertion: true}) }

// Verifies: GOV-004.
// A response with a signed assertion that answers our request gives the
// person it names, with the email, name, groups and session the provider
// asserted.
func TestVerify_SignedAssertion_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	who, err := verify(licensed(), config(idp), signedAssertion().build(t, idp), requestID, &memoryReplay{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if who.Issuer != idpEntity || who.Subject != "alice-subject" || who.Email != "alice@example.com" || !who.EmailVerified {
		t.Errorf("identity = %+v", who)
	}
	if who.DisplayName != "Alice Example" || strings.Join(who.Groups, ",") != "engineering,admins" {
		t.Errorf("name and groups = %q, %q", who.DisplayName, who.Groups)
	}
	if who.SessionIndex != "session-9" || !strings.HasSuffix(who.AuthnContext, "classes:X509") {
		t.Errorf("session = %q, %q", who.SessionIndex, who.AuthnContext)
	}
}

// Verifies: GOV-004.
// A signature on the response alone is enough, and so is one on the
// assertion alone; neither alone is refused when it is absent.
func TestVerify_ResponseSignature_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	for name, s := range map[string]spec{
		"response signed":     answering(spec{signResponse: true}),
		"both signed":         answering(spec{signResponse: true, signAssertion: true}),
		"assertion signed":    signedAssertion(),
		"signed with SHA-512": answering(spec{signAssertion: true, method: dsig.RSASHA512SignatureMethod}),
	} {
		if _, err := verify(licensed(), config(idp), s.build(t, idp), requestID, &memoryReplay{}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Verifies: GOV-004.
// An identity-provider-initiated response is accepted when the provider
// allows it, and refused when it does not or when it claims to answer a
// request.
func TestVerify_IdPInitiated_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	unsolicited := spec{signAssertion: true}.build(t, idp)
	cfg := config(idp)
	if _, err := verify(licensed(), cfg, unsolicited, "", &memoryReplay{}); err != nil {
		t.Errorf("an allowed unsolicited response: %v", err)
	}
	cfg.AllowIdPInitiated = false
	if _, err := verify(licensed(), cfg, unsolicited, "", &memoryReplay{}); !errors.Is(err, identity.ErrInvalidResponse) {
		t.Errorf("an unsolicited response the provider does not allow: %v", err)
	}
	if _, err := verify(licensed(), config(idp), signedAssertion().build(t, idp), "", &memoryReplay{}); !errors.Is(err, identity.ErrInvalidResponse) {
		t.Errorf("a response to a request, taken as unsolicited: %v", err)
	}
}

// Verifies: GOV-004, SEC-103.
// Each way a response can be wrong is refused as an invalid response, and
// none of them takes room in the replay cache.
func TestVerify_Refusals_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	later := clockNow.Add
	for name, s := range map[string]spec{
		"unsigned":                      answering(spec{}),
		"wrong audience":                answering(spec{signAssertion: true, audience: "https://other.example.com/"}),
		"wrong recipient":               answering(spec{signAssertion: true, recipient: "https://evil.example.com/acs"}),
		"wrong destination":             answering(spec{signResponse: true, destination: "https://evil.example.com/acs"}),
		"wrong issuer":                  answering(spec{signAssertion: true, issuer: "https://evil.example.com/"}),
		"another request":               {signAssertion: true, inResponseTo: "id-someone-else"},
		"no request":                    {signAssertion: true},
		"expired":                       answering(spec{signAssertion: true, notOnOrAfter: later(-31 * time.Second)}),
		"expires exactly at the skew":   answering(spec{signAssertion: true, notOnOrAfter: later(-30 * time.Second)}),
		"not yet valid":                 answering(spec{signAssertion: true, notBefore: later(31 * time.Second), notOnOrAfter: later(time.Hour)}),
		"a transient subject":           answering(spec{signAssertion: true, nameIDFormat: "urn:oasis:names:tc:SAML:2.0:nameid-format:transient"}),
		"an identity provider failure":  answering(spec{signAssertion: true, status: "urn:oasis:names:tc:SAML:2.0:status:Responder"}),
		"signed with SHA-1":             answering(spec{signAssertion: true, method: dsig.RSASHA1SignatureMethod}),
		"no audience":                   answering(spec{signAssertion: true, mutate: removeAll("Conditions/AudienceRestriction")}),
		"a condition not understood":    answering(spec{signAssertion: true, mutate: addCondition}),
		"no end to the validity window": answering(spec{signAssertion: true, mutate: dropAttribute("Conditions", "NotOnOrAfter")}),
		"no bearer confirmation":        answering(spec{signAssertion: true, mutate: removeAll("Subject/SubjectConfirmation")}),
		"no subject":                    answering(spec{signAssertion: true, mutate: removeAll("Subject")}),
	} {
		cache := &memoryReplay{}
		_, err := verify(licensed(), config(idp), s.build(t, idp), requestID, cache)
		if !errors.Is(err, identity.ErrInvalidResponse) || errors.Is(err, identity.ErrReplayed) {
			t.Errorf("%s: err = %v, want an invalid response", name, err)
		}
		if len(cache.seen) != 0 {
			t.Errorf("%s: a refused assertion was recorded", name)
		}
	}
}

// Verifies: GOV-004.
// The validity window tolerates exactly the skew the core passes in, on both
// ends.
func TestVerify_ClockSkew_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	cfg := config(idp)
	for name, s := range map[string]spec{
		"expired within the skew":   answering(spec{signAssertion: true, notOnOrAfter: clockNow.Add(-29 * time.Second)}),
		"not yet valid but within":  answering(spec{signAssertion: true, notBefore: clockNow.Add(29 * time.Second)}),
		"valid and about to expire": answering(spec{signAssertion: true, notOnOrAfter: clockNow.Add(time.Second)}),
	} {
		if _, err := verify(licensed(), cfg, s.build(t, idp), requestID, &memoryReplay{}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	strict := identity.SAMLVerify{Config: cfg, SAMLResponse: answering(spec{signAssertion: true, notOnOrAfter: clockNow.Add(-time.Second)}).build(t, idp), RequestID: requestID, Now: clockNow, Replay: &memoryReplay{}}
	if _, err := provider().Verify(licensed(), strict); !errors.Is(err, identity.ErrInvalidResponse) {
		t.Errorf("a window one second over, without skew: %v", err)
	}
}

// Verifies: GOV-004, SEC-103.
// An assertion is accepted once. A second use is a replay (PLX-6211), and a
// replay cache that cannot answer, or is absent, refuses the sign-in.
func TestVerify_Replay_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	response := signedAssertion().build(t, idp)
	cache := &memoryReplay{}
	if _, err := verify(licensed(), config(idp), response, requestID, cache); err != nil {
		t.Fatalf("the first use: %v", err)
	}
	_, err := verify(licensed(), config(idp), response, requestID, cache)
	if !errors.Is(err, identity.ErrReplayed) || errors.Is(err, identity.ErrInvalidResponse) {
		t.Errorf("the second use: %v, want a replay", err)
	}
	for name, c := range map[string]identity.ReplayCache{
		"a cache that fails": &memoryReplay{err: errors.New("valkey is down")},
		"no cache":           nil,
	} {
		if _, err := verify(licensed(), config(idp), response, requestID, c); !errors.Is(err, identity.ErrInvalidResponse) {
			t.Errorf("%s: %v, want a refusal", name, err)
		}
	}
	if len(cache.seen) != 1 {
		t.Errorf("cache holds %d keys, want 1", len(cache.seen))
	}
	for key, ttl := range cache.seen {
		if strings.Contains(key, "_assertion-1") {
			t.Errorf("the key %q carries the assertion identifier in the clear", key)
		}
		if ttl != 5*time.Minute+30*time.Second {
			t.Errorf("ttl = %v, want the rest of the window and the skew", ttl)
		}
	}
}

// Verifies: GOV-004.
// An encrypted assertion is opened with the service provider's key, whether
// the assertion is signed inside or the response is signed outside, and one
// that is encrypted but signed nowhere is refused.
func TestVerify_Encrypted_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	for name, s := range map[string]spec{
		"signed inside":  answering(spec{encrypt: true, signAssertion: true}),
		"signed outside": answering(spec{encrypt: true, signResponse: true}),
	} {
		who, err := verify(licensed(), config(idp), s.build(t, idp), requestID, &memoryReplay{})
		if err != nil || who.Subject != "alice-subject" {
			t.Errorf("%s: %+v, %v", name, who, err)
		}
	}
	unsigned := answering(spec{encrypt: true}).build(t, idp)
	if _, err := verify(licensed(), config(idp), unsigned, requestID, &memoryReplay{}); !errors.Is(err, identity.ErrInvalidResponse) {
		t.Errorf("encrypted and unsigned: %v", err)
	}
	cfg := config(idp)
	cfg.DecryptionKeyPEM = nil
	if _, err := verify(licensed(), cfg, answering(spec{encrypt: true, signAssertion: true}).build(t, idp), requestID, &memoryReplay{}); !errors.Is(err, identity.ErrInvalidResponse) {
		t.Errorf("encrypted, with no key: %v", err)
	}
}

// Verifies: GOV-004, GOV-033.
// SAML follows the licence: without one the calls report PLX-8600.
func TestProvider_Licence_GOV_033(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	response := signedAssertion().build(t, idp)
	if _, err := verify(context.Background(), config(idp), response, requestID, &memoryReplay{}); err == nil || !strings.Contains(err.Error(), "PLX-8600") {
		t.Errorf("Verify without a licence: %v", err)
	}
	if _, err := provider().Start(context.Background(), config(idp), "state", clockNow); err == nil || !strings.Contains(err.Error(), "PLX-8600") {
		t.Errorf("Start without a licence: %v", err)
	}
	if _, err := provider().ImportMetadata(context.Background(), []byte("<x/>")); err == nil || !strings.Contains(err.Error(), "PLX-8600") {
		t.Errorf("ImportMetadata without a licence: %v", err)
	}
	if _, err := provider().ServiceProviderMetadata(context.Background(), config(idp), spEntity); err == nil || !strings.Contains(err.Error(), "PLX-8600") {
		t.Errorf("ServiceProviderMetadata without a licence: %v", err)
	}
}

// Verifies: GOV-004.
// The attributes the administrator names win over the common ones, and an
// address-format subject stands in for a missing email attribute.
func TestVerify_AttributeNames_GOV_004(t *testing.T) {
	t.Parallel()
	idp := newIdP(t)
	cfg := config(idp)
	cfg.EmailAttribute, cfg.NameAttribute, cfg.GroupsAttribute = "corp-mail", "corp-name", "corp-roles"
	s := answering(spec{signAssertion: true, attributes: [][2]string{
		{"email", "decoy@example.com"}, {"corp-mail", "alice@corp.example.com"}, {"corp-name", "Alice C."}, {"corp-roles", "ops"},
	}})
	who, err := verify(licensed(), cfg, s.build(t, idp), requestID, &memoryReplay{})
	if err != nil || who.Email != "alice@corp.example.com" || who.DisplayName != "Alice C." || strings.Join(who.Groups, ",") != "ops" {
		t.Errorf("named attributes: %+v, %v", who, err)
	}
	bare := answering(spec{
		signAssertion: true, attributes: [][2]string{{"firstName", "Alice"}, {"lastName", "Example"}},
		nameID: "bob@example.com", nameIDFormat: "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress",
	})
	who, err = verify(licensed(), config(idp), bare.build(t, idp), requestID, &memoryReplay{})
	if err != nil || who.Email != "bob@example.com" || who.DisplayName != "Alice Example" || len(who.Groups) != 0 {
		t.Errorf("fallbacks: %+v, %v", who, err)
	}
}

// removeAll returns a mutation that removes the elements at a path.
func removeAll(path string) func(*etree.Element) {
	return func(a *etree.Element) {
		for _, el := range a.FindElements(path) {
			el.Parent().RemoveChild(el)
		}
	}
}

// dropAttribute returns a mutation that removes an attribute of a child.
func dropAttribute(path, name string) func(*etree.Element) {
	return func(a *etree.Element) { a.FindElement(path).RemoveAttr(name) }
}

// addCondition adds a condition the specification defines no meaning for.
func addCondition(a *etree.Element) {
	a.FindElement("Conditions").CreateElement("saml:Condition").CreateAttr("Kind", "geofence")
}
