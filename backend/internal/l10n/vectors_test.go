// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const vectorDir = "../../../schema/testdata/l10n"

// vectorCase is the union of the fields of the four vector files. A case ICU
// and CLDR 48.2.3 disagree on carries a note and the engines' value in Ours.
type vectorCase struct {
	Locale     string `json:"locale"`
	Type       string `json:"type"`
	Op         string `json:"op"`
	N          string `json:"n"`
	Value      string `json:"value"`
	Currency   string `json:"currency"`
	Amount     string `json:"amount"`
	Display    string `json:"display"`
	Accounting bool   `json:"accounting"`
	Options    struct {
		MinFraction *int   `json:"minFraction"`
		MaxFraction *int   `json:"maxFraction"`
		Numbering   string `json:"numbering"`
	} `json:"options"`
	Calendar  string `json:"calendar"`
	Date      []int  `json:"date"`
	DateStyle string `json:"dateStyle"`
	TimeStyle string `json:"timeStyle"`
	Skeleton  string `json:"skeleton"`
	Numbering string `json:"numbering"`
	Expect    string `json:"expect"`
	Ours      string `json:"ours"`
	Note      string `json:"note"`
}

func (c vectorCase) want() string {
	if c.Ours != "" {
		return c.Ours
	}
	return c.Expect
}

func loadVectors(t *testing.T, name string) []vectorCase {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(vectorDir, name))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []vectorCase `json:"cases"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if len(f.Cases) == 0 {
		t.Fatalf("%s has no case", name)
	}
	return f.Cases
}

// runVectors loads each case's locale once and compares the engine's output.
func runVectors(t *testing.T, name string, run func(l *Locale, c vectorCase) (string, error)) {
	t.Helper()
	locales := map[string]*Locale{}
	failed := 0
	for i, c := range loadVectors(t, name) {
		l := locales[c.Locale]
		if l == nil {
			var err error
			if l, err = Load(c.Locale); err != nil {
				t.Fatalf("Load(%s): %v", c.Locale, err)
			}
			locales[c.Locale] = l
		}
		got, err := run(l, c)
		if err != nil || got != c.want() {
			failed++
			if failed <= 25 {
				t.Errorf("%s[%d] %+v: got %q (err %v), want %q", name, i, c, got, err, c.want())
			}
		}
	}
	if failed > 25 {
		t.Errorf("%s: %d cases failed in all", name, failed)
	}
}

func parseDecimal(t *testing.T, s string) Decimal {
	t.Helper()
	d, err := ParseDecimal(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestPluralVectors runs plurals.json (I18N-001, I18N-002, QA-003).
func TestPluralVectors(t *testing.T) {
	runVectors(t, "plurals.json", func(l *Locale, c vectorCase) (string, error) {
		n := parseDecimal(t, c.N)
		if c.Type == "ordinal" {
			return string(l.PluralOrdinal(n)), nil
		}
		return string(l.PluralCardinal(n)), nil
	})
}

func numberOptions(c vectorCase) NumberOptions {
	o := NumberOptions{NumberingSystem: c.Options.Numbering}
	if c.Options.MinFraction != nil {
		o.Fraction = &FractionDigits{Min: *c.Options.MinFraction, Max: *c.Options.MaxFraction}
	}
	return o
}

// TestNumberVectors runs numbers.json (I18N-001, QA-003).
func TestNumberVectors(t *testing.T) {
	runVectors(t, "numbers.json", func(l *Locale, c vectorCase) (string, error) {
		x := parseDecimal(t, c.Value)
		switch c.Op {
		case "decimal":
			return l.FormatNumber(x, numberOptions(c))
		case "percent":
			return l.FormatPercent(x, numberOptions(c))
		default:
			return l.FormatCompact(x, numberOptions(c))
		}
	})
}

// TestMoneyVectors runs money.json (I18N-011, QA-003).
func TestMoneyVectors(t *testing.T) {
	runVectors(t, "money.json", func(l *Locale, c vectorCase) (string, error) {
		o := MoneyOptions{Accounting: c.Accounting, NumberingSystem: c.Options.Numbering}
		switch c.Display {
		case "narrow":
			o.Display = DisplayNarrow
		case "code":
			o.Display = DisplayCode
		}
		if c.Options.MinFraction != nil {
			o.Fraction = &FractionDigits{Min: *c.Options.MinFraction, Max: *c.Options.MaxFraction}
		}
		return l.FormatMoney(parseDecimal(t, c.Amount), c.Currency, o)
	})
}

// TestDateVectors runs dates.json (I18N-001, QA-003).
func TestDateVectors(t *testing.T) {
	runVectors(t, "dates.json", func(l *Locale, c vectorCase) (string, error) {
		cal := c.Calendar
		if cal == "" {
			cal = "gregorian"
		}
		d := CalendarDate{Era: c.Date[0], Year: c.Date[1], Month: c.Date[2], Day: c.Date[3], Weekday: c.Date[4], Hour: c.Date[5], Minute: c.Date[6], Second: c.Date[7]}
		got, err := l.FormatDate(d, cal, DateOptions{DateStyle: c.DateStyle, TimeStyle: c.TimeStyle, Skeleton: c.Skeleton, NumberingSystem: c.Numbering})
		// JavaScriptCore writes U+0020 where CLDR has U+202F (dates.json
		// "normalise"); the engine keeps CLDR's character.
		return strings.ReplaceAll(got, "\u202f", " "), err
	})
}
