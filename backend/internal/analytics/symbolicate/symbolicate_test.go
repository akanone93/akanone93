// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package symbolicate_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/analytics/symbolicate"
	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/compiler"
)

// amountNode is the amount field of the calculator page of the conformance
// project (the compiler's own tests name the same node).
const amountNode = "/root/slots/body/slots/child/children/0"

// compiled is the conformance project compiled for release: the plugin
// bundle and the source map the compiler returns beside it (CMP-041).
func compiled(t *testing.T) *compiler.Bundle {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "schema", "testdata", "documents", "loan-calculator")
	res := compiler.Compile(os.DirFS(dir), compiler.DefaultOptions())
	if len(res.Plugins) == 0 || len(res.Plugins[0].SourceMap) == 0 {
		t.Fatalf("the conformance project did not compile to a plugin with a source map: %v", res.Diagnostics)
	}
	return res.Plugins[0]
}

// nodeIndex finds the index of the node written at pointer in the page with
// the key, reading the bundle's meta section and the source map directly.
func nodeIndex(t *testing.T, b *compiler.Bundle, pageKey, pointer string) uint32 {
	t.Helper()
	read, err := bundle.ReadStructure(b.Data)
	if err != nil {
		t.Fatal(err)
	}
	var meta *fbs.Meta
	for _, s := range read.Sections {
		if s.Kind == bundle.SectionMeta {
			meta = fbs.GetRootAsMeta(s.Data, 0)
		}
	}
	var page fbs.PageEntry
	var id fbs.Uuid
	found := false
	for i := range meta.PagesLength() {
		if meta.Pages(&page, i) && string(page.Key()) == pageKey {
			page.Id(&id)
			found = true
		}
	}
	if !found {
		t.Fatalf("the plugin has no page %q", pageKey)
	}
	sm := fbs.GetRootAsSourceMap(b.SourceMap, 0)
	var loc fbs.Location
	var sec fbs.Uuid
	for i := range sm.NodesLength() {
		sm.Nodes(&loc, i)
		loc.Section(&sec)
		if sec.Hi() == id.Hi() && sec.Lo() == id.Lo() && string(loc.Pointer()) == pointer {
			return loc.Index()
		}
	}
	t.Fatalf("the source map has no node %s on page %q", pointer, pageKey)
	return 0
}

// Verifies: ANL-040, CMP-041.
// A node path the runtime reports resolves, through the retained source map
// of a real compiled project, to the document and JSON pointer it was
// compiled from.
func TestResolve_ANL_040(t *testing.T) {
	t.Parallel()
	b := compiled(t)
	idx := nodeIndex(t, b, "calculator", amountNode)
	path := fmt.Sprintf("%s/calculator/%d", b.Key, idx)
	loc, err := symbolicate.Resolve(b.Data, b.SourceMap, path)
	if err != nil {
		t.Fatalf("Resolve(%q): %v", path, err)
	}
	if loc.Pointer != amountNode || filepath.Base(loc.File) == "" {
		t.Fatalf("Resolve(%q) = %+v, want pointer %s in a document file", path, loc, amountNode)
	}
	nested, err := symbolicate.Resolve(b.Data, b.SourceMap, path+"/3/1")
	if err != nil || nested != loc {
		t.Fatalf("a path nested below the node resolves to %+v, %v; want the node itself, %+v", nested, err, loc)
	}
}

// Verifies: ANL-040, CMP-041.
// A path that names nothing in the bundle and map is not found, and a
// malformed one is refused; neither is guessed at.
func TestResolveRefusals_ANL_040(t *testing.T) {
	t.Parallel()
	b := compiled(t)
	idx := nodeIndex(t, b, "calculator", amountNode)
	for name, tc := range map[string]struct {
		path     string
		notFound bool
	}{
		"another plugin":     {"other/calculator/" + fmt.Sprint(idx), true},
		"unknown page":       {b.Key + "/missing/" + fmt.Sprint(idx), true},
		"node out of range":  {b.Key + "/calculator/999999", true},
		"no index":           {b.Key + "/calculator", false},
		"index not a number": {b.Key + "/calculator/x", false},
		"empty":              {"", false},
	} {
		_, err := symbolicate.Resolve(b.Data, b.SourceMap, tc.path)
		if err == nil || errors.Is(err, symbolicate.ErrNotFound) != tc.notFound {
			t.Errorf("%s: Resolve(%q) = %v, want not found = %v", name, tc.path, err, tc.notFound)
		}
	}
	if _, err := symbolicate.Resolve([]byte("not a bundle"), b.SourceMap, b.Key+"/calculator/1"); err == nil {
		t.Error("a corrupt bundle was accepted")
	}
}
