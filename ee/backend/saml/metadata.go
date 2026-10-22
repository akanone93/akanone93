// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml

import (
	"crypto/x509"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/beevik/etree"
	crewsaml "github.com/crewjam/saml"

	"github.com/nightCode42/plux3/backend/ext/identity"
)

// protocolSAML2 is the protocol an identity provider's descriptor must list.
const protocolSAML2 = "urn:oasis:names:tc:SAML:2.0:protocol"

// readIdPMetadata reads an identity provider's metadata. The document is
// trusted as far as the administrator who imported it is, so the checks are
// about its shape: that it is well-formed, describes one provider that speaks
// SAML 2.0, and gives an address to send people to and a certificate to
// trust. A document of several entities is refused, because which of them
// the administrator meant is not for Plux to guess.
func readIdPMetadata(data []byte) (identity.IdPDescriptor, error) {
	doc := etree.NewDocument()
	doc.ReadSettings.ValidateInput = true
	if err := doc.ReadFromBytes(data); err != nil {
		return identity.IdPDescriptor{}, fmt.Errorf("the metadata is not well-formed: %w", err)
	}
	var ed crewsaml.EntityDescriptor
	if err := xml.Unmarshal(data, &ed); err != nil {
		return identity.IdPDescriptor{}, errors.New("the metadata is not the description of one entity")
	}
	if ed.EntityID == "" {
		return identity.IdPDescriptor{}, errors.New("the metadata names no entity")
	}
	idp, err := idpDescriptor(ed)
	if err != nil {
		return identity.IdPDescriptor{}, err
	}
	sso, err := redirectEndpoint(idp)
	if err != nil {
		return identity.IdPDescriptor{}, err
	}
	certs, err := signingCertificates(idp)
	if err != nil {
		return identity.IdPDescriptor{}, err
	}
	return identity.IdPDescriptor{EntityID: ed.EntityID, SSOURL: sso, SigningCertificates: certs}, nil
}

// idpDescriptor returns the one identity provider role of an entity that
// speaks SAML 2.0.
func idpDescriptor(ed crewsaml.EntityDescriptor) (crewsaml.IDPSSODescriptor, error) {
	var found []crewsaml.IDPSSODescriptor
	for _, d := range ed.IDPSSODescriptors {
		if strings.Contains(d.ProtocolSupportEnumeration, protocolSAML2) {
			found = append(found, d)
		}
	}
	if len(found) != 1 {
		return crewsaml.IDPSSODescriptor{}, fmt.Errorf("the metadata has %d identity provider descriptors for SAML 2.0, not one", len(found))
	}
	return found[0], nil
}

// redirectEndpoint returns the single sign-on address that takes a request
// through the HTTP-Redirect binding, which must be https.
func redirectEndpoint(idp crewsaml.IDPSSODescriptor) (string, error) {
	for _, e := range idp.SingleSignOnServices {
		if e.Binding != crewsaml.HTTPRedirectBinding {
			continue
		}
		if err := requireHTTPS(e.Location); err != nil {
			return "", fmt.Errorf("the single sign-on address: %w", err)
		}
		return e.Location, nil
	}
	return "", errors.New("the identity provider has no single sign-on endpoint for the HTTP-Redirect binding")
}

// signingCertificates returns, as PEM, the certificates the provider signs
// with: those for signing, and those with no stated use, which the
// specification takes to mean both.
func signingCertificates(idp crewsaml.IDPSSODescriptor) ([]string, error) {
	var out []string
	for _, kd := range idp.KeyDescriptors {
		if kd.Use != "" && kd.Use != "signing" {
			continue
		}
		for _, c := range kd.KeyInfo.X509Data.X509Certificates {
			der, err := decodeBase64(c.Data)
			if err != nil {
				return nil, errors.New("a signing certificate is not base64")
			}
			if _, err := x509.ParseCertificate(der); err != nil {
				return nil, fmt.Errorf("a signing certificate cannot be read: %w", err)
			}
			out = append(out, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
		}
	}
	if len(out) == 0 {
		return nil, errors.New("the identity provider publishes no signing certificate")
	}
	return out, nil
}

// serviceProvider builds the library's view of the service provider for a
// call. metadataURL names the entity when the configuration has no identifier.
func serviceProvider(cfg identity.SAMLConfig, metadataURL string) (*crewsaml.ServiceProvider, error) {
	if err := requireHTTPS(cfg.ACSURL); err != nil {
		return nil, fmt.Errorf("the assertion consumer service address: %w", err)
	}
	acs, err := url.Parse(cfg.ACSURL)
	if err != nil {
		return nil, fmt.Errorf("the assertion consumer service address: %w", err)
	}
	sp := &crewsaml.ServiceProvider{EntityID: cfg.EntityID, AcsURL: *acs, AuthnNameIDFormat: crewsaml.UnspecifiedNameIDFormat}
	if sp.EntityID == "" {
		sp.EntityID = metadataURL
	}
	if sp.EntityID == "" {
		return nil, errors.New("the service provider has no entity identifier")
	}
	return sp, nil
}

// writeServiceProviderMetadata renders the service provider's metadata: the
// entity, the HTTP-POST assertion consumer service, and the certificate an
// identity provider encrypts assertions to. The document has no expiry, so it
// does not depend on the clock and is the same every time it is asked for.
func writeServiceProviderMetadata(cfg identity.SAMLConfig, metadataURL string) ([]byte, error) {
	sp, err := serviceProvider(cfg, metadataURL)
	if err != nil {
		return nil, err
	}
	if sp.Certificate, err = parseCertificate(cfg.DecryptionCertPEM); err != nil {
		return nil, err
	}
	md := sp.Metadata()
	for i := range md.SPSSODescriptors {
		d := &md.SPSSODescriptors[i]
		d.AssertionConsumerServices = d.AssertionConsumerServices[:1]
	}
	out, err := xml.Marshal(md)
	if err != nil {
		return nil, fmt.Errorf("render the metadata: %w", err)
	}
	return withoutExpiry(out)
}

// withoutExpiry drops the validUntil attributes the library stamps with the
// time of the call, and indents the document. The library renders a zero
// expiry as the year 1, which an identity provider would take as long past.
func withoutExpiry(document []byte) ([]byte, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(document); err != nil {
		return nil, fmt.Errorf("read the metadata: %w", err)
	}
	removeAttribute(doc.Root(), "validUntil")
	doc.Indent(2)
	doc.InsertChildAt(0, etree.NewProcInst("xml", `version="1.0" encoding="UTF-8"`))
	out, err := doc.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("saml: serialise the metadata: %w", err)
	}
	return out, nil
}

// removeAttribute removes the attribute from el and everything below it.
func removeAttribute(el *etree.Element, name string) {
	el.RemoveAttr(name)
	for _, c := range el.ChildElements() {
		removeAttribute(c, name)
	}
}

// parseCertificate reads one PEM certificate.
func parseCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errors.New("the service provider has no certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("the service provider's certificate cannot be read: %w", err)
	}
	return cert, nil
}
