// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"time"
)

// serverTLS makes a certificate authority for the run and a server
// certificate it signs for the given host names and addresses. It returns the
// server's TLS configuration and the authority's PEM, which is the only trust
// anchor the installation is given for the walkthrough's services.
func serverTLS(hosts []string, now func() time.Time, random io.Reader) (*tls.Config, []byte, error) {
	ca, caKey, err := newAuthority(now, random)
	if err != nil {
		return nil, nil, err
	}
	leaf, err := newLeaf(ca, caKey, hosts, now, random)
	if err != nil {
		return nil, nil, err
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{leaf}, MinVersion: tls.VersionTLS12}
	return cfg, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), nil
}

// newAuthority makes the run's certificate authority.
func newAuthority(now func() time.Time, random io.Reader) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), random)
	if err != nil {
		return nil, nil, fmt.Errorf("walkthrough.serverTLS: %w", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "plux walkthrough syslog CA"},
		NotBefore: now().Add(-time.Hour), NotAfter: now().Add(48 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(random, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("walkthrough.serverTLS: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf("walkthrough.serverTLS: %w", err)
	}
	return cert, key, nil
}

// newLeaf makes the server certificate the authority signs.
func newLeaf(ca *x509.Certificate, caKey *ecdsa.PrivateKey, hosts []string, now func() time.Time, random io.Reader) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), random)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("walkthrough.serverTLS: %w", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "plux walkthrough syslog sink"},
		NotBefore: now().Add(-time.Hour), NotAfter: now().Add(48 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(random, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("walkthrough.serverTLS: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}
