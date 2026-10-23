// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package airgap

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/edition/licensefile"
)

// indexPath is the archive path of the layout's index.
const indexPath = "images/" + layoutIndex

// seenFile is what the scan of the archive found for one path.
type seenFile struct {
	size int64
	hash string
}

// Verify reads a bundle once and checks it against keys and returns its
// manifest. The manifest is
// signed by a trusted key, every listed file is present with its listed size
// and SHA-256, nothing else is in the archive, only regular files occur, and
// the image list equals the layout's index. The first problem is returned,
// wrapping one of the Err values.
func Verify(r io.Reader, keys licensefile.KeySet) (Manifest, error) {
	seen, rawManifest, rawIndex, err := scan(tar.NewReader(r))
	if err != nil {
		return Manifest{}, err
	}
	if rawManifest == nil {
		return Manifest{}, fmt.Errorf("airgap.Verify: %w", ErrNoManifest)
	}
	m, err := openManifest(rawManifest, keys)
	if err != nil {
		return Manifest{}, err
	}
	if err := compareFiles(m.Files, seen); err != nil {
		return Manifest{}, err
	}
	images, err := imagesOf(rawIndex)
	if err != nil {
		return Manifest{}, fmt.Errorf("airgap.Verify: %w", err)
	}
	if !slices.Equal(images, m.Images) {
		return Manifest{}, fmt.Errorf("airgap.Verify: %w: the image list differs from images/index.json", ErrAltered)
	}
	return m, nil
}

// scan reads every entry once: it hashes files and keeps the bytes of
// manifest.json and images/index.json. It refuses entries that are not
// regular files, unclean or repeated paths.
func scan(tr *tar.Reader) (map[string]seenFile, []byte, []byte, error) {
	seen := map[string]seenFile{}
	var rawManifest, rawIndex []byte
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return seen, rawManifest, rawIndex, nil
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("airgap.Verify: %w: %w", ErrMalformed, err)
		}
		if h.Typeflag != tar.TypeReg || !validPath(h.Name) {
			return nil, nil, nil, fmt.Errorf("airgap.Verify: %w: %q is not a regular file with a clean path", ErrMalformed, h.Name)
		}
		if _, dup := seen[h.Name]; dup || (h.Name == ManifestPath && rawManifest != nil) {
			return nil, nil, nil, fmt.Errorf("airgap.Verify: %w: %q appears twice", ErrMalformed, h.Name)
		}
		sf, raw, err := readEntry(tr, h.Name)
		if err != nil {
			return nil, nil, nil, err
		}
		switch h.Name {
		case ManifestPath:
			rawManifest = raw
		case indexPath:
			rawIndex = raw
			seen[h.Name] = sf
		default:
			seen[h.Name] = sf
		}
	}
}

// readEntry hashes one entry. The two JSON documents are also returned, read
// within maxMetaBytes.
func readEntry(r io.Reader, name string) (seenFile, []byte, error) {
	if name == ManifestPath || name == indexPath {
		raw, err := readBounded(r)
		if err != nil {
			return seenFile{}, nil, fmt.Errorf("airgap.Verify: %s: %w: %w", name, ErrMalformed, err)
		}
		sum := sha256.Sum256(raw)
		return seenFile{size: int64(len(raw)), hash: hex.EncodeToString(sum[:])}, raw, nil
	}
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return seenFile{}, nil, fmt.Errorf("airgap.Verify: %s: %w: %w", name, ErrMalformed, err)
	}
	return seenFile{size: n, hash: hex.EncodeToString(h.Sum(nil))}, nil, nil
}

// compareFiles refuses a missing, extra or altered file.
func compareFiles(listed []File, seen map[string]seenFile) error {
	known := make(map[string]bool, len(listed))
	for _, f := range listed {
		known[f.Path] = true
		got, ok := seen[f.Path]
		switch {
		case !ok:
			return fmt.Errorf("airgap.Verify: %w: %s", ErrMissing, f.Path)
		case got.size != f.Size || got.hash != f.SHA256:
			return fmt.Errorf("airgap.Verify: %w: %s", ErrAltered, f.Path)
		}
	}
	extra := make([]string, 0)
	for p := range seen {
		if !known[p] {
			extra = append(extra, p)
		}
	}
	if len(extra) > 0 {
		slices.Sort(extra)
		return fmt.Errorf("airgap.Verify: %w: %s", ErrExtra, strings.Join(extra, ", "))
	}
	return nil
}
