// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package bundle

import (
	"bytes"
	"crypto/ecdh"
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// releaseKey is the content key of one release: every release has its own.
func releaseKey(seed byte) []byte {
	key := make([]byte, ContentKeySize)
	for i := range key {
		key[i] = seed + byte(i)
	}
	return key
}

// sealedSections reads the sections of a sealed bundle without opening them.
func sealedSections(t *testing.T, sealed []byte) (*Bundle, []Section) {
	t.Helper()
	b, err := readHeader(sealed, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := readDirectory(sealed, b); err != nil {
		t.Fatal(err)
	}
	return b, slices.Clone(b.Sections)
}

// reseal returns a sealed container for the sections with every hash
// consistent, as an attacker who edits the file and recomputes them would
// make it. The signature over the bundle hash (SEC-052) is what stops this
// on a device; the sealing must stop it too.
func reseal(t *testing.T, kind Kind, sections []Section) []byte {
	t.Helper()
	out, err := encode(kind, sections, FlagEncrypted)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Verifies: SEC-053, SEC-052, QA-008.
// Tamper attacks against a confidential bundle, one row per attack. A
// change the hashes reveal is refused before any decryption; a change that
// recomputes the hashes is refused by the authentication tag.
func TestConfidentialBundleTampering_SEC_053_QA_008(t *testing.T) {
	t.Parallel()
	key := releaseKey(1)
	sealed, err := Encrypt(plainBundle(t), key, &Nonces{})
	if err != nil {
		t.Fatal(err)
	}
	b, secs := sealedSections(t, sealed)
	if len(secs) < 3 {
		t.Fatalf("the sample bundle has %d sections, the matrix needs three", len(secs))
	}

	// edit changes section i and reseals with consistent hashes.
	edit := func(i int, f func(data []byte)) []byte {
		s := slices.Clone(secs)
		s[i].Data = slices.Clone(s[i].Data)
		f(s[i].Data)
		return reseal(t, b.Kind, s)
	}
	// inFile flips a byte of the file at an offset into the last section.
	last := secs[len(secs)-1]
	inFile := func(offsetInLast int) []byte {
		out := slices.Clone(sealed)
		out[len(out)-len(last.Data)+offsetInLast] ^= 1
		return out
	}
	swapped := slices.Clone(secs)
	swapped[0].Data, swapped[1].Data = swapped[1].Data, swapped[0].Data
	rekinded := slices.Clone(secs)
	rekinded[0].Kind, rekinded[1].Kind = rekinded[1].Kind, rekinded[0].Kind
	truncated := sealed[:len(sealed)-1]

	for _, tc := range []struct {
		name   string
		sealed []byte
		want   plxerr.Code
	}{
		{"a ciphertext byte altered in the file", inFile(NonceSize + 1), plxerr.SectionHashMismatch},
		{"a nonce byte altered in the file", inFile(0), plxerr.SectionHashMismatch},
		{"a tag byte altered in the file", inFile(len(last.Data) - 1), plxerr.SectionHashMismatch},
		{"a ciphertext byte altered, hashes recomputed", edit(0, func(d []byte) { d[NonceSize+1] ^= 1 }), plxerr.ConfidentialKeyUnavailable},
		{"a nonce byte altered, hashes recomputed", edit(1, func(d []byte) { d[0] ^= 1 }), plxerr.ConfidentialKeyUnavailable},
		{"a tag byte altered, hashes recomputed", edit(2, func(d []byte) { d[len(d)-1] ^= 1 }), plxerr.ConfidentialKeyUnavailable},
		{"two sections' contents swapped, hashes recomputed", reseal(t, b.Kind, swapped), plxerr.ConfidentialKeyUnavailable},
		{"two sections' kinds swapped, hashes recomputed", reseal(t, b.Kind, rekinded), plxerr.ConfidentialKeyUnavailable},
		{"the container truncated", truncated, plxerr.Code(0)},
		{"an empty file", nil, plxerr.Code(0)},
		{"a plain bundle served as confidential", plainBundle(t), plxerr.BundleMalformed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := Decrypt(tc.sealed, key)
			if err == nil {
				t.Fatalf("accepted; %d plain bytes returned", len(out))
			}
			if tc.want != plxerr.Code(0) {
				wantCode(t, tc.name, err, tc.want)
			}
		})
	}
}

// Verifies: SEC-053, SEC-054, QA-008.
// A device opens a release's bundle only with the key wrapped for that
// device and that release. Another device, another release's key and an
// older release replayed after an update are all refused with PLX-6050.
func TestConfidentialBundleKeyAndRollbackAttacks_SEC_053_QA_008(t *testing.T) {
	t.Parallel()
	device, err := ecdh.P256().GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	stolen, err := ecdh.P256().GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	plain := plainBundle(t)
	keys := map[int64][]byte{6: releaseKey(0x10), 7: releaseKey(0x20), 8: releaseKey(0x30)}
	sealed := map[int64][]byte{}
	wrapped := map[int64]WrappedKey{}
	info := func(seq int64) []byte { return KeyInfo("device-1", "app-1", "production", seq) }
	for seq, k := range keys {
		s, err := Encrypt(plain, k, &Nonces{})
		if err != nil {
			t.Fatal(err)
		}
		sealed[seq] = s
		w, err := WrapContentKey(k, device.PublicKey(), info(seq), nil)
		if err != nil {
			t.Fatal(err)
		}
		wrapped[seq] = w
	}

	// open is what a device does for a release: unwrap, then decrypt.
	open := func(release int64, w WrappedKey, dev *ecdh.PrivateKey, bound int64) error {
		key, err := UnwrapContentKey(w, dev, info(bound))
		if err != nil {
			return err
		}
		out, err := Decrypt(sealed[release], key)
		if err == nil && !bytes.Equal(out, plain) {
			t.Error("opened to different bytes")
		}
		return err
	}
	if err := open(7, wrapped[7], device, 7); err != nil {
		t.Fatalf("the honest path: %v", err)
	}
	for _, tc := range []struct {
		name    string
		release int64
		w       WrappedKey
		dev     *ecdh.PrivateKey
		bound   int64
	}{
		{"a wrong device key", 7, wrapped[7], stolen, 7},
		{"the key of another release for a bundle of this one", 7, wrapped[8], device, 8},
		{"the key of the older release for the newer bundle", 8, wrapped[7], device, 7},
		{"the key of the newer release for the older bundle", 6, wrapped[7], device, 7},
		{"the older release replayed: its key, claimed as the current release", 6, wrapped[6], device, 7},
		{"the older release replayed: its bundle under the current key", 6, wrapped[7], device, 7},
		{"the current key claimed for the older release", 7, wrapped[7], device, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wantCode(t, tc.name, open(tc.release, tc.w, tc.dev, tc.bound), plxerr.ConfidentialKeyUnavailable)
		})
	}
}
