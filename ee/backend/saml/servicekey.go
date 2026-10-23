// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"time"
)

// The service provider's key pair: an RSA key, because XML encryption wraps
// its content key with RSA-OAEP, and a certificate valid for ten years, which
// is how long an identity provider's administrator is not asked to import
// the service provider's metadata again.
const (
	serviceKeyBits     = 3072
	serviceKeyValidity = 10 * 365 * 24 * time.Hour
)

// GenerateServiceKey creates the service provider's key pair for encrypted
// assertions: a PKCS#8 PEM private key and a self-signed PEM certificate
// valid from now. random is the source of the key and the serial number.
func (Provider) GenerateServiceKey(random io.Reader, now time.Time, entityID string) (keyPEM, certPEM []byte, err error) {
	key, err := rsa.GenerateKey(random, serviceKeyBits)
	if err != nil {
		return nil, nil, fmt.Errorf("saml.GenerateServiceKey: %w", err)
	}
	serial, err := rand.Int(random, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, nil, fmt.Errorf("saml.GenerateServiceKey: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "Plux SAML service provider"},
		NotBefore:    now.UTC(),
		NotAfter:     now.UTC().Add(serviceKeyValidity),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}
	if u, err := url.Parse(entityID); err == nil && u.IsAbs() {
		template.URIs = []*url.URL{u}
	}
	der, err := x509.CreateCertificate(random, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("saml.GenerateServiceKey: %w", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("saml.GenerateServiceKey: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}
