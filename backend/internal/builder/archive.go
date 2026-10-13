// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"path"
	"strings"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// tarBlock is the size of a tar header; every entry is charged at least this
// much against the budget, so that an archive of millions of empty files is
// bounded like any other.
const tarBlock = 512

// ExtractArchive reads a gzip-compressed tar archive of one function's source
// into memory: Cargo.toml, Cargo.lock, src/, vendor/ and function.json at its
// root (FN-016). The archive may hold only regular files and directories on
// clean relative paths; everything extracted, charged per entry as at least a
// tar block, is bounded by maxBytes (function.sourceArchiveSize), and a larger
// archive is PLX-7203. A malformed archive is PLX-7200.
func ExtractArchive(r io.Reader, maxBytes int64) (fs.FS, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, sourceError("the source archive is not gzip-compressed: %v", err)
	}
	defer func() { _ = zr.Close() }()
	files := fstest.MapFS{}
	tr := tar.NewReader(zr)
	var used int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, sourceError("the source archive is not a tar archive: %v", err)
		}
		used += max(h.Size, tarBlock)
		if used > maxBytes {
			return nil, plxerr.New(plxerr.FunctionBuildLimitExceeded, "the source is above function.sourceArchiveSize = %d bytes once extracted", maxBytes)
		}
		if err := addEntry(files, tr, h); err != nil {
			return nil, err
		}
	}
	if len(files) == 0 {
		return nil, sourceError("the source archive is empty")
	}
	return files, nil
}

// addEntry adds one tar entry to files, refusing anything but a regular file
// or a directory on a clean relative path.
func addEntry(files fstest.MapFS, tr *tar.Reader, h *tar.Header) error {
	name := path.Clean(strings.TrimPrefix(h.Name, "./"))
	if h.Typeflag == tar.TypeDir && name == "." {
		return nil
	}
	if !fs.ValidPath(name) || name == "." || strings.HasPrefix(h.Name, "/") {
		return sourceError("the source archive has the path %q; paths must be clean and relative", h.Name)
	}
	switch h.Typeflag {
	case tar.TypeDir:
		// An empty vendor/ directory is a statement: nothing is vendored
		// (FN-003).
		if _, dup := files[name]; !dup {
			files[name] = &fstest.MapFile{Mode: fs.ModeDir | 0o755}
		}
		return nil
	case tar.TypeReg:
		if _, dup := files[name]; dup {
			return sourceError("the source archive lists %q twice", name)
		}
		data, err := io.ReadAll(io.LimitReader(tr, h.Size))
		if err != nil || int64(len(data)) != h.Size {
			return sourceError("the source archive is truncated at %q", name)
		}
		files[name] = &fstest.MapFile{Data: data, Mode: 0o644}
		return nil
	default:
		return sourceError("the source archive entry %q is neither a file nor a directory", h.Name)
	}
}
