// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package httpx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"syscall"
	"time"
)

// Dialer opens connections for one registered destination. It refuses a
// host the destination does not list before the name is resolved, and an
// address the policy blocks after (SEC-104, SEC-163).
type Dialer struct {
	destination Destination
	dialer      *net.Dialer
}

// GuardedDialer returns a dialer for the destination kind that checks the
// address a connection will use, after the name has been resolved, against
// the policy. It is the guard of every outbound connection that is not
// HTTP, such as syslog over TLS (SEC-104), and of NewTransport. It fails
// for a kind the registry does not list.
func GuardedDialer(kind Kind, policy Policy) (*Dialer, error) {
	destination, err := Lookup(kind)
	if err != nil {
		return nil, err
	}
	return &Dialer{
		destination: destination,
		dialer: &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			// Control runs after the name has been resolved, with the
			// address the connection will actually use.
			Control: func(_, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return fmt.Errorf("%w: %s", ErrBlocked, address)
				}
				addr, err := netip.ParseAddr(host)
				if err != nil {
					return fmt.Errorf("%w: %s is not an IP address", ErrBlocked, host)
				}
				return policy.Check(addr)
			},
		},
	}, nil
}

// DialContext connects to address, which is host:port, the way
// net.Dialer.DialContext does.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrBlocked, address)
	}
	if !d.destination.allows(host) {
		return nil, fmt.Errorf("%w: %s is not a host of %s", ErrBlocked, host, d.destination.Kind)
	}
	conn, err := d.dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, fmt.Errorf("httpx: %w", err)
	}
	return conn, nil
}

// PrivateNetworksPolicy is the policy of a destination that may be on a
// private network: loopback and the RFC 1918 and unique-local ranges are
// allowed, and link-local, metadata, multicast and reserved ranges stay
// refused. It exists for development and for installations that run their
// own services privately, and callers report it with a warning.
func PrivateNetworksPolicy() Policy {
	return Policy{
		AllowLoopback: true,
		Allow: []netip.Prefix{
			netip.MustParsePrefix("10.0.0.0/8"),
			netip.MustParsePrefix("172.16.0.0/12"),
			netip.MustParsePrefix("192.168.0.0/16"),
			netip.MustParsePrefix("fc00::/7"),
		},
	}
}

// CheckHostPort refuses an address that is not host:port, and a host that
// is an IP literal the policy blocks. A name is accepted here and checked
// by the dialer once it resolves; this is the cheap check made when a
// destination is configured.
func CheckHostPort(policy Policy, address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return fmt.Errorf("%w: %q is not host:port", ErrBlocked, address)
	}
	if n, err := net.LookupPort("tcp", port); err != nil || n == 0 {
		return fmt.Errorf("%w: %q is not a port", ErrBlocked, port)
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return policy.Check(addr)
	}
	return nil
}

// ClientTLSOptions configures ClientTLS.
type ClientTLSOptions struct {
	// ServerName is the name the server's certificate must carry.
	ServerName string
	// RootsPEM pins the roots that verify the server; empty uses the
	// system roots.
	RootsPEM []byte
	// Certificate is the client certificate for mutual TLS, or nil.
	Certificate *tls.Certificate
}

// ClientTLS returns the TLS configuration of an outbound connection that
// verifies the server: TLS 1.2 or later, with the AEAD ECDHE suites for
// TLS 1.2, and the optional client certificate (SEC-040).
func ClientTLS(o ClientTLSOptions) (*tls.Config, error) {
	c := &tls.Config{
		ServerName:   o.ServerName,
		MinVersion:   tls.VersionTLS12,
		CipherSuites: slices.Clone(tls12Suites),
	}
	if len(o.RootsPEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(o.RootsPEM) {
			return nil, errors.New("httpx: the pinned roots hold no certificate")
		}
		c.RootCAs = pool
	}
	if o.Certificate != nil {
		c.Certificates = []tls.Certificate{*o.Certificate}
	}
	return c, nil
}
