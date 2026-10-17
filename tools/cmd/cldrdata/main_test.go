// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSource serves documents from memory.
type fakeSource map[string]obj

func (f fakeSource) load(path string) (obj, error) {
	d, ok := f[path]
	if !ok {
		return nil, fmt.Errorf("archive has no %s", path)
	}
	return d, nil
}

func names(n int) obj {
	o := obj{}
	for i := 1; i <= n; i++ {
		o[fmt.Sprint(i)] = fmt.Sprintf("m%d", i)
	}
	return o
}

func dayNames() obj {
	return obj{"sun": "Su", "mon": "Mo", "tue": "Tu", "wed": "We", "thu": "Th", "fri": "Fr", "sat": "Sa"}
}

func calendarFixture(months int) obj {
	widthsOf := func(o obj) obj { return obj{"wide": o, "abbreviated": o, "narrow": o, "short": o} }
	periods := obj{"am": "AM", "pm": "PM"}
	ctx := func(o obj) obj { return obj{"format": widthsOf(o), "stand-alone": widthsOf(o)} }
	return obj{
		"months":     ctx(names(months)),
		"days":       ctx(dayNames()),
		"dayPeriods": ctx(periods),
		"eras": obj{
			"eraNames":  obj{"0": "Before", "1": "After", "1-alt-variant": "x"},
			"eraAbbr":   obj{"0": "B", "1": "A"},
			"eraNarrow": obj{"0": "b", "1": "a"},
		},
		"dateFormats":   obj{"full": "EEEE, d MMMM y", "long": "d MMMM y", "medium": "d MMM y", "short": "d/M/y"},
		"dateSkeletons": obj{"long": "yMMMMd", "short": "yMd"},
		"timeFormats":   obj{"full": "HH:mm:ss zzzz", "long": "HH:mm:ss z", "medium": "HH:mm:ss", "short": "HH:mm"},
		"dateTimeFormats": obj{
			"full": "{1} {0}", "long": "{1} {0}", "medium": "{1} {0}", "short": "{1} {0}",
			"availableFormats": obj{
				"Hm":       "HH:mm",
				"Hm-alt-x": "HH.mm",
				"Bh":       "h B",
				"hmv":      "h:mm a v",
				"yMd":      "d/M/y",
				"h":        "h 'Uhr' B",
				"Q":        "Q",
			},
		},
		"dateTimeFormats-atTime": obj{"standard": obj{"full": "{1} at {0}", "long": "{1} at {0}"}},
	}
}

func localeFixture(src fakeSource, l, order, defNS string) {
	sym := func() obj {
		return obj{
			"decimal": ",", "group": ".", "minusSign": "-", "percentSign": "%", "plusSign": "+",
			"infinity": "∞", "nan": "NaN", "list": ";",
		}
	}
	numbers := obj{
		"defaultNumberingSystem":    defNS,
		"minimumGroupingDigits":     "2",
		"symbols-numberSystem-latn": sym(),
		"decimalFormats-numberSystem-latn": obj{
			"standard": "#,##0.###",
			"short":    obj{"decimalFormat": obj{"1000-count-one": "0K", "1000-count-other": "0K", "ignored": "x"}},
		},
		"percentFormats-numberSystem-latn": obj{"standard": "#,##0%"},
		"currencyFormats-numberSystem-latn": obj{
			"standard":   "#,##0.00 ¤",
			"accounting": "#,##0.00 ¤;(#,##0.00 ¤)",
			"currencySpacing": obj{
				"beforeCurrency": obj{"insertBetween": " "},
				"afterCurrency":  obj{"insertBetween": " "},
			},
		},
	}
	if defNS != "latn" {
		numbers["symbols-numberSystem-"+defNS] = sym()
		numbers["decimalFormats-numberSystem-"+defNS] = numbers["decimalFormats-numberSystem-latn"]
		numbers["percentFormats-numberSystem-"+defNS] = numbers["percentFormats-numberSystem-latn"]
		numbers["currencyFormats-numberSystem-"+defNS] = numbers["currencyFormats-numberSystem-latn"]
	}
	src["cldr-numbers-full/main/"+l+"/numbers.json"] = obj{"main": obj{l: obj{"numbers": numbers}}}
	src["cldr-numbers-full/main/"+l+"/currencies.json"] = obj{"main": obj{l: obj{"numbers": obj{"currencies": obj{
		"EUR": obj{"displayName": "Euro", "displayName-count-one": "Euro", "displayName-count-other": "Euros", "symbol": "€"},
		"CHF": obj{"displayName": "Franken", "symbol": "CHF"},
		"USD": obj{"displayName": "Dollar", "symbol": "US$", "symbol-alt-narrow": "$"},
		"XXX": obj{},
	}}}}}
	src["cldr-misc-full/main/"+l+"/layout.json"] = obj{"main": obj{l: obj{"layout": obj{"orientation": obj{"characterOrder": order}}}}}
	src["cldr-localenames-full/main/"+l+"/territories.json"] = obj{"main": obj{l: obj{"localeDisplayNames": obj{"territories": obj{
		"DE": "Deutschland", "001": "Welt", "ZZ": "unbekannt", "DE-alt-short": "x",
	}}}}}
	greg := calendarFixture(12)
	src["cldr-dates-full/main/"+l+"/ca-gregorian.json"] = obj{"main": obj{l: obj{"dates": obj{"calendars": obj{"gregorian": greg}}}}}
	src["cldr-dates-full/main/"+l+"/ca-generic.json"] = obj{"main": obj{l: obj{"dates": obj{"calendars": obj{"generic": obj{
		"dateTimeFormats": obj{"availableFormats": obj{"yM": "y-M", "Hm": "H:mm"}},
	}}}}}}
	islamic := calendarFixture(12)
	src["cldr-cal-islamic-full/main/"+l+"/ca-islamic.json"] = obj{"main": obj{l: obj{"dates": obj{"calendars": obj{"islamic": islamic}}}}}
	src["cldr-cal-islamic-full/main/"+l+"/ca-islamic-umalqura.json"] = obj{"main": obj{l: obj{"dates": obj{"calendars": obj{"islamic-umalqura": obj{
		"months": obj{}, "days": obj{},
	}}}}}}
	src["cldr-cal-ethiopic-full/main/"+l+"/ca-ethiopic.json"] = obj{"main": obj{l: obj{"dates": obj{"calendars": obj{"ethiopic": calendarFixture(13)}}}}}
	src["cldr-cal-persian-full/main/"+l+"/ca-persian.json"] = obj{"main": obj{l: obj{"dates": obj{"calendars": obj{"persian": calendarFixture(12)}}}}}
}

func fixture() fakeSource {
	src := fakeSource{
		"cldr-core/coverageLevels.json": obj{"coverageLevels": obj{"de": "modern", "ar": "modern", "fr": "basic"}},
		"cldr-core/supplemental/plurals.json": obj{"supplemental": obj{"plurals-type-cardinal": obj{
			"de": obj{"pluralRule-count-one": "i = 1 and v = 0 @integer 1", "pluralRule-count-other": " @integer 0, 2~16"},
			"ar": obj{"pluralRule-count-zero": "n = 0 @integer 0", "pluralRule-count-other": " @integer 100"},
		}}},
		"cldr-core/supplemental/ordinals.json": obj{"supplemental": obj{"plurals-type-ordinal": obj{
			"de": obj{"pluralRule-count-other": " @integer 0~15"},
		}}},
		"cldr-core/supplemental/parentLocales.json": obj{"supplemental": obj{"parentLocales": obj{"parentLocale": obj{"de-AT": "de"}}}},
		"cldr-core/supplemental/currencyData.json": obj{"supplemental": obj{"currencyData": obj{"fractions": obj{
			"DEFAULT": obj{"_rounding": "0", "_digits": "2"},
			"JPY":     obj{"_rounding": "0", "_digits": "0"},
			"CHF":     obj{"_rounding": "0", "_digits": "2", "_cashRounding": "5"},
		}}}},
		"cldr-core/supplemental/likelySubtags.json": obj{"supplemental": obj{"likelySubtags": obj{
			"de": "de-Latn-DE", "de-AT": "de-Latn-AT", "ar": "ar-Arab-EG", "ar-XX": "ar-Latn-XX", "xx-YY": "xx-Latn-YY",
		}}},
		"cldr-core/supplemental/numberingSystems.json": obj{"supplemental": obj{"numberingSystems": obj{
			"latn": obj{"_digits": "0123456789", "_type": "numeric"},
			"arab": obj{"_digits": "٠١٢٣٤٥٦٧٨٩", "_type": "numeric"},
		}}},
	}
	localeFixture(src, "de", "left-to-right", "latn")
	localeFixture(src, "ar", "right-to-left", "arab")
	return src
}

func decode(t *testing.T, b []byte) obj {
	t.Helper()
	var o obj
	if err := json.Unmarshal(b, &o); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	return o
}

// TestDeriveLocale checks the shape of a derived file (I18N-001, I18N-002).
func TestDeriveLocale(t *testing.T) {
	files, err := derive(fixture(), "48.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("got %d files, want 2 modern locales and the index", len(files))
	}
	de := decode(t, files["de.json"])
	if atStr(de, "direction") != "ltr" || atStr(de, "numbering", "default") != "latn" {
		t.Errorf("direction or numbering wrong: %v", de["numbering"])
	}
	if got := atStr(de, "numbers", "latn", "compactShort", "1000", "other"); got != "0K" {
		t.Errorf("compact pattern = %q", got)
	}
	if got := atStr(de, "currencies", "EUR", "plural", "other"); got != "Euros" {
		t.Errorf("plural currency name = %q", got)
	}
	if _, ok := atObj(de, "currencies", "CHF")["symbol"]; ok {
		t.Error("a symbol equal to the code is stored")
	}
	if got := atStr(de, "currencies", "USD", "narrow"); got != "$" {
		t.Errorf("narrow symbol = %q", got)
	}
	if _, ok := atObj(de, "currencies")["XXX"]; ok {
		t.Error("an empty currency is stored")
	}
	if terr := atObj(de, "territories"); len(terr) != 1 || terr["DE"] != "Deutschland" {
		t.Errorf("territories = %v", terr)
	}
	greg := atObj(de, "calendars", "gregorian")
	avail := atObj(greg, "availableFormats")
	if len(avail) != 3 || avail["Hm"] == nil || avail["yMd"] != "d/M/y" {
		t.Errorf("availableFormats = %v, want Hm, yMd and the long style's yMMMMd", avail)
	}
	pavail := atObj(de, "calendars", "persian", "availableFormats")
	if pavail["yM"] != "y-M" || pavail["Hm"] != "HH:mm" {
		t.Errorf("persian availableFormats = %v, want yM from the generic calendar and its own Hm", pavail)
	}
	if got := atStr(greg, "dateFormats", "long"); got != "d MMMM y" || avail["yMMMMd"] != "d MMMM y" {
		t.Errorf("the long date style is not an available format of its skeleton: %v", avail)
	}
	if atStr(greg, "dateTimeAtTime", "full") != "{1} at {0}" {
		t.Error("atTime format missing")
	}
	if eras, _ := at(greg, "eras", "wide").([]any); len(eras) != 2 {
		t.Errorf("eras = %v, want the two non-alt names", eras)
	}
	if at(atObj(de, "calendars", "ethiopic"), "months", "format", "narrow") != nil {
		t.Error("a non-Gregorian calendar keeps narrow names")
	}
	if m, _ := at(atObj(de, "calendars", "ethiopic"), "months", "format", "wide").([]any); len(m) != 13 {
		t.Errorf("ethiopic has %d months", len(m))
	}
	if m, _ := at(atObj(de, "calendars", "islamic-umalqura"), "months", "format", "wide").([]any); len(m) != 12 {
		t.Error("umalqura did not fall back to the islamic names")
	}

	ar := decode(t, files["ar.json"])
	if atStr(ar, "direction") != "rtl" || atStr(ar, "numbering", "digits") != "٠١٢٣٤٥٦٧٨٩" {
		t.Errorf("ar direction or digits wrong")
	}
	if atObj(ar, "numbers", "arab") == nil || atObj(ar, "numbers", "latn") == nil {
		t.Error("ar lacks the latn or the default numbering system")
	}

	idx := decode(t, files["index.json"])
	if atStr(idx, "cldrVersion") != "48.2.3" || atStr(idx, "parentLocales", "de-AT") != "de" {
		t.Errorf("index = %v", idx)
	}
	if atStr(idx, "plurals", "de", "cardinal", "one") != "i = 1 and v = 0" {
		t.Errorf("plural rule keeps its samples: %v", at(idx, "plurals", "de"))
	}
	if atStr(idx, "plurals", "ar", "cardinal", "zero") != "n = 0" {
		t.Error("ar rule missing")
	}
	if got := at(idx, "currencies", "CHF", "cashRounding"); got != float64(5) {
		t.Errorf("cash rounding = %v", got)
	}
	if ls := atObj(idx, "likelyScripts"); len(ls) != 1 || ls["ar-XX"] != "Latn" {
		t.Errorf("likelyScripts = %v, want only ar-XX", ls)
	}
	if cd := atObj(de, "currencyDigits"); len(cd) != 1 || cd["JPY"] != float64(0) {
		t.Errorf("currencyDigits = %v, want only JPY", cd)
	}
}

// TestDeriveDeterministic checks the same input gives the same bytes (CMP-002).
func TestDeriveDeterministic(t *testing.T) {
	a, err := derive(fixture(), "48.2.3")
	if err != nil {
		t.Fatal(err)
	}
	b, err := derive(fixture(), "48.2.3")
	if err != nil {
		t.Fatal(err)
	}
	for name, x := range a {
		if !bytes.Equal(x, b[name]) {
			t.Errorf("%s differs between runs", name)
		}
	}
}

func TestDeriveRejectsIncompleteData(t *testing.T) {
	for name, mutate := range map[string]func(fakeSource){
		"no layout":      func(s fakeSource) { delete(s, "cldr-misc-full/main/de/layout.json") },
		"no territories": func(s fakeSource) { delete(s, "cldr-localenames-full/main/de/territories.json") },
		"no modern":      func(s fakeSource) { s["cldr-core/coverageLevels.json"] = obj{"coverageLevels": obj{"de": "basic"}} },
		"no default":     func(s fakeSource) { delete(s, "cldr-core/supplemental/currencyData.json") },
		"short months": func(s fakeSource) {
			s["cldr-dates-full/main/de/ca-gregorian.json"] = obj{"main": obj{"de": obj{"dates": obj{"calendars": obj{"gregorian": func() obj {
				c := calendarFixture(12)
				c["months"] = obj{"format": obj{"wide": names(11), "abbreviated": names(11), "narrow": names(11)}}
				return c
			}()}}}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			src := fixture()
			mutate(src)
			if _, err := derive(src, "48.2.3"); err == nil {
				t.Fatal("derive accepted incomplete data")
			}
		})
	}
}

func TestCanonical(t *testing.T) {
	got, err := canonical(obj{"b": []any{"x", "y z"}, "a": obj{}, "c": obj{"k": "q\"\\ ‏😀"}, "n": 3})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": {},\n  \"b\": [\"x\", \"y\\u00a0z\"],\n  \"c\": {\n    \"k\": \"q\\\"\\\\\\u202f\\u200f😀\"\n  },\n  \"n\": 3\n}\n"
	if string(got) != want {
		t.Errorf("canonical =\n%s\nwant\n%s", got, want)
	}
	if _, err := canonical(obj{"f": 1.5}); err == nil {
		t.Error("canonical accepted a float")
	}
}

func TestSurrogates(t *testing.T) {
	if a, b := surrogates(0x1F600); a != 0xd83d || b != 0xde00 {
		t.Errorf("surrogates = %x %x", a, b)
	}
}

func TestOpenArchiveAndRun(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "c.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for path, doc := range fixture() {
		w, err := zw.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(w).Encode(doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(zp, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(out, "stale.json")
	if err := os.WriteFile(stale, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-zip", zp, "-version", "48.2.3", "-out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d: %s", code, stderr.String())
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("a locale file CLDR no longer has was kept")
	}
	if _, err := os.Stat(filepath.Join(out, "de.json")); err != nil {
		t.Error(err)
	}
	if !strings.Contains(stdout.String(), "3 files") {
		t.Errorf("stdout = %q", stdout.String())
	}
	if code := run([]string{"-zip", filepath.Join(dir, "missing.zip"), "-version", "1", "-out", out}, &stdout, &stderr); code != 2 {
		t.Errorf("run with a missing archive = %d, want 2", code)
	}
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Errorf("run without flags = %d, want 2", code)
	}
}
