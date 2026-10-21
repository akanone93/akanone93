// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package auditexport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/export"
	"github.com/nightCode42/plux3/ee/backend/internal/stdwebhook"
)

// The destination kinds this package serves. They are the kind names the
// core stores.
const (
	KindCEF     = "cef"
	KindOTLP    = "otlp"
	KindWebhook = "webhook"
)

// Provider builds the enterprise sinks. The enterprise binary registers it
// with the core as serverapp.Options.AuditSinks (SEC-143).
type Provider struct{}

// Kinds lists the destination kinds the provider serves.
func (Provider) Kinds() []string { return []string{KindCEF, KindOTLP, KindWebhook} }

// Validate says why a destination's address or signing secret is not valid
// for its kind. It opens no connection. CEF needs host:port; OTLP and webhook
// need an https URL, because audit records must not travel in clear text;
// a webhook needs a signing secret.
func (Provider) Validate(d export.Destination, secret []byte) error {
	switch d.Kind {
	case KindCEF:
		return validateHostPort(d.Address)
	case KindOTLP:
		return validateHTTPSURL(d.Address)
	case KindWebhook:
		if err := validateHTTPSURL(d.Address); err != nil {
			return err
		}
		_, err := stdwebhook.ParseSecret(string(secret))
		return err
	}
	return fmt.Errorf("auditexport: the kind %q is not served", d.Kind)
}

// New builds a sink for a destination of one of the served kinds. It checks
// the configuration the core must supply for the kind before it returns.
func (p Provider) New(_ context.Context, c export.Config) (export.Sink, error) {
	if err := p.Validate(c.Destination, c.Secret); err != nil {
		return nil, err
	}
	switch c.Destination.Kind {
	case KindCEF:
		return newCEFSink(c)
	case KindOTLP:
		return newOTLPSink(c)
	default:
		return newWebhookSink(c)
	}
}

// validateHostPort reports why an address is not host:port.
func validateHostPort(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return errors.New("the address is not host:port")
	}
	return nil
}

// validateHTTPSURL reports why an address is not an https URL with a host.
func validateHTTPSURL(address string) error {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return errors.New("the address is not a URL")
	}
	if u.Scheme != "https" {
		return errors.New("the address must be an https URL")
	}
	return nil
}

// requireLicence refuses a write of the SIEM feature unless the licence
// lists it for writing. The returned error carries PLX-8600 (GOV-030).
func requireLicence(ctx context.Context) error {
	if edition.EnabledForWrite(ctx, edition.FeatureSIEMExtended) {
		return nil
	}
	return edition.Require(ctx, edition.FeatureSIEMExtended)
}
