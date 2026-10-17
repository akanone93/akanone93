// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func mustLoad(t *testing.T, tag string) *Locale {
	t.Helper()
	l, err := Load(tag)
	if err != nil {
		t.Fatalf("Load(%s): %v", tag, err)
	}
	return l
}

// TestMoneyExamples checks the examples of I18N-011.
func TestMoneyExamples(t *testing.T) {
	for _, c := range []struct{ locale, currency, amount, want string }{
		{"de-DE", "EUR", "1234.56", "1.234,56 €"},
		{"en-US", "USD", "1234.56", "$1,234.56"},
		{"en", "JPY", "1234.5", "¥1,234"},
		{"en", "USD", "-5", "-$5.00"},
	} {
		got, err := mustLoad(t, c.locale).FormatMoney(parseDecimal(t, c.amount), c.currency, MoneyOptions{})
		if err != nil || got != c.want {
			t.Errorf("%s %s %s = %q, %v; want %q", c.locale, c.currency, c.amount, got, err, c.want)
		}
	}
}

func TestMoneyOptionsAndErrors(t *testing.T) {
	en := mustLoad(t, "en")
	x := parseDecimal(t, "1234.5")
	if got, _ := en.FormatMoney(x, "CAD", MoneyOptions{Display: DisplayNarrow}); got != "$1,234.50" {
		t.Errorf("narrow = %q", got)
	}
	if got, _ := en.FormatMoney(x, "CAD", MoneyOptions{}); got != "CA$1,234.50" {
		t.Errorf("symbol = %q", got)
	}
	if got, _ := en.FormatMoney(x, "CHF", MoneyOptions{Display: DisplayCode}); got != "CHF 1,234.50" {
		t.Errorf("code = %q", got)
	}
	if got, _ := en.FormatMoney(parseDecimal(t, "-3"), "USD", MoneyOptions{Accounting: true}); got != "($3.00)" {
		t.Errorf("accounting = %q", got)
	}
	if got, _ := en.FormatMoney(x, "USD", MoneyOptions{Fraction: &FractionDigits{Min: 0, Max: 0}}); got != "$1,234" {
		t.Errorf("fraction override = %q", got)
	}
	if en.CurrencyDigits("JPY") != 0 || en.CurrencyDigits("KWD") != 3 || en.CurrencyDigits("USD") != 2 {
		t.Error("currency digits wrong")
	}
	if en.CurrencyName("EUR") != "Euro" || en.CurrencyName("ZZZ") != "ZZZ" {
		t.Error("currency names wrong")
	}
	for _, bad := range []string{"", "us", "usd", "USDX", "U1D"} {
		if _, err := en.FormatMoney(x, bad, MoneyOptions{}); !errors.Is(err, ErrInvalidCurrency) {
			t.Errorf("FormatMoney(%q) err = %v, want ErrInvalidCurrency", bad, err)
		}
	}
	if _, err := en.FormatMoney(x, "USD", MoneyOptions{Display: 9}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("bad display err = %v", err)
	}
	if _, err := en.FormatMoney(x, "USD", MoneyOptions{Fraction: &FractionDigits{Min: 3, Max: 2}}); !errors.Is(err, ErrInvalidOption) {
		t.Errorf("bad fraction err = %v", err)
	}
	if _, err := en.FormatMoney(x, "USD", MoneyOptions{NumberingSystem: "arab"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("unknown numbering err = %v", err)
	}
}

func TestNumberFormatting(t *testing.T) {
	hi := mustLoad(t, "hi")
	got, err := hi.FormatNumber(parseDecimal(t, "1234567.891"), NumberOptions{})
	if err != nil || got != "12,34,567.891" {
		t.Errorf("secondary grouping = %q, %v", got, err)
	}
	es := mustLoad(t, "es")
	if got, _ := es.FormatNumber(parseDecimal(t, "1234"), NumberOptions{}); got != "1234" {
		t.Errorf("minimum grouping digits = %q", got)
	}
	if got, _ := es.FormatNumber(parseDecimal(t, "12345"), NumberOptions{}); got != "12.345" {
		t.Errorf("minimum grouping digits = %q", got)
	}
	en := mustLoad(t, "en")
	for _, c := range []struct {
		in   string
		opts NumberOptions
		want string
	}{
		{"2.5", NumberOptions{Fraction: &FractionDigits{0, 0}}, "2"},
		{"3.5", NumberOptions{Fraction: &FractionDigits{0, 0}}, "4"},
		{"0.125", NumberOptions{Fraction: &FractionDigits{0, 2}}, "0.12"},
		{"1", NumberOptions{Fraction: &FractionDigits{2, 4}}, "1.00"},
		{"1234.5", NumberOptions{NoGrouping: true}, "1234.5"},
		{"-0.001", NumberOptions{Fraction: &FractionDigits{0, 1}}, "-0"},
		{"99999999999999999999.5", NumberOptions{Fraction: &FractionDigits{0, 0}}, "100,000,000,000,000,000,000"},
	} {
		if got, err := en.FormatNumber(parseDecimal(t, c.in), c.opts); err != nil || got != c.want {
			t.Errorf("FormatNumber(%s) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	if got, _ := mustLoad(t, "fa").FormatNumber(parseDecimal(t, "1234.5"), NumberOptions{NumberingSystem: "default"}); got != "۱٬۲۳۴٫۵" {
		t.Errorf("native digits = %q", got)
	}
	if _, err := en.FormatNumber(parseDecimal(t, "1"), NumberOptions{Fraction: &FractionDigits{0, 25}}); !errors.Is(err, ErrInvalidOption) {
		t.Errorf("too many digits err = %v", err)
	}
	if _, err := en.FormatCompact(parseDecimal(t, "1"), NumberOptions{Fraction: &FractionDigits{0, 1}}); !errors.Is(err, ErrInvalidOption) {
		t.Errorf("compact with fraction err = %v", err)
	}
	if got, _ := en.FormatPercent(parseDecimal(t, "0.256"), NumberOptions{Fraction: &FractionDigits{1, 1}}); got != "25.6%" {
		t.Errorf("percent = %q", got)
	}
}

func TestCompact(t *testing.T) {
	for _, c := range []struct{ tag, in, want string }{
		{"en", "1234", "1.2K"},
		{"en", "999999", "1M"},
		{"en", "12500", "12K"},
		{"en", "-1500000", "-1.5M"},
		{"en", "0.0123", "0.012"},
		{"de", "1500000", "1,5 Mio."},
		{"de", "2000000", "2 Mio."},
		{"ja", "12345678", "1235万"},
	} {
		got, err := mustLoad(t, c.tag).FormatCompact(parseDecimal(t, c.in), NumberOptions{})
		if err != nil || got != c.want {
			t.Errorf("FormatCompact(%s, %s) = %q, %v; want %q", c.tag, c.in, got, err, c.want)
		}
	}
}

func TestDecimal(t *testing.T) {
	for in, want := range map[string]string{
		"0": "0", "-0": "-0", "+1.50": "1.50", "007": "7", "1.5e3": "1500", "1.5E-3": "0.0015", "12e0": "12", ".5": "0.5", "5.": "5",
	} {
		d, err := ParseDecimal(in)
		if err != nil || d.String() != want {
			t.Errorf("ParseDecimal(%q) = %q, %v; want %q", in, d, err, want)
		}
	}
	for _, in := range []string{"", "-", "abc", "1.2.3", "1e", "1e9999", "1e+", "--1", "1 2", "٣"} {
		if _, err := ParseDecimal(in); !errors.Is(err, ErrInvalidDecimal) {
			t.Errorf("ParseDecimal(%q) err = %v", in, err)
		}
	}
	if d := DecimalFromInt(-42); d.String() != "-42" || !d.IsNegative() {
		t.Errorf("DecimalFromInt = %s", d)
	}
	if DecimalFromInt(0).String() != "0" || !DecimalFromInt(0).IsZero() {
		t.Error("zero wrong")
	}
	var zero Decimal
	if zero.String() != "0" || !zero.IsZero() {
		t.Error("the zero value is not 0")
	}
	if d, _ := ParseDecimal("9.995"); d.roundFrac(2).String() != "10.00" {
		t.Errorf("carry = %s", d.roundFrac(2))
	}
	if d, _ := ParseDecimal("1250"); d.roundFrac(-2).String() != "1200" {
		t.Errorf("round to hundreds = %s", d.roundFrac(-2))
	}
}

func TestPluralRuleSyntax(t *testing.T) {
	good := []string{
		"n = 1", "n != 0..2,5", "i = 1 and v = 0", "n % 10 = 2..4 and n % 100 != 12..14", "e = 0 and i != 0 and i % 1000000 = 0 and v = 0 or e != 0..5",
		"t = 0 and i % 10 = 1 or f % 10 = 1", "w = 0", "c = 0",
	}
	for _, r := range good {
		if _, err := parseRule(r); err != nil {
			t.Errorf("parseRule(%q) = %v", r, err)
		}
	}
	for _, r := range []string{"", "x = 1", "n", "n = ", "n = a", "n % 0 = 1", "n = 5..2", "n = 1 and", "n = 1 xor n = 2", "n % = 1", "n ~ 1"} {
		if _, err := parseRule(r); err == nil {
			t.Errorf("parseRule(%q) succeeded", r)
		}
	}
	if _, err := compileRules(map[string]string{"several": "n = 1"}); !errors.Is(err, ErrInvalidData) {
		t.Errorf("unknown category err = %v", err)
	}
	if _, err := compileRules(map[string]string{"one": "n ="}); !errors.Is(err, ErrInvalidData) {
		t.Errorf("bad rule err = %v", err)
	}
}

func TestPlurals(t *testing.T) {
	for _, c := range []struct {
		tag, n string
		want   Category
	}{
		{"en", "1", One},
		{"en", "1.0", Other},
		{"en", "2", Other},
		{"ar", "0", Zero},
		{"ar", "2", Two},
		{"ar", "3", Few},
		{"ar", "11", Many},
		{"ar", "100", Other},
		{"ru", "21", One},
		{"ru", "22", Few},
		{"ru", "25", Many},
		{"ru", "1.5", Other},
		{"fr", "1000000", Many},
		{"fr", "0", One},
		{"ja", "1", Other},
	} {
		if got := mustLoad(t, c.tag).PluralCardinal(parseDecimal(t, c.n)); got != c.want {
			t.Errorf("cardinal %s %s = %s, want %s", c.tag, c.n, got, c.want)
		}
	}
	if got := mustLoad(t, "en").PluralOrdinal(parseDecimal(t, "-22")); got != Two {
		t.Errorf("ordinal en -22 = %s", got)
	}
	if got := mustLoad(t, "ar").Categories(); !slices.Equal(got, []Category{Zero, One, Two, Few, Many, Other}) {
		t.Errorf("ar categories = %v", got)
	}
	if got := mustLoad(t, "ba").PluralCardinal(DecimalFromInt(1)); got != Other {
		t.Errorf("ba (no CLDR rules) = %s", got)
	}
}

func TestResolve(t *testing.T) {
	for _, c := range []struct {
		tag       string
		supported []string
		fallback  string
		want      string
	}{
		{"de-AT", []string{"en", "de"}, "en", "de"},
		{"de_at", []string{"en", "de-AT"}, "en", "de-AT"},
		{"DE-de", []string{"de-DE"}, "en", "de-DE"},
		{"de", []string{"en-US", "de-DE"}, "en-US", "de-DE"},
		{"fr-CA", []string{"en", "de"}, "en", "en"},
		{"zh-TW", []string{"zh", "zh-Hant"}, "zh", "zh-Hant"},
		{"zh-HK", []string{"zh", "zh-Hant-HK"}, "zh", "zh-Hant-HK"},
		{"zh-Hant-TW", []string{"zh", "zh-Hant"}, "zh", "zh-Hant"},
		{"iw", []string{"he"}, "en", "he"},
		{"en-GB-u-ca-gregory", []string{"en"}, "de", "en"},
		{"sr-Latn-RS", []string{"sr-Latn", "sr"}, "en", "sr-Latn"},
		{"xx", []string{"en"}, "de_de", "de-DE"},
	} {
		got, err := Resolve(c.tag, c.supported, c.fallback)
		if err != nil || got != c.want {
			t.Errorf("Resolve(%q, %v, %q) = %q, %v; want %q", c.tag, c.supported, c.fallback, got, err, c.want)
		}
	}
	if _, err := Resolve("xx", []string{"en"}, ""); !errors.Is(err, ErrNoLocale) {
		t.Errorf("no match without fallback err = %v", err)
	}
	for _, bad := range []string{"", " en", "e", "english", "en-", "und", "en-US-$", "123"} {
		if _, err := Resolve(bad, []string{"en"}, "en"); !errors.Is(err, ErrInvalidTag) {
			t.Errorf("Resolve(%q) err = %v, want ErrInvalidTag", bad, err)
		}
	}
	if _, err := Resolve("en", []string{"!!"}, "en"); !errors.Is(err, ErrInvalidTag) {
		t.Errorf("bad supported entry err = %v", err)
	}
}

func TestLoad(t *testing.T) {
	l, err := Load("de-DE")
	if err != nil || l.ID() != "de" || l.IsRTL() || l.DefaultNumberingSystem() != "latn" {
		t.Fatalf("Load(de-DE) = %v, %v", l, err)
	}
	if !mustLoad(t, "ar").IsRTL() || !mustLoad(t, "fa").IsRTL() || !mustLoad(t, "he").IsRTL() {
		t.Error("ar, fa and he are right to left")
	}
	if n, ok := mustLoad(t, "am-ET").TerritoryName("ET"); !ok || n != "ኢትዮጵያ" {
		t.Errorf("territory ET in am = %q", n)
	}
	if _, ok := l.TerritoryName("ZZ"); ok {
		t.Error("ZZ has a name")
	}
	if _, err := Load("xx"); !errors.Is(err, ErrNoLocale) {
		t.Errorf("Load(xx) err = %v", err)
	}
	locales, err := Locales()
	if err != nil || len(locales) != 104 || !slices.IsSorted(locales) {
		t.Errorf("Locales() = %d locales, %v", len(locales), err)
	}
	for _, id := range locales {
		if _, err := Load(id); err != nil {
			t.Errorf("Load(%s): %v", id, err)
		}
	}
}

func TestParseRejectsBadData(t *testing.T) {
	for name, data := range map[string]string{
		"not json":   "{",
		"empty":      "{}",
		"no digits":  `{"locale":"x","numbering":{"default":"arab","digits":"12"},"numbers":{"latn":{}},"calendars":{"gregorian":{}}}`,
		"no symbols": `{"locale":"x","numbering":{"default":"latn"},"numbers":{"latn":{"symbols":{}}},"calendars":{"gregorian":{}}}`,
	} {
		if _, err := Parse([]byte(data)); !errors.Is(err, ErrInvalidData) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestPatternErrors(t *testing.T) {
	for _, p := range []string{"", "abc", "'#", "#,##0.0,0", "#,,0", "0 'x"} {
		if _, err := compilePattern(p); err == nil {
			t.Errorf("compilePattern(%q) succeeded", p)
		}
	}
	p, err := compilePattern("¤#,##,##0.00;(¤#,##,##0.00)")
	if err != nil || p.primary != 3 || p.secondary != 2 || p.minFrac != 2 || !p.hasNeg || p.scale != 0 {
		t.Errorf("pattern = %+v, %v", p, err)
	}
	if p, _ := compilePattern("#,##0 %"); p.scale != 2 {
		t.Error("percent pattern has no scale")
	}
	if p, _ := compilePattern("0 ‰"); p.scale != 3 {
		t.Error("permille pattern has no scale")
	}
}

func TestGregorianDate(t *testing.T) {
	for _, c := range []struct {
		y, m, d, wd, era, year int
	}{
		{1970, 1, 1, 4, 1, 1970},
		{2025, 10, 16, 4, 1, 2025},
		{2000, 2, 29, 2, 1, 2000},
		{1, 1, 1, 1, 1, 1},
		{0, 12, 31, 0, 0, 1},
		{-399, 3, 1, 4, 0, 400},
		{2024, 2, 29, 4, 1, 2024},
	} {
		got := GregorianDate(c.y, c.m, c.d, 1, 2, 3)
		if got.Weekday != c.wd || got.Era != c.era || got.Year != c.year || got.Hour != 1 || got.Second != 3 {
			t.Errorf("GregorianDate(%d-%d-%d) = %+v, want weekday %d era %d year %d", c.y, c.m, c.d, got, c.wd, c.era, c.year)
		}
	}
}

func TestFormatDate(t *testing.T) {
	en := mustLoad(t, "en")
	d := GregorianDate(2025, 10, 16, 15, 4, 5)
	for _, c := range []struct {
		opts DateOptions
		want string
	}{
		{DateOptions{DateStyle: "full"}, "Thursday, October 16, 2025"},
		{DateOptions{TimeStyle: "medium"}, "3:04:05 PM"},
		{DateOptions{TimeStyle: "full", ZoneName: "UTC"}, "3:04:05 PM UTC"},
		{DateOptions{Skeleton: "yMMMMEEEEd"}, "Thursday, October 16, 2025"},
		{DateOptions{Skeleton: "GyMMMd"}, "Oct 16, 2025 AD"},
		{DateOptions{Skeleton: "yyMd"}, "10/16/25"},
		{DateOptions{Skeleton: "LLLL"}, "October"},
		{DateOptions{Skeleton: "Hm", NumberingSystem: "latn"}, "15:04"},
	} {
		got, err := en.FormatDate(d, "gregorian", c.opts)
		if err != nil || got != c.want {
			t.Errorf("FormatDate(%+v) = %q, %v; want %q", c.opts, got, err, c.want)
		}
	}
	for name, c := range map[string]struct {
		cal  string
		d    CalendarDate
		opts DateOptions
	}{
		"unknown calendar":  {"julian", d, DateOptions{DateStyle: "full"}},
		"no format":         {"gregorian", d, DateOptions{}},
		"both":              {"gregorian", d, DateOptions{DateStyle: "full", Skeleton: "yMd"}},
		"bad date style":    {"gregorian", d, DateOptions{DateStyle: "tiny"}},
		"bad time style":    {"gregorian", d, DateOptions{TimeStyle: "tiny"}},
		"bad skeleton":      {"gregorian", d, DateOptions{Skeleton: "yQ"}},
		"empty skeleton":    {"gregorian", d, DateOptions{Skeleton: "a"}},
		"repeated field":    {"gregorian", d, DateOptions{Skeleton: "yMy"}},
		"no format fields":  {"gregorian", d, DateOptions{Skeleton: "ymsd"}},
		"bad day":           {"gregorian", CalendarDate{Era: 1, Year: 1, Month: 1, Day: 0}, DateOptions{DateStyle: "short"}},
		"month out of data": {"gregorian", CalendarDate{Era: 1, Year: 2000, Month: 13, Day: 1}, DateOptions{DateStyle: "long"}},
		"era out of data":   {"gregorian", CalendarDate{Era: 7, Year: 2000, Month: 1, Day: 1}, DateOptions{Skeleton: "GyMMMd"}},
		"numbering":         {"gregorian", d, DateOptions{DateStyle: "short", NumberingSystem: "arab"}},
	} {
		if _, err := en.FormatDate(c.d, c.cal, c.opts); err == nil {
			t.Errorf("%s: FormatDate succeeded", name)
		}
	}
	if !strings.Contains(strings.ToLower(ErrUnsupported.Error()), "unsupported") {
		t.Error("error text")
	}
}

func TestTokenize(t *testing.T) {
	toks, err := tokenize("d 'de' MMMM ''y''")
	if err != nil {
		t.Fatal(err)
	}
	again, err := tokenize(untokenize(toks))
	if err != nil || !slices.Equal(toks, again) {
		t.Errorf("untokenize does not round-trip: %v, %v -> %v", toks, err, again)
	}
	if _, err := tokenize("d 'x"); err == nil {
		t.Error("an unterminated quote was accepted")
	}
}
