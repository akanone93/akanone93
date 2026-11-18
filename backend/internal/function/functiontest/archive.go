// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package functiontest makes the archives that tests of Plux Functions send:
// a function's source archive and the published crate archives it depends on
// (FN-003, FN-016, ADR-0105). The archives are deterministic, so the same
// arguments give the same bytes and the same SHA-256.
package functiontest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// cratesIO is the source Cargo.lock records for a crates.io package.
const cratesIO = "registry+https://github.com/rust-lang/crates.io-index"

// Crate is a published crate archive.
type Crate struct {
	Name, Version string
	// Archive is the `.crate` file: every file below <name>-<version>/.
	Archive []byte
}

// SHA256 is the checksum Cargo.lock records for the crate.
func (c Crate) SHA256() string {
	sum := sha256.Sum256(c.Archive)
	return hex.EncodeToString(sum[:])
}

// NewCrate returns a crate archive with a manifest and a library whose text
// is lib.
func NewCrate(name, version, lib string) Crate {
	return Crate{Name: name, Version: version, Archive: TarGz(name+"-"+version+"/", map[string]string{
		"Cargo.toml": fmt.Sprintf("[package]\nname = %q\nversion = %q\n", name, version),
		"src/lib.rs": lib,
	})}
}

// SourceArchive returns a function's source archive as `plux fn deploy`
// sends it: Cargo.toml, a Cargo.lock that names the crates, and a src/lib.rs
// that holds seed, so that different seeds give different archives.
func SourceArchive(seed string, crates ...Crate) []byte {
	var lock strings.Builder
	lock.WriteString("version = 4\n\n[[package]]\nname = \"fn\"\nversion = \"0.1.0\"\n")
	for _, c := range crates {
		fmt.Fprintf(&lock, "\n[[package]]\nname = %q\nversion = %q\nsource = %q\nchecksum = %q\n", c.Name, c.Version, cratesIO, c.SHA256())
	}
	return TarGz("", map[string]string{
		"Cargo.toml": "[package]\nname = \"fn\"\nversion = \"0.1.0\"\n",
		"Cargo.lock": lock.String(),
		"src/lib.rs": "// " + seed + "\n",
	})
}

// TarGz packs files, below prefix, into a gzip-compressed tar archive in
// sorted order.
func TarGz(prefix string, files map[string]string) []byte {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	// Writing to a bytes.Buffer does not fail, and the headers are the
	// package's own, so the writers' errors cannot occur.
	for _, n := range names {
		body := files[n]
		_ = tw.WriteHeader(&tar.Header{Name: prefix + n, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = zw.Close()
	return buf.Bytes()
}
