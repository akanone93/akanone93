// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"crypto/x509"
	"net/netip"
	"strconv"
	"strings"
)

// Federation is how the server reaches the identity providers people sign in
// with (SEC-100, GOV-004). By default it reaches public addresses only and
// trusts the system's certificate authorities (SEC-105).
type Federation struct {
	// AllowPrivateNetworks lists private address ranges, as CIDR prefixes,
	// where an identity provider may be reached: an on-premises provider on
	// an internal network. Empty refuses them all; every start with ranges
	// set logs a warning.
	AllowPrivateNetworks []string `json:"allowPrivateNetworks"`
	// TrustedCAs is a PEM bundle of certificate authorities trusted for
	// identity providers beside the system's: a provider whose certificate
	// an internal authority issued. Empty adds none; every start with one
	// set logs a warning.
	TrustedCAs string `json:"trustedCAs"`
}

// PrivateNetworks returns the allowed ranges; validation has checked them.
func (f Federation) PrivateNetworks() []netip.Prefix {
	out := make([]netip.Prefix, 0, len(f.AllowPrivateNetworks))
	for _, cidr := range f.AllowPrivateNetworks {
		if p, err := netip.ParsePrefix(cidr); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// CAPool returns the system's certificate authorities and the trusted ones,
// or nil when none are added.
func (f Federation) CAPool() *x509.CertPool {
	if f.TrustedCAs == "" {
		return nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	pool.AppendCertsFromPEM([]byte(f.TrustedCAs))
	return pool
}

// validateFederation checks the ranges and the certificate bundle.
func (c *Config) validateFederation(p *problems) {
	f := c.Auth.Federation
	for i, cidr := range f.AllowPrivateNetworks {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			p.addf("auth.federation.allowPrivateNetworks["+strconv.Itoa(i)+"]", "%q is not a CIDR prefix such as 10.0.0.0/8", cidr)
		}
	}
	if f.TrustedCAs != "" && !x509.NewCertPool().AppendCertsFromPEM([]byte(f.TrustedCAs)) {
		p.addf("auth.federation.trustedCAs", "holds no PEM certificate")
	}
}

// federationWarnings lists the federation settings that are valid but
// widen what the server reaches or trusts.
func (c *Config) federationWarnings() []string {
	f := c.Auth.Federation
	var w []string
	if len(f.AllowPrivateNetworks) > 0 {
		w = append(w, "auth.federation.allowPrivateNetworks is set: identity providers may be reached on "+strings.Join(f.AllowPrivateNetworks, ", ")+", which the server otherwise refuses (SEC-105)")
	}
	if f.TrustedCAs != "" {
		w = append(w, "auth.federation.trustedCAs is set: identity providers' certificates may be issued by authorities beside the system's")
	}
	return w
}
