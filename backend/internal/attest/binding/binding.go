// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package binding computes the values a device's platform evidence is
// bound to (SEC-004, ADR-0012 §Registration). At registration it is the
// server's challenge and the RFC 7638 thumbprints of every key the device
// registers, so that attested hardware vouches for all of them and a
// rooted device cannot add a software key to an attested registration. At
// a function invocation it is the function, its version, the input and the
// request's DPoP proof identifier (FN-020).
package binding

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
)

// invocationLabel separates the invocation binding from every other use of
// platform evidence.
const invocationLabel = "plux-fn-invoke-v1"

// Hash returns SHA-256(challenge ‖ jkt ‖ "." ‖ kaJKT ‖ "." ‖ scaJKT). jkt
// is the thumbprint of the DPoP key, kaJKT that of the key-agreement key
// and scaJKT that of the SCA key; a key the device does not have
// contributes the empty string, and the separators stay so that a
// thumbprint cannot move from one field to another. It is the Android
// request hash (before base64url) and the iOS App Attest client data hash.
func Hash(challenge []byte, jkt, kaJKT, scaJKT string) [32]byte {
	h := sha256.New()
	h.Write(challenge)
	h.Write([]byte(jkt))
	h.Write([]byte("."))
	h.Write([]byte(kaJKT))
	h.Write([]byte("."))
	h.Write([]byte(scaJKT))
	var out [32]byte
	h.Sum(out[:0])
	return out
}

// RequestHash is Hash as the Play Integrity requestHash: base64url
// without padding.
func RequestHash(challenge []byte, jkt, kaJKT, scaJKT string) string {
	sum := Hash(challenge, jkt, kaJKT, scaJKT)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// InvocationHash returns the value the platform evidence of a function
// invocation is bound to (FN-020, SEC-004):
//
//	SHA-256("plux-fn-invoke-v1" ‖ function ‖ "@" ‖ version ‖ SHA-256(input) ‖ jti)
//
// where version is a 4-byte big-endian unsigned integer, so that no digit
// of it can be read as part of the input's hash, and jti is the identifier
// of the DPoP proof of the request, whose replay check makes the value
// single-use. It is the App Attest client data hash and, base64url without
// padding, the Play Integrity requestHash.
func InvocationHash(function string, version uint32, input []byte, jti string) [32]byte {
	digest := sha256.Sum256(input)
	var v [4]byte
	binary.BigEndian.PutUint32(v[:], version)
	h := sha256.New()
	h.Write([]byte(invocationLabel))
	h.Write([]byte(function))
	h.Write([]byte("@"))
	h.Write(v[:])
	h.Write(digest[:])
	h.Write([]byte(jti))
	var out [32]byte
	h.Sum(out[:0])
	return out
}

// InvocationRequestHash is InvocationHash as the Play Integrity
// requestHash.
func InvocationRequestHash(function string, version uint32, input []byte, jti string) string {
	sum := InvocationHash(function, version, input, jti)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
