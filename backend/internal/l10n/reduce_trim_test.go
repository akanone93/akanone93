// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"encoding/json"
	"testing"
)

// Verifies: I18N-002.
// Reduce keeps only the territories and the skeleton formats it is asked
// for, and a listed skeleton formats exactly as with the full data.
func TestReduceTrimsTerritoriesAndSkeletons(t *testing.T) {
	skeletons := []string{"y", "yMMMd", "yMMMEdhm", "MMMEd", "Hm", "EEEEE", "LLLL", "yMMMMEEEEd"}
	d := GregorianDate(2026, 10, 18, 14, 5, 9)
	for _, tag := range []string{"en", "de", "ar", "ja", "am"} {
		full, err := Load(tag)
		if err != nil {
			t.Fatal(err)
		}
		data, _, err := Reduce(tag, ReduceOptions{Territories: []string{"CH"}, Skeletons: skeletons})
		if err != nil {
			t.Fatal(err)
		}
		small, err := Parse(data)
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		for _, sk := range skeletons {
			want, werr := full.FormatDate(d, "gregorian", DateOptions{Skeleton: sk})
			got, gerr := small.FormatDate(d, "gregorian", DateOptions{Skeleton: sk})
			if want != got || (werr == nil) != (gerr == nil) {
				t.Errorf("%s %s: %q (%v), want %q (%v)", tag, sk, got, gerr, want, werr)
			}
		}
		for _, style := range []string{"full", "long", "medium", "short"} {
			want, _ := full.FormatDate(d, "gregorian", DateOptions{DateStyle: style, TimeStyle: style})
			got, _ := small.FormatDate(d, "gregorian", DateOptions{DateStyle: style, TimeStyle: style})
			if want != got {
				t.Errorf("%s style %s: %q, want %q", tag, style, got, want)
			}
		}
		if _, ok := small.TerritoryName("CH"); !ok {
			t.Errorf("%s: CH is trimmed", tag)
		}
		if _, ok := small.TerritoryName("FR"); ok {
			t.Errorf("%s: FR is kept", tag)
		}
	}
	type reduced struct {
		Territories map[string]string `json:"territories"`
		Calendars   map[string]struct {
			Formats map[string]string `json:"availableFormats"`
		} `json:"calendars"`
	}
	for _, o := range []ReduceOptions{{Territories: []string{}, Skeletons: []string{}}, {}} {
		data, _, err := Reduce("en", o)
		if err != nil {
			t.Fatal(err)
		}
		var doc reduced
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		trimmed := o.Territories != nil
		if (len(doc.Territories) == 0) != trimmed || (len(doc.Calendars["gregorian"].Formats) == 0) != trimmed {
			t.Errorf("%+v: %d territories, %d formats", o, len(doc.Territories), len(doc.Calendars["gregorian"].Formats))
		}
		if _, err := Parse(data); err != nil {
			t.Errorf("%+v: %v", o, err)
		}
	}
	data, _, _ := Reduce("en", ReduceOptions{Skeletons: []string{"bogus!"}})
	var doc reduced
	if err := json.Unmarshal(data, &doc); err != nil || len(doc.Calendars["gregorian"].Formats) < 20 {
		t.Errorf("an invalid skeleton keeps every format: %d, %v", len(doc.Calendars["gregorian"].Formats), err)
	}
}
