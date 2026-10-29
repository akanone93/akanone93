// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package renderworker

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"testing/fstest"
	"time"

	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Limits bound what one render may consume: the files of a project, the cases
// of a run and the size of an image. The server sends the values it resolved
// for the organisation; the worker clamps each to the registry (LIM-001).
type Limits struct {
	// ProjectBytes is the expanded size of one project (archive.expandedBytes).
	ProjectBytes int64
	// Entries is the number of files of one project (archive.entries).
	Entries int64
	// Cases is the number of cases of one run (golden.casesPerRun).
	Cases int64
	// ImageBytes is the size of one image (golden.imageBytes).
	ImageBytes int64
}

// LimitsOf reads the limits of a render from a registry set.
func LimitsOf(set limits.Set) Limits {
	return Limits{
		ProjectBytes: set.Get(limits.ArchiveExpandedBytes), Entries: set.Get(limits.ArchiveEntries),
		Cases: set.Get(limits.GoldenCasesPerRun), ImageBytes: set.Get(limits.GoldenImageBytes),
	}
}

// Clamped returns the limits with each value inside the registry: a value
// that is not positive becomes the default, one above the maximum the maximum.
func (l Limits) Clamped() Limits {
	clamp := func(v int64, k limits.Key) int64 {
		def, ok := limits.Lookup(k)
		switch {
		case !ok:
			return v
		case v <= 0:
			return def.Default
		default:
			return min(v, def.Max)
		}
	}
	return Limits{
		ProjectBytes: clamp(l.ProjectBytes, limits.ArchiveExpandedBytes), Entries: clamp(l.Entries, limits.ArchiveEntries),
		Cases: clamp(l.Cases, limits.GoldenCasesPerRun), ImageBytes: clamp(l.ImageBytes, limits.GoldenImageBytes),
	}
}

// zipEpoch is the earliest time a ZIP header holds, written so that no clock
// reaches the bytes: the same project always packs to the same archive.
var zipEpoch = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

// ErrTooLarge is returned for a project over its limits.
var ErrTooLarge = errors.New("renderworker: the project exceeds its limits")

// Pack writes the regular files of fsys as a stored ZIP, in path order, with
// no timestamps. A project over lim.Entries files or lim.ProjectBytes bytes
// is ErrTooLarge.
func Pack(fsys fs.FS, lim Limits) ([]byte, error) {
	var (
		buf     bytes.Buffer
		total   int64
		entries int64
	)
	zw := zip.NewWriter(&buf)
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		entries++
		if entries > lim.Entries {
			return fmt.Errorf("%w: more than %d files", ErrTooLarge, lim.Entries)
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		total += int64(len(data))
		if total > lim.ProjectBytes {
			return fmt.Errorf("%w: more than %d bytes", ErrTooLarge, lim.ProjectBytes)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: path, Method: zip.Store, Modified: zipEpoch})
		if err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("renderworker: pack the project: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("renderworker: %w", err)
	}
	return buf.Bytes(), nil
}

// Unpack reads a project archive into memory. It refuses a name that is not a
// clean relative path or is repeated, and a project over its limits; the sizes
// are counted while reading, not trusted from the headers.
func Unpack(data []byte, lim Limits) (fstest.MapFS, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("renderworker: read the project archive: %w", err)
	}
	if int64(len(zr.File)) > lim.Entries {
		return nil, fmt.Errorf("%w: more than %d files", ErrTooLarge, lim.Entries)
	}
	out := make(fstest.MapFS, len(zr.File))
	remaining := lim.ProjectBytes
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !fs.ValidPath(f.Name) || f.Name == "." {
			return nil, fmt.Errorf("renderworker: the archive names %q, which is not a relative path", f.Name)
		}
		if _, dup := out[f.Name]; dup {
			return nil, fmt.Errorf("renderworker: the archive repeats %q", f.Name)
		}
		body, err := readEntry(f, remaining)
		if err != nil {
			return nil, err
		}
		remaining -= int64(len(body))
		out[f.Name] = &fstest.MapFile{Data: body, Mode: 0o644}
	}
	return out, nil
}

func readEntry(f *zip.File, remaining int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("renderworker: open %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()
	body, err := io.ReadAll(io.LimitReader(rc, remaining+1))
	if err != nil {
		return nil, fmt.Errorf("renderworker: read %s: %w", f.Name, err)
	}
	if int64(len(body)) > remaining {
		return nil, fmt.Errorf("%w: expands past the project limit", ErrTooLarge)
	}
	return body, nil
}
