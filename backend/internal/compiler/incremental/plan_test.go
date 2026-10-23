// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incremental

import (
	"slices"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// TestEditOfAGraphRecompilesItsPage checks that an edited action graph
// recompiles the page it is bound to, with the page's other graphs
// (CMP-051).
func TestEditOfAGraphRecompilesItsPage(t *testing.T) {
	t.Parallel()
	m := manyPages(t, 3, 20)
	c, _ := newCached(t, m)
	g := graphFiles(1)[0]
	edit(t, m, g, func(doc map[string]any) { doc["description"] = "edited" })
	res := incr(t, c, m)
	if res.Full {
		t.Fatalf("a full compilation: %s", res.FullReason)
	}
	want := append([]string{pageFile(1)}, graphFiles(1)...)
	slices.Sort(want)
	if !slices.Equal(res.Recompiled, want) {
		t.Errorf("recompiled %v, want %v", res.Recompiled, want)
	}
	mustMatch(t, res, full(m))
}

// TestGraphMovedToAnotherPage checks that a graph bound to another page, and
// one bound to none, give the bytes of the full compilation: here the first
// page still refers to the graph, which is a diagnostic, so the compiler
// declines to reuse anything (CMP-051, CMP-002).
func TestGraphMovedToAnotherPage(t *testing.T) {
	t.Parallel()
	m := manyPages(t, 3, 20)
	c, _ := newCached(t, m)
	g := graphFiles(0)[1]
	to := pageID(t, m, pageFile(1))
	edit(t, m, g, func(doc map[string]any) { doc["page"] = to })
	res := incr(t, c, m)
	mustMatch(t, res, full(m))
	if !res.Full {
		t.Error("a page refers to a graph of another page")
	}
	edit(t, m, g, func(doc map[string]any) { delete(doc, "page") })
	res = incr(t, c, m)
	if !res.Full {
		t.Error("an unbound graph is a flow, which is not a page")
	}
	mustMatch(t, res, full(m))
}

// TestUpdatesRefuseWhatTheCacheCannotReason checks the parse step's reasons.
func TestUpdatesRefuseWhatTheCacheCannotReason(t *testing.T) {
	t.Parallel()
	m := manyPages(t, 2, 12)
	c, _ := newCached(t, m)
	cases := map[string]func(){
		"not json":       func() { m[pageFile(0)].Data = []byte("{") },
		"identity (key)": func() { edit(t, m, pageFile(1), func(doc map[string]any) { doc["key"] = "other" }) },
		"graph identity": func() { edit(t, m, graphFiles(1)[0], func(doc map[string]any) { doc["key"] = "other" }) },
		"unknown page of a graph": func() {
			edit(t, m, graphFiles(1)[1], func(doc map[string]any) { doc["page"] = "01a0c450-6c00-7012-8000-0000000deadb" })
		},
	}
	for name, change := range cases {
		snap := map[string][]byte{}
		for k, f := range m {
			snap[k] = f.Data
		}
		change()
		res := incr(t, c, m)
		if !res.Full || res.FullReason == "" {
			t.Errorf("%s: full %v, reason %q", name, res.Full, res.FullReason)
		}
		mustMatch(t, res, full(m))
		for k, data := range snap {
			m[k].Data = data
		}
		if back := incr(t, c, m); back.Full {
			t.Errorf("%s: undoing the edit was a full compilation: %s", name, back.FullReason)
		}
	}
}

// TestDependantsFallBackForNonPages checks that a document that is not a
// page and refers to an edited page makes the compilation a full one, and
// that references among pages are followed to the fixpoint (CMP-051).
func TestDependantsFallBackForNonPages(t *testing.T) {
	t.Parallel()
	m := manyPages(t, 1, 12)
	l, err := compiler.NewLoader(opts())
	if err != nil {
		t.Fatal(err)
	}
	p, diags := l.Load(m)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	g := groupsOf(p)
	a, b := calculatorPage, resultPage
	target := pageID(t, m, b)
	// calculator -> result (a graph navigates), and a plugin file refers to
	// the calculator.
	byTarget := map[string][]string{target: {calculateGraph}}
	got, reason := dependants(map[string]bool{b: true}, g, g, byTarget)
	if reason != "" || !slices.Equal(got, []string{a, b}) {
		t.Errorf("dependants %v, reason %q", got, reason)
	}
	// The calculator is itself referred to by something that is no page.
	byTarget[pageID(t, m, a)] = []string{pluginFile}
	if got, reason := dependants(map[string]bool{b: true}, g, g, byTarget); reason == "" || got != nil {
		t.Errorf("dependants %v, reason %q", got, reason)
	}
	// Transitively: the copy refers to the calculator.
	byTarget = map[string][]string{target: {calculateGraph}, pageID(t, m, a): {pageFile(0)}}
	got, reason = dependants(map[string]bool{b: true}, g, g, byTarget)
	if want := []string{pageFile(0), a, b}; reason != "" || !slices.Equal(got, want) {
		t.Errorf("transitive dependants %v, reason %q", got, reason)
	}
	files := g.files([]string{a})
	if !slices.Contains(files, calculateGraph) || !slices.Contains(files, a) {
		t.Errorf("the files of the calculator are %v", files)
	}
}

// TestGroupsIgnoreBrokenDocuments checks that documents that failed to
// decode are left out of the index.
func TestGroupsIgnoreBrokenDocuments(t *testing.T) {
	t.Parallel()
	p := &schema.Project{Plugins: []schema.Plugin{{
		Pages:  []schema.Loaded[schema.PageDocument]{{}},
		Graphs: []schema.Loaded[schema.ActionGraphDocument]{{}},
	}}}
	g := groupsOf(p)
	if len(g.owner) != 0 || len(g.pages) != 0 || len(g.graphs) != 0 {
		t.Errorf("%+v", g)
	}
	if got := replaced(p.Plugins[0].Pages, nil); len(got) != 1 {
		t.Errorf("%d pages", len(got))
	}
}
