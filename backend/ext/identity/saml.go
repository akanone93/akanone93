// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"io"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// IdPDescriptor is what Plux needs to know about a SAML identity provider.
// It is read from the provider's metadata and stored with the provider.
type IdPDescriptor struct {
	// EntityID is the provider's entity identifier; an assertion's issuer
	// must equal it.
	EntityID string `json:"entityId"`
	// SSOURL is the single sign-on endpoint that accepts an AuthnRequest
	// through the HTTP-Redirect binding.
	SSOURL string `json:"ssoUrl"`
	// SigningCertificates are the PEM certificates whose keys sign the
	// provider's responses and assertions. Nothing else is trusted.
	SigningCertificates []string `json:"signingCertificates"`
}

// SAMLConfig is one SAML provider as the core stores it, handed to the
// implementation for each call.
type SAMLConfig struct {
	// EntityID is the service provider's entity identifier, the audience an
	// assertion must name.
	EntityID string
	// ACSURL is the assertion consumer service URL: the recipient of every
	// assertion and the destination of every response.
	ACSURL string
	// IdP is the identity provider.
	IdP IdPDescriptor
	// DecryptionKeyPEM and DecryptionCertPEM are the service provider's key
	// pair for encrypted assertions. The private key is sensitive: it is
	// used within the call and not kept.
	DecryptionKeyPEM  []byte
	DecryptionCertPEM []byte
	// AllowIdPInitiated accepts a response that answers no request of ours.
	AllowIdPInitiated bool
	// EmailAttribute, NameAttribute and GroupsAttribute name the assertion
	// attributes that carry the address, the display name and the groups.
	// An empty name selects the commonly used names.
	EmailAttribute  string
	NameAttribute   string
	GroupsAttribute string
}

// SAMLStart is a sign-in handed to the identity provider.
type SAMLStart struct {
	// RequestID is the AuthnRequest's identifier; the response must answer
	// it. The core keeps it with the sign-in.
	RequestID string
	// URL is where the browser goes: the provider's single sign-on endpoint
	// with the request (HTTP-Redirect binding).
	URL string
}

// SAMLVerify is a response to verify.
type SAMLVerify struct {
	Config SAMLConfig
	// SAMLResponse is the form field as posted: the base64 of the XML.
	SAMLResponse string
	// RequestID is the request this response must answer. Empty means an
	// IdP-initiated response, accepted only if Config.AllowIdPInitiated.
	RequestID string
	// Now and ClockSkew bound the validity windows of the assertion.
	Now       time.Time
	ClockSkew time.Duration
	// Replay refuses an assertion identifier seen before.
	Replay ReplayCache
}

// SAMLProvider is SAML 2.0 single sign-on, the enterprise part of GOV-004.
// Implementations are safe for concurrent use and hold no state.
type SAMLProvider interface {
	// ImportMetadata reads an identity provider's metadata document. A
	// document that cannot be trusted structurally is refused.
	ImportMetadata(ctx context.Context, metadata []byte) (IdPDescriptor, error)
	// GenerateServiceKey creates the service provider's key pair for
	// encrypted assertions: a PEM private key and a self-signed PEM
	// certificate valid from now.
	GenerateServiceKey(random io.Reader, now time.Time, entityID string) (keyPEM, certPEM []byte, err error)
	// ServiceProviderMetadata returns the service provider's metadata
	// document, which an identity provider's administrator imports.
	ServiceProviderMetadata(ctx context.Context, cfg SAMLConfig, metadataURL string) ([]byte, error)
	// Start creates an AuthnRequest for the provider.
	Start(ctx context.Context, cfg SAMLConfig, relayState string, now time.Time) (SAMLStart, error)
	// Verify checks a response and returns the person it vouches for. It
	// requires a signature of the identity provider's on the response or on
	// the assertion, exactly one assertion, and the audience, recipient,
	// destination, issuer, request and validity-window checks; it refuses a
	// replayed assertion.
	Verify(ctx context.Context, v SAMLVerify) (Identity, error)
}

// NoSAML is the community edition's SAML: every call reports that the
// feature is not licensed (PLX-8600).
type NoSAML struct{}

var _ SAMLProvider = NoSAML{}

// notLicensed is the refusal of an enterprise feature the build lacks.
func notLicensed(f edition.Feature) error {
	return plxerr.New(plxerr.EnterpriseFeatureNotLicensed,
		"the enterprise feature %q is not enabled by this installation's licence", string(f))
}

// ImportMetadata reports that SAML is not licensed.
func (NoSAML) ImportMetadata(context.Context, []byte) (IdPDescriptor, error) {
	return IdPDescriptor{}, notLicensed(edition.FeatureSAML)
}

// GenerateServiceKey reports that SAML is not licensed.
func (NoSAML) GenerateServiceKey(io.Reader, time.Time, string) ([]byte, []byte, error) {
	return nil, nil, notLicensed(edition.FeatureSAML)
}

// ServiceProviderMetadata reports that SAML is not licensed.
func (NoSAML) ServiceProviderMetadata(context.Context, SAMLConfig, string) ([]byte, error) {
	return nil, notLicensed(edition.FeatureSAML)
}

// Start reports that SAML is not licensed.
func (NoSAML) Start(context.Context, SAMLConfig, string, time.Time) (SAMLStart, error) {
	return SAMLStart{}, notLicensed(edition.FeatureSAML)
}

// Verify reports that SAML is not licensed.
func (NoSAML) Verify(context.Context, SAMLVerify) (Identity, error) {
	return Identity{}, notLicensed(edition.FeatureSAML)
}
