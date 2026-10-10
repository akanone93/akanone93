// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package bundle

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Confidential bundles (SEC-053, ADR-0055). The release step, not the
// compiler, turns a plain bundle into its confidential form: every
// section is sealed with AES-256-GCM under the release's content key and
// the container is marked encrypted. The directory hashes, and with them
// the bundle hash the manifest names, cover the sealed bytes, so a device
// verifies signature, hashes and structure before it decrypts anything
// (SEC-052).

// FeatureConfidential is the required feature of a confidential bundle: a
// runtime that cannot decrypt it refuses the release up front (BND-008).
const FeatureConfidential = "security.confidential.v1"

// Sizes of a sealed section, which is nonce, ciphertext and tag.
const (
	// ContentKeySize is the size of a content key: AES-256.
	ContentKeySize = 32
	// NonceSize is the size of the nonce a sealed section starts with.
	NonceSize = 12
	// TagSize is the size of the GCM tag a sealed section ends with.
	TagSize = 16
	// SealOverhead is how many bytes sealing adds to a section.
	SealOverhead = NonceSize + TagSize
)

// Nonces issues the nonces of one content key: a counter, so no two
// sections of a release ever share one. A release's bundles are sealed
// from one Nonces; a fresh content key starts a fresh counter.
type Nonces struct{ next uint64 }

// take returns the next nonce: four zero bytes and the counter, big
// endian.
func (n *Nonces) take() [NonceSize]byte {
	var out [NonceSize]byte
	binary.BigEndian.PutUint64(out[4:], n.next)
	n.next++
	return out
}

// sectionAAD is the associated data of a sealed section: the container
// kind, the section kind and the section ID, so a section sealed for one
// place does not open in another. The section's hash cannot be part of it,
// because the hash covers the sealed bytes.
func sectionAAD(k Kind, s SectionKind, id ID) []byte {
	aad := make([]byte, 0, 4+len(id))
	aad = binary.LittleEndian.AppendUint16(aad, uint16(k))
	aad = binary.LittleEndian.AppendUint16(aad, uint16(s))
	return append(aad, id[:]...)
}

// newGCM returns AES-256-GCM under a content key.
func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != ContentKeySize {
		return nil, fmt.Errorf("bundle: a content key has %d bytes, not %d", len(key), ContentKeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("bundle: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("bundle: %w", err)
	}
	return aead, nil
}

// Encrypt returns the confidential form of a plain bundle: the same
// sections in the same order, each sealed as nonce, ciphertext and tag,
// in a container whose encrypted flag is set. The input is checked first
// (header, directory and section hashes). Development bundles, which carry
// a source map, are not confidential.
func Encrypt(plain, key []byte, nonces *Nonces) ([]byte, error) {
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	b, err := ReadStructure(plain)
	if err != nil {
		return nil, err
	}
	if b.Kind == KindDevelopment {
		return nil, errors.New("bundle: a development bundle cannot be confidential")
	}
	sealed := make([]Section, len(b.Sections))
	for i, s := range b.Sections {
		nonce := nonces.take()
		data := aead.Seal(nonce[:], nonce[:], s.Data, sectionAAD(b.Kind, s.Kind, s.ID))
		sealed[i] = Section{Kind: s.Kind, ID: s.ID, Data: data}
	}
	return encode(b.Kind, sealed, FlagEncrypted)
}

// Decrypt returns the plain bundle inside a confidential one. It checks
// the header, the directory and every section hash before it decrypts a
// byte, then opens each section; the result is byte-identical to the plain
// bundle that Encrypt was given (the layout is a function of the sections).
// A section that does not open under key fails with PLX-6050.
func Decrypt(sealed, key []byte) ([]byte, error) {
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	b, err := readHeader(sealed, nil, true)
	if err != nil {
		return nil, err
	}
	if b.Flags&FlagEncrypted == 0 {
		return nil, newErr(plxerr.BundleMalformed, "the bundle is not encrypted")
	}
	if err := readDirectory(sealed, b); err != nil {
		return nil, err
	}
	for _, s := range b.Sections {
		if err := checkHash(s); err != nil {
			return nil, err
		}
	}
	plain := make([]Section, len(b.Sections))
	for i, s := range b.Sections {
		if len(s.Data) < SealOverhead {
			return nil, malformed("%s section %x is too short to be sealed", s.Kind, s.ID)
		}
		nonce, body := s.Data[:NonceSize], s.Data[NonceSize:]
		data, err := aead.Open(nil, nonce, body, sectionAAD(b.Kind, s.Kind, s.ID))
		if err != nil {
			return nil, wrapErr(plxerr.ConfidentialKeyUnavailable, err, "%s section %x does not open under the content key", s.Kind, s.ID)
		}
		plain[i] = Section{Kind: s.Kind, ID: s.ID, Data: data}
	}
	return Encode(b.Kind, plain)
}
