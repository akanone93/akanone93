// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// supportedLocales lists the locales the app bundle's Meta names.
func supportedLocales(t *testing.T, res *Result) []string {
	t.Helper()
	m := metaOf(t, readAll(t, res)[0])
	var out []string
	for i := range m.SupportedLocalesLength() {
		out = append(out, string(m.SupportedLocales(i)))
	}
	return out
}

// Verifies: I18N-002, I18N-010. The app bundle lists the generated
// pseudo-locales among its supported locales, and only then.
func TestGeneratedPseudoLocalesAreSupportedLocales(t *testing.T) {
	t.Parallel()
	plain := supportedLocales(t, compileFS(fixture(t)))
	if slices.Contains(plain, "en-XA") || slices.Contains(plain, "ar-XB") {
		t.Errorf("pseudo-locales listed without being generated: %v", plain)
	}
	m := fixture(t)
	enableLocalisation(t, m, "pseudoLocales")
	got := supportedLocales(t, compileFS(m))
	if !slices.Contains(got, "en-XA") || !slices.Contains(got, "ar-XB") {
		t.Errorf("supported locales = %v, want the pseudo-locales too", got)
	}
}

// Verifies: I18N-007. A literal page title warns like a literal prop.
func TestLiteralPageTitleWarns(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	edit(t, m, calculatorPage, func(doc map[string]any) { doc["title"] = "Loan calculator" })
	res := compileFS(m)
	wantDiag(t, res, plxerr.LiteralTextUntranslated, calculatorPage, "/title")
	if res.Diagnostics.HasErrors() {
		t.Errorf("a literal must only warn:\n%s", list(res.Diagnostics))
	}
	edit(t, m, calculatorPage, func(doc map[string]any) { doc["title"] = map[string]any{"$t": keyLoansTitle} })
	if countCode(compileFS(m), plxerr.LiteralTextUntranslated) != 0 {
		t.Error("a translated title still warns")
	}
}

// Verifies: I18N-008. The localisation report matches its JSON Schema.
func TestLocalisationReportMatchesItsSchema(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	enableLocalisation(t, m, "requireApprovedTranslations")
	enableLocalisation(t, m, "pseudoLocales")
	res := compileFS(m)
	if res.Localisation == nil {
		t.Fatalf("no report:\n%s", list(res.Diagnostics))
	}
	data, err := json.Marshal(res.Localisation)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join("..", "..", "..", "schema", "json", "reports", "l10n-report.schema.json")) //nolint:gosec // G304: the repository's schema.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	const url = "https://plux.dev/schema/l10n-report.schema.json"
	if err := c.AddResource(url, doc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Errorf("the report does not match its schema: %v\n%s", err, data)
	}
	bad, _ := jsonschema.UnmarshalJSON(strings.NewReader(`{"defaultLocale": "en", "requireApproved": false, "locales": []}`))
	if sch.Validate(bad) == nil {
		t.Error("the schema accepts a report without unusedKeys")
	}
}
