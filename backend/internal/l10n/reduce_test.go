// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

// Verifies: I18N-002.
// Reduce keeps gregorian and the named calendars, filters currency names, is
// deterministic, and its output parses as locale data.
func TestReduce(t *testing.T) {
	opts := ReduceOptions{Calendars: []string{"persian"}, Currencies: []string{"EUR"}}
	a, id, err := Reduce("de-DE", opts)
	if err != nil || id != "de" {
		t.Fatalf("Reduce = %q, %v", id, err)
	}
	b, _, _ := Reduce("de-DE", opts)
	if !bytes.Equal(a, b) {
		t.Error("Reduce is not deterministic")
	}
	var doc struct {
		Calendars  map[string]json.RawMessage `json:"calendars"`
		Currencies map[string]json.RawMessage `json:"currencies"`
	}
	if err := json.Unmarshal(a, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Calendars) != 2 || doc.Calendars["gregorian"] == nil || doc.Calendars["persian"] == nil {
		t.Errorf("calendars = %d", len(doc.Calendars))
	}
	if len(doc.Currencies) != 1 || doc.Currencies["EUR"] == nil {
		t.Errorf("currencies = %d", len(doc.Currencies))
	}
	if _, err := Parse(a); err != nil {
		t.Errorf("the reduced data does not parse: %v", err)
	}
	if _, _, err := Reduce("xx", ReduceOptions{}); !errors.Is(err, ErrNoLocale) {
		t.Errorf("xx: %v", err)
	}
	if c, err := Chain("de-AT"); err != nil || len(c) != 2 || c[1] != "de" {
		t.Errorf("Chain = %v, %v", c, err)
	}
}
