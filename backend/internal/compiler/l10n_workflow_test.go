// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/l10n/pseudo"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const (
	appJSON = "app.json"
	// Keys of the loan-calculator fixture.
	keyLoansAmount = "01a0c450-6c00-7003-8000-000000005ccd"
	keyLoansMonths = "01a0c450-6c00-7004-8000-000000007bbc"
	keyLoansTitle  = "01a0c450-6c00-7002-8000-000000003dde"
	// The IDs the tests give the nodes and keys they add.
	literalNode = "01a0c450-6c00-7fff-8000-0000000003a1"
	unusedKeyID = "01a0c450-6c00-7fff-8000-0000000003b1"
)

// withLiteral adds a Text node with the props to the calculator's column.
func withLiteral(t *testing.T, m fstest.MapFS, node map[string]any) {
	t.Helper()
	edit(t, m, calculatorPage, func(doc map[string]any) {
		col := at(t, doc, column)
		col["children"] = append(col["children"].([]any), node)
	})
}

// onlyEnglish limits the app to its default locale.
func onlyEnglish(t *testing.T, m fstest.MapFS) {
	t.Helper()
	edit(t, m, appJSON, func(doc map[string]any) { doc["supportedLocales"] = []any{"en"} })
}

// localisation sets a setting of the app's localisation object.
func localisation(t *testing.T, m fstest.MapFS, name string, v any) {
	t.Helper()
	edit(t, m, appJSON, func(doc map[string]any) {
		l, _ := doc["localisation"].(map[string]any)
		if l == nil {
			l = map[string]any{}
			doc["localisation"] = l
		}
		l[name] = v
	})
}

// countCode counts the diagnostics with a code.
func countCode(res *Result, code plxerr.Code) int {
	n := 0
	for _, d := range res.Diagnostics {
		if d.Code == code {
			n++
		}
	}
	return n
}

// Verifies: I18N-007.
func TestLiteralTextInATranslatablePropWarns(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	withLiteral(t, m, map[string]any{
		"id": literalNode, "type": "Text", "props": map[string]any{"data": "Hello", "semanticsLabel": "Greeting"},
	})
	res := compileFS(m)
	ptr := column + "/children/6"
	wantDiag(t, res, plxerr.LiteralTextUntranslated, calculatorPage, "/"+ptr+"/props/data")
	wantDiag(t, res, plxerr.LiteralTextUntranslated, calculatorPage, "/"+ptr+"/props/semanticsLabel")
	if res.Diagnostics.HasErrors() {
		t.Errorf("a literal must only warn:\n%s", list(res.Diagnostics))
	}
	for _, d := range res.Diagnostics {
		if strings.Contains(d.Message, "Hello") {
			t.Errorf("the diagnostic repeats the text: %s", d.Message)
		}
	}
}

// Verifies: I18N-007.
func TestLiteralSemanticsLabelWarns(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	withLiteral(t, m, map[string]any{
		"id": literalNode, "type": "Text",
		"props":     map[string]any{"data": map[string]any{"$t": keyLoansAmount}},
		"semantics": map[string]any{"label": "Read aloud"},
	})
	res := compileFS(m)
	wantDiag(t, res, plxerr.LiteralTextUntranslated, calculatorPage, "/"+column+"/children/6/semantics/label")
}

// Verifies: I18N-007.
func TestTranslatedAndNonTextLiteralsDoNotWarn(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	withLiteral(t, m, map[string]any{
		"id": literalNode, "type": "Text",
		"props": map[string]any{"data": map[string]any{"$t": keyLoansMonths}, "semanticsLabel": "42"},
	})
	res := compileFS(m)
	if n := countCode(res, plxerr.LiteralTextUntranslated); n != 0 {
		t.Errorf("PLX-1609 reported %d times:\n%s", n, list(res.Diagnostics))
	}
}

// Verifies: I18N-007.
func TestAppInOneLocaleHasNothingToTranslate(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	onlyEnglish(t, m)
	withLiteral(t, m, map[string]any{"id": literalNode, "type": "Text", "props": map[string]any{"data": "Hello"}})
	if n := countCode(compileFS(m), plxerr.LiteralTextUntranslated); n != 0 {
		t.Errorf("PLX-1609 reported %d times in a one-locale app", n)
	}
}

// Verifies: I18N-007.
func TestUnusedKeyWithADefaultMessageWarns(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	edit(t, m, messageKeys, func(doc map[string]any) {
		doc["keys"] = append(doc["keys"].([]any),
			map[string]any{"id": unusedKeyID, "key": "loans.unused"},
			map[string]any{"id": "01a0c450-6c00-7fff-8000-0000000003b2", "key": "loans.untranslated"})
	})
	edit(t, m, messageEN, func(doc map[string]any) { at(t, doc, "messages")[unusedKeyID] = "Not shown" })
	res := compileFS(m)
	wantDiag(t, res, plxerr.TranslationKeyUnused, messageKeys, "/keys/7")
	if n := countCode(res, plxerr.TranslationKeyUnused); n != 1 {
		t.Errorf("PLX-1608 reported %d times, want only the key with a default message:\n%s", n, list(res.Diagnostics))
	}
	if res.Diagnostics.HasErrors() {
		t.Errorf("an unused key must only warn:\n%s", list(res.Diagnostics))
	}
}

// Verifies: I18N-007.
func TestPXLTranslationCallUsesAKey(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	edit(t, m, messageKeys, func(doc map[string]any) {
		doc["keys"] = append(doc["keys"].([]any), map[string]any{"id": unusedKeyID, "key": "loans.by-code"})
	})
	edit(t, m, messageEN, func(doc map[string]any) { at(t, doc, "messages")[unusedKeyID] = "By code" })
	withLiteral(t, m, map[string]any{
		"id": literalNode, "type": "Text", "props": map[string]any{"data": map[string]any{"$expr": `t("loans.by-code")`}},
	})
	res := compileFS(m)
	if n := countCode(res, plxerr.TranslationKeyUnused); n != 0 {
		t.Errorf("a key read with t() is used:\n%s", list(res.Diagnostics))
	}
}

// approve sets the review states of the German translations.
func approve(t *testing.T, m fstest.MapFS, states map[string]any) {
	t.Helper()
	edit(t, m, messageDE, func(doc map[string]any) { doc["states"] = states })
}

// section reads the messages of a locale section of the app bundle, by
// key ID, or nil when the bundle has none.
func section(t *testing.T, res *Result, tag string) map[string]string {
	t.Helper()
	id, err := bundle.LocaleID(tag)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := readAll(t, res)[0].Section(bundle.SectionL10n, id)
	if !ok {
		return nil
	}
	loc := fbs.GetRootAsLocale(s.Data, 0)
	out := map[string]string{}
	for i := range loc.MessagesLength() {
		var msg fbs.Message
		loc.Messages(&msg, i)
		k := msg.Key(nil)
		var id [16]byte
		binary.BigEndian.PutUint64(id[:8], k.Hi())
		binary.BigEndian.PutUint64(id[8:], k.Lo())
		out[uuidString(id)] = string(msg.Text())
	}
	return out
}

// Verifies: I18N-008.
func TestUnapprovedTranslationsAreLeftOutWhenApprovalIsRequired(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	localisation(t, m, "requireApprovedTranslations", true)
	approve(t, m, map[string]any{keyLoansAmount: "approved", keyLoansTitle: "approved", keyLoansMonths: "reviewed"})
	res := compileFS(m)
	if res.Diagnostics.HasErrors() {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	de := section(t, res, "de-DE")
	if len(de) != 2 || de[keyLoansAmount] != "Betrag" {
		t.Errorf("de-DE holds %v, want only the two approved messages", de)
	}
	if en := section(t, res, "en"); len(en) != 7 {
		t.Errorf("the default locale ships %d messages, want all 7", len(en))
	}
	perFile := map[string]int{}
	for _, d := range res.Diagnostics {
		if d.Code != plxerr.TranslationNotApproved {
			continue
		}
		perFile[d.File]++
		if d.Severity != plxerr.SeverityInfo {
			t.Errorf("PLX-1610 = %s, want info", d)
		}
		if d.File == messageDE && !strings.Contains(d.Message, "5 of 7") {
			t.Errorf("PLX-1610 = %s", d)
		}
	}
	if perFile[messageDE] != 1 || perFile["translations/am-ET.json"] != 1 || len(perFile) != 2 {
		t.Errorf("PLX-1610 per file = %v, want once per locale with unapproved texts", perFile)
	}
}

// Verifies: I18N-008.
func TestAllTranslationsShipWithoutTheApprovalSetting(t *testing.T) {
	t.Parallel()
	res := compileFS(fixture(t))
	if de := section(t, res, "de-DE"); len(de) != 7 {
		t.Errorf("de-DE holds %d messages, want 7", len(de))
	}
	if n := countCode(res, plxerr.TranslationNotApproved); n != 0 {
		t.Errorf("PLX-1610 reported without the setting")
	}
}

// Verifies: I18N-002, I18N-010.
func TestPseudoLocalesAreGeneratedFromTheDefaultLocale(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	localisation(t, m, "pseudoLocales", true)
	res := compileFS(m)
	if res.Diagnostics.HasErrors() {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	en := section(t, res, "en")
	for _, tag := range pseudo.Tags() {
		got := section(t, res, tag)
		if len(got) != len(en) {
			t.Fatalf("%s holds %d messages, want %d", tag, len(got), len(en))
		}
		want, err := pseudo.Message(tag, en[keyLoansTitle])
		if err != nil || got[keyLoansTitle] != want {
			t.Errorf("%s title = %q, want %q (%v)", tag, got[keyLoansTitle], want, err)
		}
	}
	if !strings.HasPrefix(section(t, res, "en-XA")[keyLoansAmount], "[") {
		t.Error("en-XA is not bracketed")
	}
	again := compileFS(m)
	if !reflect.DeepEqual(res.App.Data, again.App.Data) {
		t.Error("the pseudo-locales are not deterministic (CMP-002)")
	}
	if got := res.Localisation; got == nil || !got.Locales[len(got.Locales)-1].Pseudo {
		t.Errorf("the report does not list the pseudo-locales last: %+v", got)
	}
}

// Verifies: I18N-002, I18N-010.
func TestPseudoLocalesCarryTheDataOfTheirBaseLocale(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	localisation(t, m, "pseudoLocales", true)
	res := compileFS(m)
	for tag, want := range map[string]string{"en-XA": "en", "ar-XB": "ar"} {
		id, _ := bundle.LocaleID(tag)
		s, ok := readAll(t, res)[0].Section(bundle.SectionL10n, id)
		if !ok {
			t.Fatalf("no %s section", tag)
		}
		loc := fbs.GetRootAsLocale(s.Data, 0)
		if string(loc.Tag()) != tag || string(loc.TagResolved()) != want {
			t.Errorf("%s: tag %q resolved %q, want resolved %q", tag, loc.Tag(), loc.TagResolved(), want)
		}
		if len(loc.DataBytes()) == 0 {
			t.Errorf("%s has no locale data", tag)
		}
	}
}

// Verifies: I18N-002, I18N-010.
func TestPseudoLocalesInAProductionBuildAreRefused(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	localisation(t, m, "pseudoLocales", true)
	opts := DefaultOptions()
	opts.Production = true
	res := Compile(m, opts)
	wantDiag(t, res, plxerr.PseudoLocaleInProduction, appJSON, "/localisation/pseudoLocales")
	if res.App != nil {
		t.Error("a production build with pseudo-locales produced a bundle")
	}
	opts.Production = false
	if res := Compile(m, opts); res.Diagnostics.HasErrors() {
		t.Errorf("the same project must build outside production:\n%s", list(res.Diagnostics))
	}
	off := fixture(t)
	opts.Production = true
	if res := Compile(off, opts); res.Diagnostics.HasErrors() || section(t, res, "en-XA") != nil {
		t.Errorf("a production build without the setting must be clean and ship none:\n%s", list(res.Diagnostics))
	}
}

// Verifies: I18N-008.
func TestLocalisationReportCountsPerLocaleAndPlugin(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	localisation(t, m, "requireApprovedTranslations", true)
	approve(t, m, map[string]any{keyLoansAmount: "approved", keyLoansTitle: "approved"})
	edit(t, m, messageDE, func(doc map[string]any) { delete(at(t, doc, "messages"), keyLoansMonths) })
	edit(t, m, messageKeys, func(doc map[string]any) {
		doc["keys"] = append(doc["keys"].([]any), map[string]any{"id": unusedKeyID, "key": "loans.unused"})
	})
	res := compileFS(m)
	r := res.Localisation
	if r == nil {
		t.Fatalf("no report; diagnostics:\n%s", list(res.Diagnostics))
	}
	if r.DefaultLocale != "en" || !r.RequireApproved {
		t.Errorf("report header = %+v", r)
	}
	find := func(locale, plugin string) L10nCoverage {
		for _, c := range r.Locales {
			if c.Locale == locale && c.Plugin == plugin {
				return c
			}
		}
		t.Fatalf("no entry for %s/%q in %+v", locale, plugin, r.Locales)
		return L10nCoverage{}
	}
	de := find("de-DE", "")
	if de.Keys != 8 || de.Translated != 6 || de.Approved != 2 || de.Missing != 2 || de.LeftOut != 4 {
		t.Errorf("de-DE = %+v", de)
	}
	if !reflect.DeepEqual(de.MissingKeys, []string{"loans.months", "loans.unused"}) {
		t.Errorf("missing keys = %v", de.MissingKeys)
	}
	if len(de.LeftOutKeys) != 4 {
		t.Errorf("left-out keys = %v", de.LeftOutKeys)
	}
	loans := find("de-DE", "loans")
	if loans.Keys != 7 || loans.Missing != 1 || loans.LeftOut != 4 || loans.MissingKeys != nil {
		t.Errorf("de-DE/loans = %+v", loans)
	}
	if en := find("en", ""); en.Missing != 1 || en.LeftOut != 0 || en.Translated != 7 {
		t.Errorf("en = %+v", en)
	}
	if am := find("am-ET", ""); am.Translated != 7 || am.LeftOut != 7 {
		t.Errorf("am-ET = %+v", am)
	}
}

// Verifies: I18N-007.
func TestLocalisationReportListsUnusedKeys(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	edit(t, m, messageKeys, func(doc map[string]any) {
		doc["keys"] = append(doc["keys"].([]any), map[string]any{"id": unusedKeyID, "key": "loans.unused"})
	})
	edit(t, m, messageEN, func(doc map[string]any) { at(t, doc, "messages")[unusedKeyID] = "Not shown" })
	res := compileFS(m)
	if r := res.Localisation; r == nil || !reflect.DeepEqual(r.UnusedKeys, []string{"loans.unused"}) {
		t.Errorf("report = %+v", r)
	}
}

// Verifies: I18N-002.
func TestPseudoLocaleDocumentsOfTheProjectWinOverGeneratedOnes(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	localisation(t, m, "pseudoLocales", true)
	m["translations/en-XA.json"] = &fstest.MapFile{Data: []byte(`{"schemaVersion": "1.0.0", "kind": "translations", "id": "01a0c450-6c00-7fff-8000-0000000003c1", "locale": "en-XA", "messages": {"` + keyLoansTitle + `": "Mine"}}`)}
	res := compileFS(m)
	if res.Diagnostics.HasErrors() {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	if got := section(t, res, "en-XA"); len(got) != 1 || got[keyLoansTitle] != "Mine" {
		t.Errorf("en-XA = %v, want the project's own document", got)
	}
	if got := section(t, res, "ar-XB"); len(got) != 7 {
		t.Errorf("ar-XB holds %d messages, want it generated", len(got))
	}
}
