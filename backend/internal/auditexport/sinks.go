// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auditexport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/export"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

const (
	// connectTimeout bounds a TLS handshake and each write of a stream.
	connectTimeout = 30 * time.Second
	// requestTimeout bounds one HTTP delivery.
	requestTimeout = 30 * time.Second
)

// opener validates destinations and builds their sinks. The core's syslog
// sink is built here; every other kind is the provider's, and only while
// the licence lists FeatureSIEMExtended (GOV-030).
type opener struct {
	crypter  signing.Crypter
	provider export.Provider
	hostname string
	version  string
	now      func() time.Time
}

// policyOf is the address policy of a destination: public addresses only,
// unless it was allowed private ones.
func policyOf(allowPrivate bool) httpx.Policy {
	if allowPrivate {
		return httpx.PrivateNetworksPolicy()
	}
	return httpx.Policy{}
}

// providerKind reports whether the provider serves a kind.
func (o opener) providerKind(kind string) bool {
	return o.provider != nil && slices.Contains(o.provider.Kinds(), kind)
}

// validate reports why a destination cannot be registered or changed to
// these settings. It opens no connection.
func (o opener) validate(ctx context.Context, kind, address string, allowPrivate bool, caPEM string, creds Credentials) error {
	if kind != KindSyslog {
		if err := o.requireProvided(ctx, kind); err != nil {
			return err
		}
	}
	if err := creds.check(kind); err != nil {
		return err
	}
	if caPEM != "" {
		if _, err := httpx.ClientTLS(httpx.ClientTLSOptions{RootsPEM: []byte(caPEM)}); err != nil {
			return plxerr.New(plxerr.InvalidFormat, "the pinned roots hold no certificate")
		}
	}
	if err := checkAddress(kind, address, policyOf(allowPrivate)); err != nil {
		return err
	}
	if kind == KindSyslog {
		return nil
	}
	d := export.Destination{Kind: kind, Address: address}
	if err := o.provider.Validate(d, []byte(creds.SigningSecret)); err != nil {
		return plxerr.New(plxerr.InvalidFormat, "the %s destination is not valid: %v", kind, err)
	}
	return nil
}

// requireProvided refuses a provided kind (CEF, OTLP, webhook) unless the
// server registered a provider for it and the licence lists
// FeatureSIEMExtended for writing. The open-source core has no such sink, so
// the refusal is PLX-8600 (SEC-143, GOV-030).
func (o opener) requireProvided(ctx context.Context, kind string) error {
	if !o.providerKind(kind) {
		return plxerr.New(plxerr.EnterpriseFeatureNotLicensed, "the %q destination needs the enterprise edition", kind)
	}
	if err := edition.Require(ctx, edition.FeatureSIEMExtended); err != nil {
		return fmt.Errorf("auditexport: %w", err)
	}
	return nil
}

// checkAddress refuses an address of the wrong form, and one that names a
// blocked IP address. A host name is checked by the dialer when it resolves.
func checkAddress(kind, address string, policy httpx.Policy) error {
	if address == "" {
		return plxerr.New(plxerr.MissingProperty, "a destination needs an address")
	}
	if kind == KindSyslog {
		if err := httpx.CheckHostPort(policy, address); err != nil {
			return blocked(err)
		}
		return nil
	}
	if !strings.Contains(address, "://") {
		if err := httpx.CheckHostPort(policy, address); err != nil {
			return blocked(err)
		}
		return nil
	}
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return plxerr.New(plxerr.InvalidFormat, "the address is not a URL")
	}
	if err := httpx.CheckURL(u.Scheme, u.Hostname()); err != nil {
		return blocked(err)
	}
	if addr, err := netip.ParseAddr(u.Hostname()); err == nil {
		if err := policy.Check(addr); err != nil {
			return blocked(err)
		}
	}
	return nil
}

// blocked wraps a refusal of the egress guard.
func blocked(err error) error {
	return plxerr.Wrap(plxerr.OutboundRequestBlocked, err, "the destination address is refused")
}

// hostOf returns the host of an address that is host:port or a URL.
func hostOf(address string) (string, error) {
	if strings.Contains(address, "://") {
		u, err := url.Parse(address)
		if err != nil || u.Hostname() == "" {
			return "", errors.New("the address is not a URL")
		}
		return u.Hostname(), nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return "", errors.New("the address is not host:port")
	}
	return host, nil
}

// open builds the sink of a stored destination. The returned function
// releases what the sink holds beyond its own Close; call it when the run
// is over.
func (o opener) open(ctx context.Context, row dbgen.AuditExportDestination) (export.Sink, func(), error) {
	creds, err := o.credentialsOf(ctx, row)
	if err != nil {
		return nil, nil, err
	}
	host, err := hostOf(row.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("auditexport: %w", err)
	}
	opts := httpx.ClientTLSOptions{RootsPEM: []byte(row.CaPem)}
	if creds.ClientCertificatePEM != "" {
		pair, err := tls.X509KeyPair([]byte(creds.ClientCertificatePEM), []byte(creds.ClientKeyPEM))
		if err != nil {
			return nil, nil, errors.New("auditexport: the stored client certificate and key are not a valid pair")
		}
		opts.Certificate = &pair
	}
	policy := policyOf(row.AllowPrivate)
	dial := httpx.GuardedDialer(policy).DialContext
	if row.Kind == KindSyslog {
		opts.ServerName = host
		cfg, err := httpx.ClientTLS(opts)
		if err != nil {
			return nil, nil, fmt.Errorf("auditexport: %w", err)
		}
		stream, err := export.NewStream(row.Address, dial, cfg, connectTimeout)
		if err != nil {
			return nil, nil, fmt.Errorf("auditexport: %w", err)
		}
		return newSyslogSink(stream, o.hostname), func() {}, nil
	}
	return o.openProvided(ctx, row, creds, opts, policy, dial)
}

// openProvided builds a sink of the enterprise provider. It needs a licence
// that may still read the feature, so a lapsed licence stops the delivery
// without losing the cursor.
func (o opener) openProvided(ctx context.Context, row dbgen.AuditExportDestination, creds Credentials, opts httpx.ClientTLSOptions, policy httpx.Policy, dial export.DialFunc) (export.Sink, func(), error) {
	if err := edition.RequireRead(ctx, edition.FeatureSIEMExtended); err != nil {
		return nil, nil, fmt.Errorf("auditexport: %w", err)
	}
	if !o.providerKind(row.Kind) {
		return nil, nil, plxerr.New(plxerr.InvalidFormat, "%q is not a destination kind this server offers", row.Kind)
	}
	host, err := hostOf(row.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("auditexport: %w", err)
	}
	if !strings.Contains(row.Address, "://") {
		opts.ServerName = host
	}
	cfg, err := httpx.ClientTLS(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("auditexport: %w", err)
	}
	transport := httpx.NewTransport(httpx.TransportOptions{Policy: policy, TLS: cfg, ResponseHeaderTimeout: requestTimeout})
	client := &http.Client{
		Transport: transport,
		Timeout:   requestTimeout,
		// A redirect would leave the destination the administrator named.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	sink, err := o.provider.New(ctx, export.Config{
		Destination: export.Destination{
			ID: storage.ID(row.ID), OrganizationID: storage.ID(row.OrganizationID), Name: row.Name, Kind: row.Kind, Address: row.Address,
		},
		Hostname: o.hostname, Version: o.version, Secret: []byte(creds.SigningSecret), TLS: cfg, Dial: dial, HTTP: client, Now: o.now,
	})
	if err != nil {
		transport.CloseIdleConnections()
		return nil, nil, fmt.Errorf("auditexport: open the %s sink: %w", row.Kind, err)
	}
	return sink, transport.CloseIdleConnections, nil
}

// credentialsOf opens a destination's sealed credentials; the zero value
// when it has none.
func (o opener) credentialsOf(ctx context.Context, row dbgen.AuditExportDestination) (Credentials, error) {
	if len(row.CredentialsCiphertext) == 0 {
		return Credentials{}, nil
	}
	plain, err := signing.Open(ctx, o.crypter, signing.Sealed{
		Ciphertext: row.CredentialsCiphertext, WrappedKey: row.CredentialsWrappedKey, KeyID: row.CredentialsKeyID,
	}, bindingOf(storage.ID(row.ID)))
	if err != nil {
		return Credentials{}, fmt.Errorf("auditexport: open the credentials: %w", err)
	}
	return unmarshalCredentials(plain)
}

// closeSink closes a sink that holds a connection.
func closeSink(s export.Sink) error {
	if c, ok := s.(io.Closer); ok {
		if err := c.Close(); err != nil {
			return fmt.Errorf("auditexport: close the sink: %w", err)
		}
	}
	return nil
}
