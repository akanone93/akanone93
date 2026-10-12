// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package bundle

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// loadConfidentialVectors reads the conformance vectors the fuzz targets
// are seeded from.
func loadConfidentialVectors(tb testing.TB) (v confidentialVectors, key []byte) {
	tb.Helper()
	raw, err := os.ReadFile(confidentialVectorsPath)
	if err != nil {
		tb.Fatal(err)
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		tb.Fatal(err)
	}
	key, err = hex.DecodeString(v.ContentKey)
	if err != nil {
		tb.Fatal(err)
	}
	return v, key
}

func mustB64(tb testing.TB, s string) []byte {
	tb.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		tb.Fatal(err)
	}
	return b
}

// FuzzDecrypt checks that opening a confidential bundle never panics, and
// that what it accepts is a well-formed plain bundle. The seeds are the
// sealed vector and every damaged one, so mutation starts at the checks.
//
// Verifies: QA-004, SEC-052, SEC-053.
func FuzzDecrypt(f *testing.F) {
	v, key := loadConfidentialVectors(f)
	f.Add(mustB64(f, v.Sealed))
	f.Add(mustB64(f, v.Plain))
	for _, r := range v.Refused {
		f.Add(mustB64(f, r.Sealed))
	}
	f.Add([]byte{})
	f.Add(mustB64(f, v.Sealed)[:headerSize])
	f.Fuzz(func(t *testing.T, sealed []byte) {
		plain, err := Decrypt(sealed, key)
		if err != nil {
			if plain != nil {
				t.Fatal("a plain bundle returned with an error")
			}
			return
		}
		if _, err := ReadStructure(plain); err != nil {
			t.Fatalf("an accepted bundle does not read: %v", err)
		}
	})
}

// FuzzUnwrapContentKey checks that unwrapping never panics and that only
// the exact bytes the server wrapped yield a key.
//
// Verifies: QA-004, SEC-053.
func FuzzUnwrapContentKey(f *testing.F) {
	v, key := loadConfidentialVectors(f)
	scalar, err := hex.DecodeString(v.Wrap.DevicePrivateKey)
	if err != nil {
		f.Fatal(err)
	}
	device, err := ecdh.P256().NewPrivateKey(scalar)
	if err != nil {
		f.Fatal(err)
	}
	info := KeyInfo(v.Wrap.DeviceID, v.Wrap.App, v.Wrap.Environment, v.Wrap.Sequence)
	ephemeral, wrapped := mustB64(f, v.Wrap.EphemeralPublicKey), mustB64(f, v.Wrap.Wrapped)
	f.Add(ephemeral, wrapped, info)
	f.Add(ephemeral[:64], wrapped, info)
	f.Add(ephemeral, wrapped[:WrappedKeySize-1], info)
	f.Add([]byte{}, []byte{}, []byte{})
	f.Fuzz(func(t *testing.T, e, w, i []byte) {
		got, err := UnwrapContentKey(WrappedKey{EphemeralPublicKey: e, Wrapped: w}, device, i)
		if err != nil {
			return
		}
		if !bytes.Equal(got, key) || !bytes.Equal(e, ephemeral) || !bytes.Equal(w, wrapped) || !bytes.Equal(i, info) {
			t.Fatalf("a key unwrapped from input that was not the server's wrap")
		}
	})
}
