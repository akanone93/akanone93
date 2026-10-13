// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package receipt

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"golang.org/x/crypto/cryptobyte"
	cbasn1 "golang.org/x/crypto/cryptobyte/asn1"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// The field types of the receipt payload (Apple, "Assessing fraud risk").
const (
	fieldAppID      = 2
	fieldPublicKey  = 3
	fieldClientHash = 4
	fieldToken      = 5
	fieldType       = 6
	fieldCreated    = 12
	fieldRisk       = 17
	fieldNotBefore  = 19
	fieldExpiration = 21

	// maxFields bounds the attributes of one payload.
	maxFields = 64
	// maxRisk keeps a risk metric within the int32 the device record
	// stores.
	maxRisk = 1<<31 - 1
)

// Type is the kind of a receipt.
type Type string

const (
	// TypeAttest is the receipt issued with an attestation. It carries no
	// risk metric and is only a request token for a RECEIPT.
	TypeAttest Type = "ATTEST"
	// TypeReceipt is a receipt that carries the risk metric.
	TypeReceipt Type = "RECEIPT"
)

// Receipt is a verified fraud-metric receipt. Raw is sensitive: it is
// stored for the next refresh and never logged (SEC-092).
type Receipt struct {
	// Raw is the receipt as Apple issued it.
	Raw []byte
	// RiskMetric is the number of attested keys Apple has seen on the
	// device recently; a low number is good.
	RiskMetric int
	// CreatedAt is when Apple issued the receipt; zero when it is not
	// stated.
	CreatedAt time.Time
	// NotBefore and Expiration bound the receipt's validity.
	NotBefore, Expiration time.Time
}

// LogValue logs a receipt without its bytes.
func (r Receipt) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("riskMetric", r.RiskMetric), slog.Time("expiration", r.Expiration))
}

// Expect is what a receipt must say about the app and the device it is
// for. Both fields are required.
type Expect struct {
	// AppID is the team and bundle identifier, "TEAMID.com.example.app".
	AppID string
	// Key is the App Attest key the server stored for the device.
	Key *ecdsa.PublicKey
}

// Verifier opens receipts. It is safe for concurrent use as long as its
// fields are not modified.
type Verifier struct {
	// Roots anchors the signer certificate chain; it must not be nil.
	Roots *x509.CertPool
	// Now returns the time at which certificates and the receipt must be
	// valid.
	Now func() time.Time
	// MaxBytes bounds a receipt before it is parsed; zero means the
	// registry default, attest.appAttestObjectBytes (LIM-001).
	MaxBytes int64
}

// unavailable is the failure of every check of this package: the device
// has no usable receipt (PLX-6080). The message names the check only.
func unavailable(format string, args ...any) error {
	return plxerr.New(plxerr.FraudReceiptUnavailable, "fraud receipt: "+format, args...)
}

// maxBytes resolves the configured input bound.
func maxBytes(configured int64) int64 {
	if configured > 0 {
		return configured
	}
	def, _ := limits.Lookup(limits.AttestAppAttestObjectBytes)
	return def.Default
}

// Verify opens a receipt: the CMS signature and the chain of its signer to
// the roots, then the payload against want and the clock. It accepts only
// a RECEIPT with a risk metric (SEC-007). Any failure is PLX-6080.
func (v *Verifier) Verify(raw []byte, want Expect) (Receipt, error) {
	if want.AppID == "" || want.Key == nil {
		return Receipt{}, unavailable("the app and the attested key to expect are required")
	}
	if v.Roots == nil || v.Now == nil {
		return Receipt{}, unavailable("the verifier has no roots or clock")
	}
	der, err := normaliseBER(raw, maxBytes(v.MaxBytes))
	if err != nil {
		return Receipt{}, unavailable("%v", err)
	}
	sd, err := parseSignedData(der)
	if err != nil {
		return Receipt{}, unavailable("%v", err)
	}
	now := v.Now()
	if err := verifyCMS(sd, v.Roots, now); err != nil {
		return Receipt{}, unavailable("%v", err)
	}
	p, err := parsePayload(sd.content)
	if err != nil {
		return Receipt{}, unavailable("%v", err)
	}
	if err := p.check(want, now); err != nil {
		return Receipt{}, err
	}
	return Receipt{
		Raw: bytes.Clone(raw), RiskMetric: p.risk, CreatedAt: p.created, NotBefore: p.notBefore, Expiration: p.expiration,
	}, nil
}

// payload is the decoded content of a receipt.
type payload struct {
	appID, publicKey      []byte
	kind                  Type
	risk                  int
	hasRisk               bool
	created               time.Time
	notBefore, expiration time.Time
}

// parsePayload decodes the attribute set of a receipt. The content may be
// BER too, so it is normalised first.
func parsePayload(content []byte) (payload, error) {
	der, err := normaliseBER(content, int64(len(content)))
	if err != nil {
		return payload{}, err
	}
	in := cryptobyte.String(der)
	var set cryptobyte.String
	if !(in.ReadASN1(&set, cbasn1.SET) || in.ReadASN1(&set, cbasn1.SEQUENCE)) || !in.Empty() {
		return payload{}, errMalformed
	}
	fields := map[int64][]byte{}
	for !set.Empty() {
		var attr, value cryptobyte.String
		var typ, version int64
		if !set.ReadASN1(&attr, cbasn1.SEQUENCE) || !attr.ReadASN1Integer(&typ) || !attr.ReadASN1Integer(&version) ||
			!attr.ReadASN1(&value, cbasn1.OCTET_STRING) || !attr.Empty() || len(fields) >= maxFields {
			return payload{}, errors.New("the receipt payload is malformed")
		}
		if _, dup := fields[typ]; dup {
			return payload{}, errors.New("the receipt payload repeats a field")
		}
		fields[typ] = value
	}
	return payloadOf(fields)
}

// payloadOf interprets the fields the server relies on.
func payloadOf(fields map[int64][]byte) (payload, error) {
	var p payload
	var err error
	p.appID = fields[fieldAppID]
	p.publicKey = fields[fieldPublicKey]
	p.kind = Type(textValue(fields[fieldType]))
	if raw, ok := fields[fieldRisk]; ok {
		if p.risk, err = parseRisk(raw); err != nil {
			return payload{}, err
		}
		p.hasRisk = true
	}
	for _, t := range []struct {
		field int64
		dst   *time.Time
	}{{fieldCreated, &p.created}, {fieldNotBefore, &p.notBefore}, {fieldExpiration, &p.expiration}} {
		raw, ok := fields[t.field]
		if !ok {
			continue
		}
		if *t.dst, err = time.Parse(time.RFC3339, textValue(raw)); err != nil {
			return payload{}, errors.New("a time in the receipt is not RFC 3339")
		}
	}
	return p, nil
}

// check compares the payload with what the caller expects.
func (p payload) check(want Expect, now time.Time) error {
	switch {
	case textValue(p.appID) != want.AppID:
		return unavailable("the receipt is for another app")
	case !keyMatches(p.publicKey, want.Key):
		return unavailable("the receipt is for another key")
	case p.kind != TypeReceipt:
		return unavailable("the receipt is not of type RECEIPT")
	case !p.hasRisk:
		return unavailable("the receipt carries no risk metric")
	case p.notBefore.IsZero() || p.expiration.IsZero():
		return unavailable("the receipt states no validity")
	case now.Before(p.notBefore):
		return unavailable("the receipt is not valid yet")
	case !now.Before(p.expiration):
		return unavailable("the receipt has expired")
	}
	return nil
}

// textValue returns a field's text. Apple writes ASCII; a DER string
// wrapper (UTF8String, PrintableString, IA5String) is accepted too.
func textValue(b []byte) string {
	if len(b) >= 2 && (b[0] == 0x0c || b[0] == 0x13 || b[0] == 0x16) && int(b[1]) == len(b)-2 {
		return string(b[2:])
	}
	return string(b)
}

// parseRisk reads a risk metric written as ASCII digits or as a DER
// INTEGER. The two cannot be confused: an INTEGER starts with 0x02.
func parseRisk(b []byte) (int, error) {
	if len(b) > 0 && len(b) <= 9 && allDigits(b) {
		return strconv.Atoi(string(b))
	}
	in := cryptobyte.String(b)
	var n int64
	if !in.ReadASN1Integer(&n) || !in.Empty() || n < 0 || n > maxRisk {
		return 0, errors.New("the risk metric is not a non-negative integer")
	}
	return int(n), nil
}

// allDigits reports whether b is made of ASCII digits only.
func allDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// keyMatches reports whether the attested key a receipt names is the
// device's key. Apple's documentation does not fix its encoding, so a
// certificate, a SubjectPublicKeyInfo and an uncompressed point are all
// accepted.
func keyMatches(value []byte, want *ecdsa.PublicKey) bool {
	if len(value) == 0 {
		return false
	}
	if c, err := x509.ParseCertificate(value); err == nil {
		k, ok := c.PublicKey.(*ecdsa.PublicKey)
		return ok && k.Equal(want)
	}
	if k, err := x509.ParsePKIXPublicKey(value); err == nil {
		pub, ok := k.(*ecdsa.PublicKey)
		return ok && pub.Equal(want)
	}
	point, err := want.Bytes()
	return err == nil && bytes.Equal(value, point)
}
