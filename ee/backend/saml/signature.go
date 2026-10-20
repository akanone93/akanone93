// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
	"github.com/russellhaering/goxmldsig/etreeutils"
)

// trust is the identity provider's signing certificates and the moment at
// which they must be valid.
type trust struct {
	certs []*x509.Certificate
	now   time.Time
}

// newTrust reads the PEM certificates a provider's messages may be signed
// with. Nothing else is trusted, so a descriptor without any is unusable.
func newTrust(pems []string, now time.Time) (trust, error) {
	t := trust{now: now}
	for _, p := range pems {
		for rest := []byte(p); ; {
			var block *pem.Block
			block, rest = pem.Decode(rest)
			if block == nil {
				break
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return trust{}, fmt.Errorf("a signing certificate cannot be read: %w", err)
			}
			t.certs = append(t.certs, cert)
		}
	}
	if len(t.certs) == 0 {
		return trust{}, errors.New("the identity provider has no signing certificate")
	}
	return t, nil
}

// signature returns the enveloped signature of el: its direct child
// Signature. It is nil when el carries none. A signature that is not a
// direct child is not one of el's: the one an attacker nests inside a
// document to make it look signed, or the one of another element moved here.
func signature(el *etree.Element) (*etree.Element, error) {
	return optional(el, nsSignature, "Signature")
}

// verify checks the signature of el against the trusted certificates and
// returns the element as it was signed: the content the digest was computed
// over, not the document that was posted. A caller reads only what verify
// returns.
func (t trust) verify(el *etree.Element) (*etree.Element, error) {
	sig, err := signature(el)
	if err != nil {
		return nil, err
	}
	if sig == nil {
		return nil, errors.New("the element is not signed")
	}
	if err := checkSignedInfo(el, sig); err != nil {
		return nil, err
	}
	detached, err := detach(el)
	if err != nil {
		return nil, err
	}
	var failure error
	for _, cert := range t.certs {
		signed, err := t.validate(detached, cert)
		if err == nil {
			return signed, nil
		}
		failure = err
	}
	return nil, fmt.Errorf("the signature of <%s> cannot be verified: %w", el.Tag, failure)
}

// validate checks detached's signature against one certificate.
func (t trust) validate(detached *etree.Element, cert *x509.Certificate) (*etree.Element, error) {
	ctx := dsig.NewDefaultValidationContext(&dsig.MemoryX509CertificateStore{Roots: []*x509.Certificate{cert}})
	ctx.IdAttribute = "ID"
	ctx.Clock = dsig.NewFakeClockAt(t.now)
	candidate := detached.Copy()
	if candidate.FindElement("./Signature/KeyInfo/X509Data/X509Certificate") == nil {
		if keyInfo := candidate.FindElement("./Signature/KeyInfo"); keyInfo != nil {
			keyInfo.Parent().RemoveChild(keyInfo)
		}
	}
	signed, err := ctx.Validate(candidate)
	if err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	return signed, nil
}

// detach copies el with the namespaces in scope declared on it, which is
// what the signature validation needs to canonicalise it as it was signed.
func detach(el *etree.Element) (*etree.Element, error) {
	parent, err := etreeutils.NSBuildParentContext(el)
	if err != nil {
		return nil, fmt.Errorf("read the namespaces: %w", err)
	}
	scope, err := parent.SubContext(el)
	if err != nil {
		return nil, fmt.Errorf("read the namespaces: %w", err)
	}
	detached, err := etreeutils.NSDetatch(scope, el)
	if err != nil {
		return nil, fmt.Errorf("read the namespaces: %w", err)
	}
	return detached, nil
}

// checkSignedInfo refuses a signature that does not sign el, and only el,
// with a modern algorithm. The library finds a signature by walking the whole
// tree and takes the first that points at the element, so the rules that make
// the one it finds the one that matters are ours: the signature is el's own,
// it has a single reference, the reference is el's identifier, no other
// signature in the tree points at el, and neither the signature nor the
// digest use SHA-1.
func checkSignedInfo(el, sig *etree.Element) error {
	id := el.SelectAttrValue("ID", "")
	if id == "" {
		return fmt.Errorf("<%s> has no ID to be signed by", el.Tag)
	}
	info, err := required(sig, nsSignature, "SignedInfo")
	if err != nil {
		return err
	}
	refs := children(info, nsSignature, "Reference")
	if len(refs) != 1 {
		return fmt.Errorf("the signature has %d references, not one", len(refs))
	}
	if uri := refs[0].SelectAttrValue("URI", ""); uri != "#"+id {
		return errors.New("the signature does not reference the element it is in")
	}
	if rival := rivalSignature(el, sig, id); rival != nil {
		return errors.New("another signature in the element points at the element")
	}
	return checkAlgorithms(info, refs[0])
}

// rivalSignature returns a signature other than sig, anywhere below el, that
// references el itself, by its identifier or as the whole document.
func rivalSignature(el, sig *etree.Element, id string) *etree.Element {
	for _, c := range el.ChildElements() {
		if c != sig && is(c, nsSignature, "Signature") && referencesElement(c, id) {
			return c
		}
		if rival := rivalSignature(c, sig, id); rival != nil {
			return rival
		}
	}
	return nil
}

// referencesElement reports whether a signature has a reference to the whole
// document or to the element with the identifier id.
func referencesElement(sig *etree.Element, id string) bool {
	for _, ref := range sig.FindElements(".//Reference") {
		if uri := ref.SelectAttrValue("URI", ""); uri == "" || uri == "#"+id {
			return true
		}
	}
	return false
}

// checkAlgorithms refuses SHA-1 in a signature method or a digest method.
func checkAlgorithms(info, ref *etree.Element) error {
	method, err := required(info, nsSignature, "SignatureMethod")
	if err != nil {
		return err
	}
	digest, err := required(ref, nsSignature, "DigestMethod")
	if err != nil {
		return err
	}
	for _, m := range []*etree.Element{method, digest} {
		if alg := m.SelectAttrValue("Algorithm", ""); strings.Contains(strings.ToLower(alg), "sha1") {
			return fmt.Errorf("the algorithm %q is not accepted", alg)
		}
	}
	return nil
}
