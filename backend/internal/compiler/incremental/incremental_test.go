// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incremental

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
)

// newCached compiles m once and returns the cache and the first result.
func newCached(t testing.TB, m fstest.MapFS) (*Cache, *Result) {
	t.Helper()
	c := NewCache(opts())
	res := incr(t, c, m)
	if res.App == nil {
		t.Fatalf("the project does not compile:\n%s", strings.Join(diagLines(res.Diagnostics), "\n"))
	}
	return c, res
}

// setMax raises the maximum of the slider of a calculator page, an edit that
// touches no other document.
func setMax(t testing.TB, m fstest.MapFS, file string) {
	t.Helper()
	edit(t, m, file, func(doc map[string]any) { at(t, doc, sliderNode)["props"].(map[string]any)["max"] = 72 })
}

// pageSectionChanged reports whether the changed sections include the
// section of the page with the ID.
func pageSectionChanged(r *Result, id string) bool {
	return slices.ContainsFunc(r.Changed, func(s Section) bool {
		return s.Kind == bundle.SectionPage && uuid7.UUID(s.ID).String() == id
	})
}

// TestFirstCompileIsFull checks that the first call compiles everything and
// reports every section as changed (CMP-051).
func TestFirstCompileIsFull(t *testing.T) {
	t.Parallel()
	m := manyPages(t, 3, 20)
	res := incr(t, NewCache(opts()), m)
	if !res.Full || res.FullReason == "" {
		t.Errorf("Full = %v, reason %q", res.Full, res.FullReason)
	}
	mustMatch(t, res, full(m))
	if len(res.Changed) == 0 || len(res.Removed) != 0 {
		t.Errorf("%d changed and %d removed sections on a first compilation", len(res.Changed), len(res.Removed))
	}
	if len(res.Pages) != 5 {
		t.Errorf("%d page verdicts, want 5", len(res.Pages))
	}
	for _, v := range res.Pages {
		if !v.Compatible {
			t.Errorf("new page %s is incompatible: %v", v.File, v.Incompatible)
		}
	}
}

// TestEditLeafRecompilesItAndNothingElse checks that editing a page that
// nothing refers to recompiles that page and its graphs only, and that the
// bytes are the full compiler's (CMP-051, CMP-002).
func TestEditLeafRecompilesItAndNothingElse(t *testing.T) {
	t.Parallel()
	m := manyPages(t, 6, 30)
	c, _ := newCached(t, m)
	setMax(t, m, pageFile(2))
	res := incr(t, c, m)
	if res.Full {
		t.Fatalf("a full compilation: %s", res.FullReason)
	}
	want := append([]string{pageFile(2)}, graphFiles(2)...)
	slices.Sort(want)
	if !slices.Equal(res.Recompiled, want) {
		t.Errorf("recompiled %v, want %v", res.Recompiled, want)
	}
	mustMatch(t, res, full(m))
	// Of the page sections, only the edited page's changes.
	var pages int
	for _, s := range res.Changed {
		if s.Kind == bundle.SectionPage {
			pages++
		}
	}
	if pages != 1 || !pageSectionChanged(res, pageID(t, m, pageFile(2))) {
		t.Errorf("%d page sections changed, want the one of %s", pages, pageFile(2))
	}
	if len(res.Pages) != 1 || res.Pages[0].File != pageFile(2) || !res.Pages[0].Compatible {
		t.Errorf("verdicts %+v", res.Pages)
	}
}

// TestEditRecompilesDependants checks that the pages that refer to an edited
// page are recompiled with their graphs (CMP-051).
func TestEditRecompilesDependants(t *testing.T) {
	t.Parallel()
	m := manyPages(t, 2, 20)
	c, _ := newCached(t, m)
	// The calculate graph of the calculator and of each copy navigates to
	// the result page, so the result page's dependants are all of them.
	edit(t, m, resultPage, func(doc map[string]any) { doc["description"] = "edited" })
	res := incr(t, c, m)
	if res.Full {
		t.Fatalf("a full compilation: %s", res.FullReason)
	}
	for _, f := range []string{resultPage, calculatorPage, calculateGraph, pageFile(0), pageFile(1)} {
		if !slices.Contains(res.Recompiled, f) {
			t.Errorf("%s was not recompiled; got %v", f, res.Recompiled)
		}
	}
	mustMatch(t, res, full(m))
}

// TestLeafEditDoesNotRecompileTheReferencedPage checks the other direction:
// a page that refers to another does not make the other a dependant.
func TestLeafEditDoesNotRecompileTheReferencedPage(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	c, _ := newCached(t, m)
	setMax(t, m, calculatorPage)
	res := incr(t, c, m)
	if res.Full {
		t.Fatalf("a full compilation: %s", res.FullReason)
	}
	if slices.Contains(res.Recompiled, resultPage) {
		t.Errorf("the result page was recompiled: %v", res.Recompiled)
	}
	mustMatch(t, res, full(m))
}

// TestNothingChanged checks that a call with the same files compiles nothing.
func TestNothingChanged(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	c, first := newCached(t, m)
	res := incr(t, c, m)
	if res.Full || len(res.Recompiled) != 0 || len(res.Changed) != 0 || len(res.Pages) != 0 {
		t.Errorf("full %v, recompiled %v, changed %d", res.Full, res.Recompiled, len(res.Changed))
	}
	if res.App != first.App {
		t.Error("the bundles were made again")
	}
}

// TestCosmeticEditRecompilesNothing checks that a change of the file that
// leaves the document the same recompiles nothing (CMP-051).
func TestCosmeticEditRecompilesNothing(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	c, _ := newCached(t, m)
	m[calculatorPage] = &fstest.MapFile{Data: []byte(strings.ReplaceAll(string(m[calculatorPage].Data), "\n", "\n\n"))}
	res := incr(t, c, m)
	if res.Full || len(res.Recompiled) != 0 || len(res.Changed) != 0 {
		t.Errorf("full %v, recompiled %v, changed %d", res.Full, res.Recompiled, len(res.Changed))
	}
	mustMatch(t, res, full(m))
	// The cosmetic edit is now the base: a real edit is incremental.
	setMax(t, m, calculatorPage)
	res = incr(t, c, m)
	if res.Full {
		t.Errorf("a full compilation: %s", res.FullReason)
	}
	mustMatch(t, res, full(m))
}

// TestEditsOutsidePagesAreFull checks that a change to any other document,
// and to the set of files, falls back to a full compilation with a reason
// and the same bytes (CMP-051, CMP-002).
func TestEditsOutsidePagesAreFull(t *testing.T) {
	t.Parallel()
	edits := map[string]func(t *testing.T, m fstest.MapFS){
		"plugin": func(t *testing.T, m fstest.MapFS) {
			edit(t, m, pluginFile, func(doc map[string]any) { doc["description"] = "edited" })
		},
		"theme": func(t *testing.T, m fstest.MapFS) {
			edit(t, m, "theme.json", func(doc map[string]any) { doc["description"] = "edited" })
		},
		"app": func(t *testing.T, m fstest.MapFS) {
			edit(t, m, "app.json", func(doc map[string]any) { doc["description"] = "edited" })
		},
		"added file": func(t *testing.T, m fstest.MapFS) {
			m["notes.txt"] = &fstest.MapFile{Data: []byte("a note")}
		},
		"removed file": func(t *testing.T, m fstest.MapFS) {
			delete(m, "templates/"+firstTemplate(m))
		},
		"page identity": func(t *testing.T, m fstest.MapFS) {
			edit(t, m, resultPage, func(doc map[string]any) { doc["id"] = "01a0c450-6c00-7013-8000-000000024bbe" })
		},
	}
	for name, change := range edits {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := fixture(t)
			c, _ := newCached(t, m)
			change(t, m)
			res := incr(t, c, m)
			if !res.Full || res.FullReason == "" {
				t.Errorf("Full = %v, reason %q", res.Full, res.FullReason)
			}
			mustMatch(t, res, full(m))
		})
	}
}

// firstTemplate returns the name of a template file of the fixture.
func firstTemplate(m fstest.MapFS) string {
	for name := range m {
		if rest, ok := strings.CutPrefix(name, "templates/"); ok {
			return rest
		}
	}
	return ""
}

// TestDiagnosticsFallBackToTheFullCompiler checks that an edit with a problem
// gives the full compiler's diagnostics and no sections, that the cache
// goes on from the last good compilation, and that fixing the page is an
// incremental compilation (CMP-051).
func TestDiagnosticsFallBackToTheFullCompiler(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	c, _ := newCached(t, m)
	good := string(m[calculatorPage].Data)
	edit(t, m, calculatorPage, func(doc map[string]any) { at(t, doc, sliderNode)["type"] = "NoSuchWidget" })
	res := incr(t, c, m)
	if !res.Full || len(res.Diagnostics) == 0 || res.App != nil || len(res.Changed) != 0 {
		t.Fatalf("full %v, %d diagnostics, app %v", res.Full, len(res.Diagnostics), res.App != nil)
	}
	mustMatch(t, res, full(m))
	if again := incr(t, c, m); again != res {
		t.Error("the same broken files were compiled twice")
	}
	m[calculatorPage] = &fstest.MapFile{Data: []byte(good)}
	setMax(t, m, calculatorPage)
	res = incr(t, c, m)
	if res.Full {
		t.Errorf("fixing the page was a full compilation: %s", res.FullReason)
	}
	mustMatch(t, res, full(m))
	if len(res.Changed) == 0 {
		t.Error("no changed sections measured against the last good compilation")
	}
}

// TestContextAndArguments checks the error cases of Compile.
func TestContextAndArguments(t *testing.T) {
	t.Parallel()
	if _, err := Compile(context.Background(), nil, Input{}); err == nil {
		t.Error("no error without a cache")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Compile(ctx, NewCache(opts()), Input{FS: fixture(t)}); err == nil {
		t.Error("no error for a done context")
	}
}

// TestWarningsAreNeverReused checks that a project whose compilation has
// diagnostics other than errors still compiles, in full each time, because a
// recompilation cannot reproduce the diagnostics of the whole project.
func TestWarningsAreNeverReused(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	edit(t, m, "app.json", func(doc map[string]any) { doc["requiredFeatures"] = "raise"; doc["minRuntimeVersion"] = "0.1.0" })
	if len(full(m).Diagnostics) == 0 {
		t.Skip("the fixture raises no warning with this edit")
	}
	c := NewCache(opts())
	first := incr(t, c, m)
	mustMatch(t, first, full(m))
	setMax(t, m, calculatorPage)
	res := incr(t, c, m)
	if !res.Full {
		t.Error("recompiled a project with warnings")
	}
	mustMatch(t, res, full(m))
}
