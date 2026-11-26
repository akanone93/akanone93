// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// newValidator returns a validator of the conformance project.
func newValidator(t testing.TB, m fstest.MapFS) *Validator {
	t.Helper()
	v, diags := NewValidator(m, DefaultOptions())
	if v == nil || len(diags) > 0 {
		t.Fatalf("validator: %v", diags)
	}
	return v
}

// pageEdit returns the calculator page with an edit applied.
func pageEdit(t testing.TB, f func(doc map[string]any)) []byte {
	t.Helper()
	m := fixture(t)
	edit(t, m, calculatorPage, f)
	return m[calculatorPage].Data
}

// Verifies: SCH-042.
func TestValidatePage(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	redirect(t, m, calculatorPage, calculatorRedirect)
	v := newValidator(t, m)
	for _, tc := range []struct {
		name string
		file string
		data []byte
		code plxerr.Code
		ptr  string
	}{
		{name: "unchanged", file: calculatorPage, data: m[calculatorPage].Data},
		{name: "type error", file: calculatorPage, data: pageEdit(t, func(doc map[string]any) {
			at(t, doc, sliderNode)["props"].(map[string]any)["value"] = map[string]any{"$expr": "page.nothing"}
		}), code: plxerr.PXLUnknownField, ptr: "/" + sliderNode + "/props/value/$expr"},
		{name: "structural error", file: calculatorPage, data: []byte(`{"kind": "page"}`), code: plxerr.MissingProperty, ptr: ""},
		{name: "not JSON", file: calculatorPage, data: []byte(`{`), code: plxerr.InvalidJSON},
		{name: "new page", file: "plugins/loans/pages/extra.page.json", data: pageEdit(t, func(doc map[string]any) {
			doc["id"] = "01a0c450-6c00-7fff-8000-000000000010"
			doc["key"] = "extra"
			doc["route"] = "extra"
			at(t, doc, "root")["id"] = "01a0c450-6c00-7fff-8000-000000000011"
			at(t, doc, "root")["type"] = "Knob"
		}), code: plxerr.UnknownWidgetType, ptr: "/root/type"},
		{name: "state a graph needs", file: calculatorPage, data: pageEdit(t, func(doc map[string]any) {
			at(t, doc, "state/0")["name"] = "principal"
		}), code: plxerr.PXLUnknownField, ptr: "/steps/0/input/value/totalInterest/$expr"},
		{name: "redirect loop", file: resultPage, data: func() []byte {
			r := fixture(t)
			redirect(t, r, resultPage, resultRedirect)
			return r[resultPage].Data
		}(), code: plxerr.RedirectLoop, ptr: "/lifecycle/onEnter"},
		// The calculator page is listed first, so it claims the ID first.
		{name: "ID of another page", file: calculatorPage, data: pageEdit(t, func(doc map[string]any) {
			at(t, doc, amountNode)["id"] = "01a0c450-6c00-702c-8000-000000055114" // the result page's root
		}), code: plxerr.DuplicateID, ptr: "/" + amountNode + "/id"},
		{name: "route of another page", file: calculatorPage, data: pageEdit(t, func(doc map[string]any) {
			doc["route"] = "result"
		}), code: plxerr.DuplicateRouteName, ptr: "/route"},
		{name: "outside a plugin", file: "pages/extra.page.json", data: m[calculatorPage].Data, code: plxerr.InvalidProjectLayout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diags := v.ValidatePage(tc.file, tc.data)
			if tc.code == 0 {
				if len(diags) > 0 {
					t.Fatalf("diagnostics:\n%s", list(diags))
				}
				return
			}
			for _, d := range diags {
				if d.File != tc.file && !strings.Contains(d.File, "/actions/") {
					t.Errorf("diagnostic outside the page and its graphs: %s", d)
				}
			}
			found := false
			for _, d := range diags {
				found = found || d.Code == tc.code && strings.HasPrefix(d.Path, tc.ptr)
			}
			if !found {
				t.Errorf("no %s at %s; got:\n%s", tc.code, tc.ptr, list(diags))
			}
		})
	}
}

// Verifies: SCH-042.
// Validating an edit leaves the loaded project as it was: a broken edit
// followed by the original page reports nothing.
func TestValidatePageKeepsTheProject(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	v := newValidator(t, m)
	broken := pageEdit(t, func(doc map[string]any) { at(t, doc, sliderNode)["type"] = "Knob" })
	if diags := v.ValidatePage(calculatorPage, broken); !diags.HasErrors() {
		t.Fatal("the broken page was accepted")
	}
	if diags := v.ValidatePage(calculatorPage, m[calculatorPage].Data); len(diags) > 0 {
		t.Fatalf("the original page reports:\n%s", list(diags))
	}
	if res := compileFS(m); len(res.Diagnostics) > 0 {
		t.Fatalf("the project no longer compiles:\n%s", list(res.Diagnostics))
	}
}

// Verifies: CMP-052.
func TestNewValidatorReportsTheProject(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	m["app.json"] = &fstest.MapFile{Data: []byte("{")}
	v, diags := NewValidator(m, DefaultOptions())
	if v == nil || !diags.HasErrors() {
		t.Fatalf("validator %v, diagnostics %v", v, diags)
	}
	// A page of a project without an app still validates without panicking.
	for _, d := range v.ValidatePage(calculatorPage, m[calculatorPage].Data) {
		if d.Code == plxerr.InternalCompilerError {
			t.Errorf("internal error: %s", d)
		}
	}
}

// Verifies: PXL-007.
// PXLSites reports the scope of an expression of the page, so the language
// service completes against what that site can read.
func TestPXLSites(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	v := newValidator(t, m)
	data := pageEdit(t, func(doc map[string]any) {
		at(t, doc, sliderNode)["props"].(map[string]any)["value"] = map[string]any{"$expr": "page.nothing"}
	})
	sites, envs := v.PXLSites(calculatorPage, data)
	var site *PXLSite
	for i := range sites {
		if sites[i].Pointer == "/"+sliderNode+"/props/value" {
			site = &sites[i]
		}
	}
	if site == nil || site.Env < 0 || site.Env >= len(envs) {
		t.Fatalf("no site for the slider value in %v", sites)
	}
	if _, ok := envs[site.Env].Roots["page"]; !ok {
		t.Errorf("the page root is missing from %v", envs[site.Env].Roots)
	}
	if got, _ := v.PXLSites("pages/extra.page.json", data); got != nil {
		t.Errorf("a page outside a plugin has sites: %v", got)
	}
	if got, _ := v.PXLSites(calculatorPage, []byte(`{`)); got != nil {
		t.Errorf("a page that does not parse has sites: %v", got)
	}
}

// Verifies: I18N-006, I18N-007.
// ValidateProject reports what no page validation sees: a translation key
// nothing uses and the locales that lack its message, located in the app's
// translation documents, and nothing located in a page.
func TestValidateProjectReportsAppFindings(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	const unused = "01a0c450-6c00-7fff-8000-00000000beef"
	edit(t, m, "translations/keys.json", func(doc map[string]any) {
		doc["keys"] = append(doc["keys"].([]any), map[string]any{"id": unused, "key": "loans.unused"})
	})
	edit(t, m, "translations/en.json", func(doc map[string]any) {
		doc["messages"].(map[string]any)[unused] = "Never shown"
	})
	v := newValidator(t, m)
	diags := v.ValidateProject()
	var unusedKey, missing bool
	for _, d := range diags {
		switch {
		case d.Code == plxerr.TranslationKeyUnused && d.File == "translations/keys.json" && strings.Contains(d.Message, "loans.unused"):
			unusedKey = true
		case d.Code == plxerr.TranslationMissing && strings.Contains(d.Message, "loans.unused"):
			missing = true
		}
		if strings.HasSuffix(d.File, ".page.json") {
			t.Errorf("a page's finding is reported by ValidatePage, not here: %s", d)
		}
		if d.Code == plxerr.InternalCompilerError {
			t.Errorf("internal error: %s", d)
		}
	}
	if !unusedKey || !missing {
		t.Errorf("unused %v, missing %v:\n%s", unusedKey, missing, list(diags))
	}
}

// Verifies: STU-064, SCH-006.
// Every document is checked by the kind its path holds: a valid plugin manifest, canvas or app document is not
// reported as a misplaced page, and a broken one is reported against its own schema.
func TestValidateDocumentByKind(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	v := newValidator(t, m)
	for _, file := range []string{"app.json", "plugins/loans/plugin.json", "theme.json"} {
		if got := v.ValidateDocument(file, m[file].Data); got.HasErrors() {
			t.Errorf("%s as it is: %v", file, got)
		}
	}
	canvas := "plugins/loans/canvas.json"
	if got := v.ValidateDocument(canvas, []byte(`{"schemaVersion":"1.0.0","kind":"canvas","id":"01a0c450-6c00-7fff-8000-000000000020","positions":[]}`)); got.HasErrors() {
		t.Errorf("a valid canvas document: %v", got)
	}
	if got := v.ValidateDocument("plugins/loans/plugin.json", []byte(`{"kind":"plugin"}`)); !got.HasErrors() || got[0].Code == plxerr.InvalidProjectLayout {
		t.Errorf("a broken manifest is reported against its schema: %v", got)
	}
	renamed := strings.Replace(string(m["plugins/loans/plugin.json"].Data), `"key": "loans"`, `"key": "cards"`, 1)
	renamed = strings.Replace(renamed, `"key":"loans"`, `"key":"cards"`, 1)
	if got := v.ValidateDocument("plugins/loans/plugin.json", []byte(renamed)); !hasLayoutFinding(got) {
		t.Errorf("a manifest whose key is not its directory: %v", got)
	}
	if got := v.ValidateDocument("notes.txt", []byte(`{}`)); !hasLayoutFinding(got) {
		t.Errorf("a path outside the layout: %v", got)
	}
	if got := v.ValidateDocument(calculatorPage, m[calculatorPage].Data); got.HasErrors() {
		t.Errorf("a page still validates as a page: %v", got)
	}
}

// hasLayoutFinding reports whether a layout finding (PLX-1020) is among diags.
func hasLayoutFinding(diags plxerr.Diagnostics) bool {
	for _, d := range diags {
		if d.Code == plxerr.InvalidProjectLayout {
			return true
		}
	}
	return false
}

// Verifies: STU-062, SRV-060.
// Studio gives the browser validator empty stand-ins for the asset files the server stores: a listed file that has one
// is not reported missing, and one that has none still is (PLX-1020).
func TestValidatorTakesAssetStandIns(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	for name := range m {
		if strings.HasPrefix(name, "assets/") && name != "assets/index.json" {
			m[name] = &fstest.MapFile{Data: nil}
		}
	}
	v, diags := NewValidator(m, DefaultOptions())
	if v == nil || hasLayoutFinding(diags) {
		t.Fatalf("stand-ins are files that exist: %v", diags)
	}
	for name := range m {
		if strings.HasPrefix(name, "assets/") && name != "assets/index.json" {
			delete(m, name)
		}
	}
	if _, diags := NewValidator(m, DefaultOptions()); !hasLayoutFinding(diags) {
		t.Errorf("a listed file that is not stored is still reported: %v", diags)
	}
}

// Verifies: ACT-001.
// Without a function catalogue, as in Studio, validating a page does not
// refuse an expression that reads a granted function's output, whose type
// only the catalogue knows; any other expression is still checked, and a
// compilation without the catalogue still refuses the read. A compilation of
// drafts that builds no release (Options.Draft: the app canvas's counts and
// ValidateDraft) judges the read as validating a page does, so the counts
// agree with the editor's Problems (build log #184), and still checks every
// other expression.
func TestValidatePageWithoutCatalogueReadsFunctionOutput(t *testing.T) {
	t.Parallel()
	m := withFunctions(t, "loans.quote")
	v := newValidator(t, m)
	read := func(value string) []byte {
		return pageEdit(t, func(doc map[string]any) {
			doc["lifecycle"] = map[string]any{"onEnter": map[string]any{"steps": []any{
				map[string]any{
					"id": "quote", "action": "invokeFunction", "next": "keep",
					"input": map[string]any{"function": "loans.quote", "input": map[string]any{"months": 12}},
				},
				map[string]any{
					"id": "keep", "action": "setState",
					"input": map[string]any{"path": "page.months", "value": map[string]any{"$expr": value}},
				},
			}}}
		})
	}
	if diags := v.ValidatePage(calculatorPage, read("steps.quote.output?.months ?? 0")); diags.HasErrors() {
		t.Errorf("a function's output without a catalogue: %v", diags)
	}
	if diags := v.ValidatePage(calculatorPage, read("page.nothing")); !diags.HasErrors() {
		t.Error("an expression that reads no function output is still checked")
	}
	edit(t, m, calculatorPage, func(doc map[string]any) {
		var page map[string]any
		if err := json.Unmarshal(read("steps.quote.output?.months ?? 0"), &page); err != nil {
			t.Fatal(err)
		}
		doc["lifecycle"] = page["lifecycle"]
	})
	if res := Compile(m, DefaultOptions()); len(errorsOf(res)) == 0 {
		t.Error("a compilation without a catalogue refuses the read")
	}
	draft := DefaultOptions()
	draft.Draft = true
	if res := Compile(m, draft); len(errorsOf(res)) != 0 {
		t.Errorf("a compilation of drafts without a catalogue: %v", errorsOf(res))
	}
	edit(t, m, calculatorPage, func(doc map[string]any) {
		var page map[string]any
		if err := json.Unmarshal(read("page.nothing"), &page); err != nil {
			t.Fatal(err)
		}
		doc["lifecycle"] = page["lifecycle"]
	})
	if res := Compile(m, draft); len(errorsOf(res)) == 0 {
		t.Error("a compilation of drafts still checks an expression that reads no function output")
	}
}

// The function list Studio sends its validator types a function step's
// output, so the editor reports a field the output does not have (build log
// rows 133, 184; FN-004).
func TestValidatePageWithAFunctionListTypesTheOutput(t *testing.T) {
	t.Parallel()
	m := withFunctions(t, "loans.quote")
	fns, err := ParseFunctionList([]byte(`[{"name": "loans.quote", "placement": "server", "version": 3,
		"inputSchema": "{\"type\": \"object\", \"properties\": {\"months\": {\"type\": \"integer\"}}}",
		"outputSchema": "{\"type\": \"object\", \"properties\": {\"months\": {\"type\": \"integer\"}}, \"required\": [\"months\"]}"}]`))
	if err != nil {
		t.Fatal(err)
	}
	opts := DefaultOptions()
	opts.Functions = fns
	v, diags := NewValidator(m, opts)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	read := func(value string) []byte {
		return pageEdit(t, func(doc map[string]any) {
			doc["lifecycle"] = map[string]any{"onEnter": map[string]any{"steps": []any{
				map[string]any{
					"id": "quote", "action": "invokeFunction", "next": "keep",
					"input": map[string]any{"function": "loans.quote", "input": map[string]any{"months": 12}},
				},
				map[string]any{
					"id": "keep", "action": "setState",
					"input": map[string]any{"path": "page.months", "value": map[string]any{"$expr": value}},
				},
			}}}
		})
	}
	if diags := v.ValidatePage(calculatorPage, read("steps.quote.output?.months ?? 0")); diags.HasErrors() {
		t.Errorf("a field the output has: %v", diags)
	}
	if diags := v.ValidatePage(calculatorPage, read("steps.quote.output?.rate ?? 0")); !diags.HasErrors() {
		t.Error("a field the output does not have is not reported")
	}
	if _, err := ParseFunctionList([]byte(`[{"placement": "server"}]`)); err == nil {
		t.Error("an entry without a name is accepted")
	}
}
