// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// PairingType is the typ header of a pairing token.
const PairingType = "plux-pair+jwt"

// codeLength is the number of characters of the code a person types instead
// of opening the pairing link: 40 bits of the token's SHA-256.
const codeLength = 8

// crockford is Crockford's base32 alphabet: digits and capitals without I,
// L, O and U, so that a code read aloud or typed is not misread.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Claims are the claims of a pairing token.
type Claims struct {
	// Session is the session the token pairs a device with.
	Session string `json:"sid"`
	// Subject is the person who created the session (DEV-002).
	Subject string `json:"sub"`
	// App and Environment are the session's app and environment
	// identifiers.
	App         string `json:"app"`
	Environment string `json:"env"`
	// Expiry is when the token stops working, in seconds since the epoch.
	Expiry int64 `json:"exp"`
	// Nonce makes every token unique, 16 random bytes in base64url.
	Nonce string `json:"nonce"`
}

// header is the protected header of a pairing token.
type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
}

// invalidToken is the refusal of a token that is malformed or not signed by
// a development token key of this server.
func invalidToken() error {
	return plxerr.New(plxerr.PermissionDenied, "the pairing token is not valid")
}

// VerifyPairing checks a pairing token's form, header and signature against
// the development token keys and returns its claims. The expiry is the
// caller's to check, against its clock.
func VerifyPairing(token string, keys []signing.TokenKey) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, invalidToken()
	}
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, invalidToken()
	}
	var h header
	if err := json.Unmarshal(rawHeader, &h); err != nil || h.Alg != "ES256" || h.Typ != PairingType || h.Kid == "" {
		return Claims{}, invalidToken()
	}
	pub := developmentKey(keys, h.Kid)
	if pub == nil {
		return Claims{}, invalidToken()
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != signing.TokenSignatureSize {
		return Claims{}, invalidToken()
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	half := signing.TokenSignatureSize / 2
	r, s := new(big.Int).SetBytes(sig[:half]), new(big.Int).SetBytes(sig[half:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return Claims{}, invalidToken()
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, invalidToken()
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil || c.Session == "" || c.Expiry == 0 {
		return Claims{}, invalidToken()
	}
	return c, nil
}

// developmentKey returns the development token key with the identifier, or
// nil. A production key never verifies a pairing token: dev sessions exist
// only outside production (DEV-003).
func developmentKey(keys []signing.TokenKey, kid string) *ecdsa.PublicKey {
	for _, k := range keys {
		if k.ID == kid && k.Class == signing.TokenDevelopment {
			return k.Public
		}
	}
	return nil
}

// digest is the SHA-256 of a credential's text: what the tables keep of a
// pairing token or a socket ticket.
func digest(credential string) []byte {
	sum := sha256.Sum256([]byte(credential))
	return sum[:]
}

// Code is the short code of a pairing token, for typing: the first eight
// characters of the Crockford base32 form of the token's SHA-256, as the
// tables keep it.
func Code(token string) string {
	return base32.NewEncoding(crockford).WithPadding(base32.NoPadding).EncodeToString(digest(token))[:codeLength]
}

// FormatCode shows a code as two groups of four, XXXX-XXXX.
func FormatCode(code string) string {
	if len(code) != codeLength {
		return code
	}
	return code[:codeLength/2] + "-" + code[codeLength/2:]
}

// NormalizeCode reads a code a person typed: case, dashes and spaces do not
// matter, and the letters Crockford's decoding reads as digits (O as 0, I
// and L as 1) are accepted. It reports false when the result is not a code.
func NormalizeCode(typed string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(typed) {
		switch {
		case r == '-' || r == ' ':
			continue
		case r == 'O':
			r = '0'
		case r == 'I' || r == 'L':
			r = '1'
		case !strings.ContainsRune(crockford, r):
			return "", false
		}
		b.WriteRune(r)
	}
	if b.Len() != codeLength {
		return "", false
	}
	return b.String(), true
}
