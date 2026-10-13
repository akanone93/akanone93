// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package receipt

import (
	"bytes"
	"crypto"
	_ "crypto/sha256" // Registers SHA-256 for crypto.Hash.
	_ "crypto/sha512" // Registers SHA-384 and SHA-512 for crypto.Hash.
	"crypto/subtle"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"math/big"
	"time"

	"golang.org/x/crypto/cryptobyte"
	cbasn1 "golang.org/x/crypto/cryptobyte/asn1"
)

// Object identifiers of RFC 5652 and of the algorithms a receipt may use.
var (
	oidSignedData    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidMessageDigest = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}

	oidSHA256 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidSHA384 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	oidSHA512 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}

	oidECPublicKey = asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1}
	oidECDSASHA256 = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	oidECDSASHA384 = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 3}
	oidECDSASHA512 = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 4}
	oidRSA         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidRSASHA256   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
	oidRSASHA384   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 12}
	oidRSASHA512   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 13}
)

// The context-specific tags of CMS: [0] primitive, [0] and [1] constructed.
var (
	tagContext0 = cbasn1.Tag(0).ContextSpecific()
	tagCtx0Cons = cbasn1.Tag(0).ContextSpecific().Constructed()
	tagCtx1Cons = cbasn1.Tag(1).ContextSpecific().Constructed()
)

// maxCertificates bounds the certificates a receipt may carry.
const maxCertificates = 8

// signedData is the part of a CMS SignedData the verifier uses: a single
// signer over embedded content.
type signedData struct {
	content []byte
	certs   [][]byte
	signer  signerInfo
}

// signerInfo is a CMS SignerInfo.
type signerInfo struct {
	issuer      []byte
	serial      *big.Int
	keyID       []byte
	digestOID   asn1.ObjectIdentifier
	sigOID      asn1.ObjectIdentifier
	signedAttrs []byte
	signature   []byte
}

// errMalformed is the failure of every structural parse; callers add no
// detail so that nothing of the input leaks into a message.
var errMalformed = errors.New("the CMS structure is malformed")

// parseSignedData reads a DER ContentInfo holding a SignedData with one
// signer and embedded content.
func parseSignedData(der []byte) (signedData, error) {
	var sd signedData
	in := cryptobyte.String(der)
	var info, wrap, body cryptobyte.String
	var oid asn1.ObjectIdentifier
	if !in.ReadASN1(&info, cbasn1.SEQUENCE) || !in.Empty() ||
		!info.ReadASN1ObjectIdentifier(&oid) || !oid.Equal(oidSignedData) ||
		!info.ReadASN1(&wrap, tagCtx0Cons) || !wrap.ReadASN1(&body, cbasn1.SEQUENCE) {
		return signedData{}, errMalformed
	}
	var version int
	var digests cryptobyte.String
	if !body.ReadASN1Integer(&version) || !body.ReadASN1(&digests, cbasn1.SET) {
		return signedData{}, errMalformed
	}
	var err error
	if sd.content, err = readEncapsulated(&body); err != nil {
		return signedData{}, err
	}
	if sd.certs, err = readCertificates(&body); err != nil {
		return signedData{}, err
	}
	if body.PeekASN1Tag(tagCtx1Cons) && !body.SkipASN1(tagCtx1Cons) {
		return signedData{}, errMalformed
	}
	var signers cryptobyte.String
	if !body.ReadASN1(&signers, cbasn1.SET) || !body.Empty() {
		return signedData{}, errMalformed
	}
	if sd.signer, err = readSignerInfo(&signers); err != nil {
		return signedData{}, err
	}
	if !signers.Empty() {
		return signedData{}, errors.New("the receipt has more than one signer")
	}
	return sd, nil
}

// readEncapsulated reads the EncapsulatedContentInfo and returns its
// content octets.
func readEncapsulated(body *cryptobyte.String) ([]byte, error) {
	var encap, wrap, content cryptobyte.String
	var oid asn1.ObjectIdentifier
	if !body.ReadASN1(&encap, cbasn1.SEQUENCE) || !encap.ReadASN1ObjectIdentifier(&oid) ||
		!encap.ReadASN1(&wrap, tagCtx0Cons) || !wrap.ReadASN1(&content, cbasn1.OCTET_STRING) || !wrap.Empty() || !encap.Empty() {
		return nil, errors.New("the receipt has no embedded content")
	}
	return content, nil
}

// readCertificates reads the optional certificate set.
func readCertificates(body *cryptobyte.String) ([][]byte, error) {
	if !body.PeekASN1Tag(tagCtx0Cons) {
		return nil, nil
	}
	var set cryptobyte.String
	if !body.ReadASN1(&set, tagCtx0Cons) {
		return nil, errMalformed
	}
	var certs [][]byte
	for !set.Empty() {
		var c cryptobyte.String
		if !set.ReadASN1Element(&c, cbasn1.SEQUENCE) || len(certs) >= maxCertificates {
			return nil, errMalformed
		}
		certs = append(certs, c)
	}
	return certs, nil
}

// readSignerInfo reads one SignerInfo from a SET OF.
func readSignerInfo(set *cryptobyte.String) (signerInfo, error) {
	var si signerInfo
	var in cryptobyte.String
	var version int
	if !set.ReadASN1(&in, cbasn1.SEQUENCE) || !in.ReadASN1Integer(&version) {
		return signerInfo{}, errMalformed
	}
	switch {
	case in.PeekASN1Tag(cbasn1.SEQUENCE):
		var ias, issuer cryptobyte.String
		si.serial = new(big.Int)
		if !in.ReadASN1(&ias, cbasn1.SEQUENCE) || !ias.ReadASN1Element(&issuer, cbasn1.SEQUENCE) || !ias.ReadASN1Integer(si.serial) {
			return signerInfo{}, errMalformed
		}
		si.issuer = issuer
	case in.PeekASN1Tag(tagContext0):
		var ski cryptobyte.String
		if !in.ReadASN1(&ski, tagContext0) {
			return signerInfo{}, errMalformed
		}
		si.keyID = ski
	default:
		return signerInfo{}, errMalformed
	}
	if !readAlgorithm(&in, &si.digestOID) {
		return signerInfo{}, errMalformed
	}
	if in.PeekASN1Tag(tagCtx0Cons) {
		var attrs cryptobyte.String
		if !in.ReadASN1Element(&attrs, tagCtx0Cons) {
			return signerInfo{}, errMalformed
		}
		si.signedAttrs = attrs
	}
	var sig cryptobyte.String
	if !readAlgorithm(&in, &si.sigOID) || !in.ReadASN1(&sig, cbasn1.OCTET_STRING) {
		return signerInfo{}, errMalformed
	}
	si.signature = sig
	return si, nil
}

// readAlgorithm reads an AlgorithmIdentifier and keeps its OID.
func readAlgorithm(in *cryptobyte.String, oid *asn1.ObjectIdentifier) bool {
	var alg cryptobyte.String
	return in.ReadASN1(&alg, cbasn1.SEQUENCE) && alg.ReadASN1ObjectIdentifier(oid)
}

// digestOf hashes data with the digest algorithm named by an OID.
func digestOf(oid asn1.ObjectIdentifier, data []byte) ([]byte, bool) {
	var h crypto.Hash
	switch {
	case oid.Equal(oidSHA256):
		h = crypto.SHA256
	case oid.Equal(oidSHA384):
		h = crypto.SHA384
	case oid.Equal(oidSHA512):
		h = crypto.SHA512
	default:
		return nil, false
	}
	w := h.New()
	w.Write(data)
	return w.Sum(nil), true
}

// signatureAlgorithm maps the signature and digest OIDs of a SignerInfo to
// the algorithm crypto/x509 checks. A signature OID that names its own
// hash must agree with the digest algorithm.
func signatureAlgorithm(sig, digest asn1.ObjectIdentifier) (x509.SignatureAlgorithm, bool) {
	type pair struct{ sig, digest asn1.ObjectIdentifier }
	pairs := map[x509.SignatureAlgorithm][]pair{
		x509.ECDSAWithSHA256: {{oidECDSASHA256, oidSHA256}, {oidECPublicKey, oidSHA256}},
		x509.ECDSAWithSHA384: {{oidECDSASHA384, oidSHA384}, {oidECPublicKey, oidSHA384}},
		x509.ECDSAWithSHA512: {{oidECDSASHA512, oidSHA512}, {oidECPublicKey, oidSHA512}},
		x509.SHA256WithRSA:   {{oidRSASHA256, oidSHA256}, {oidRSA, oidSHA256}},
		x509.SHA384WithRSA:   {{oidRSASHA384, oidSHA384}, {oidRSA, oidSHA384}},
		x509.SHA512WithRSA:   {{oidRSASHA512, oidSHA512}, {oidRSA, oidSHA512}},
	}
	for algo, ps := range pairs {
		for _, p := range ps {
			if p.sig.Equal(sig) && p.digest.Equal(digest) {
				return algo, true
			}
		}
	}
	return 0, false
}

// verifyCMS checks the signature of a SignedData and the chain of its
// signer certificate to roots at time now (SEC-007).
func verifyCMS(sd signedData, roots *x509.CertPool, now time.Time) error {
	certs := make([]*x509.Certificate, 0, len(sd.certs))
	for _, raw := range sd.certs {
		c, err := x509.ParseCertificate(raw)
		if err != nil {
			return errors.New("a certificate in the receipt does not parse")
		}
		certs = append(certs, c)
	}
	signer := findSigner(certs, sd.signer)
	if signer == nil {
		return errors.New("the receipt carries no certificate for its signer")
	}
	signed, err := signedBytes(sd)
	if err != nil {
		return err
	}
	algo, ok := signatureAlgorithm(sd.signer.sigOID, sd.signer.digestOID)
	if !ok {
		return errors.New("the receipt uses an unsupported signature algorithm")
	}
	if err := signer.CheckSignature(algo, signed, sd.signer.signature); err != nil {
		return errors.New("the receipt signature does not verify")
	}
	pool := x509.NewCertPool()
	for _, c := range certs {
		if c != signer {
			pool.AddCert(c)
		}
	}
	if _, err := signer.Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: pool, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return errors.New("the receipt signer does not chain to a trusted root")
	}
	return nil
}

// findSigner returns the certificate a SignerInfo names.
func findSigner(certs []*x509.Certificate, si signerInfo) *x509.Certificate {
	for _, c := range certs {
		switch {
		case si.serial != nil:
			if bytes.Equal(c.RawIssuer, si.issuer) && c.SerialNumber.Cmp(si.serial) == 0 {
				return c
			}
		case len(si.keyID) > 0:
			if bytes.Equal(c.SubjectKeyId, si.keyID) {
				return c
			}
		}
	}
	return nil
}

// signedBytes returns what the signer signed: the DER of the signed
// attributes when there are some, after checking that their message digest
// covers the content, or else the content itself (RFC 5652 §5.4).
func signedBytes(sd signedData) ([]byte, error) {
	if sd.signer.signedAttrs == nil {
		return sd.content, nil
	}
	want, ok := digestOf(sd.signer.digestOID, sd.content)
	if !ok {
		return nil, errors.New("the receipt uses an unsupported digest algorithm")
	}
	var attrs cryptobyte.String = sd.signer.signedAttrs
	var inner cryptobyte.String
	if !attrs.ReadASN1(&inner, tagCtx0Cons) {
		return nil, errMalformed
	}
	var got []byte
	for !inner.Empty() {
		var attr, vals cryptobyte.String
		var oid asn1.ObjectIdentifier
		if !inner.ReadASN1(&attr, cbasn1.SEQUENCE) || !attr.ReadASN1ObjectIdentifier(&oid) || !attr.ReadASN1(&vals, cbasn1.SET) {
			return nil, errMalformed
		}
		if oid.Equal(oidMessageDigest) {
			var md cryptobyte.String
			if got != nil || !vals.ReadASN1(&md, cbasn1.OCTET_STRING) || !vals.Empty() {
				return nil, errMalformed
			}
			got = md
		}
	}
	if got == nil || subtle.ConstantTimeCompare(got, want) != 1 {
		return nil, errors.New("the receipt content does not match its signed digest")
	}
	// The signature covers the attributes with an explicit SET tag in
	// place of the implicit [0] (RFC 5652 §5.4); the length octets are
	// the same.
	out := append([]byte(nil), sd.signer.signedAttrs...)
	out[0] = 0x31
	return out, nil
}
