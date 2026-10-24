// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// firstTemplate names a template file of the fixture.
func firstTemplate(m fstest.MapFS) string {
	for name := range m {
		if rest, ok := strings.CutPrefix(name, "templates/"); ok {
			return rest
		}
	}
	return ""
}

// withAssetChange returns the project with its asset files changed and one
// added.
func withAssetChange(p *schema.Project) *schema.Project {
	n := *p
	n.AssetFiles = map[string][]byte{}
	for name, data := range p.AssetFiles {
		n.AssetFiles[name] = append([]byte("changed"), data...)
	}
	n.AssetFiles["added.bin"] = []byte("added")
	return &n
}

// TestRecompileDeclines checks every reason Recompile has to decline: the
// caller then runs a full compilation (CMP-051).
func TestRecompileDeclines(t *testing.T) {
	t.Parallel()
	base := fixture(t)
	_, memo := CompileMemo(base, DefaultOptions())
	if memo == nil {
		t.Fatal("no memo")
	}
	project := loadProject(t, base)
	changed := func(f func(m fstest.MapFS)) *schema.Project {
		m := fixture(t)
		f(m)
		return loadProject(t, m)
	}
	cases := map[string]struct {
		memo    *Memo
		project *schema.Project
		dirty   []string
	}{
		"no memo":       {nil, project, []string{calculatorPage}},
		"no project":    {memo, nil, []string{calculatorPage}},
		"nothing dirty": {memo, project, nil},
		"not a page":    {memo, project, []string{pluginFile}},
		"unknown page":  {memo, project, []string{"plugins/loans/pages/none.page.json"}},
		"other document changed": {memo, changed(func(m fstest.MapFS) {
			edit(t, m, pluginFile, func(doc map[string]any) { doc["description"] = "edited" })
		}), []string{calculatorPage}},
		"other page changed": {memo, changed(func(m fstest.MapFS) {
			edit(t, m, resultPage, func(doc map[string]any) { doc["description"] = "edited" })
		}), []string{calculatorPage}},
		"document removed": {memo, changed(func(m fstest.MapFS) {
			delete(m, "templates/"+firstTemplate(m))
		}), []string{calculatorPage}},
		"asset changed": {memo, withAssetChange(project), []string{calculatorPage}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if res, next, ok := Recompile(tc.memo, tc.project, DefaultOptions(), tc.dirty); ok || res != nil || next != nil {
				t.Error("Recompile did not decline")
			}
		})
	}
}

// TestRecompileDeclinesWhatItCannotShow checks the cases in which the edit is
// the cause: a diagnostic on the edited page, and a page that stops
// contributing to a project-wide aggregate (CMP-051).
func TestRecompileDeclinesWhatItCannotShow(t *testing.T) {
	t.Parallel()
	withIcon := fixture(t)
	setProp(amountNode, "decoration", `{"labelText": {"$t": "01a0c450-6c00-7003-8000-000000005ccd"}, "suffixIcon": {"name": "home"}}`)(t, withIcon)
	_, iconMemo := CompileMemo(withIcon, DefaultOptions())
	if iconMemo == nil {
		t.Fatal("no memo")
	}
	t.Run("diagnostic", func(t *testing.T) {
		t.Parallel()
		m := fixture(t)
		edit(t, m, calculatorPage, func(doc map[string]any) { at(t, doc, sliderNode)["type"] = "NoSuchWidget" })
		_, memo := CompileMemo(fixture(t), DefaultOptions())
		if _, _, ok := Recompile(memo, loadProject(t, m), DefaultOptions(), []string{calculatorPage}); ok {
			t.Error("Recompile accepted a page with a diagnostic")
		}
	})
	t.Run("aggregate shrinks", func(t *testing.T) {
		t.Parallel()
		if _, _, ok := Recompile(iconMemo, loadProject(t, fixture(t)), DefaultOptions(), []string{calculatorPage}); ok {
			t.Error("Recompile accepted a page that no longer contributes an icon")
		}
	})
	t.Run("aggregate grows", func(t *testing.T) {
		t.Parallel()
		_, memo := CompileMemo(fixture(t), DefaultOptions())
		res, _, ok := Recompile(memo, loadProject(t, withIcon), DefaultOptions(), []string{calculatorPage})
		if !ok {
			t.Fatal("Recompile declined a page that contributes an icon")
		}
		opts := DefaultOptions()
		sameBundles(t, res, Compile(withIcon, opts))
	})
}

// TestMemoSourcesCoverTheProject checks that sourcesOf finds every document
// the loader loaded, so a document kind added to the project model cannot
// escape the check that nothing outside the edited pages changed.
func TestMemoSourcesCoverTheProject(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	var documents int
	for name := range m {
		if strings.HasSuffix(name, ".json") {
			documents++
		}
	}
	if got := len(sourcesOf(loadProject(t, m))); got != documents {
		t.Errorf("sourcesOf finds %d documents, the project has %d", got, documents)
	}
}

// TestCompileMemoWithDiagnostics checks that a compilation with diagnostics
// leaves no Memo, because a recompilation cannot reproduce them.
func TestCompileMemoWithDiagnostics(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	edit(t, m, calculatorPage, func(doc map[string]any) { at(t, doc, sliderNode)["type"] = "NoSuchWidget" })
	if res, memo := CompileMemo(m, DefaultOptions()); memo != nil || res.App != nil {
		t.Error("a memo for a project with errors")
	}
	if _, memo := CompileMemo(fstest.MapFS{}, DefaultOptions()); memo != nil {
		t.Error("a memo for an empty project")
	}
}
