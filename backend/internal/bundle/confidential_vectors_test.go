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
	"path/filepath"
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// confidentialVectorsPath is read by the runtime's tests.
var confidentialVectorsPath = filepath.Join("..", "..", "..", "schema", "testdata", "confidential", "vectors.json")

// confidentialVectors pin what the server writes and the device opens
// (SEC-053, ADR-0055): sealed bundles, wrapped keys and the damage a
// device must refuse. Bytes are base64, scalars and keys hex.
type confidentialVectors struct {
	Comment string `json:"$comment"`
	// ContentKey seals every bundle below.
	ContentKey string `json:"contentKey"`
	// Plain is the bundle, Sealed its confidential form made with the
	// nonce counter starting at zero.
	Plain  string `json:"plain"`
	Sealed string `json:"sealed"`
	// Refused are sealed bundles a device must refuse, with the error.
	Refused []refusedVector `json:"refused"`
	// Wrap is a content key wrapped for a device.
	Wrap wrapVector `json:"wrap"`
}

type refusedVector struct {
	Name   string `json:"name"`
	Sealed string `json:"sealed"`
	// Key is the content key to open it with; empty means ContentKey.
	Key   string `json:"key,omitempty"`
	Error string `json:"error"`
}

type wrapVector struct {
	// DevicePrivateKey is the 32-byte P-256 scalar of the device's
	// key-agreement key, hex.
	DevicePrivateKey string `json:"devicePrivateKey"`
	// DeviceID, App, Environment and Sequence are the KeyInfo inputs.
	DeviceID    string `json:"deviceId"`
	App         string `json:"app"`
	Environment string `json:"environment"`
	Sequence    int64  `json:"sequence"`
	// EphemeralPublicKey and Wrapped are the server's output, base64.
	EphemeralPublicKey string `json:"ephemeralPublicKey"`
	Wrapped            string `json:"wrapped"`
	// Wrong lists inputs that must not unwrap it.
	Wrong []wrongWrap `json:"wrong"`
}

type wrongWrap struct {
	Name        string `json:"name"`
	DeviceID    string `json:"deviceId,omitempty"`
	App         string `json:"app,omitempty"`
	Environment string `json:"environment,omitempty"`
	Sequence    int64  `json:"sequence,omitempty"`
	// Wrapped replaces the vector's wrapped key when set, base64.
	Wrapped string `json:"wrapped,omitempty"`
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// buildConfidentialVectors makes the vectors. The bundle part is a
// function of the key and the bundle; the wrap part draws an ephemeral key,
// so it is made once and pinned by the file.
func buildConfidentialVectors(t *testing.T, previous *confidentialVectors) confidentialVectors {
	t.Helper()
	plain, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", "testdata", "bundles", "loan-calculator", "loans.pxb"))
	if err != nil {
		t.Fatal(err)
	}
	key := testKey()
	sealed, err := Encrypt(plain, key, &Nonces{})
	if err != nil {
		t.Fatal(err)
	}
	v := confidentialVectors{
		Comment: "Confidential bundle conformance vectors (SEC-053, ADR-0055), written by backend/internal/bundle (go test -update). Bytes are base64, scalars hex. " +
			"A device checks the bundle hash and every section hash, then opens each section with AES-256-GCM (nonce, ciphertext, tag; associated data: container kind, section kind, section ID) and gets `plain` back; a refused bundle fails with the error. " +
			"The wrap part is ECDH-ES (P-256) with HKDF-SHA-256 (salt: ephemeral then device public point; info: KeyInfo) and AES-KW.",
		ContentKey: hex.EncodeToString(key), Plain: b64(plain), Sealed: b64(sealed),
	}
	b, err := readHeader(sealed, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := readDirectory(sealed, b); err != nil {
		t.Fatal(err)
	}
	flipped := bytes.Clone(sealed)
	flipped[len(flipped)-SealOverhead-1] ^= 1
	forged := slices.Clone(b.Sections)
	forged[0].Data = bytes.Clone(forged[0].Data)
	forged[0].Data[len(forged[0].Data)-1] ^= 1
	resealed, err := encode(b.Kind, forged, FlagEncrypted)
	if err != nil {
		t.Fatal(err)
	}
	wrongKey := bytes.Clone(key)
	wrongKey[0] ^= 1
	v.Refused = []refusedVector{
		{Name: "a flipped ciphertext byte fails its section hash first", Sealed: b64(flipped), Error: plxerr.SectionHashMismatch.String()},
		{Name: "a flipped tag with consistent hashes fails to open", Sealed: b64(resealed), Error: plxerr.ConfidentialKeyUnavailable.String()},
		{Name: "another content key fails to open", Sealed: b64(sealed), Key: hex.EncodeToString(wrongKey), Error: plxerr.ConfidentialKeyUnavailable.String()},
		{Name: "a plain bundle is not confidential", Sealed: b64(plain), Error: plxerr.BundleMalformed.String()},
	}
	if previous != nil {
		v.Wrap = previous.Wrap
		return v
	}
	d := make([]byte, 32)
	for i := range d {
		d[i] = byte(0x40 + i)
	}
	device, err := ecdh.P256().NewPrivateKey(d)
	if err != nil {
		t.Fatal(err)
	}
	w := wrapVector{DevicePrivateKey: hex.EncodeToString(d), DeviceID: "device-1", App: "app-1", Environment: "production", Sequence: 7}
	wrapped, err := WrapContentKey(key, device.PublicKey(), KeyInfo(w.DeviceID, w.App, w.Environment, w.Sequence), nil)
	if err != nil {
		t.Fatal(err)
	}
	w.EphemeralPublicKey, w.Wrapped = b64(wrapped.EphemeralPublicKey), b64(wrapped.Wrapped)
	damaged := bytes.Clone(wrapped.Wrapped)
	damaged[3] ^= 1
	w.Wrong = []wrongWrap{
		{Name: "another device", DeviceID: "device-2"},
		{Name: "another app", App: "app-2"},
		{Name: "another environment", Environment: "staging"},
		{Name: "another release", Sequence: 8},
		{Name: "an altered wrapped key", Wrapped: b64(damaged)},
	}
	v.Wrap = w
	return v
}

// Verifies: SEC-053, SEC-052.
// The vectors pin the sealed bundle and the key wrap, so the runtime's
// reader is checked against the server's writer byte for byte.
func TestConfidentialConformanceVectors(t *testing.T) {
	var previous *confidentialVectors
	if golden, err := os.ReadFile(confidentialVectorsPath); err == nil && !*update {
		previous = new(confidentialVectors)
		if err := json.Unmarshal(golden, previous); err != nil {
			t.Fatal(err)
		}
	}
	got := buildConfidentialVectors(t, previous)
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if *update {
		if err := os.MkdirAll(filepath.Dir(confidentialVectorsPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(confidentialVectorsPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	golden, err := os.ReadFile(confidentialVectorsPath)
	if err != nil {
		t.Fatalf("%v; run the test with -update", err)
	}
	if !bytes.Equal(data, golden) {
		t.Fatalf("%s differs; run the test with -update and review", confidentialVectorsPath)
	}
	// The file is what the runtime reads: check it says what it claims.
	key, _ := hex.DecodeString(got.ContentKey)
	sealed, _ := base64.StdEncoding.DecodeString(got.Sealed)
	plain, _ := base64.StdEncoding.DecodeString(got.Plain)
	if out, err := Decrypt(sealed, key); err != nil || !bytes.Equal(out, plain) {
		t.Errorf("the sealed bundle does not open to the plain one: %v", err)
	}
	for _, r := range got.Refused {
		data, _ := base64.StdEncoding.DecodeString(r.Sealed)
		k := key
		if r.Key != "" {
			k, _ = hex.DecodeString(r.Key)
		}
		_, err := Decrypt(data, k)
		wantCode(t, r.Name, err, mustParseCode(t, r.Error))
	}
	checkWrapVector(t, got.Wrap, key)
}

func mustParseCode(t *testing.T, s string) plxerr.Code {
	t.Helper()
	for _, c := range []plxerr.Code{plxerr.SectionHashMismatch, plxerr.ConfidentialKeyUnavailable, plxerr.BundleMalformed} {
		if c.String() == s {
			return c
		}
	}
	t.Fatalf("unknown code %q", s)
	return 0
}

func checkWrapVector(t *testing.T, w wrapVector, key []byte) {
	t.Helper()
	d, _ := hex.DecodeString(w.DevicePrivateKey)
	device, err := ecdh.P256().NewPrivateKey(d)
	if err != nil {
		t.Fatal(err)
	}
	eph, _ := base64.StdEncoding.DecodeString(w.EphemeralPublicKey)
	wrapped, _ := base64.StdEncoding.DecodeString(w.Wrapped)
	got, err := UnwrapContentKey(WrappedKey{EphemeralPublicKey: eph, Wrapped: wrapped}, device, KeyInfo(w.DeviceID, w.App, w.Environment, w.Sequence))
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("the wrapped key does not unwrap: %x, %v", got, err)
	}
	for _, x := range w.Wrong {
		deviceID, app, env, seq, wr := w.DeviceID, w.App, w.Environment, w.Sequence, wrapped
		if x.DeviceID != "" {
			deviceID = x.DeviceID
		}
		if x.App != "" {
			app = x.App
		}
		if x.Environment != "" {
			env = x.Environment
		}
		if x.Sequence != 0 {
			seq = x.Sequence
		}
		if x.Wrapped != "" {
			wr, _ = base64.StdEncoding.DecodeString(x.Wrapped)
		}
		_, err := UnwrapContentKey(WrappedKey{EphemeralPublicKey: eph, Wrapped: wr}, device, KeyInfo(deviceID, app, env, seq))
		wantCode(t, x.Name, err, plxerr.ConfidentialKeyUnavailable)
	}
}
