// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package bundle

import (
	"crypto/aes"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Delivery of a content key to a device (SEC-053, ADR-0055): the key is
// wrapped with ECDH-ES against the device's key-agreement key (P-256),
// HKDF-SHA-256 bound to the device and the release, and AES-KW (RFC 3394).
// The server never learns anything it needs to unwrap later: the wrapping
// key is derived from an ephemeral key it discards.

// WrappedKey is a content key wrapped for one device and one release.
type WrappedKey struct {
	// EphemeralPublicKey is the server's one-time key, an uncompressed
	// SEC1 point of 65 bytes.
	EphemeralPublicKey []byte
	// Wrapped is the AES-KW output: 40 bytes for a 32-byte content key.
	Wrapped []byte
}

// WrappedKeySize is the size of Wrapped.
const WrappedKeySize = ContentKeySize + 8

// keyContext is the label every derivation starts with.
const keyContext = "plux.confidential.v1"

// KeyInfo is the HKDF info a content key is bound to: the device, and the
// release as the device knows it from the signed manifest — app,
// environment and sequence. A wrapped key therefore opens only for that
// device, for that release.
func KeyInfo(deviceID, appID, environment string, sequence int64) []byte {
	info := []byte(keyContext)
	for _, part := range []string{deviceID, appID, environment, strconv.FormatInt(sequence, 10)} {
		info = append(info, 0)
		info = append(info, part...)
	}
	return info
}

// wrapKEK derives the key-encryption key from the ECDH secret; the salt is
// both public keys, so the derivation is bound to the exchange.
func wrapKEK(secret, ephemeral, recipient, info []byte) ([]byte, error) {
	salt := append(append([]byte{}, ephemeral...), recipient...)
	kek, err := hkdf.Key(sha256.New, secret, salt, string(info), ContentKeySize)
	if err != nil {
		return nil, fmt.Errorf("bundle: derive the wrapping key: %w", err)
	}
	return kek, nil
}

// WrapContentKey wraps a content key for a device's key-agreement key.
// random supplies the ephemeral key; nil means crypto/rand.
func WrapContentKey(contentKey []byte, recipient *ecdh.PublicKey, info []byte, random io.Reader) (WrappedKey, error) {
	if len(contentKey) != ContentKeySize {
		return WrappedKey{}, fmt.Errorf("bundle: a content key has %d bytes, not %d", len(contentKey), ContentKeySize)
	}
	if recipient.Curve() != ecdh.P256() {
		return WrappedKey{}, errors.New("bundle: the key-agreement key is not a P-256 key")
	}
	if random == nil {
		random = rand.Reader
	}
	ephemeral, err := ecdh.P256().GenerateKey(random)
	if err != nil {
		return WrappedKey{}, fmt.Errorf("bundle: ephemeral key: %w", err)
	}
	secret, err := ephemeral.ECDH(recipient)
	if err != nil {
		return WrappedKey{}, fmt.Errorf("bundle: key agreement: %w", err)
	}
	pub := ephemeral.PublicKey().Bytes()
	kek, err := wrapKEK(secret, pub, recipient.Bytes(), info)
	if err != nil {
		return WrappedKey{}, err
	}
	wrapped, err := aesKeyWrap(kek, contentKey)
	if err != nil {
		return WrappedKey{}, err
	}
	return WrappedKey{EphemeralPublicKey: pub, Wrapped: wrapped}, nil
}

// UnwrapContentKey is the device's side of WrapContentKey, in Go for tests
// and for the vectors the runtime is checked against. A wrong device,
// release or altered bytes fail with PLX-6050.
func UnwrapContentKey(w WrappedKey, device *ecdh.PrivateKey, info []byte) ([]byte, error) {
	fail := func(cause error) error {
		return wrapErr(plxerr.ConfidentialKeyUnavailable, cause, "the content key does not unwrap")
	}
	ephemeral, err := ecdh.P256().NewPublicKey(w.EphemeralPublicKey)
	if err != nil {
		return nil, fail(err)
	}
	secret, err := device.ECDH(ephemeral)
	if err != nil {
		return nil, fail(err)
	}
	kek, err := wrapKEK(secret, w.EphemeralPublicKey, device.PublicKey().Bytes(), info)
	if err != nil {
		return nil, err
	}
	key, err := aesKeyUnwrap(kek, w.Wrapped)
	if err != nil {
		return nil, fail(err)
	}
	return key, nil
}

// kwIV is the default initial value of RFC 3394.
var kwIV = [8]byte{0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6}

// aesKeyWrap wraps key (a multiple of 8 bytes, at least 16) under kek.
func aesKeyWrap(kek, key []byte) ([]byte, error) {
	if len(key) < 16 || len(key)%8 != 0 {
		return nil, errors.New("bundle: a key to wrap is a multiple of 8 bytes, at least 16")
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("bundle: %w", err)
	}
	n := len(key) / 8
	a := kwIV
	r := append([]byte{}, key...)
	var buf [16]byte
	for j := range 6 {
		for i := range n {
			copy(buf[:8], a[:])
			copy(buf[8:], r[8*i:8*i+8])
			block.Encrypt(buf[:], buf[:])
			t := uint64(n*j + i + 1) //nolint:gosec // G115: at most 6n+n, tiny.
			copy(a[:], buf[:8])
			binary.BigEndian.PutUint64(a[:], binary.BigEndian.Uint64(a[:])^t)
			copy(r[8*i:], buf[8:])
		}
	}
	return append(a[:], r...), nil
}

// aesKeyUnwrap reverses aesKeyWrap and checks the integrity value.
func aesKeyUnwrap(kek, wrapped []byte) ([]byte, error) {
	if len(wrapped) < 24 || len(wrapped)%8 != 0 {
		return nil, errors.New("bundle: a wrapped key is a multiple of 8 bytes, at least 24")
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("bundle: %w", err)
	}
	n := len(wrapped)/8 - 1
	var a [8]byte
	copy(a[:], wrapped[:8])
	r := append([]byte{}, wrapped[8:]...)
	var buf [16]byte
	for j := 5; j >= 0; j-- {
		for i := n - 1; i >= 0; i-- {
			t := uint64(n*j + i + 1) //nolint:gosec // G115: at most 6n+n, tiny.
			binary.BigEndian.PutUint64(a[:], binary.BigEndian.Uint64(a[:])^t)
			copy(buf[:8], a[:])
			copy(buf[8:], r[8*i:8*i+8])
			block.Decrypt(buf[:], buf[:])
			copy(a[:], buf[:8])
			copy(r[8*i:], buf[8:])
		}
	}
	if subtle.ConstantTimeCompare(a[:], kwIV[:]) != 1 {
		return nil, errors.New("bundle: the key wrap integrity check failed")
	}
	return r, nil
}
