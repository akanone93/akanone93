// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
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
