// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package bundle

import (
	"bytes"
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// testKey is a content key for tests.
func testKey() []byte {
	key := make([]byte, ContentKeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}

// plainBundle is a plugin bundle of the sample sections.
func plainBundle(t *testing.T) []byte {
	t.Helper()
	return mustEncode(t, KindPlugin, sampleSections())
}

// Verifies: SEC-053, SEC-052.
func TestEncryptSealsEverySection(t *testing.T) {
	t.Parallel()
	plain := plainBundle(t)
	sealed, err := Encrypt(plain, testKey(), &Nonces{})
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(sealed[8:])&FlagEncrypted == 0 {
		t.Fatal("the encrypted flag is clear")
	}
	p, err := ReadStructure(plain)
	if err != nil {
		t.Fatal(err)
	}
	// ReadStructure refuses an encrypted bundle: only Decrypt opens one.
	if _, err := ReadStructure(sealed); err == nil {
		t.Fatal("ReadStructure accepted an encrypted bundle")
	}
	wantCode(t, "Read", func() error { _, err := Read(sealed, defaultOptions()); return err }(), plxerr.BundleEncryptedUnsupported)
	s, err := readHeader(sealed, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := readDirectory(sealed, s); err != nil {
		t.Fatal(err)
	}
	if len(s.Sections) != len(p.Sections) {
		t.Fatalf("%d sections, want %d", len(s.Sections), len(p.Sections))
	}
	for i, sec := range s.Sections {
		if sec.Kind != p.Sections[i].Kind || sec.ID != p.Sections[i].ID {
			t.Errorf("section %d moved", i)
		}
		if len(sec.Data) != len(p.Sections[i].Data)+SealOverhead {
			t.Errorf("section %d has %d bytes, want %d", i, len(sec.Data), len(p.Sections[i].Data)+SealOverhead)
		}
		if bytes.Contains(sec.Data, p.Sections[i].Data) {
			t.Errorf("section %d holds its plaintext", i)
		}
		// The directory hash covers the sealed bytes (SEC-052).
		if sha256.Sum256(sec.Data) != sec.Hash {
			t.Errorf("section %d: the directory hash is not over the sealed bytes", i)
		}
	}
	if s.Hash == p.Hash {
		t.Error("the bundle hash does not change with encryption")
	}
}

// Verifies: SEC-053, CMP-002.
func TestDecryptReturnsTheExactPlainBundle(t *testing.T) {
	t.Parallel()
	plain := plainBundle(t)
	sealed, err := Encrypt(plain, testKey(), &Nonces{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(sealed, testKey())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Error("the decrypted bundle is not the plain one")
	}
	again, err := Encrypt(plain, testKey(), &Nonces{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, sealed) {
		t.Error("the same key and nonce counter give different bytes")
	}
}

// Verifies: SEC-053.
func TestNoncesAreUniquePerKey(t *testing.T) {
	t.Parallel()
	nonces := &Nonces{}
	seen := map[string]bool{}
	for range 3 {
		sealed, err := Encrypt(plainBundle(t), testKey(), nonces)
		if err != nil {
			t.Fatal(err)
		}
		b, err := readHeader(sealed, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := readDirectory(sealed, b); err != nil {
			t.Fatal(err)
		}
		for _, s := range b.Sections {
			n := string(s.Data[:NonceSize])
			if seen[n] {
				t.Fatalf("nonce %x used twice", n)
			}
			seen[n] = true
		}
	}
	if len(seen) != 12 {
		t.Errorf("%d nonces for 12 sections", len(seen))
	}
}

// Verifies: SEC-052, SEC-053.
func TestDecryptChecksHashesBeforeDecrypting(t *testing.T) {
	t.Parallel()
	sealed, err := Encrypt(plainBundle(t), testKey(), &Nonces{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := readHeader(sealed, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := readDirectory(sealed, b); err != nil {
		t.Fatal(err)
	}
	// Flip one byte of the last section's ciphertext, leaving the directory.
	last := b.Sections[len(b.Sections)-1]
	tampered := slices.Clone(sealed)
	tampered[len(tampered)-len(last.Data)+NonceSize] ^= 1
	_, err = Decrypt(tampered, testKey())
	wantCode(t, "tampered section", err, plxerr.SectionHashMismatch)
	// A wrong key is not seen as a hash problem: hashes pass, the tag fails.
	other := testKey()
	other[0] ^= 1
	_, err = Decrypt(sealed, other)
	wantCode(t, "wrong key", err, plxerr.ConfidentialKeyUnavailable)
}

// Verifies: SEC-053.
func TestDecryptRefusesAttackerEditsThatKeepTheHashesConsistent(t *testing.T) {
	t.Parallel()
	sealed, err := Encrypt(plainBundle(t), testKey(), &Nonces{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := readHeader(sealed, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := readDirectory(sealed, b); err != nil {
		t.Fatal(err)
	}
	// Re-encode a section with a flipped tag, hashes recomputed: the
	// signature over the bundle hash would catch it, and so does the tag.
	edited := slices.Clone(b.Sections)
	data := slices.Clone(edited[0].Data)
	data[len(data)-1] ^= 1
	edited[0].Data = data
	forged, err := encode(b.Kind, edited, FlagEncrypted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Decrypt(forged, testKey())
	wantCode(t, "flipped tag", err, plxerr.ConfidentialKeyUnavailable)
	// A section moved to another ID opens nowhere: its place is authenticated.
	moved := slices.Clone(b.Sections)
	moved[1].ID = ID{9}
	forged, err = encode(b.Kind, moved, FlagEncrypted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Decrypt(forged, testKey())
	wantCode(t, "moved section", err, plxerr.ConfidentialKeyUnavailable)
	// A plain bundle is not a confidential one.
	_, err = Decrypt(plainBundle(t), testKey())
	wantCode(t, "plain bundle", err, plxerr.BundleMalformed)
}

// Verifies: SEC-053.
func TestEncryptRefusesWhatCannotBeConfidential(t *testing.T) {
	t.Parallel()
	if _, err := Encrypt(plainBundle(t), testKey()[:16], &Nonces{}); err == nil {
		t.Error("a 16-byte key was accepted")
	}
	dev := mustEncode(t, KindDevelopment, append(sampleSections(), Section{Kind: SectionSourceMap, ID: ID{1}, Data: []byte("map")}))
	if _, err := Encrypt(dev, testKey(), &Nonces{}); err == nil {
		t.Error("a development bundle was encrypted")
	}
	sealed, err := Encrypt(plainBundle(t), testKey(), &Nonces{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Encrypt(sealed, testKey(), &Nonces{}); err == nil {
		t.Error("an encrypted bundle was encrypted again")
	}
}

// Verifies: SEC-053.
func TestKeyWrapKnownAnswers(t *testing.T) {
	t.Parallel()
	// RFC 3394 section 4.1 and 4.6.
	for _, c := range []struct{ kek, key, want string }{
		{"000102030405060708090A0B0C0D0E0F", "00112233445566778899AABBCCDDEEFF", "1FA68B0A8112B447AEF34BD8FB5A7B829D3E862371D2CFE5"},
		{"000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F", "00112233445566778899AABBCCDDEEFF000102030405060708090A0B0C0D0E0F", "28C9F404C4B810F4CBCCB35CFB87F8263F5786E2D80ED326CBC7F0E71A99F43BFB988B9B7A02DD21"},
	} {
		kek, _ := hex.DecodeString(c.kek)
		key, _ := hex.DecodeString(c.key)
		want, _ := hex.DecodeString(c.want)
		got, err := aesKeyWrap(kek, key)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("wrap with a %d-byte KEK: %x, %v", len(kek), got, err)
		}
		back, err := aesKeyUnwrap(kek, want)
		if err != nil || !bytes.Equal(back, key) {
			t.Errorf("unwrap with a %d-byte KEK: %x, %v", len(kek), back, err)
		}
	}
}

// Verifies: SEC-053.
func TestWrapContentKeyBindsDeviceAndRelease(t *testing.T) {
	t.Parallel()
	device, err := ecdh.P256().GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	info := KeyInfo("device-1", "app-1", "production", 7)
	w, err := WrapContentKey(testKey(), device.PublicKey(), info, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.EphemeralPublicKey) != 65 || len(w.Wrapped) != WrappedKeySize {
		t.Fatalf("sizes %d and %d", len(w.EphemeralPublicKey), len(w.Wrapped))
	}
	if bytes.Contains(w.Wrapped, testKey()) {
		t.Error("the wrapped key holds the content key")
	}
	got, err := UnwrapContentKey(w, device, info)
	if err != nil || !bytes.Equal(got, testKey()) {
		t.Fatalf("unwrap: %x, %v", got, err)
	}
	// Each wrap uses a fresh ephemeral key.
	w2, err := WrapContentKey(testKey(), device.PublicKey(), info, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(w.EphemeralPublicKey, w2.EphemeralPublicKey) || bytes.Equal(w.Wrapped, w2.Wrapped) {
		t.Error("two wraps are identical")
	}
	other, err := ecdh.P256().GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		w    WrappedKey
		dev  *ecdh.PrivateKey
		info []byte
	}{
		"another device key":     {w, other, info},
		"another device ID":      {w, device, KeyInfo("device-2", "app-1", "production", 7)},
		"another release":        {w, device, KeyInfo("device-1", "app-1", "production", 8)},
		"another environment":    {w, device, KeyInfo("device-1", "app-1", "staging", 7)},
		"an altered wrapped key": {WrappedKey{w.EphemeralPublicKey, flip(w.Wrapped)}, device, info},
		"another ephemeral key":  {WrappedKey{w2.EphemeralPublicKey, w.Wrapped}, device, info},
		"a malformed point":      {WrappedKey{w.EphemeralPublicKey[:64], w.Wrapped}, device, info},
	} {
		_, err := UnwrapContentKey(c.w, c.dev, c.info)
		wantCode(t, name, err, plxerr.ConfidentialKeyUnavailable)
	}
}

func flip(b []byte) []byte {
	out := slices.Clone(b)
	out[0] ^= 1
	return out
}

// Verifies: SEC-053.
func TestWrapContentKeyChecksItsInputs(t *testing.T) {
	t.Parallel()
	device, err := ecdh.P256().GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WrapContentKey(testKey()[:31], device.PublicKey(), nil, nil); err == nil {
		t.Error("a 31-byte content key was accepted")
	}
	x, err := ecdh.X25519().GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WrapContentKey(testKey(), x.PublicKey(), nil, nil); err == nil {
		t.Error("an X25519 recipient was accepted")
	}
}
