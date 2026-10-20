// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml

import (
	"context"
	"fmt"
	"net/url"
	"time"

	crewsaml "github.com/crewjam/saml"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/identity"
)

// Provider is SAML 2.0 single sign-on. It holds no state: everything a call
// needs, the provider's configuration included, is passed in, so one value
// serves every provider of the installation from any number of goroutines.
type Provider struct{}

var _ identity.SAMLProvider = Provider{}

// ImportMetadata reads an identity provider's metadata document and returns
// what Plux needs of it. The document must describe one provider that signs
// with a certificate and accepts a request through the HTTP-Redirect binding
// at an https address.
func (Provider) ImportMetadata(ctx context.Context, metadata []byte) (identity.IdPDescriptor, error) {
	if err := edition.Require(ctx, edition.FeatureSAML); err != nil {
		return identity.IdPDescriptor{}, err //nolint:wrapcheck // a domain error
	}
	d, err := readIdPMetadata(metadata)
	if err != nil {
		return identity.IdPDescriptor{}, fmt.Errorf("saml.ImportMetadata: %w", err)
	}
	return d, nil
}

// ServiceProviderMetadata returns the service provider's metadata document
// for cfg. metadataURL names the entity when cfg has no identifier of its
// own.
func (Provider) ServiceProviderMetadata(ctx context.Context, cfg identity.SAMLConfig, metadataURL string) ([]byte, error) {
	if err := edition.RequireRead(ctx, edition.FeatureSAML); err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	doc, err := writeServiceProviderMetadata(cfg, metadataURL)
	if err != nil {
		return nil, fmt.Errorf("saml.ServiceProviderMetadata: %w", err)
	}
	return doc, nil
}

// Start creates the AuthnRequest of a sign-in and the address of the identity
// provider's single sign-on endpoint that carries it. The request is not
// signed: the HTTP-Redirect binding carries it in the query, the response is
// what is signed, and the request's identifier is what binds the two.
func (Provider) Start(ctx context.Context, cfg identity.SAMLConfig, relayState string, now time.Time) (identity.SAMLStart, error) {
	if err := edition.RequireRead(ctx, edition.FeatureSAML); err != nil {
		return identity.SAMLStart{}, err //nolint:wrapcheck // a domain error
	}
	sp, err := serviceProvider(cfg, "")
	if err != nil {
		return identity.SAMLStart{}, fmt.Errorf("saml.Start: %w", err)
	}
	if err := requireHTTPS(cfg.IdP.SSOURL); err != nil {
		return identity.SAMLStart{}, fmt.Errorf("saml.Start: the single sign-on address: %w", err)
	}
	req, err := sp.MakeAuthenticationRequest(cfg.IdP.SSOURL, crewsaml.HTTPRedirectBinding, crewsaml.HTTPPostBinding)
	if err != nil {
		return identity.SAMLStart{}, fmt.Errorf("saml.Start: %w", err)
	}
	req.IssueInstant = now.UTC()
	target, err := req.Redirect(url.QueryEscape(relayState), sp)
	if err != nil {
		return identity.SAMLStart{}, fmt.Errorf("saml.Start: %w", err)
	}
	return identity.SAMLStart{RequestID: req.ID, URL: target.String()}, nil
}

// requireHTTPS refuses an address that is not an absolute https URL.
func requireHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%q is not a URL", raw)
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%q is not an https URL", raw)
	}
	return nil
}
