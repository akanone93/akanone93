// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// PatchFile is the name of the patch's manifest inside the archive.
const PatchFile = "plux-patch.json"

// PatchVersion is the version of the patch manifest; it changes only
// additively.
const PatchVersion = 1

// PatchEntry is one file of a patch and the SHA-256 of its content (of the
// old content for a deleted file).
type PatchEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// PatchManifest is plux-patch.json: what a patch changes in a project
// generated from From, to bring it to To (GEN-004). The lists are sorted by
// path.
type PatchManifest struct {
	Version int          `json:"version"`
	From    ShellSpec    `json:"from"`
	To      ShellSpec    `json:"to"`
	Changed []PatchEntry `json:"changed"`
	Added   []PatchEntry `json:"added"`
	Deleted []PatchEntry `json:"deleted"`
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Patch returns the files that bring the project old to the project updated:
// every added or changed file as it is in updated, and plux-patch.json
// naming the changed, added and deleted paths. Deleted files are only
// listed; the reader removes them. The result is sorted by path and depends
// only on its arguments, so the same projects give the same patch.
func Patch(old, updated []File, from, to ShellSpec) ([]File, error) {
	from.IconAssetID, to.IconAssetID = "", ""
	before := make(map[string]File, len(old))
	for _, f := range old {
		before[f.Path] = f
	}
	m := PatchManifest{Version: PatchVersion, From: from, To: to, Changed: []PatchEntry{}, Added: []PatchEntry{}, Deleted: []PatchEntry{}}
	var out []File
	seen := make(map[string]bool, len(updated))
	for _, f := range updated {
		seen[f.Path] = true
		prev, existed := before[f.Path]
		switch {
		case !existed:
			m.Added = append(m.Added, PatchEntry{Path: f.Path, SHA256: digest(f.Data)})
		case !bytes.Equal(prev.Data, f.Data) || prev.Mode != f.Mode:
			m.Changed = append(m.Changed, PatchEntry{Path: f.Path, SHA256: digest(f.Data)})
		default:
			continue
		}
		out = append(out, f)
	}
	for _, f := range old {
		if !seen[f.Path] {
			m.Deleted = append(m.Deleted, PatchEntry{Path: f.Path, SHA256: digest(f.Data)})
		}
	}
	for _, list := range [][]PatchEntry{m.Changed, m.Added, m.Deleted} {
		slices.SortFunc(list, func(a, b PatchEntry) int { return strings.Compare(a.Path, b.Path) })
	}
	doc, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode the patch manifest: %w", err)
	}
	out = append(out, File{Path: PatchFile, Data: append(doc, '\n'), Mode: 0o644})
	slices.SortFunc(out, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}
