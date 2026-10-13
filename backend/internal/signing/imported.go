// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package signing

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// The imported backend holds a private key that an issuer generated and
// an operator imports, such as the key Apple issues for the App Attest
// fraud-metric receipt (SEC-123, ADR-0065). The key is kept
// envelope-encrypted: the PKCS#8 is encrypted with AES-256-GCM under a
// fresh data key, and the data key is wrapped through a Crypter, so the
// backend that guards the wrapping key still guards the imported one at
// rest (SEC-106, SEC-120).
//
// Per operation the data key is unwrapped, the key is decrypted into a
// buffer owned by this file, one signature is made and the buffers are
// overwritten. Nothing is cached. The key reaches no log, no error and no
// return value: callers receive signatures and the public half only.
//
// Go cannot promise that no copy of a value survives in memory: the
// parsed key's big integers are cleared and so are the byte buffers this
// code owns, but the runtime's own working copies during a signature are
// beyond its reach. ADR-0065 records that native import into PKCS#11 and
// Vault Transit, which keeps the key out of process memory, follows as its
// own change.

// ES256SignatureSize is the length of a JWS ES256 signature: r and s,
// each 32 bytes, left-padded (RFC 7518 section 3.4).
const ES256SignatureSize = 64

// importedDataKeySize is the size of the AES-256 data key.
const importedDataKeySize = 32

// maxImportedField bounds a role, key ID or Team ID. It is a sanity bound
// on identifiers, not a quota.
const maxImportedField = 64

// importedAADPrefix versions the additional data of the envelope.
const importedAADPrefix = "plux/imported-key/v1"

// ImportedKeyMeta names an imported key as its issuer does: Apple's key
// identifier (the JWT kid) and Team ID (the JWT iss).
type ImportedKeyMeta struct {
	KeyID  string
	TeamID string
}

// ImportedKeyRecord is the stored envelope of one imported key. It holds
// no plaintext: Ciphertext is the PKCS#8 under a data key that only
// WrappedDataKey, opened by the Crypter, reveals.
type ImportedKeyRecord struct {
	Role            string
	KeyID           string
	TeamID          string
	WrappedDataKey  []byte
	WrapKeyID       string
	Nonce           []byte
	Ciphertext      []byte
	PublicKeySHA256 []byte
	CreatedAt       time.Time
}

// ImportedKeyStore persists imported-key envelopes, one per role. An
// implementation is safe for concurrent use.
type ImportedKeyStore interface {
	// GetImportedKey returns the envelope of a role, or an error wrapping
	// ErrNoKey when the role has none.
	GetImportedKey(ctx context.Context, role string) (ImportedKeyRecord, error)
	// PutImportedKey stores an envelope, replacing the role's previous one.
	PutImportedKey(ctx context.Context, rec ImportedKeyRecord) error
}

// ImportedKeyInfo describes a stored key without opening it.
type ImportedKeyInfo struct {
	Role string
	ImportedKeyMeta
	// PublicKeySHA256 is the SHA-256 of the public key's PKIX DER.
	PublicKeySHA256 [sha256.Size]byte
	CreatedAt       time.Time
}

// ImportedOptions configures Imported. Every field is required.
type ImportedOptions struct {
	// Crypter wraps and unwraps the data keys.
	Crypter Crypter
	// Store persists the envelopes.
	Store ImportedKeyStore
	// Rand is the source of data keys and nonces: crypto/rand.Reader in
	// production.
	Rand io.Reader
	// Now supplies the import time.
	Now func() time.Time
}

// Imported is the envelope backend for imported ES256 keys. It is safe for
// concurrent use: it holds no mutable state and caches no key.
type Imported struct {
	crypter Crypter
	store   ImportedKeyStore
	rand    io.Reader
	now     func() time.Time
}

// NewImported returns the backend, or an error when a dependency is
// missing.
func NewImported(o ImportedOptions) (*Imported, error) {
	if o.Crypter == nil || o.Store == nil || o.Rand == nil || o.Now == nil {
		return nil, errors.New("signing: the imported backend needs a crypter, a store, a random source and a clock")
	}
	return &Imported{crypter: o.Crypter, store: o.Store, rand: o.Rand, now: o.Now}, nil
}

// Import stores a private key under a role (SEC-123). pkcs8 is a PKCS#8
// ECDSA P-256 key, DER or a single PEM "PRIVATE KEY" block, as Apple's
// .p8 file; anything else is IMPORTED_KEY_INVALID. A role that already
// holds a key is replaced, which is how a key is rotated.
//
// Import overwrites pkcs8 before it returns, whatever the outcome, so a
// caller must not reuse the slice.
func (i *Imported) Import(ctx context.Context, role string, pkcs8 []byte, meta ImportedKeyMeta) error {
	defer clearBytes(pkcs8)
	if err := validateImportedIdentity(role, meta); err != nil {
		return err
	}
	der, err := privateKeyDER(pkcs8)
	if err != nil {
		return err
	}
	defer clearBytes(der)
	key, err := parseP256(der)
	if err != nil {
		return err
	}
	defer clearKey(key)
	digest, err := publicKeyDigest(&key.PublicKey)
	if err != nil {
		return err
	}
	rec, err := i.seal(ctx, role, meta, der, digest)
	if err != nil {
		return err
	}
	if err := i.store.PutImportedKey(ctx, rec); err != nil {
		return fmt.Errorf("signing: store the imported key %q: %w", role, err)
	}
	return nil
}

// seal encrypts the PKCS#8 under a fresh data key and wraps that key.
func (i *Imported) seal(ctx context.Context, role string, meta ImportedKeyMeta, der, digest []byte) (ImportedKeyRecord, error) {
	dataKey := make([]byte, importedDataKeySize)
	defer clearBytes(dataKey)
	if _, err := io.ReadFull(i.rand, dataKey); err != nil {
		return ImportedKeyRecord{}, fmt.Errorf("signing: generate a data key: %w", err)
	}
	aead, err := newAEAD(dataKey)
	if err != nil {
		return ImportedKeyRecord{}, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(i.rand, nonce); err != nil {
		return ImportedKeyRecord{}, fmt.Errorf("signing: generate a nonce: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, der, importedAAD(role, meta))
	wrapped, wrapKeyID, err := i.crypter.Wrap(ctx, dataKey)
	if err != nil {
		return ImportedKeyRecord{}, fmt.Errorf("signing: wrap the data key of the imported key %q: %w", role, err)
	}
	return ImportedKeyRecord{
		Role: role, KeyID: meta.KeyID, TeamID: meta.TeamID,
		WrappedDataKey: wrapped, WrapKeyID: wrapKeyID, Nonce: nonce, Ciphertext: ciphertext,
		PublicKeySHA256: digest, CreatedAt: i.now().UTC(),
	}, nil
}

// SignES256 signs a SHA-256 digest with the key of a role and returns the
// JWS form of the signature, r followed by s, 64 bytes (SEC-123). The key
// is decrypted for this call only.
func (i *Imported) SignES256(ctx context.Context, role string, digest []byte) ([]byte, error) {
	if len(digest) != sha256.Size {
		return nil, fmt.Errorf("signing: ES256 signs a %d-byte digest, not %d bytes", sha256.Size, len(digest))
	}
	key, _, err := i.open(ctx, role)
	if err != nil {
		return nil, err
	}
	defer clearKey(key)
	// Since Go 1.26 the reader is ignored and a secure source is always
	// used; it is passed so that GODEBUG=cryptocustomrand=1 keeps a test
	// deterministic.
	r, s, err := ecdsa.Sign(i.rand, key, digest)
	if err != nil {
		return nil, fmt.Errorf("signing: sign with the imported key %q: %w", role, err)
	}
	sig := make([]byte, ES256SignatureSize)
	r.FillBytes(sig[:ES256SignatureSize/2])
	s.FillBytes(sig[ES256SignatureSize/2:])
	return sig, nil
}

// PublicKey returns the public half of the key of a role and its
// metadata, for verifying what it signed and for the Apple simulator. The
// key is opened to derive it, so a damaged envelope is found here too.
func (i *Imported) PublicKey(ctx context.Context, role string) (*ecdsa.PublicKey, ImportedKeyMeta, error) {
	key, rec, err := i.open(ctx, role)
	if err != nil {
		return nil, ImportedKeyMeta{}, err
	}
	defer clearKey(key)
	pub := key.PublicKey
	return &pub, ImportedKeyMeta{KeyID: rec.KeyID, TeamID: rec.TeamID}, nil
}

// Info returns what is stored about a role without opening the key, so
// that it needs no access to the wrapping key.
func (i *Imported) Info(ctx context.Context, role string) (ImportedKeyInfo, error) {
	if err := validateRole(role); err != nil {
		return ImportedKeyInfo{}, err
	}
	rec, err := i.store.GetImportedKey(ctx, role)
	if err != nil {
		return ImportedKeyInfo{}, fmt.Errorf("signing: imported key %q: %w", role, err)
	}
	info := ImportedKeyInfo{Role: rec.Role, ImportedKeyMeta: ImportedKeyMeta{KeyID: rec.KeyID, TeamID: rec.TeamID}, CreatedAt: rec.CreatedAt}
	if len(rec.PublicKeySHA256) != sha256.Size {
		return ImportedKeyInfo{}, invalidImportedKey(role, nil)
	}
	copy(info.PublicKeySHA256[:], rec.PublicKeySHA256)
	return info, nil
}

// open loads, unwraps and decrypts the key of a role, and checks it
// against the digest stored with it. Any failure to open an envelope that
// exists is IMPORTED_KEY_INVALID, and the message names the role only. The
// caller clears the returned key.
func (i *Imported) open(ctx context.Context, role string) (*ecdsa.PrivateKey, ImportedKeyRecord, error) {
	if err := validateRole(role); err != nil {
		return nil, ImportedKeyRecord{}, err
	}
	rec, err := i.store.GetImportedKey(ctx, role)
	if err != nil {
		return nil, ImportedKeyRecord{}, fmt.Errorf("signing: imported key %q: %w", role, err)
	}
	if rec.Role != role || len(rec.PublicKeySHA256) != sha256.Size {
		return nil, ImportedKeyRecord{}, invalidImportedKey(role, nil)
	}
	dataKey, err := i.crypter.Unwrap(ctx, rec.WrappedDataKey, rec.WrapKeyID)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ImportedKeyRecord{}, fmt.Errorf("signing: unwrap the data key of the imported key %q: %w", role, err)
		}
		return nil, ImportedKeyRecord{}, invalidImportedKey(role, err)
	}
	defer clearBytes(dataKey)
	if len(dataKey) != importedDataKeySize {
		return nil, ImportedKeyRecord{}, invalidImportedKey(role, nil)
	}
	aead, err := newAEAD(dataKey)
	if err != nil || len(rec.Nonce) != aead.NonceSize() {
		return nil, ImportedKeyRecord{}, invalidImportedKey(role, nil)
	}
	der, err := aead.Open(nil, rec.Nonce, rec.Ciphertext, importedAAD(role, ImportedKeyMeta{KeyID: rec.KeyID, TeamID: rec.TeamID}))
	if err != nil {
		return nil, ImportedKeyRecord{}, invalidImportedKey(role, nil)
	}
	defer clearBytes(der)
	key, err := parseP256(der)
	if err != nil {
		return nil, ImportedKeyRecord{}, invalidImportedKey(role, nil)
	}
	digest, err := publicKeyDigest(&key.PublicKey)
	if err != nil || subtle.ConstantTimeCompare(digest, rec.PublicKeySHA256) != 1 {
		clearKey(key)
		return nil, ImportedKeyRecord{}, invalidImportedKey(role, nil)
	}
	return key, rec, nil
}

// invalidImportedKey is the error for a key that cannot be used. It names
// the role and, when a backend failed, carries that failure; it never
// carries key material.
func invalidImportedKey(role string, cause error) error {
	if cause != nil {
		return plxerr.Wrap(plxerr.ImportedKeyInvalid, cause, "the imported key %q cannot be opened", role)
	}
	return plxerr.New(plxerr.ImportedKeyInvalid, "the imported key %q cannot be opened", role)
}

// privateKeyDER returns the DER of the key in the input, which is DER or
// exactly one PEM "PRIVATE KEY" block. A PEM input with a second block,
// headers or trailing data is refused. The returned slice is the input
// itself for DER, so the caller clears it once either way.
func privateKeyDER(in []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(in)
	if !bytes.HasPrefix(trimmed, []byte("-----BEGIN")) {
		return in, nil
	}
	block, rest := pem.Decode(trimmed)
	if block == nil || block.Type != "PRIVATE KEY" || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		if block != nil {
			clearBytes(block.Bytes)
		}
		return nil, plxerr.New(plxerr.ImportedKeyInvalid, "the key file is not a single PEM PRIVATE KEY block")
	}
	return block.Bytes, nil
}

// parseP256 parses a PKCS#8 key and accepts an ECDSA P-256 key only
// (ADR-0065). The parser's own error is dropped: it must not describe the
// key.
func parseP256(der []byte) (*ecdsa.PrivateKey, error) {
	// The standard parser ignores bytes after the key, so the outer
	// structure is checked to end where the input does.
	var outer asn1.RawValue
	rest, err := asn1.Unmarshal(der, &outer)
	if err != nil || len(rest) != 0 {
		return nil, plxerr.New(plxerr.ImportedKeyInvalid, "the key is not a PKCS#8 private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, plxerr.New(plxerr.ImportedKeyInvalid, "the key is not a PKCS#8 private key")
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, plxerr.New(plxerr.ImportedKeyInvalid, "the key is not an ECDSA P-256 private key")
	}
	return key, nil
}

// publicKeyDigest is the SHA-256 of the public key's PKIX DER, the value
// stored beside the envelope and shown to the operator.
func publicKeyDigest(pub *ecdsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, plxerr.New(plxerr.ImportedKeyInvalid, "the public key cannot be encoded")
	}
	sum := sha256.Sum256(der)
	return sum[:], nil
}

// importedAAD binds the envelope to its role and identity, so a
// ciphertext copied to another row, or a row whose key ID or Team ID was
// edited, fails to open. The fields are restricted to characters that
// exclude NUL, so the encoding is unambiguous.
func importedAAD(role string, meta ImportedKeyMeta) []byte {
	return []byte(importedAADPrefix + "\x00" + role + "\x00" + meta.KeyID + "\x00" + meta.TeamID)
}

// validateImportedIdentity checks the names given with a key.
func validateImportedIdentity(role string, meta ImportedKeyMeta) error {
	if err := validateRole(role); err != nil {
		return err
	}
	if !identifier(meta.KeyID) {
		return plxerr.New(plxerr.ImportedKeyInvalid, "the key ID is empty, too long or has characters outside A-Z a-z 0-9 . _ -")
	}
	if !identifier(meta.TeamID) {
		return plxerr.New(plxerr.ImportedKeyInvalid, "the Team ID is empty, too long or has characters outside A-Z a-z 0-9 . _ -")
	}
	return nil
}

// validateRole accepts lower-case letters, digits and hyphens, at most 64.
func validateRole(role string) error {
	if len(role) == 0 || len(role) > maxImportedField || role[0] == '-' {
		return plxerr.New(plxerr.ImportedKeyInvalid, "the role is empty, too long or starts with a hyphen")
	}
	for k := range len(role) {
		c := role[k]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return plxerr.New(plxerr.ImportedKeyInvalid, "the role has characters outside a-z 0-9 -")
		}
	}
	return nil
}

// identifier reports whether s is 1 to 64 characters of A-Z a-z 0-9 . _ -.
func identifier(s string) bool {
	if len(s) == 0 || len(s) > maxImportedField {
		return false
	}
	for k := range len(s) {
		c := s[k]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '.' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

// clearBytes overwrites a buffer holding secret material.
func clearBytes(b []byte) { clear(b) }

// clearKey overwrites the words of a private scalar that big.Int exposes.
// D is deprecated for callers that build keys; here it is the only handle
// on the words of a key this package parsed and is about to discard.
func clearKey(k *ecdsa.PrivateKey) {
	//nolint:staticcheck // SA1019: the only handle on the scalar's words
	if k != nil && k.D != nil {
		clear(k.D.Bits()) //nolint:staticcheck // SA1019: as above
	}
}
