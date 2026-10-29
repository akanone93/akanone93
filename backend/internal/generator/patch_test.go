// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
)

// TestPatchListsWhatChanged_GEN_004 diffs two generated projects: the patch
// holds the added and changed files and a manifest with sorted lists, and
// the same projects give the same bytes.
func TestPatchListsWhatChanged_GEN_004(t *testing.T) {
	t.Parallel()
	oldIn := sample()
	oldFiles, err := Generate(oldIn)
	if err != nil {
		t.Fatal(err)
	}
	newIn := sample()
	newIn.Spec.AppVersion, newIn.Spec.Push, newIn.Spec.IconAssetID = "1.3.0+8", false, "asset-1"
	newIn.Baseline = map[string][]byte{"baseline.json": []byte("{}\n"), "extra.json": []byte("{}\n")}
	newFiles, err := Generate(newIn)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := Patch(oldFiles, newFiles, oldIn.Spec, newIn.Spec)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Patch(oldFiles, newFiles, oldIn.Spec, newIn.Spec)
	if err != nil {
		t.Fatal(err)
	}
	a, err := Zip("loans", patch)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Zip("loans", again)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("the same projects gave different patches")
	}
	got := byPath(patch)
	var m PatchManifest
	if err := json.Unmarshal(got[PatchFile].Data, &m); err != nil {
		t.Fatal(err)
	}
	paths := func(es []PatchEntry) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.Path)
		}
		return out
	}
	if !slices.IsSorted(paths(m.Changed)) || !slices.IsSorted(paths(m.Added)) || !slices.IsSorted(paths(m.Deleted)) {
		t.Errorf("the lists are not sorted: %+v", m)
	}
	if !slices.Contains(paths(m.Changed), "pubspec.yaml") || !slices.Contains(paths(m.Changed), "plux.shell.json") {
		t.Errorf("changed: %v", paths(m.Changed))
	}
	if !slices.Equal(paths(m.Added), []string{"assets/plux/extra.json"}) {
		t.Errorf("added: %v", paths(m.Added))
	}
	if !slices.Contains(paths(m.Deleted), "assets/plux/bundles/_app.pxb") {
		t.Errorf("deleted: %v", paths(m.Deleted))
	}
	for p := range got {
		if p != PatchFile && !slices.Contains(paths(m.Changed), p) && !slices.Contains(paths(m.Added), p) {
			t.Errorf("%s is in the patch but not listed", p)
		}
	}
	if m.From.AppVersion != "1.2.0+7" || m.To.AppVersion != "1.3.0+8" || m.To.IconAssetID != "" {
		t.Errorf("specs: %+v %+v", m.From, m.To)
	}
	for _, e := range m.Changed {
		if e.SHA256 != digest(got[e.Path].Data) {
			t.Errorf("%s: wrong hash", e.Path)
		}
	}
	if same, err := Patch(oldFiles, oldFiles, oldIn.Spec, oldIn.Spec); err != nil || len(same) != 1 || same[0].Path != PatchFile {
		t.Errorf("identical projects: %d files %v", len(same), err)
	}
}
