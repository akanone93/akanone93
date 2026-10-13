// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package signing_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// roleReceipt is the role the tests import under.
const roleReceipt = "apple-appattest-receipt"

// memoryKeys is an in-memory ImportedKeyStore that tests can damage.
type memoryKeys struct {
	mu   sync.Mutex
	recs map[string]signing.ImportedKeyRecord
}

func newMemoryKeys() *memoryKeys { return &memoryKeys{recs: map[string]signing.ImportedKeyRecord{}} }

func (m *memoryKeys) GetImportedKey(_ context.Context, role string) (signing.ImportedKeyRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.recs[role]
	if !ok {
		return signing.ImportedKeyRecord{}, signing.ErrNoKey
	}
	return cloneRecord(rec), nil
}

func (m *memoryKeys) PutImportedKey(_ context.Context, rec signing.ImportedKeyRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recs[rec.Role] = cloneRecord(rec)
	return nil
}

// damage applies f to the stored record of a role.
func (m *memoryKeys) damage(role string, f func(*signing.ImportedKeyRecord)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := cloneRecord(m.recs[role])
	f(&rec)
	m.recs[role] = rec
}

func (m *memoryKeys) record() (signing.ImportedKeyRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.recs[roleReceipt]
	return cloneRecord(rec), ok
}

func cloneRecord(r signing.ImportedKeyRecord) signing.ImportedKeyRecord {
	r.WrappedDataKey = bytes.Clone(r.WrappedDataKey)
	r.Nonce = bytes.Clone(r.Nonce)
	r.Ciphertext = bytes.Clone(r.Ciphertext)
	r.PublicKeySHA256 = bytes.Clone(r.PublicKeySHA256)
	return r
}

// fixture is an Imported backend over a file Crypter and a memory store.
type fixture struct {
	imp     *signing.Imported
	store   *memoryKeys
	crypter signing.Crypter
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{store: newMemoryKeys(), crypter: backend(t)}
	f.imp = f.with(t, f.crypter, rand.Reader)
	return f
}

// with builds another Imported over the same store.
func (f *fixture) with(t *testing.T, c signing.Crypter, r interface{ Read([]byte) (int, error) }) *signing.Imported {
	t.Helper()
	imp, err := signing.NewImported(signing.ImportedOptions{
		Crypter: c, Store: f.store, Rand: r, Now: func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return imp
}

// p256 is a generated key with its encodings.
type p256 struct {
	key *ecdsa.PrivateKey
	der []byte
}

func newP256(t *testing.T) p256 {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return p256{key: key, der: der}
}

// scalar returns the private scalar, 32 bytes.
func scalar(k *ecdsa.PrivateKey) []byte {
	b, err := k.Bytes()
	if err != nil {
		panic(err)
	}
	return b
}

// pemOf is the PEM form of a PKCS#8 key, as Apple's .p8 file.
func pemOf(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// secrets returns every encoding of a key that must never appear in an
// error or an output.
func (k p256) secrets() []string {
	d := scalar(k.key)
	return []string{
		string(k.der), hex.EncodeToString(k.der), base64.StdEncoding.EncodeToString(k.der),
		string(d), hex.EncodeToString(d), base64.StdEncoding.EncodeToString(d),
	}
}

// noSecret fails when an error mentions any encoding of the key.
func noSecret(t *testing.T, err error, secrets []string) {
	t.Helper()
	if err == nil {
		return
	}
	for _, s := range secrets {
		if strings.Contains(err.Error(), s) {
			t.Errorf("an error carries key material: %q", err)
		}
	}
}

func wantInvalid(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, plxerr.Sentinel(plxerr.ImportedKeyInvalid)) {
		t.Fatalf("error = %v, want PLX-6081", err)
	}
}

var meta = signing.ImportedKeyMeta{KeyID: "ABC123DEFG", TeamID: "TEAM123456"}

func digestOf(msg string) []byte {
	sum := sha256.Sum256([]byte(msg))
	return sum[:]
}

// verifyJWS checks a 64-byte r‖s signature.
func verifyJWS(pub *ecdsa.PublicKey, digest, sig []byte) bool {
	if len(sig) != signing.ES256SignatureSize {
		return false
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	return ecdsa.Verify(pub, digest, r, s)
}

// Verifies: SEC-123.
// A key imported as DER or as Apple's PEM signs, and the signature
// verifies in JWS form with the public key the backend reports.
func TestImported_RoundTrip_SEC_123(t *testing.T) {
	t.Parallel()
	for name, encode := range map[string]func([]byte) []byte{
		"DER": bytes.Clone,
		"PEM": pemOf,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			k := newP256(t)
			ctx := context.Background()
			if err := f.imp.Import(ctx, roleReceipt, encode(k.der), meta); err != nil {
				t.Fatalf("Import: %v", err)
			}
			digest := digestOf("header.claims")
			sig, err := f.imp.SignES256(ctx, roleReceipt, digest)
			if err != nil {
				t.Fatalf("SignES256: %v", err)
			}
			if len(sig) != signing.ES256SignatureSize {
				t.Fatalf("signature is %d bytes", len(sig))
			}
			pub, got, err := f.imp.PublicKey(ctx, roleReceipt)
			if err != nil || got != meta {
				t.Fatalf("PublicKey = %v, %v", got, err)
			}
			if !pub.Equal(&k.key.PublicKey) || !verifyJWS(pub, digest, sig) || !verifyJWS(&k.key.PublicKey, digest, sig) {
				t.Error("the signature does not verify under the imported key")
			}
			if verifyJWS(pub, digestOf("other"), sig) {
				t.Error("the signature verifies another digest")
			}
			info, err := f.imp.Info(ctx, roleReceipt)
			pubDER, _ := x509.MarshalPKIXPublicKey(&k.key.PublicKey)
			want := sha256.Sum256(pubDER)
			if err != nil || info.PublicKeySHA256 != want || info.KeyID != meta.KeyID || info.TeamID != meta.TeamID || info.Role != roleReceipt {
				t.Errorf("Info = %+v, %v", info, err)
			}
		})
	}
}

// Verifies: SEC-123.
// Signatures are left-padded to 64 bytes whatever r and s are, and two
// signatures of one digest differ (the signature is randomised).
func TestImported_SignatureShape_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	k := newP256(t)
	ctx := context.Background()
	if err := f.imp.Import(ctx, roleReceipt, k.der, meta); err != nil {
		t.Fatal(err)
	}
	pub := &k.key.PublicKey
	var previous []byte
	for i := range 64 {
		digest := digestOf(fmt.Sprintf("%d", i))
		sig, err := f.imp.SignES256(ctx, roleReceipt, digest)
		if err != nil || !verifyJWS(pub, digest, sig) {
			t.Fatalf("signature %d: %v", i, err)
		}
		if bytes.Equal(sig, previous) {
			t.Error("two signatures are equal")
		}
		previous = sig
	}
}

// Verifies: SEC-123.
// Anything but an ECDSA P-256 PKCS#8 key is IMPORTED_KEY_INVALID, nothing
// is stored, and no error repeats the key.
func TestImported_RejectsOtherKeys_SEC_123(t *testing.T) {
	t.Parallel()
	good := newP256(t)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaDER, _ := x509.MarshalPKCS8PrivateKey(rsaKey)
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p384DER, _ := x509.MarshalPKCS8PrivateKey(p384)
	_, edKey, _ := ed25519.GenerateKey(rand.Reader)
	edDER, _ := x509.MarshalPKCS8PrivateKey(edKey)
	sec1, _ := x509.MarshalECPrivateKey(good.key)
	cases := map[string][]byte{
		"RSA":                 rsaDER,
		"P-384":               p384DER,
		"Ed25519":             edDER,
		"SEC1 not PKCS#8":     sec1,
		"garbage":             []byte("this is not a key at all"),
		"empty":               nil,
		"truncated":           good.der[:len(good.der)-9],
		"half":                good.der[:len(good.der)/2],
		"trailing bytes":      append(bytes.Clone(good.der), 0x00),
		"two PEM blocks":      append(pemOf(good.der), pemOf(good.der)...),
		"PEM and garbage":     append(pemOf(good.der), []byte("junk\n")...),
		"wrong PEM type":      pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1}),
		"PEM with headers":    pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Headers: map[string]string{"k": "v"}, Bytes: good.der}),
		"PEM with RSA":        pemOf(rsaDER),
		"PEM truncated body":  pemOf(good.der[:40]),
		"PEM unterminated":    pemOf(good.der)[:80],
		"public key as PKCS8": mustPKIX(t, &good.key.PublicKey),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			err := f.imp.Import(context.Background(), roleReceipt, bytes.Clone(input), meta)
			wantInvalid(t, err)
			noSecret(t, err, good.secrets())
			if _, ok := f.store.record(); ok {
				t.Error("a rejected key was stored")
			}
		})
	}
}

func mustPKIX(t *testing.T, pub *ecdsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// Verifies: SEC-123.
// A role, key ID or Team ID outside the allowed characters is refused, so
// they can be bound into the envelope unambiguously.
func TestImported_RejectsBadNames_SEC_123(t *testing.T) {
	t.Parallel()
	k := newP256(t)
	long := strings.Repeat("a", 65)
	cases := []struct {
		name string
		role string
		meta signing.ImportedKeyMeta
	}{
		{"empty role", "", meta},
		{"upper-case role", "Apple", meta},
		{"role with NUL", "a\x00b", meta},
		{"role with slash", "a/b", meta},
		{"leading hyphen", "-a", meta},
		{"long role", long, meta},
		{"empty key ID", roleReceipt, signing.ImportedKeyMeta{TeamID: "T"}},
		{"empty team ID", roleReceipt, signing.ImportedKeyMeta{KeyID: "K"}},
		{"key ID with NUL", roleReceipt, signing.ImportedKeyMeta{KeyID: "K\x00", TeamID: "T"}},
		{"team ID with space", roleReceipt, signing.ImportedKeyMeta{KeyID: "K", TeamID: "T T"}},
		{"long key ID", roleReceipt, signing.ImportedKeyMeta{KeyID: long, TeamID: "T"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			wantInvalid(t, f.imp.Import(context.Background(), tt.role, bytes.Clone(k.der), tt.meta))
			if len(f.store.recs) != 0 {
				t.Error("a rejected key was stored")
			}
		})
	}
	f := newFixture(t)
	if _, err := f.imp.SignES256(context.Background(), "Bad Role", digestOf("x")); err == nil {
		t.Error("signing accepted an invalid role")
	}
}

// Verifies: SEC-123.
// Import overwrites the buffer it is given, whatever the outcome.
func TestImported_ClearsItsInput_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	k := newP256(t)
	for name, input := range map[string][]byte{"DER": bytes.Clone(k.der), "PEM": pemOf(k.der), "garbage": []byte("not a key")} {
		if err := f.imp.Import(context.Background(), roleReceipt, input, meta); (name == "garbage") == (err == nil) {
			t.Errorf("%s: Import = %v", name, err)
		}
		if !bytes.Equal(input, make([]byte, len(input))) {
			t.Errorf("%s: the input buffer still holds its bytes", name)
		}
	}
}

// Verifies: SEC-123, SEC-106.
// The stored envelope holds no plaintext, a fresh data key and nonce are
// used for every import, and the record names the wrapping key.
func TestImported_EnvelopeIsSealed_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	k := newP256(t)
	ctx := context.Background()
	var recs []signing.ImportedKeyRecord
	for range 2 {
		if err := f.imp.Import(ctx, roleReceipt, bytes.Clone(k.der), meta); err != nil {
			t.Fatal(err)
		}
		rec, _ := f.store.record()
		recs = append(recs, rec)
	}
	rec := recs[0]
	d := scalar(k.key)
	for name, field := range map[string][]byte{"ciphertext": rec.Ciphertext, "wrapped key": rec.WrappedDataKey, "nonce": rec.Nonce} {
		if bytes.Contains(field, k.der) || bytes.Contains(field, d) {
			t.Errorf("the %s holds the key", name)
		}
	}
	if rec.Role != roleReceipt || rec.KeyID != meta.KeyID || rec.TeamID != meta.TeamID || rec.WrapKeyID == "" ||
		len(rec.Nonce) != 12 || len(rec.PublicKeySHA256) != 32 || !rec.CreatedAt.Equal(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("record = %+v", rec)
	}
	if bytes.Equal(recs[0].Ciphertext, recs[1].Ciphertext) || bytes.Equal(recs[0].WrappedDataKey, recs[1].WrappedDataKey) ||
		bytes.Equal(recs[0].Nonce, recs[1].Nonce) {
		t.Error("two imports share a data key or a nonce")
	}
}

// Verifies: SEC-123.
// Damage to any part of the envelope, or to the metadata bound into it,
// is IMPORTED_KEY_INVALID for signing and for the public key, and the
// message names the role and no key material.
func TestImported_DamagedEnvelope_SEC_123(t *testing.T) {
	t.Parallel()
	flip := func(b []byte) []byte { c := bytes.Clone(b); c[len(c)/2] ^= 0x01; return c }
	cases := map[string]func(*signing.ImportedKeyRecord){
		"ciphertext":        func(r *signing.ImportedKeyRecord) { r.Ciphertext = flip(r.Ciphertext) },
		"truncated cipher":  func(r *signing.ImportedKeyRecord) { r.Ciphertext = r.Ciphertext[:10] },
		"empty cipher":      func(r *signing.ImportedKeyRecord) { r.Ciphertext = nil },
		"nonce":             func(r *signing.ImportedKeyRecord) { r.Nonce = flip(r.Nonce) },
		"short nonce":       func(r *signing.ImportedKeyRecord) { r.Nonce = r.Nonce[:4] },
		"wrapped key":       func(r *signing.ImportedKeyRecord) { r.WrappedDataKey = flip(r.WrappedDataKey) },
		"empty wrapped key": func(r *signing.ImportedKeyRecord) { r.WrappedDataKey = nil },
		"wrap key ID":       func(r *signing.ImportedKeyRecord) { r.WrapKeyID = "other" },
		"public key digest": func(r *signing.ImportedKeyRecord) { r.PublicKeySHA256 = flip(r.PublicKeySHA256) },
		"short digest":      func(r *signing.ImportedKeyRecord) { r.PublicKeySHA256 = r.PublicKeySHA256[:8] },
		"key ID":            func(r *signing.ImportedKeyRecord) { r.KeyID = "OTHER" },
		"team ID":           func(r *signing.ImportedKeyRecord) { r.TeamID = "OTHER" },
		"stored role":       func(r *signing.ImportedKeyRecord) { r.Role = "other-role" },
	}
	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			k := newP256(t)
			ctx := context.Background()
			if err := f.imp.Import(ctx, roleReceipt, bytes.Clone(k.der), meta); err != nil {
				t.Fatal(err)
			}
			f.store.damage(roleReceipt, damage)
			_, err := f.imp.SignES256(ctx, roleReceipt, digestOf("x"))
			wantInvalid(t, err)
			noSecret(t, err, k.secrets())
			if !strings.Contains(err.Error(), roleReceipt) {
				t.Errorf("the error does not name the role: %v", err)
			}
			_, _, err = f.imp.PublicKey(ctx, roleReceipt)
			wantInvalid(t, err)
		})
	}
}

// Verifies: SEC-123.
// A digest stored for another key is refused even when the envelope
// itself decrypts: the key is checked against the digest recorded at
// import.
func TestImported_DigestMismatch_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	if err := f.imp.Import(ctx, roleReceipt, newP256(t).der, meta); err != nil {
		t.Fatal(err)
	}
	other := sha256.Sum256([]byte("another public key"))
	f.store.damage(roleReceipt, func(r *signing.ImportedKeyRecord) { r.PublicKeySHA256 = other[:] })
	_, err := f.imp.SignES256(ctx, roleReceipt, digestOf("x"))
	wantInvalid(t, err)
}

// Verifies: SEC-123.
// A copy of one role's row under another role does not open.
func TestImported_RowCopiedToAnotherRole_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	if err := f.imp.Import(ctx, roleReceipt, newP256(t).der, meta); err != nil {
		t.Fatal(err)
	}
	rec, _ := f.store.record()
	rec.Role = "other-role"
	if err := f.store.PutImportedKey(ctx, rec); err != nil {
		t.Fatal(err)
	}
	_, err := f.imp.SignES256(ctx, "other-role", digestOf("x"))
	wantInvalid(t, err)
}

// Verifies: SEC-123.
// Another Crypter cannot open the envelope.
func TestImported_WrongCrypter_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	k := newP256(t)
	ctx := context.Background()
	if err := f.imp.Import(ctx, roleReceipt, bytes.Clone(k.der), meta); err != nil {
		t.Fatal(err)
	}
	other := f.with(t, backend(t), rand.Reader)
	_, err := other.SignES256(ctx, roleReceipt, digestOf("x"))
	wantInvalid(t, err)
	noSecret(t, err, k.secrets())
	_, _, err = other.PublicKey(ctx, roleReceipt)
	wantInvalid(t, err)
}

// shortCrypter returns a data key of the wrong size.
type shortCrypter struct{ signing.Crypter }

func (shortCrypter) Unwrap(context.Context, []byte, string) ([]byte, error) {
	return []byte("short"), nil
}

// failingCrypter fails every call.
type failingCrypter struct{ err error }

func (c failingCrypter) Wrap(context.Context, []byte) ([]byte, string, error) { return nil, "", c.err }

func (c failingCrypter) Unwrap(context.Context, []byte, string) ([]byte, error) {
	return nil, c.err
}

// Verifies: SEC-123.
// A Crypter that returns a data key of the wrong size, or fails, cannot
// produce a signature; a failure to wrap on import stores nothing; and a
// cancelled request is not reported as an invalid key.
func TestImported_CrypterFailures_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	if err := f.imp.Import(ctx, roleReceipt, newP256(t).der, meta); err != nil {
		t.Fatal(err)
	}
	_, err := f.with(t, shortCrypter{f.crypter}, rand.Reader).SignES256(ctx, roleReceipt, digestOf("x"))
	wantInvalid(t, err)
	down := errors.New("backend down")
	_, err = f.with(t, failingCrypter{down}, rand.Reader).SignES256(ctx, roleReceipt, digestOf("x"))
	wantInvalid(t, err)
	if !errors.Is(err, down) {
		t.Error("the backend failure is not in the chain")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = f.with(t, failingCrypter{context.Canceled}, rand.Reader).SignES256(cancelled, roleReceipt, digestOf("x"))
	if err == nil || errors.Is(err, plxerr.Sentinel(plxerr.ImportedKeyInvalid)) || !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled request = %v", err)
	}
	fresh := newFixture(t)
	err = fresh.with(t, failingCrypter{down}, rand.Reader).Import(ctx, roleReceipt, newP256(t).der, meta)
	if !errors.Is(err, down) {
		t.Errorf("Import with a failing crypter = %v", err)
	}
	if _, ok := fresh.store.record(); ok {
		t.Error("an envelope was stored without a wrapped key")
	}
}

// failingReader fails every read.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

// Verifies: SEC-123.
// A failing random source stops an import and stores nothing.
func TestImported_RandomnessFailure_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	imp := f.with(t, f.crypter, failingReader{})
	if err := imp.Import(context.Background(), roleReceipt, newP256(t).der, meta); err == nil {
		t.Fatal("Import succeeded without entropy")
	}
	if _, ok := f.store.record(); ok {
		t.Error("an envelope was stored")
	}
}

// Verifies: SEC-123.
// A role with no key is ErrNoKey, which is not an invalid key; and a
// digest that is not 32 bytes is refused before any key is opened.
func TestImported_MissingAndBadDigest_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	_, err := f.imp.SignES256(ctx, roleReceipt, digestOf("x"))
	if !errors.Is(err, signing.ErrNoKey) || errors.Is(err, plxerr.Sentinel(plxerr.ImportedKeyInvalid)) {
		t.Errorf("SignES256 without a key = %v", err)
	}
	if _, _, err := f.imp.PublicKey(ctx, roleReceipt); !errors.Is(err, signing.ErrNoKey) {
		t.Errorf("PublicKey without a key = %v", err)
	}
	if _, err := f.imp.Info(ctx, roleReceipt); !errors.Is(err, signing.ErrNoKey) {
		t.Errorf("Info without a key = %v", err)
	}
	if err := f.imp.Import(ctx, roleReceipt, newP256(t).der, meta); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 31, 33, 64} {
		if _, err := f.imp.SignES256(ctx, roleReceipt, make([]byte, n)); err == nil {
			t.Errorf("a %d-byte digest was signed", n)
		}
	}
}

// Verifies: SEC-123.
// Importing a role again replaces its key: that is rotation. The old key's
// signatures no longer come from the role.
func TestImported_Rotation_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	first, second := newP256(t), newP256(t)
	if err := f.imp.Import(ctx, roleReceipt, bytes.Clone(first.der), meta); err != nil {
		t.Fatal(err)
	}
	next := signing.ImportedKeyMeta{KeyID: "NEWKEY0001", TeamID: meta.TeamID}
	if err := f.imp.Import(ctx, roleReceipt, bytes.Clone(second.der), next); err != nil {
		t.Fatal(err)
	}
	digest := digestOf("x")
	sig, err := f.imp.SignES256(ctx, roleReceipt, digest)
	if err != nil {
		t.Fatal(err)
	}
	pub, got, err := f.imp.PublicKey(ctx, roleReceipt)
	if err != nil || got != next || !pub.Equal(&second.key.PublicKey) {
		t.Fatalf("PublicKey = %v, %v", got, err)
	}
	if !verifyJWS(&second.key.PublicKey, digest, sig) || verifyJWS(&first.key.PublicKey, digest, sig) {
		t.Error("the role still signs with the old key")
	}
	if len(f.store.recs) != 1 {
		t.Errorf("%d envelopes after a rotation", len(f.store.recs))
	}
}

// Verifies: SEC-123.
// Two roles hold independent keys.
func TestImported_RolesAreIndependent_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	a, b := newP256(t), newP256(t)
	if err := f.imp.Import(ctx, "role-a", bytes.Clone(a.der), meta); err != nil {
		t.Fatal(err)
	}
	if err := f.imp.Import(ctx, "role-b", bytes.Clone(b.der), meta); err != nil {
		t.Fatal(err)
	}
	pa, _, _ := f.imp.PublicKey(ctx, "role-a")
	pb, _, _ := f.imp.PublicKey(ctx, "role-b")
	if !pa.Equal(&a.key.PublicKey) || !pb.Equal(&b.key.PublicKey) {
		t.Error("a role returned another role's key")
	}
}

// Verifies: SEC-123.
// Many goroutines sign at once with one backend, and every signature
// verifies. Run with -race.
func TestImported_ConcurrentSigning_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	k := newP256(t)
	ctx := context.Background()
	if err := f.imp.Import(ctx, roleReceipt, bytes.Clone(k.der), meta); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range 8 {
				digest := digestOf(fmt.Sprintf("%d/%d", g, n))
				sig, err := f.imp.SignES256(ctx, roleReceipt, digest)
				if err != nil {
					errs <- err
					return
				}
				if !verifyJWS(&k.key.PublicKey, digest, sig) {
					errs <- errors.New("a concurrent signature does not verify")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// Verifies: SEC-123.
// Signing and rotating at once is safe, and a signature always comes from
// one whole key.
func TestImported_SignWhileRotating_SEC_123(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	keys := []p256{newP256(t), newP256(t)}
	if err := f.imp.Import(ctx, roleReceipt, bytes.Clone(keys[0].der), meta); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for n := range 20 {
			if err := f.imp.Import(ctx, roleReceipt, bytes.Clone(keys[n%2].der), meta); err != nil {
				t.Error(err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for n := range 40 {
			digest := digestOf(fmt.Sprintf("%d", n))
			sig, err := f.imp.SignES256(ctx, roleReceipt, digest)
			if err != nil {
				t.Error(err)
				return
			}
			if !verifyJWS(&keys[0].key.PublicKey, digest, sig) && !verifyJWS(&keys[1].key.PublicKey, digest, sig) {
				t.Error("a signature verifies under neither key")
			}
		}
	}()
	wg.Wait()
}

// Verifies: SEC-123.
// A backend is not built without its dependencies.
func TestNewImported_RequiresDependencies_SEC_123(t *testing.T) {
	t.Parallel()
	good := signing.ImportedOptions{Crypter: backend(t), Store: newMemoryKeys(), Rand: rand.Reader, Now: time.Now}
	if _, err := signing.NewImported(good); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*signing.ImportedOptions){
		"crypter": func(o *signing.ImportedOptions) { o.Crypter = nil },
		"store":   func(o *signing.ImportedOptions) { o.Store = nil },
		"random":  func(o *signing.ImportedOptions) { o.Rand = nil },
		"clock":   func(o *signing.ImportedOptions) { o.Now = nil },
	} {
		o := good
		mutate(&o)
		if _, err := signing.NewImported(o); err == nil {
			t.Errorf("NewImported without a %s succeeded", name)
		}
	}
}
