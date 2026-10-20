// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml_test

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/beevik/etree"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// FuzzSAMLResponse feeds Verify the bytes of a response and checks what must
// hold of every answer: it does not panic, it refuses with an invalid
// response or a replay and nothing else, and whatever it accepts is the
// person the identity provider signed for, however the bytes were mangled.
//
// Verifies: GOV-004.
func FuzzSAMLResponse(f *testing.F) {
	idp := newIdP(f)
	cfg := config(idp)
	valid := signedAssertion().build(f, idp)
	both := answering(spec{signAssertion: true, signResponse: true}).build(f, idp)
	encrypted := answering(spec{signAssertion: true, encrypt: true}).build(f, idp)
	wrapped := signedAssertion()
	wrapped.tamper = func(r *etree.Element) { r.AddChild(evilAssertion()) }
	for _, seed := range []string{
		valid, both, encrypted, wrapped.build(f, idp), "", "AAAA",
		base64.StdEncoding.EncodeToString([]byte("<a/>")),
		base64.StdEncoding.EncodeToString([]byte(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol"/>`)),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, response string) {
		who, err := verify(licensed(), cfg, response, requestID, &memoryReplay{})
		switch {
		case err == nil:
			if who.Subject != "alice-subject" || who.Issuer != idpEntity {
				t.Fatalf("accepted a response for %+v", who)
			}
		case !errors.Is(err, identity.ErrInvalidResponse) && !errors.Is(err, identity.ErrReplayed):
			t.Fatalf("refused with an error that is neither: %v", err)
		}
	})
}
