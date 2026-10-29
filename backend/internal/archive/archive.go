// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package archive reads and writes the .plux archive of ADR-0102: a ZIP of
// an app's SCH-006 project layout and a manifest. Writing is deterministic
// (the same files give the same bytes) and reading treats the archive as
// untrusted input, checking sizes, entry counts, names and hashes before
// anything reaches the document service.
package archive

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// The names and versions of the format (ADR-0102).
const (
	// ManifestName is the manifest's entry name.
	ManifestName = "plux-archive.json"
	// Format identifies the manifest.
	Format = "plux-archive"
	// Version is the manifest version this package writes and reads.
	Version = 1
	// Prefix is the directory the project layout lives in.
	Prefix = "project/"
	// maxName bounds one entry name.
	maxName = 1024
)

// AppInfo is the descriptive part of an app that travels with its files.
type AppInfo struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// Entry is one file the manifest lists.
type Entry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest is plux-archive.json.
type Manifest struct {
	Format  string  `json:"format"`
	Version int     `json:"version"`
	App     AppInfo `json:"app"`
	Files   []Entry `json:"files"`
}

// Limits are the size and count limits of an import, from the registry.
type Limits struct {
	Bytes         int64
	Entries       int64
	ExpandedBytes int64
}

// LimitsOf reads the archive limits from a registry set (LIM-001).
func LimitsOf(set limits.Set) Limits {
	return Limits{
		Bytes: set.Get(limits.ArchiveBytes), Entries: set.Get(limits.ArchiveEntries), ExpandedBytes: set.Get(limits.ArchiveExpandedBytes),
	}
}

// zipEpoch is 1980-01-01 00:00, the earliest time a ZIP header holds, written
// so that no clock reaches the bytes.
var zipEpoch = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

// Pack writes the archive for an app's files. The output depends only on
// the arguments: entries are sorted, stored without compression, with the
// ZIP epoch as time and mode 0644, and the manifest is the first entry.
func Pack(app AppInfo, files []document.File) ([]byte, error) {
	sorted := slices.Clone(files)
	slices.SortFunc(sorted, func(a, b document.File) int { return strings.Compare(a.Path, b.Path) })
	m := Manifest{Format: Format, Version: Version, App: app, Files: make([]Entry, 0, len(sorted))}
	if m.App.Tags == nil {
		m.App.Tags = []string{}
	}
	for i, f := range sorted {
		if err := validName(f.Path); err != nil {
			return nil, err
		}
		if i > 0 && sorted[i-1].Path == f.Path {
			return nil, plxerr.New(plxerr.InvalidProjectLayout, "the file %q appears twice", f.Path)
		}
		sum := sha256.Sum256(f.Content)
		m.Files = append(m.Files, Entry{Path: f.Path, Size: int64(len(f.Content)), SHA256: hex.EncodeToString(sum[:])})
	}
	manifest, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("archive: encode the manifest: %w", err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writeEntry(zw, ManifestName, append(manifest, '\n')); err != nil {
		return nil, err
	}
	for _, f := range sorted {
		if err := writeEntry(zw, Prefix+f.Path, f.Content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("archive: close: %w", err)
	}
	return buf.Bytes(), nil
}

func writeEntry(zw *zip.Writer, name string, data []byte) error {
	h := &zip.FileHeader{Name: name, Method: zip.Store, Modified: zipEpoch}
	h.SetMode(0o644)
	w, err := zw.CreateHeader(h)
	if err != nil {
		return fmt.Errorf("archive: write %s: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("archive: write %s: %w", name, err)
	}
	return nil
}

// validName refuses every name that could leave the project directory or
// that is not the cleaned form of itself.
func validName(name string) error {
	bad := name == "" || len(name) > maxName || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") ||
		strings.ContainsAny(name, "\\:") || path.Clean(name) != name
	if !bad {
		for _, r := range name {
			if unicode.IsControl(r) {
				bad = true
				break
			}
		}
	}
	if !bad {
		bad = slices.Contains(strings.Split(name, "/"), "..")
	}
	if bad {
		return plxerr.New(plxerr.InvalidProjectLayout, "%q is not a valid file name in a project", name)
	}
	return nil
}

// Unpack reads an archive. The archive is untrusted: every limit is checked
// on what is actually read, not on what a header declares.
func Unpack(data []byte, lim Limits) (Manifest, []document.File, error) {
	if int64(len(data)) > lim.Bytes {
		return Manifest{}, nil, plxerr.New(plxerr.LimitExceeded, "the archive has %d bytes; at most %d are accepted", len(data), lim.Bytes)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Manifest{}, nil, plxerr.Wrap(plxerr.InvalidFormat, err, "the file is not a .plux archive")
	}
	if int64(len(zr.File)) > lim.Entries {
		return Manifest{}, nil, plxerr.New(plxerr.LimitExceeded, "the archive has %d entries; at most %d are accepted", len(zr.File), lim.Entries)
	}
	var (
		manifest []byte
		files    = map[string][]byte{}
		seen     = map[string]bool{}
		total    int64
	)
	for _, f := range zr.File {
		if seen[f.Name] {
			return Manifest{}, nil, plxerr.New(plxerr.InvalidProjectLayout, "the entry %q appears twice", f.Name)
		}
		seen[f.Name] = true
		if err := validName(f.Name); err != nil {
			return Manifest{}, nil, err
		}
		if !f.Mode().IsRegular() {
			return Manifest{}, nil, plxerr.New(plxerr.InvalidProjectLayout, "the entry %q is not a regular file", f.Name)
		}
		remaining := lim.ExpandedBytes - total
		if f.UncompressedSize64 > uint64(max(remaining, 0)) {
			return Manifest{}, nil, plxerr.New(plxerr.LimitExceeded, "the archive expands beyond %d bytes", lim.ExpandedBytes)
		}
		content, err := readEntry(f, remaining)
		if err != nil {
			return Manifest{}, nil, err
		}
		total += int64(len(content))
		switch {
		case f.Name == ManifestName:
			manifest = content
		case strings.HasPrefix(f.Name, Prefix):
			files[strings.TrimPrefix(f.Name, Prefix)] = content
		default:
			return Manifest{}, nil, plxerr.New(plxerr.InvalidProjectLayout, "the entry %q is outside %s", f.Name, Prefix)
		}
	}
	m, err := parseManifest(manifest)
	if err != nil {
		return Manifest{}, nil, err
	}
	out, err := verify(m, files)
	return m, out, err
}

func readEntry(f *zip.File, remaining int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, plxerr.Wrap(plxerr.InvalidFormat, err, "the entry %q cannot be read", f.Name)
	}
	defer func() { _ = rc.Close() }()
	content, err := io.ReadAll(io.LimitReader(rc, max(remaining, 0)+1))
	if err != nil {
		return nil, plxerr.Wrap(plxerr.InvalidFormat, err, "the entry %q cannot be read", f.Name)
	}
	if int64(len(content)) > remaining {
		return nil, plxerr.New(plxerr.LimitExceeded, "the archive expands beyond its limit")
	}
	return content, nil
}

func parseManifest(data []byte) (Manifest, error) {
	if data == nil {
		return Manifest{}, plxerr.New(plxerr.InvalidProjectLayout, "the archive has no %s", ManifestName)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, plxerr.Wrap(plxerr.InvalidFormat, err, "%s is not valid JSON", ManifestName)
	}
	switch {
	case m.Format != Format:
		return Manifest{}, plxerr.New(plxerr.InvalidFormat, "%s is not a %s manifest", ManifestName, Format)
	case m.Version < 1 || m.Version > Version:
		return Manifest{}, plxerr.New(plxerr.InvalidFormat, "the archive has version %d; this server reads version %d and below", m.Version, Version)
	}
	return m, nil
}

// verify matches the manifest against the entries read, both ways.
func verify(m Manifest, files map[string][]byte) ([]document.File, error) {
	if len(m.Files) != len(files) {
		return nil, plxerr.New(plxerr.InvalidProjectLayout, "the manifest lists %d files; the archive holds %d", len(m.Files), len(files))
	}
	out := make([]document.File, 0, len(m.Files))
	listed := map[string]bool{}
	for _, e := range m.Files {
		content, ok := files[e.Path]
		if !ok || listed[e.Path] {
			return nil, plxerr.New(plxerr.InvalidProjectLayout, "the manifest names %q, which the archive does not hold once", e.Path)
		}
		listed[e.Path] = true
		sum := sha256.Sum256(content)
		if int64(len(content)) != e.Size || hex.EncodeToString(sum[:]) != e.SHA256 {
			return nil, plxerr.New(plxerr.InvalidProjectLayout, "%q does not match its size and hash in the manifest", e.Path)
		}
		out = append(out, document.File{Path: e.Path, Content: content})
	}
	return out, nil
}
