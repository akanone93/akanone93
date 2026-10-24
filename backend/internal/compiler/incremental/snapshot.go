// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incremental

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// snapshot is the project's files at one moment: the hash of each, and the
// content of the ones that differ from the previous snapshot.
type snapshot struct {
	hashes map[string][sha256.Size]byte
	// data holds the content of the files whose hash is not in the
	// snapshot taken before.
	data map[string][]byte
	// digest identifies the whole snapshot.
	digest [sha256.Size]byte
}

// takeSnapshot reads every file of the project except the hidden ones (the
// loader skips them too). prev is the hashes to compare with; nil keeps the
// content of every file.
func takeSnapshot(ctx context.Context, fsys fs.FS, prev map[string][sha256.Size]byte) (*snapshot, error) {
	s := &snapshot{hashes: map[string][sha256.Size]byte{}, data: map[string][]byte{}}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err //nolint:wrapcheck // wrapped below, with the call's context.
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr //nolint:wrapcheck // wrapped below, with the call's context.
		}
		if name != "." && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err //nolint:wrapcheck // wrapped below, with the call's context.
		}
		h := sha256.Sum256(data)
		s.hashes[name] = h
		if was, ok := prev[name]; prev == nil || !ok || was != h {
			s.data[name] = data
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("incremental.Compile: read the project: %w", err)
	}
	s.digest = digestOf(s.hashes)
	return s, nil
}

// digestOf hashes a set of file hashes, in file order.
func digestOf(hashes map[string][sha256.Size]byte) [sha256.Size]byte {
	names := make([]string, 0, len(hashes))
	for name := range hashes {
		names = append(names, name)
	}
	slices.Sort(names)
	h := sha256.New()
	for _, name := range names {
		sum := hashes[name]
		fmt.Fprintf(h, "%d:%s:", len(name), name)
		h.Write(sum[:])
	}
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

// diff returns the files that differ from an earlier snapshot, sorted: those
// whose content changed, and those added or removed.
func (s *snapshot) diff(prev map[string][sha256.Size]byte) (changed []string, structural bool) {
	for name, h := range s.hashes {
		was, ok := prev[name]
		switch {
		case !ok:
			structural = true
			changed = append(changed, name)
		case was != h:
			changed = append(changed, name)
		}
	}
	for name := range prev {
		if _, ok := s.hashes[name]; !ok {
			structural = true
			changed = append(changed, name)
		}
	}
	slices.Sort(changed)
	return changed, structural
}
