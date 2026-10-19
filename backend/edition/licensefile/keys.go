// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package licensefile

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
)

// KeySet is a set of trusted licence-signing public keys by key identifier.
// The zero value trusts nothing.
type KeySet struct {
	keys map[string]ed25519.PublicKey
}

// NewKeySet builds a set from public keys. It is for tests and for tools
// that take a key on their command line; the server trusts TrustedKeys.
func NewKeySet(public ...ed25519.PublicKey) KeySet {
	m := make(map[string]ed25519.PublicKey, len(public))
	for _, p := range public {
		m[KeyID(p)] = p
	}
	return KeySet{keys: m}
}

// Lookup returns the key with an identifier.
func (s KeySet) Lookup(id string) (ed25519.PublicKey, bool) {
	k, ok := s.keys[id]
	return k, ok
}

// Len returns the number of keys.
func (s KeySet) Len() int { return len(s.keys) }

// ParsePublicKey decodes a licence-signing public key in standard base64.
func ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("licensefile: a public key is %d bytes in standard base64", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// productionKeys are the licence-signing public keys this build trusts, in
// standard base64. The private keys stay offline with the maintainer
// (ADR-0084); rotation ships a new public key beside the old one. The
// production slot is empty until the maintainer adds the key from the
// signing ceremony; until then a build trusts no licence.
func productionKeys() []string {
	return []string{}
}

// TrustedKeys returns the keys compiled into this build.
func TrustedKeys() KeySet {
	return keySetOf(productionKeys())
}

// TrustedKeysWith returns the compiled-in keys and the given ones. It is for
// tools that verify a file against a key from their command line; the server
// never calls it.
func TrustedKeysWith(extra ...ed25519.PublicKey) KeySet {
	m := TrustedKeys().keys
	out := make(map[string]ed25519.PublicKey, len(m)+len(extra))
	for id, k := range m {
		out[id] = k
	}
	for _, k := range extra {
		out[KeyID(k)] = k
	}
	return KeySet{keys: out}
}

// keySetOf builds a set from base64 keys, skipping any that does not decode.
func keySetOf(encoded []string) KeySet {
	public := make([]ed25519.PublicKey, 0, len(encoded))
	for _, e := range encoded {
		if p, err := ParsePublicKey(e); err == nil {
			public = append(public, p)
		}
	}
	return NewKeySet(public...)
}
