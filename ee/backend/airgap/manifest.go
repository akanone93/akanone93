// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package airgap

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/edition/licensefile"
)

// ManifestPath is the archive path of the signed manifest.
const ManifestPath = "manifest.json"

// FormatVersion is the manifest format this package writes and reads.
const FormatVersion = 1

// Errors a verification returns, wrapped with the offending path where one
// applies. Test with errors.Is.
var (
	// ErrSignature means the manifest is unsigned or signed by no trusted key.
	ErrSignature = errors.New("the manifest is not signed by a trusted key")
	// ErrNoManifest means the archive has no manifest.json.
	ErrNoManifest = errors.New("the archive has no manifest.json")
	// ErrMissing means a file the manifest lists is not in the archive.
	ErrMissing = errors.New("a listed file is missing")
	// ErrExtra means the archive holds a file the manifest does not list.
	ErrExtra = errors.New("an unlisted file is present")
	// ErrAltered means a file differs from its listed size or SHA-256, or the
	// image list differs from the layout's index.
	ErrAltered = errors.New("a file was altered")
	// ErrMalformed means the archive or its manifest is not shaped as a bundle.
	ErrMalformed = errors.New("the bundle is malformed")
)

// File is one file of the bundle.
type File struct {
	// Path is the slash-separated path inside the archive.
	Path string `json:"path"`
	// Size is the length in bytes.
	Size int64 `json:"size"`
	// SHA256 is the lower-case hexadecimal SHA-256 of the content.
	SHA256 string `json:"sha256"`
}

// Image is one image of the layout, as its index.json entry names it.
type Image struct {
	// Ref is the name the layout gives the image, for example
	// ghcr.io/nightcode42/plux-server:0.0.1; empty when the source named none.
	Ref string `json:"ref"`
	// Digest is the digest of the image's manifest or index.
	Digest string `json:"digest"`
}

// Manifest is the signed content of a bundle.
type Manifest struct {
	// Format is FormatVersion.
	Format int `json:"format"`
	// Version is the Plux version the bundle installs.
	Version string `json:"version"`
	// Files lists every file but the manifest, sorted by path.
	Files []File `json:"files"`
	// Images lists the images of the layout, sorted by reference and digest.
	Images []Image `json:"images"`
}

// envelope is manifest.json on disk, shaped like a licence file.
type envelope struct {
	Signed     json.RawMessage         `json:"signed"`
	Signatures []licensefile.Signature `json:"signatures"`
}

// signManifest returns the bytes of manifest.json: the manifest and an
// Ed25519 signature over the exact compact JSON of the signed part.
func signManifest(m Manifest, key ed25519.PrivateKey) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("airgap.signManifest: the signing key must be %d bytes", ed25519.PrivateKeySize)
	}
	pub, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("airgap.signManifest: the signing key has no Ed25519 public half")
	}
	signed, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("airgap.signManifest: %w", err)
	}
	out, err := json.MarshalIndent(envelope{
		Signed: signed,
		Signatures: []licensefile.Signature{{
			KeyID: licensefile.KeyID(pub), Alg: licensefile.AlgEd25519,
			Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(key, signed)),
		}},
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("airgap.signManifest: %w", err)
	}
	return append(out, '\n'), nil
}

// openManifest checks the signature of manifest.json against keys and only
// then decodes the signed part, so nothing unsigned is interpreted.
func openManifest(raw []byte, keys licensefile.KeySet) (Manifest, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Manifest{}, fmt.Errorf("airgap.openManifest: %w: %w", ErrMalformed, err)
	}
	var signed bytes.Buffer
	if err := json.Compact(&signed, env.Signed); err != nil {
		return Manifest{}, fmt.Errorf("airgap.openManifest: %w: the signed part is not JSON", ErrMalformed)
	}
	if !signedBy(keys, env.Signatures, signed.Bytes()) {
		return Manifest{}, fmt.Errorf("airgap.openManifest: %w", ErrSignature)
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(signed.Bytes()))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("airgap.openManifest: %w: %w", ErrMalformed, err)
	}
	if err := m.validate(); err != nil {
		return Manifest{}, fmt.Errorf("airgap.openManifest: %w", err)
	}
	return m, nil
}

// signedBy reports whether a trusted key made one of the signatures over
// signed.
func signedBy(keys licensefile.KeySet, sigs []licensefile.Signature, signed []byte) bool {
	for _, s := range sigs {
		pub, ok := keys.Lookup(s.KeyID)
		if !ok || s.Alg != licensefile.AlgEd25519 {
			continue
		}
		sig, err := base64.StdEncoding.DecodeString(s.Sig)
		if err == nil && ed25519.Verify(pub, signed, sig) {
			return true
		}
	}
	return false
}

// validate checks the shape of a signed manifest: the format, and sorted,
// unique, well-formed file paths with well-formed hashes.
func (m Manifest) validate() error {
	if m.Format != FormatVersion {
		return fmt.Errorf("%w: unsupported manifest format %d", ErrMalformed, m.Format)
	}
	if !slices.IsSortedFunc(m.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) }) {
		return fmt.Errorf("%w: the files are not sorted", ErrMalformed)
	}
	for i, f := range m.Files {
		if !validPath(f.Path) || f.Path == ManifestPath || (i > 0 && m.Files[i-1].Path == f.Path) {
			return fmt.Errorf("%w: bad or repeated path %q", ErrMalformed, f.Path)
		}
		if raw, err := hex.DecodeString(f.SHA256); err != nil || len(raw) != 32 || f.Size < 0 {
			return fmt.Errorf("%w: bad size or hash for %q", ErrMalformed, f.Path)
		}
	}
	return nil
}

// validPath reports whether p is a clean, relative, slash-separated path.
func validPath(p string) bool {
	return fs.ValidPath(p) && p != "."
}
