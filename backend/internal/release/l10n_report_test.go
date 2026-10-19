// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/compiler"
)

// sampleL10n is a report with a plugin entry, a pseudo-locale and a key name
// that needs escaping.
func sampleL10n() *compiler.L10nReport {
	return &compiler.L10nReport{
		DefaultLocale: "en", RequireApproved: true,
		Locales: []compiler.L10nCoverage{
			{Locale: "de", Keys: 3, Translated: 2, Approved: 1, Missing: 1, LeftOut: 1, MissingKeys: []string{"a.b"}, LeftOutKeys: []string{"<x>"}},
			{Locale: "de", Plugin: "shop", Keys: 2, Translated: 2, Approved: 1, LeftOut: 1},
			{Locale: "en-XA", Pseudo: true, Keys: 3, Translated: 3},
		},
		UnusedKeys: []string{"old.key"},
	}
}

// Verifies: I18N-008.
func TestLocalisationReportHasAProducerAndARenderer(t *testing.T) {
	t.Parallel()
	produce, ok := reportProducers()[ReportLocalisation]
	if !ok {
		t.Fatal("no producer for the localisation report")
	}
	if v, err := produce(&compiler.Result{}); v != nil || err != nil {
		t.Errorf("a compilation without a report produced %v, %v", v, err)
	}
	reports, err := canonicalReports(&compiler.Result{Localisation: sampleL10n()})
	if err != nil {
		t.Fatal(err)
	}
	raw := reports[ReportLocalisation]
	if len(raw) == 0 {
		t.Fatal("the report is not stored")
	}
	var back compiler.L10nReport
	if err := json.Unmarshal(raw, &back); err != nil || back.Locales[1].Plugin != "shop" || back.Locales[0].LeftOut != 1 {
		t.Errorf("the canonical JSON does not round-trip: %v\n%s", err, raw)
	}
	again, err := canonicalReports(&compiler.Result{Localisation: sampleL10n()})
	if err != nil || !bytes.Equal(again[ReportLocalisation], raw) {
		t.Errorf("the canonical JSON is not deterministic: %v", err)
	}
	if _, ok := reportRenderers()[ReportLocalisation]; !ok {
		t.Fatal("no HTML renderer for the localisation report")
	}
}

// Verifies: I18N-008.
func TestLocalisationReportRendersStandaloneHTML(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(sampleL10n())
	if err != nil {
		t.Fatal(err)
	}
	page, err := renderL10nHTML(raw)
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	for _, want := range []string{"Plux localisation report", "<code>en-XA</code> (pseudo)", "whole app", "shop", "old.key", "&lt;x&gt;", "left out of the release"} {
		if !strings.Contains(html, want) {
			t.Errorf("the page lacks %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "<x>") || strings.Contains(html, "<script") {
		t.Error("the page does not escape its values or carries a script")
	}
	again, err := renderL10nHTML(raw)
	if err != nil || !bytes.Equal(page, again) {
		t.Errorf("the page is not deterministic: %v", err)
	}
	if _, err := renderL10nHTML([]byte("{")); err == nil {
		t.Error("a broken report must fail")
	}
}
