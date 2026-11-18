// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// The dependencies of a function arrive as the `.crate` archives crates.io
// publishes, never as a vendored tree (ADR-0105). A vendored tree carries its
// own checksums, which whoever uploads it can rewrite; a `.crate` archive has
// one SHA-256, which Cargo.lock records and crates.io publishes. The builder
// checks every archive against it and makes vendor/ itself.

// checksumFileName is the file Cargo reads in each crate of a directory
// source.
const checksumFileName = ".cargo-checksum.json"

// CrateSHA256 returns the hex SHA-256 of a `.crate` archive, which is the
// checksum Cargo.lock records for it.
func CrateSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// assembleSource returns the files of a function with vendor/ made from its
// crates (FN-003, FN-016). Every registry package of the lock file must have
// exactly one archive whose SHA-256 is its checksum; an archive the lock file
// does not name, an archive given twice and a vendor/ directory among the
// uploaded files are refused with PLX-7201. Each archive is extracted under
// vendor/<name>-<version>/ with the checksum file Cargo expects, written by the
// builder from the bytes it extracted. Extraction is charged to budget.
func assembleSource(s FunctionSource, lock []lockPackage, budget *sizeBudget) (fstest.MapFS, error) {
	files, err := uploadedFiles(s)
	if err != nil {
		return nil, err
	}
	byChecksum, err := indexCrates(s)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, p := range lock {
		if !p.registry() || used[p.Checksum] {
			continue
		}
		data, ok := byChecksum[p.Checksum]
		if !ok {
			return nil, plxerr.New(plxerr.FunctionDependencyNotVendored,
				"function %q: the crate %s %s (sha256 %s) that Cargo.lock names was not sent", s.Name, p.Name, p.Version, p.Checksum)
		}
		used[p.Checksum] = true
		if err := extractCrate(files, data, p, budget); err != nil {
			return nil, err
		}
	}
	for sum := range byChecksum {
		if !used[sum] {
			return nil, plxerr.New(plxerr.FunctionDependencyNotVendored,
				"function %q: the crate archive %s is not named by Cargo.lock", s.Name, sum)
		}
	}
	return files, nil
}

// uploadedFiles copies the uploaded files of a source, refusing a vendor/
// directory and anything but regular files.
func uploadedFiles(s FunctionSource) (fstest.MapFS, error) {
	files := fstest.MapFS{}
	var refused error
	err := fs.WalkDir(s.Files, ".", func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case p == vendorDir || strings.HasPrefix(p, vendorDir+"/"):
			refused = plxerr.New(plxerr.FunctionDependencyNotVendored,
				"function %q: the source holds a vendor/ directory; dependencies are sent as their published .crate archives, and the builder makes vendor/ from them (ADR-0105)", s.Name)
			return fs.SkipAll
		case d.IsDir():
			return nil
		case !d.Type().IsRegular():
			refused = sourceError("function %q: %s is not a regular file", s.Name, p)
			return fs.SkipAll
		}
		data, err := fs.ReadFile(s.Files, p)
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		files[p] = &fstest.MapFile{Data: data, Mode: 0o644}
		return nil
	})
	if refused != nil {
		return nil, refused
	}
	if err != nil {
		return nil, sourceError("function %q: reading the source: %v", s.Name, err)
	}
	return files, nil
}

// indexCrates maps the crate archives of a source by their SHA-256 and refuses
// an archive given twice.
func indexCrates(s FunctionSource) (map[string][]byte, error) {
	byChecksum := make(map[string][]byte, len(s.Crates))
	for _, data := range s.Crates {
		sum := CrateSHA256(data)
		if _, dup := byChecksum[sum]; dup {
			return nil, plxerr.New(plxerr.FunctionDependencyNotVendored, "function %q: the crate archive %s is given twice", s.Name, sum)
		}
		byChecksum[sum] = data
	}
	return byChecksum, nil
}

// crateFailure is the PLX-7201 error about one crate archive.
func crateFailure(p lockPackage, format string, args ...any) error {
	return plxerr.New(plxerr.FunctionDependencyNotVendored, "crate %s %s: "+format, append([]any{p.Name, p.Version}, args...)...)
}

// extractCrate extracts one `.crate` archive into files under
// vendor/<name>-<version>/ and writes its checksum file. The archive must hold
// only regular files and directories below the single top directory
// <name>-<version>/ that crates.io gives every crate, and its manifest must
// name the package and version the lock file names.
func extractCrate(files fstest.MapFS, data []byte, p lockPackage, budget *sizeBudget) error {
	top := vendorDir + "/" + p.Name + "-" + p.Version
	sums, err := readCrateFiles(files, data, p, budget)
	if err != nil {
		return err
	}
	manifest, ok := files[top+"/Cargo.toml"]
	if !ok {
		return crateFailure(p, "the archive has no Cargo.toml")
	}
	fields := packageFields(manifest.Data)
	if fields["name"] != p.Name || fields["version"] != p.Version {
		return crateFailure(p, "the archive's manifest names %s %s", fields["name"], fields["version"])
	}
	sum, err := json.Marshal(struct {
		Files   map[string]string `json:"files"`
		Package string            `json:"package"`
	}{Files: sums, Package: p.Checksum})
	if err != nil {
		return crateFailure(p, "writing the checksum file: %v", err)
	}
	files[top+"/"+checksumFileName] = &fstest.MapFile{Data: sum, Mode: 0o644}
	return nil
}

// readCrateFiles adds the regular files of a crate archive to files and
// returns their SHA-256 digests by path inside the crate.
func readCrateFiles(files fstest.MapFS, data []byte, p lockPackage, budget *sizeBudget) (map[string]string, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, crateFailure(p, "the archive is not gzip-compressed: %v", err)
	}
	defer func() { _ = zr.Close() }()
	tr := tar.NewReader(zr)
	sums := map[string]string{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return sums, nil
		}
		if err != nil {
			return nil, crateFailure(p, "the archive is not a tar archive: %v", err)
		}
		if err := budget.take(max(h.Size, tarBlock)); err != nil {
			return nil, err
		}
		rel, err := crateEntryPath(h, p)
		if err != nil || rel == "" {
			return nil, err
		}
		dst := vendorDir + "/" + p.Name + "-" + p.Version + "/" + rel
		if _, dup := files[dst]; dup {
			return nil, crateFailure(p, "the archive lists %q twice", h.Name)
		}
		content, err := io.ReadAll(io.LimitReader(tr, h.Size))
		if err != nil || int64(len(content)) != h.Size {
			return nil, crateFailure(p, "the archive is truncated at %q", h.Name)
		}
		files[dst] = &fstest.MapFile{Data: content, Mode: 0o644}
		sums[rel] = hexSum(content)
	}
}

// crateEntryPath returns the path of a regular file inside the crate, or ""
// for a directory, and refuses anything else: a path outside the top
// directory <name>-<version>/, a path that is not clean, an entry that is
// neither a file nor a directory, and a checksum file of its own.
func crateEntryPath(h *tar.Header, p lockPackage) (string, error) {
	top := p.Name + "-" + p.Version
	name := strings.TrimSuffix(h.Name, "/")
	if name == top && h.Typeflag == tar.TypeDir {
		return "", nil
	}
	rel, below := strings.CutPrefix(name, top+"/")
	if !below || !fs.ValidPath(rel) || rel == "." || path.Clean(rel) != rel {
		return "", crateFailure(p, "the archive has the path %q outside %s/", h.Name, top)
	}
	switch {
	case h.Typeflag == tar.TypeDir:
		return "", nil
	case h.Typeflag != tar.TypeReg:
		return "", crateFailure(p, "the archive entry %q is neither a file nor a directory", h.Name)
	case rel == checksumFileName:
		return "", crateFailure(p, "the archive holds its own %s", checksumFileName)
	}
	return rel, nil
}

// LockedCrate is a registry package of a lock file with the checksum of its
// published archive.
type LockedCrate struct {
	Name, Version, Checksum string
}

// LockedCrates lists the crates.io packages of a lock file, sorted by name and
// version. A package from any other registry or a git source, or one without
// a checksum, is PLX-7201: the builder takes crates.io archives only (FN-003).
func LockedCrates(lockData []byte) ([]LockedCrate, error) {
	lock, err := parseLock(lockData)
	if err != nil {
		return nil, plxerr.Wrap(plxerr.FunctionDependencyNotVendored, err, "reading Cargo.lock")
	}
	var out []LockedCrate
	for _, p := range lock {
		if !p.registry() {
			continue
		}
		if p.Source != cratesIOSource || !sha256Hex.MatchString(p.Checksum) {
			return nil, plxerr.New(plxerr.FunctionDependencyNotVendored,
				"crate %s %s comes from %q; only crates.io crates with a checksum are accepted", p.Name, p.Version, p.Source)
		}
		out = append(out, LockedCrate{Name: p.Name, Version: p.Version, Checksum: p.Checksum})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// LocalCrates reads, from the package cache of the Cargo home on a
// developer's machine, the archive of every crates.io package of a lock file:
// registry/cache/<index>/<name>-<version>.crate, the one whose SHA-256 is the
// checksum the lock file records. A crate Cargo has not downloaded is an
// error that says to run `cargo fetch`.
func LocalCrates(cargoHome string, lockData []byte) ([][]byte, error) {
	locked, err := LockedCrates(lockData)
	if err != nil {
		return nil, err
	}
	indexes, err := os.ReadDir(filepath.Join(cargoHome, "registry", "cache"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("builder.LocalCrates: %w", err)
	}
	out := make([][]byte, 0, len(locked))
	for _, c := range locked {
		data, found := []byte(nil), false
		for _, idx := range indexes {
			p := filepath.Join(cargoHome, "registry", "cache", idx.Name(), c.Name+"-"+c.Version+".crate")
			b, err := os.ReadFile(p) //nolint:gosec // G304: the developer's own Cargo cache.
			if err != nil {
				continue
			}
			if CrateSHA256(b) == c.Checksum {
				data, found = b, true
				break
			}
		}
		if !found {
			return nil, plxerr.New(plxerr.FunctionDependencyNotVendored,
				"crate %s %s (sha256 %s) is not in Cargo's cache under %s; run `cargo fetch` in the function's directory", c.Name, c.Version, c.Checksum, cargoHome)
		}
		out = append(out, data)
	}
	return out, nil
}

// CratesFor returns, from a pool of crate archives, those whose SHA-256 the
// Cargo.lock among files names, each once: the archives a link gives each of
// its sources from the one pool they share. A source whose lock file cannot
// be read gets none, and the build reports the lock file.
func CratesFor(files fs.FS, pool [][]byte) [][]byte {
	data, err := fs.ReadFile(files, "Cargo.lock")
	if err != nil {
		return nil
	}
	lock, err := parseLock(data)
	if err != nil {
		return nil
	}
	want := map[string]bool{}
	for _, p := range lock {
		if p.registry() {
			want[p.Checksum] = true
		}
	}
	var out [][]byte
	for _, c := range pool {
		sum := CrateSHA256(c)
		if want[sum] {
			out = append(out, c)
			delete(want, sum)
		}
	}
	return out
}
