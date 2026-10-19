// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// usedNames are the currency codes, territory codes and available-format
// keys of the gregorian calendar in the "en" data of a compiled project.
type usedNames struct {
	currencies, territories, formats []string
}

func namesOf(t *testing.T, m fstest.MapFS) usedNames {
	t.Helper()
	res := compileFS(m)
	if res.Diagnostics.HasErrors() {
		t.Fatal(list(res.Diagnostics))
	}
	var doc struct {
		Currencies map[string]json.RawMessage `json:"currencies"`
		Territory  map[string]json.RawMessage `json:"territories"`
		Calendars  struct {
			Gregorian struct {
				Formats map[string]json.RawMessage `json:"availableFormats"`
			} `json:"gregorian"`
		} `json:"calendars"`
	}
	if err := json.Unmarshal(localeSection(t, res, "en").DataBytes(), &doc); err != nil {
		t.Fatal(err)
	}
	keys := func(m map[string]json.RawMessage) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		slices.Sort(out)
		return out
	}
	return usedNames{keys(doc.Currencies), keys(doc.Territory), keys(doc.Calendars.Gregorian.Formats)}
}

// replaceResultNode turns the Text node of the fixture's result page into a
// widget of the given type with the given props.
func replaceResultNode(t *testing.T, typ, props string) fstest.MapFS {
	t.Helper()
	m := fixture(t)
	edit(t, m, appFile, func(doc map[string]any) { doc["minRuntimeVersion"] = "0.5.0" })
	edit(t, m, resultPage, func(doc map[string]any) {
		node := at(t, doc, "root/slots/body/slots/child")
		node["type"] = typ
		node["props"] = raw(t, props)
		node["semantics"] = raw(t, `{"label": "x"}`)
	})
	return m
}

// Verifies: I18N-002.
// A project that formats no money, lists no territories and uses no
// skeletons of its own carries no currency or territory names and only the
// date formats the runtime asks for (ADR-0066 Revision 2026-10-18).
func TestLocaleDataKeepsOnlyWhatTheAppUses(t *testing.T) {
	t.Parallel()
	got := namesOf(t, fixture(t))
	if len(got.currencies) != 0 || len(got.territories) != 0 {
		t.Errorf("currencies %v, territories %d", got.currencies, len(got.territories))
	}
	for _, key := range []string{"yMd", "yMMMd"} {
		if !slices.Contains(got.formats, key) {
			t.Errorf("the runtime's format %s is missing: %v", key, got.formats)
		}
	}
	if slices.Contains(got.formats, "Ehms") || len(got.formats) > 20 {
		t.Errorf("unused formats are kept: %v", got.formats)
	}
}

// Verifies: I18N-002.
// A literal currency in an expression, and in a currency picker's list, is
// kept and no other.
func TestLocaleDataKeepsLiteralCurrencies(t *testing.T) {
	t.Parallel()
	m := replaceResultNode(t, "Text", `{"data": {"$expr": "format.money(money(\"1.50\", \"CHF\"))"}}`)
	if got := namesOf(t, m).currencies; !slices.Equal(got, []string{"CHF"}) {
		t.Errorf("an expression: %v", got)
	}
	m = replaceResultNode(t, "CurrencyPicker", `{"currencies": ["JPY", "EUR"], "value": "JPY"}`)
	if got := namesOf(t, m).currencies; !slices.Equal(got, []string{"EUR", "JPY"}) {
		t.Errorf("a picker with a list: %v", got)
	}
}

// Verifies: I18N-002.
// A reference that is not a literal keeps every currency: a picker without a
// list, a computed currency prop and a money typed declaration.
func TestLocaleDataKeepsAllCurrenciesForDynamicReferences(t *testing.T) {
	t.Parallel()
	for name, m := range map[string]fstest.MapFS{
		"picker": replaceResultNode(t, "CurrencyPicker", `{}`),
		"amount": replaceResultNode(t, "AmountInput", `{"currency": {"$expr": "\"EUR\""}}`),
	} {
		if got := namesOf(t, m).currencies; len(got) < 100 {
			t.Errorf("%s: %d currencies", name, len(got))
		}
	}
	m := fixture(t)
	edit(t, m, appFile, func(doc map[string]any) {
		types, _ := doc["types"].([]any)
		doc["types"] = append(types, raw(t, `{"name": "Price", "fields": [{"name": "total", "type": "money"}]}`))
	})
	if got := namesOf(t, m).currencies; len(got) < 100 {
		t.Errorf("a money declaration: %d currencies", len(got))
	}
}

// Verifies: I18N-002.
// Territory names are kept whole when a widget lists them.
func TestLocaleDataKeepsTerritoriesForPickers(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{"CountryPicker", "PhoneInput"} {
		if got := namesOf(t, replaceResultNode(t, typ, `{}`)).territories; len(got) < 200 {
			t.Errorf("%s: %d territories", typ, len(got))
		}
	}
}

// Verifies: I18N-002.
// The skeletons and currencies messages format with are collected, sorted,
// from every translation.
func TestMessageUse(t *testing.T) {
	t.Parallel()
	doc := func(msgs map[string]string) schema.Loaded[schema.TranslationsDocument] {
		return schema.Loaded[schema.TranslationsDocument]{Doc: &schema.TranslationsDocument{Messages: msgs}}
	}
	u := &unit{project: &schema.Project{Translations: []schema.Loaded[schema.TranslationsDocument]{
		doc(map[string]string{
			"a": "{d, date, ::yMMMEdhm} {n, number, ::currency/GBP} {t, time, short}",
			"b": "{c, plural, one {{d, date, ::yMMMM}} other {{n, number, ::percent}}}",
		}),
		doc(map[string]string{"c": "broken {"}),
	}}}
	sk, cur := u.messageUse()
	if !slices.Equal(sk, []string{"yMMMEdhm", "yMMMM"}) || !slices.Equal(cur, []string{"GBP"}) {
		t.Errorf("skeletons %v, currencies %v", sk, cur)
	}
}

// Verifies: I18N-002.
// runtimeSkeletons lists exactly the skeletons packages/plux_flutter reads
// from the locale data for Flutter's own widgets.
func TestRuntimeSkeletonsMatchTheRuntime(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("../../../packages/plux_flutter/lib/src/l10n/flutter_localizations.dart")
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, m := range regexp.MustCompile(`strings\.date\([^;]*?,\s*'([A-Za-z]+)'\)`).FindAllStringSubmatch(string(src), -1) {
		set[m[1]] = true
	}
	if got := sortedKeys(set); !slices.Equal(got, runtimeSkeletons) {
		t.Errorf("flutter_localizations.dart uses %v; runtimeSkeletons is %v", got, runtimeSkeletons)
	}
}
