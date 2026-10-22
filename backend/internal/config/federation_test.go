// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"
)

// Verifies: SEC-105.
// Identity providers on private networks and private authorities are refused
// unless named, and naming them is never silent.
func TestFederationAllowlist_SEC_105(t *testing.T) {
	c := parse(t, withServer(""))
	if c.Auth.Federation.CAPool() != nil || len(c.Auth.Federation.PrivateNetworks()) != 0 {
		t.Error("the defaults must add no range and no authority")
	}
	if slices.ContainsFunc(c.Warnings(), func(w string) bool { return strings.Contains(w, "auth.federation") }) {
		t.Errorf("warnings for the defaults: %v", c.Warnings())
	}
	c = parse(t, withServer("")+"auth:\n  federation:\n    allowPrivateNetworks: [172.18.0.0/16]\n    trustedCAs: |\n"+indent(testCAPEM(t), "      "))
	if got := c.Auth.Federation.PrivateNetworks(); !slices.Equal(got, []netip.Prefix{netip.MustParsePrefix("172.18.0.0/16")}) {
		t.Errorf("ranges = %v", got)
	}
	if c.Auth.Federation.CAPool() == nil {
		t.Error("the trusted authority was not loaded")
	}
	var named int
	for _, w := range c.Warnings() {
		if strings.Contains(w, "auth.federation.allowPrivateNetworks") && strings.Contains(w, "172.18.0.0/16") ||
			strings.Contains(w, "auth.federation.trustedCAs") {
			named++
		}
	}
	if named != 2 {
		t.Errorf("warnings = %v; want one for the ranges and one for the authorities", c.Warnings())
	}
	mentions(t, []struct{ name, src, want string }{
		{"not a range", withServer("") + "auth:\n  federation:\n    allowPrivateNetworks: [intranet]\n", "auth.federation.allowPrivateNetworks[0]"},
		{"not a certificate", withServer("") + "auth:\n  federation:\n    trustedCAs: not a certificate\n", "auth.federation.trustedCAs"},
	})
}

// indent prefixes every line of s.
func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n") + "\n"
}

// testCAPEM returns a self-signed authority made for the test.
func testCAPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
