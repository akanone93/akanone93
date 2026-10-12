// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package binding computes the value a device's platform evidence is bound
// to at registration (SEC-004, ADR-0012 §Registration): the server's
// challenge and the RFC 7638 thumbprints of every key the device registers,
// so that attested hardware vouches for all of them and a rooted device
// cannot add a software key to an attested registration.
package binding

import (
	"crypto/sha256"
	"encoding/base64"
)

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
