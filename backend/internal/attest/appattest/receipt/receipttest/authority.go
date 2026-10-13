// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package receipttest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/cryptobyte"
	cbasn1 "golang.org/x/crypto/cryptobyte/asn1"
)

// Encoding selects how a receipt's CMS structure is encoded.
type Encoding int

const (
	// DER uses definite lengths throughout.
	DER Encoding = iota
	// BERIndefinite uses indefinite lengths and a constructed content
	// OCTET STRING, as some CMS producers do.
	BERIndefinite
)

// KeyForm selects how the attested key is written into a receipt, which
// Apple's documentation leaves open.
type KeyForm int

const (
	// KeyPoint is the uncompressed X9.62 point.
	KeyPoint KeyForm = iota
	// KeySPKI is a DER SubjectPublicKeyInfo.
	KeySPKI
	// KeyCertificate is a DER certificate for the key.
	KeyCertificate
)

// IssueOptions describes one receipt. The zero value of a field means its
// default.
type IssueOptions struct {
	// AppID is the application identifier, "TEAMID.com.example.app".
	AppID string
	// Key is the attested key.
	Key *ecdsa.PublicKey
	// KeyForm is how Key is written.
	KeyForm KeyForm
	// Type is "RECEIPT" by default; "ATTEST" makes an attestation receipt.
	Type string
	// Risk is the risk metric.
	Risk int
	// RiskAsInteger writes the metric as a DER INTEGER rather than ASCII
	// digits.
	RiskAsInteger bool
	// OmitRisk leaves the risk metric out.
	OmitRisk bool
	// Created, NotBefore and Expiration default to the authority's now,
	// one hour earlier and 30 days later.
	Created, NotBefore, Expiration time.Time
	// Encoding is the CMS encoding.
	Encoding Encoding
	// TamperSignature flips a bit in the CMS signature.
	TamperSignature bool
	// TamperContent changes the payload after it was signed.
	TamperContent bool
}

// Authority is a test certificate chain (root, intermediate, signer) that
// issues receipts.
type Authority struct {
	now      time.Time
	root     *x509.Certificate
	inter    *x509.Certificate
	leaf     *x509.Certificate
	interKey *ecdsa.PrivateKey
	leafKey  *ecdsa.PrivateKey

	mu     sync.Mutex
	serial int64
}

// NewAuthority generates a chain valid around now.
func NewAuthority(t testing.TB, now time.Time) *Authority {
	t.Helper()
	a, err := newAuthority(now)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func newAuthority(now time.Time) (*Authority, error) {
	a := &Authority{now: now, serial: 1}
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err //nolint:wrapcheck // a test helper
	}
	root, err := a.certificate("Receipt Test Root", nil, nil, &rootKey.PublicKey, rootKey, true)
	if err != nil {
		return nil, err
	}
	if a.interKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
		return nil, err //nolint:wrapcheck // a test helper
	}
	if a.inter, err = a.certificate("Receipt Test Intermediate", root, rootKey, &a.interKey.PublicKey, nil, true); err != nil {
		return nil, err
	}
	if a.leafKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
		return nil, err //nolint:wrapcheck // a test helper
	}
	if a.leaf, err = a.certificate("Receipt Test Signer", a.inter, a.interKey, &a.leafKey.PublicKey, nil, false); err != nil {
		return nil, err
	}
	a.root = root
	return a, nil
}

// certificate issues a certificate for pub, signed by parent with signer,
// or self-signed with self when parent is nil.
func (a *Authority) certificate(name string, parent *x509.Certificate, signer *ecdsa.PrivateKey, pub *ecdsa.PublicKey, self *ecdsa.PrivateKey, ca bool) (*x509.Certificate, error) {
	a.mu.Lock()
	a.serial++
	serial := a.serial
	a.mu.Unlock()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name},
		NotBefore: a.now.Add(-24 * time.Hour), NotAfter: a.now.Add(10 * 365 * 24 * time.Hour),
		BasicConstraintsValid: true, IsCA: ca,
	}
	if ca {
		tmpl.KeyUsage = x509.KeyUsageCertSign
	} else {
		tmpl.KeyUsage = x509.KeyUsageDigitalSignature
	}
	if parent == nil {
		parent, signer = tmpl, self
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, signer)
	if err != nil {
		return nil, err //nolint:wrapcheck // a test helper
	}
	return x509.ParseCertificate(der) //nolint:wrapcheck // a test helper
}

// Roots returns a pool holding the authority's root.
func (a *Authority) Roots() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(a.root)
	return p
}

// Issue builds a receipt and fails the test if it cannot.
func (a *Authority) Issue(t testing.TB, o IssueOptions) []byte {
	t.Helper()
	out, err := a.Build(o)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Build builds a receipt. The authority's clock decides the defaults of
// the validity window.
func (a *Authority) Build(o IssueOptions) ([]byte, error) {
	if o.Type == "" {
		o.Type = "RECEIPT"
	}
	if o.Created.IsZero() {
		o.Created = a.now
	}
	if o.NotBefore.IsZero() {
		o.NotBefore = a.now.Add(-time.Hour)
	}
	if o.Expiration.IsZero() {
		o.Expiration = a.now.Add(30 * 24 * time.Hour)
	}
	content, err := a.payload(o)
	if err != nil {
		return nil, err
	}
	return a.sign(content, o)
}

// field is one typed attribute of the payload.
type field struct {
	typ   int64
	value []byte
}

// payload encodes the SET of typed fields.
func (a *Authority) payload(o IssueOptions) ([]byte, error) {
	key, err := a.keyValue(o)
	if err != nil {
		return nil, err
	}
	fields := []field{
		{2, []byte(o.AppID)},
		{3, key},
		{4, []byte("client-hash")},
		{5, []byte("token")},
		{6, []byte(o.Type)},
		{12, []byte(o.Created.UTC().Format(time.RFC3339))},
		{19, []byte(o.NotBefore.UTC().Format(time.RFC3339))},
		{21, []byte(o.Expiration.UTC().Format(time.RFC3339))},
	}
	if !o.OmitRisk {
		fields = append(fields, field{17, riskValue(o)})
	}
	var b cryptobyte.Builder
	b.AddASN1(cbasn1.SET, func(set *cryptobyte.Builder) {
		for _, f := range fields {
			set.AddASN1(cbasn1.SEQUENCE, func(seq *cryptobyte.Builder) {
				seq.AddASN1Int64(f.typ)
				seq.AddASN1Int64(1)
				seq.AddASN1OctetString(f.value)
			})
		}
	})
	return b.Bytes() //nolint:wrapcheck // a test helper
}

func riskValue(o IssueOptions) []byte {
	if !o.RiskAsInteger {
		return []byte(strconv.Itoa(o.Risk))
	}
	var b cryptobyte.Builder
	b.AddASN1Int64(int64(o.Risk))
	out, _ := b.Bytes()
	return out
}

func (a *Authority) keyValue(o IssueOptions) ([]byte, error) {
	switch o.KeyForm {
	case KeySPKI:
		return x509.MarshalPKIXPublicKey(o.Key) //nolint:wrapcheck // a test helper
	case KeyCertificate:
		c, err := a.certificate("Receipt Test Credential", a.inter, a.interKey, o.Key, nil, false)
		if err != nil {
			return nil, err
		}
		return c.Raw, nil
	default:
		return o.Key.Bytes() //nolint:wrapcheck // a test helper
	}
}

// Object identifiers written into the CMS structure, pre-encoded as
// complete OBJECT IDENTIFIER elements.
var (
	oidSignedData    = []byte{0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x07, 0x02}
	oidData          = []byte{0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x07, 0x01}
	oidContentType   = []byte{0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x09, 0x03}
	oidMessageDigest = []byte{0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x09, 0x04}
	oidSHA256        = []byte{0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01}
	oidECDSASHA256   = []byte{0x06, 0x08, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x04, 0x03, 0x02}
)

// sign wraps content in a CMS SignedData signed by the authority's signer
// certificate.
func (a *Authority) sign(content []byte, o IssueOptions) ([]byte, error) {
	digest := sha256.Sum256(content)
	attrs := concat(
		seq(false, oidContentType, set(false, oidData)),
		seq(false, oidMessageDigest, set(false, octets(digest[:]))),
	)
	signedAttrs := tlv(0x31, attrs)
	h := sha256.Sum256(signedAttrs)
	sig, err := ecdsa.SignASN1(rand.Reader, a.leafKey, h[:])
	if err != nil {
		return nil, err //nolint:wrapcheck // a test helper
	}
	if o.TamperSignature {
		sig[len(sig)-1] ^= 0x01
	}
	if o.TamperContent {
		content = append([]byte(nil), content...)
		content[len(content)-1] ^= 0x01
	}
	ber := o.Encoding == BERIndefinite
	null := []byte{0x05, 0x00}
	digestAlg := seq(false, oidSHA256, null)
	issuerSerial := seq(false, a.leaf.RawIssuer, integer(a.leaf.SerialNumber))
	signerInfo := seq(ber, []byte{0x02, 0x01, 0x01}, issuerSerial, digestAlg,
		append([]byte{0xa0}, signedAttrs[1:]...), seq(false, oidECDSASHA256), octets(sig))
	certs := cons(ber, 0xa0, a.inter.Raw, a.leaf.Raw)
	body := seq(ber,
		[]byte{0x02, 0x01, 0x01},
		set(ber, digestAlg),
		seq(ber, oidData, cons(ber, 0xa0, contentOctets(ber, content))),
		certs,
		set(ber, signerInfo),
	)
	return seq(ber, oidSignedData, cons(ber, 0xa0, body)), nil
}

// contentOctets encodes the embedded content: one primitive OCTET STRING,
// or in BER a constructed one in two chunks.
func contentOctets(ber bool, content []byte) []byte {
	if !ber {
		return octets(content)
	}
	half := len(content) / 2
	return cons(true, 0x24, octets(content[:half]), octets(content[half:]))
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func seq(ber bool, parts ...[]byte) []byte { return cons(ber, 0x30, parts...) }
func set(ber bool, parts ...[]byte) []byte { return cons(ber, 0x31, parts...) }
func octets(b []byte) []byte               { return tlv(0x04, b) }

// cons encodes a constructed element with definite or indefinite length.
func cons(ber bool, tag byte, parts ...[]byte) []byte {
	body := concat(parts...)
	if !ber {
		return tlv(tag, body)
	}
	out := append([]byte{tag, 0x80}, body...)
	return append(out, 0x00, 0x00)
}

// tlv encodes an element with a definite length.
func tlv(tag byte, body []byte) []byte {
	out := []byte{tag}
	n := len(body)
	switch {
	case n < 0x80:
		out = append(out, byte(n))
	case n < 0x100:
		out = append(out, 0x81, byte(n))
	case n < 0x10000:
		out = append(out, 0x82, byte(n>>8), byte(n))
	default:
		out = append(out, 0x83, byte(n>>16), byte(n>>8), byte(n))
	}
	return append(out, body...)
}

func integer(n *big.Int) []byte {
	var b cryptobyte.Builder
	b.AddASN1BigInt(n)
	out, _ := b.Bytes()
	return out
}
