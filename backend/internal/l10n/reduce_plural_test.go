// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"encoding/json"
	"testing"
)

// Verifies: I18N-002.
// Reduce drops the plural forms of currency names, which neither engine reads,
// and keeps what they do read: each currency's name, symbol and narrow symbol.
func TestReduceDropsCurrencyPlurals(t *testing.T) {
	type names struct {
		Name   string `json:"name"`
		Symbol string `json:"symbol"`
		Narrow string `json:"narrow"`
	}
	type withPlural struct {
		names
		Plural map[string]string `json:"plural"`
	}
	type doc struct {
		Currencies map[string]withPlural `json:"currencies"`
	}
	src, err := dataFS.ReadFile("data/en.json")
	if err != nil {
		t.Fatal(err)
	}
	var full, got doc
	if err := json.Unmarshal(src, &full); err != nil {
		t.Fatal(err)
	}
	plurals := 0
	for _, n := range full.Currencies {
		if len(n.Plural) > 0 {
			plurals++
		}
	}
	if plurals == 0 {
		t.Fatal("the source data has no plural names: the test proves nothing")
	}
	data, _, err := Reduce("en", ReduceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Currencies) != len(full.Currencies) {
		t.Fatalf("currencies = %d, want %d", len(got.Currencies), len(full.Currencies))
	}
	for code, n := range got.Currencies {
		if len(n.Plural) != 0 {
			t.Errorf("%s keeps its plural names", code)
		}
		if n.names != full.Currencies[code].names {
			t.Errorf("%s = %+v, want %+v", code, n.names, full.Currencies[code].names)
		}
	}
	reduced, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	whole, err := Load("en")
	if err != nil {
		t.Fatal(err)
	}
	for code := range full.Currencies {
		if a, b := reduced.CurrencyName(code), whole.CurrencyName(code); a != b {
			t.Errorf("CurrencyName(%s) = %q, want %q", code, a, b)
		}
	}
}
