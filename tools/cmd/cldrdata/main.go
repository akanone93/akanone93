// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Command cldrdata derives schema/l10n/cldr, the locale data of Plux's
// formatting engines, from the cldr-json release pinned in mk/l10n.mk
// (make cldr-data, ADR-0066). It writes one self-contained JSON file for each
// locale CLDR rates at "modern" coverage, and an index.json with the locale
// list, the parent-locale map, the plural rules and the ISO 4217 digits.
//
// cldr-json already resolves inheritance (aliases and the parent chain up to
// root) when it flattens the CLDR XML, so each source file is complete on its
// own; this command checks that every field it needs is present rather than
// resolving it a second time. The output is deterministic: the same archive
// gives the same bytes.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run returns 0 on success and 2 on usage, input or output errors.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cldrdata", flag.ContinueOnError)
	fs.SetOutput(stderr)
	zipPath := fs.String("zip", "", "the cldr-<version>-json-full.zip archive")
	version := fs.String("version", "", "the CLDR release, e.g. 48.2.3")
	out := fs.String("out", "", "the directory to write the locale files and index.json to")
	if err := fs.Parse(args); err != nil || *zipPath == "" || *version == "" || *out == "" {
		_, _ = fmt.Fprintln(stderr, "usage: cldrdata -zip <cldr-json-full.zip> -version <release> -out <dir>")
		return 2
	}
	src, err := openArchive(*zipPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cldrdata:", err)
		return 2
	}
	defer func() { _ = src.close() }()
	files, err := derive(src, *version)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cldrdata:", err)
		return 2
	}
	if err := writeAll(*out, files); err != nil {
		_, _ = fmt.Fprintln(stderr, "cldrdata:", err)
		return 2
	}
	var total, largest int
	var largestName string
	for name, b := range files {
		total += len(b)
		if len(b) > largest {
			largest, largestName = len(b), name
		}
	}
	_, _ = fmt.Fprintf(stdout, "cldrdata: %d files, %d bytes in total, largest %s (%d bytes)\n", len(files), total, largestName, largest)
	return 0
}

// maxFileBytes is the size every locale file must stay under: the commit
// hook refuses files of 500 KB and more.
const maxFileBytes = 450 * 1000

// obj is a decoded JSON object.
type obj = map[string]any

// archive reads the JSON documents of a cldr-json zip by path.
type archive struct {
	r     *zip.ReadCloser
	files map[string]*zip.File
}

func openArchive(path string) (*archive, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	a := &archive{r: r, files: make(map[string]*zip.File, len(r.File))}
	for _, f := range r.File {
		a.files[f.Name] = f
	}
	return a, nil
}

func (a *archive) close() error { return a.r.Close() }

// load decodes the JSON document at path.
func (a *archive) load(path string) (obj, error) {
	f, ok := a.files[path]
	if !ok {
		return nil, fmt.Errorf("archive has no %s", path)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = rc.Close() }()
	var doc obj
	if err := json.NewDecoder(rc).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return doc, nil
}

// source is what derive needs from a cldr-json release.
type source interface {
	load(path string) (obj, error)
}

// at walks keys down from v and returns nil when any step is missing.
func at(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(obj)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func atObj(v any, keys ...string) obj {
	m, _ := at(v, keys...).(obj)
	return m
}

func atStr(v any, keys ...string) string {
	s, _ := at(v, keys...).(string)
	return s
}

// derive builds every output file, keyed by file name.
func derive(src source, version string) (map[string][]byte, error) {
	cov, err := src.load("cldr-core/coverageLevels.json")
	if err != nil {
		return nil, err
	}
	var locales []string
	for l, lvl := range atObj(cov, "coverageLevels") {
		if lvl == "modern" {
			locales = append(locales, l)
		}
	}
	if len(locales) == 0 {
		return nil, errors.New("coverageLevels.json lists no modern locale")
	}
	sort.Strings(locales)

	cardinal, ordinal, err := pluralTables(src)
	if err != nil {
		return nil, err
	}
	plurals := make(obj, len(locales))
	for _, l := range locales {
		plurals[l] = localePlurals(l, cardinal, ordinal)
	}

	digits, err := currencyDigits(src)
	if err != nil {
		return nil, err
	}
	nonDefault := obj{}
	defDigits := atInt(digits, "DEFAULT", "digits")
	for code, e := range digits {
		if d := atInt(e, "digits"); code != "DEFAULT" && d != defDigits {
			nonDefault[code] = d
		}
	}

	files := make(map[string][]byte, len(locales)+1)
	for _, l := range locales {
		doc, err := buildLocale(src, l)
		if err != nil {
			return nil, fmt.Errorf("locale %s: %w", l, err)
		}
		doc["plurals"] = plurals[l]
		doc["currencyDigits"] = nonDefault
		b, err := canonical(doc)
		if err != nil {
			return nil, err
		}
		if len(b) >= maxFileBytes {
			return nil, fmt.Errorf("locale %s: %d bytes exceeds the %d byte limit", l, len(b), maxFileBytes)
		}
		files[l+".json"] = b
	}

	parents, err := parentLocales(src)
	if err != nil {
		return nil, err
	}
	scripts, err := likelyScripts(src, locales)
	if err != nil {
		return nil, err
	}
	index := obj{
		"cldrVersion":   version,
		"currencies":    digits,
		"likelyScripts": scripts,
		"locales":       strSlice(locales),
		"parentLocales": parents,
		"plurals":       plurals,
	}
	b, err := canonical(index)
	if err != nil {
		return nil, err
	}
	files["index.json"] = b
	return files, nil
}

func strSlice(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// buildLocale assembles the self-contained document of one locale.
func buildLocale(src source, l string) (obj, error) {
	doc := obj{"locale": l}

	layout, err := src.load("cldr-misc-full/main/" + l + "/layout.json")
	if err != nil {
		return nil, err
	}
	switch atStr(layout, "main", l, "layout", "orientation", "characterOrder") {
	case "right-to-left":
		doc["direction"] = "rtl"
	case "left-to-right":
		doc["direction"] = "ltr"
	default:
		return nil, errors.New("layout.json has no character order")
	}

	if err := addNumbers(src, l, doc); err != nil {
		return nil, err
	}
	if err := addCurrencies(src, l, doc); err != nil {
		return nil, err
	}
	if err := addTerritories(src, l, doc); err != nil {
		return nil, err
	}

	cals := obj{}
	g, err := src.load("cldr-dates-full/main/" + l + "/ca-gregorian.json")
	if err != nil {
		return nil, err
	}
	gc, err := calendar(atObj(g, "main", l, "dates", "calendars", "gregorian"), nil, true)
	if err != nil {
		return nil, fmt.Errorf("gregorian: %w", err)
	}
	cals["gregorian"] = gc

	islamic, err := src.load("cldr-cal-islamic-full/main/" + l + "/ca-islamic.json")
	if err != nil {
		return nil, err
	}
	islamicRaw := atObj(islamic, "main", l, "dates", "calendars", "islamic")
	// The generic calendar supplies the flexible formats a calendar lacks,
	// as it does in ICU.
	gen, err := src.load("cldr-dates-full/main/" + l + "/ca-generic.json")
	if err != nil {
		return nil, err
	}
	genericRaw := atObj(gen, "main", l, "dates", "calendars", "generic")
	for _, c := range []struct{ id, pkg, file string }{
		{"ethiopic", "cldr-cal-ethiopic-full", "ca-ethiopic"},
		{"islamic-umalqura", "cldr-cal-islamic-full", "ca-islamic-umalqura"},
		{"persian", "cldr-cal-persian-full", "ca-persian"},
	} {
		d, err := src.load(c.pkg + "/main/" + l + "/" + c.file + ".json")
		if err != nil {
			return nil, err
		}
		raw := atObj(d, "main", l, "dates", "calendars", c.id)
		if c.id == "islamic-umalqura" {
			raw = withFallback(raw, islamicRaw)
		}
		cc, err := calendar(raw, genericRaw, false)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c.id, err)
		}
		cals[c.id] = cc
	}
	doc["calendars"] = cals
	return doc, nil
}

// withFallback returns raw with every top-level section it lacks (or has
// empty) taken from fallback.
func withFallback(raw, fallback obj) obj {
	out := obj{}
	for k, v := range fallback {
		out[k] = v
	}
	for k, v := range raw {
		if m, ok := v.(obj); ok && len(m) == 0 {
			continue
		}
		out[k] = v
	}
	return out
}

var symbolKeys = []string{
	"approximatelySign", "currencyDecimal", "currencyGroup", "decimal", "exponential", "group",
	"infinity", "list", "minusSign", "nan", "percentSign", "perMille", "plusSign",
	"superscriptingExponent", "timeSeparator",
}

func addNumbers(src source, l string, doc obj) error {
	n, err := src.load("cldr-numbers-full/main/" + l + "/numbers.json")
	if err != nil {
		return err
	}
	num := atObj(n, "main", l, "numbers")
	def := atStr(num, "defaultNumberingSystem")
	if def == "" {
		return errors.New("numbers.json has no default numbering system")
	}
	ns, err := src.load("cldr-core/supplemental/numberingSystems.json")
	if err != nil {
		return err
	}
	digitsOf := func(id string) (string, error) {
		d := atStr(ns, "supplemental", "numberingSystems", id, "_digits")
		if atStr(ns, "supplemental", "numberingSystems", id, "_type") != "numeric" || d == "" {
			return "", fmt.Errorf("numbering system %q is not a decimal system", id)
		}
		return d, nil
	}
	dd, err := digitsOf(def)
	if err != nil {
		return err
	}
	doc["numbering"] = obj{"default": def, "digits": dd}
	mg, err := strconv.Atoi(atStr(num, "minimumGroupingDigits"))
	if err != nil {
		return fmt.Errorf("minimumGroupingDigits: %w", err)
	}
	doc["minimumGroupingDigits"] = mg

	systems := []string{"latn"}
	if def != "latn" {
		systems = append(systems, def)
	}
	numbers := obj{}
	for _, s := range systems {
		entry := obj{}
		syms := obj{}
		for _, k := range symbolKeys {
			if v := atStr(num, "symbols-numberSystem-"+s, k); v != "" {
				syms[k] = v
			}
		}
		for _, req := range []string{"decimal", "group", "minusSign", "percentSign", "plusSign", "infinity", "nan"} {
			if _, ok := syms[req]; !ok {
				return fmt.Errorf("numbers.json has no %s symbol for %s", req, s)
			}
		}
		entry["symbols"] = syms
		for _, p := range []struct {
			key  string
			path []string
		}{
			{"decimal", []string{"decimalFormats-numberSystem-" + s, "standard"}},
			{"percent", []string{"percentFormats-numberSystem-" + s, "standard"}},
			{"currency", []string{"currencyFormats-numberSystem-" + s, "standard"}},
			{"accounting", []string{"currencyFormats-numberSystem-" + s, "accounting"}},
		} {
			v := atStr(num, p.path...)
			if v == "" {
				return fmt.Errorf("numbers.json has no %s pattern for %s", p.key, s)
			}
			entry[p.key] = v
		}
		compact, err := compactShort(atObj(num, "decimalFormats-numberSystem-"+s, "short", "decimalFormat"))
		if err != nil {
			return fmt.Errorf("compact patterns of %s: %w", s, err)
		}
		entry["compactShort"] = compact
		numbers[s] = entry
	}
	doc["numbers"] = numbers

	sp := atObj(num, "currencyFormats-numberSystem-latn", "currencySpacing")
	before := atStr(sp, "beforeCurrency", "insertBetween")
	after := atStr(sp, "afterCurrency", "insertBetween")
	if before == "" || after == "" {
		return errors.New("numbers.json has no currency spacing")
	}
	doc["currencySpacing"] = obj{"after": after, "before": before}
	return nil
}

var compactKey = regexp.MustCompile(`^(\d+)-count-([a-z]+)$`)

// compactShort turns CLDR's "1000-count-one" keys into {"1000":{"one":…}}.
func compactShort(m obj) (obj, error) {
	out := obj{}
	for k, v := range m {
		sub := compactKey.FindStringSubmatch(k)
		if sub == nil {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("pattern %s is not a string", k)
		}
		row, _ := out[sub[1]].(obj)
		if row == nil {
			row = obj{}
			out[sub[1]] = row
		}
		row[sub[2]] = s
	}
	if len(out) == 0 {
		return nil, errors.New("no patterns")
	}
	return out, nil
}

func addCurrencies(src source, l string, doc obj) error {
	c, err := src.load("cldr-numbers-full/main/" + l + "/currencies.json")
	if err != nil {
		return err
	}
	cur := atObj(c, "main", l, "numbers", "currencies")
	out := obj{}
	for code, v := range cur {
		e, _ := v.(obj)
		// A currency without a symbol is written as its ISO code, so the code
		// is the default and only a different symbol is stored; one without a
		// name is shown by its code.
		name := atStr(e, "displayName")
		sym := atStr(e, "symbol")
		if sym == "" {
			sym = code
		}
		entry := obj{}
		if name != "" {
			entry["name"] = name
		}
		if sym != code {
			entry["symbol"] = sym
		}
		if n := atStr(e, "symbol-alt-narrow"); n != "" && n != sym {
			entry["narrow"] = n
		}
		plural := obj{}
		for k, pv := range e {
			if cat, ok := strings.CutPrefix(k, "displayName-count-"); ok {
				if s, _ := pv.(string); s != "" && s != name {
					plural[cat] = s
				}
			}
		}
		if len(plural) > 0 {
			entry["plural"] = plural
		}
		if len(entry) > 0 {
			out[code] = entry
		}
	}
	if len(out) == 0 {
		return errors.New("currencies.json has no currency")
	}
	doc["currencies"] = out
	return nil
}

func addTerritories(src source, l string, doc obj) error {
	t, err := src.load("cldr-localenames-full/main/" + l + "/territories.json")
	if err != nil {
		return err
	}
	out := obj{}
	for code, v := range atObj(t, "main", l, "localeDisplayNames", "territories") {
		s, _ := v.(string)
		if len(code) == 2 && code != "ZZ" && s != "" && code[0] >= 'A' && code[0] <= 'Z' {
			out[code] = s
		}
	}
	if len(out) == 0 {
		return errors.New("territories.json has no region")
	}
	doc["territories"] = out
	return nil
}

var (
	widths    = []string{"wide", "abbreviated", "narrow"}
	dayWidths = []string{"wide", "abbreviated", "short", "narrow"}
	dayKeys   = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
	contexts  = []struct{ cldr, out string }{{"format", "format"}, {"stand-alone", "standAlone"}}
	styles    = []string{"full", "long", "medium", "short"}
)

// supportedLetters are the pattern letters the engines implement for flexible
// formats; a skeleton or pattern that needs any other field is left out of
// availableFormats. The standard date and time styles are kept as they are
// (the engines also read Y, B and z there).
const supportedLetters = "GyMLdEcahHKkms"

// calendar reduces one CLDR calendar to what the engines use. Non-Gregorian
// calendars drop the narrow name widths to stay within the file budget, and
// take the flexible formats of the generic calendar they do not define.
func calendar(raw, generic obj, gregorian bool) (obj, error) {
	if raw == nil {
		return nil, errors.New("calendar data is missing")
	}
	out := obj{}
	nMonths := 12
	if raw["months"] != nil {
		nMonths = len(atObj(raw, "months", "format", "wide"))
	}

	months := obj{}
	days := obj{}
	periods := obj{}
	for _, ctx := range contexts {
		m, d, p := obj{}, obj{}, obj{}
		for _, w := range widths {
			if !gregorian && w == "narrow" {
				continue
			}
			names, err := indexed(atObj(raw, "months", ctx.cldr, w), 1, nMonths)
			if err != nil {
				return nil, fmt.Errorf("months %s %s: %w", ctx.cldr, w, err)
			}
			m[w] = names
			ap := obj{}
			for _, k := range []string{"am", "pm"} {
				s := atStr(raw, "dayPeriods", ctx.cldr, w, k)
				if s == "" {
					return nil, fmt.Errorf("dayPeriods %s %s has no %s", ctx.cldr, w, k)
				}
				ap[k] = s
			}
			p[w] = ap
		}
		for _, w := range dayWidths {
			if !gregorian && w == "narrow" {
				continue
			}
			src := atObj(raw, "days", ctx.cldr, w)
			names := make([]any, len(dayKeys))
			for i, k := range dayKeys {
				s, _ := src[k].(string)
				if s == "" {
					return nil, fmt.Errorf("days %s %s has no %s", ctx.cldr, w, k)
				}
				names[i] = s
			}
			d[w] = names
		}
		months[ctx.out], days[ctx.out], periods[ctx.out] = m, d, p
	}
	out["months"], out["days"], out["dayPeriods"] = months, days, periods

	eras := obj{}
	for w, key := range map[string]string{"wide": "eraNames", "abbreviated": "eraAbbr", "narrow": "eraNarrow"} {
		if !gregorian && w == "narrow" {
			continue
		}
		src := atObj(raw, "eras", key)
		var names []any
		for i := 0; ; i++ {
			s, ok := src[strconv.Itoa(i)].(string)
			if !ok {
				break
			}
			names = append(names, s)
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("eras %s is empty", key)
		}
		eras[w] = names
	}
	out["eras"] = eras

	for _, kind := range []struct{ key, src string }{{"dateFormats", "dateFormats"}, {"timeFormats", "timeFormats"}} {
		f := obj{}
		for _, s := range styles {
			v := atStr(raw, kind.src, s)
			if v == "" {
				return nil, fmt.Errorf("%s has no %s", kind.src, s)
			}
			f[s] = v
		}
		out[kind.key] = f
	}
	dt := obj{}
	for _, s := range styles {
		v := atStr(raw, "dateTimeFormats", s)
		if v == "" {
			return nil, fmt.Errorf("dateTimeFormats has no %s", s)
		}
		dt[s] = v
	}
	out["dateTimeFormats"] = dt
	if atTime := atObj(raw, "dateTimeFormats-atTime", "standard"); len(atTime) > 0 {
		m := obj{}
		for _, s := range styles {
			if v := atStr(atTime, s); v != "" {
				m[s] = v
			}
		}
		out["dateTimeAtTime"] = m
	}
	avail := obj{}
	for _, source := range []obj{generic, raw} {
		for skel, v := range atObj(source, "dateTimeFormats", "availableFormats") {
			p, _ := v.(string)
			if p == "" || strings.Contains(skel, "-alt-") || !onlyLetters(skel, supportedLetters) || !onlyLetters(unquoted(p), supportedLetters) {
				continue
			}
			avail[skel] = p
		}
	}
	// The date and time styles are formats of their own skeletons (CLDR's
	// dateSkeletons and timeSkeletons) where no available format has that
	// skeleton, as in ICU's pattern generator. The day period a is implied
	// by the hour.
	for _, kind := range []struct{ formats, skeletons string }{{"dateFormats", "dateSkeletons"}, {"timeFormats", "timeSkeletons"}} {
		for _, s := range styles {
			skel := strings.ReplaceAll(atStr(raw, kind.skeletons, s), "a", "")
			p := atStr(raw, kind.formats, s)
			if _, have := avail[skel]; !have && skel != "" && onlyLetters(skel, supportedLetters) && onlyLetters(unquoted(p), supportedLetters) {
				avail[skel] = p
			}
		}
	}
	if len(avail) == 0 {
		return nil, errors.New("availableFormats is empty")
	}
	out["availableFormats"] = avail
	return out, nil
}

// unquoted returns the pattern with its quoted literals removed.
func unquoted(p string) string {
	var b strings.Builder
	quoted := false
	for _, r := range p {
		switch {
		case r == '\'':
			quoted = !quoted
		case !quoted:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// onlyLetters reports whether every ASCII letter of s is in allowed.
func onlyLetters(s, allowed string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') && !strings.ContainsRune(allowed, r) {
			return false
		}
	}
	return true
}

// indexed returns the values of keys first..first+n-1 of m as a slice.
func indexed(m obj, first, n int) ([]any, error) {
	out := make([]any, n)
	for i := range n {
		s, _ := m[strconv.Itoa(first+i)].(string)
		if s == "" {
			return nil, fmt.Errorf("no name for %d", first+i)
		}
		out[i] = s
	}
	return out, nil
}

// pluralTables returns the cardinal and ordinal rule tables by language.
func pluralTables(src source) (cardinal, ordinal obj, err error) {
	c, err := src.load("cldr-core/supplemental/plurals.json")
	if err != nil {
		return nil, nil, err
	}
	o, err := src.load("cldr-core/supplemental/ordinals.json")
	if err != nil {
		return nil, nil, err
	}
	return atObj(c, "supplemental", "plurals-type-cardinal"), atObj(o, "supplemental", "plurals-type-ordinal"), nil
}

// localePlurals looks up the rules of locale l: its own entry, else its
// language's. A locale CLDR gives no rules (such as ba) has only the "other"
// category, as in root.
func localePlurals(l string, cardinal, ordinal obj) obj {
	lang := strings.SplitN(l, "-", 2)[0]
	pick := func(t obj) obj {
		if r, ok := t[l].(obj); ok {
			return r
		}
		r, _ := t[lang].(obj)
		return r
	}
	rules := func(t obj) obj {
		out := obj{}
		for k, v := range pick(t) {
			cat, ok := strings.CutPrefix(k, "pluralRule-count-")
			if !ok {
				continue
			}
			rule, _, _ := strings.Cut(v.(string), "@")
			if rule = strings.TrimSpace(rule); rule != "" {
				out[cat] = rule
			}
		}
		return out
	}
	return obj{"cardinal": rules(cardinal), "ordinal": rules(ordinal)}
}

func parentLocales(src source) (obj, error) {
	p, err := src.load("cldr-core/supplemental/parentLocales.json")
	if err != nil {
		return nil, err
	}
	m := atObj(p, "supplemental", "parentLocales", "parentLocale")
	if len(m) == 0 {
		return nil, errors.New("parentLocales.json has no parent locale")
	}
	return m, nil
}

// atInt returns the integer at keys, or 0.
func atInt(v any, keys ...string) int {
	n, _ := at(v, keys...).(int)
	return n
}

// likelyScripts returns, for each language-region pair of the locales'
// languages whose script differs from the language's own (zh-TW is Hant, zh
// is Hans), the script CLDR's likely subtags give it.
func likelyScripts(src source, locales []string) (obj, error) {
	d, err := src.load("cldr-core/supplemental/likelySubtags.json")
	if err != nil {
		return nil, err
	}
	likely := atObj(d, "supplemental", "likelySubtags")
	if len(likely) == 0 {
		return nil, errors.New("likelySubtags.json is empty")
	}
	languages := map[string]bool{}
	for _, l := range locales {
		languages[strings.SplitN(l, "-", 2)[0]] = true
	}
	scriptOf := func(tag any) string {
		parts := strings.Split(fmt.Sprint(tag), "-")
		if len(parts) >= 2 && len(parts[1]) == 4 {
			return parts[1]
		}
		return ""
	}
	out := obj{}
	for key, v := range likely {
		lang, region, ok := strings.Cut(key, "-")
		if !ok || !languages[lang] || len(region) != 2 {
			continue
		}
		if s := scriptOf(v); s != "" && s != scriptOf(likely[lang]) {
			out[key] = s
		}
	}
	return out, nil
}

// currencyDigits returns the ISO 4217 fraction digits and rounding by code,
// with "DEFAULT" for a currency that has no entry.
func currencyDigits(src source) (obj, error) {
	c, err := src.load("cldr-core/supplemental/currencyData.json")
	if err != nil {
		return nil, err
	}
	out := obj{}
	for code, v := range atObj(c, "supplemental", "currencyData", "fractions") {
		e, _ := v.(obj)
		entry := obj{}
		for from, to := range map[string]string{"_digits": "digits", "_rounding": "rounding", "_cashDigits": "cashDigits", "_cashRounding": "cashRounding"} {
			s := atStr(e, from)
			if s == "" {
				continue
			}
			n, err := strconv.Atoi(s)
			if err != nil {
				return nil, fmt.Errorf("currency %s %s: %w", code, from, err)
			}
			entry[to] = n
		}
		if _, ok := entry["digits"]; !ok {
			return nil, fmt.Errorf("currency %s has no digits", code)
		}
		out[code] = entry
	}
	if _, ok := out["DEFAULT"]; !ok {
		return nil, errors.New("currencyData.json has no DEFAULT fractions")
	}
	return out, nil
}

// writeAll writes files into dir, replacing the *.json files already there so
// a locale CLDR dropped does not linger.
func writeAll(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	old, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return fmt.Errorf("list %s: %w", dir, err)
	}
	for _, f := range old {
		if err := os.Remove(f); err != nil {
			return fmt.Errorf("remove %s: %w", f, err)
		}
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

// canonical encodes v as canonical JSON: object keys sorted, two-space
// indentation, arrays of scalars on one line, a trailing newline, and every
// space-like or control character escaped so it stays visible in review.
func canonical(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeValue(&buf, v, 0); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func writeValue(buf *bytes.Buffer, v any, depth int) error {
	switch x := v.(type) {
	case obj:
		if len(x) == 0 {
			buf.WriteString("{}")
			return nil
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteString("{\n")
		for i, k := range keys {
			buf.WriteString(strings.Repeat("  ", depth+1))
			writeString(buf, k)
			buf.WriteString(": ")
			if err := writeValue(buf, x[k], depth+1); err != nil {
				return err
			}
			if i < len(keys)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		buf.WriteString(strings.Repeat("  ", depth))
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteString(", ")
			}
			if err := writeValue(buf, e, depth+1); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case string:
		writeString(buf, x)
	case int:
		buf.WriteString(strconv.Itoa(x))
	default:
		return fmt.Errorf("cannot encode %T", v)
	}
	return nil
}

func writeString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			buf.WriteString(`\"`)
		case r == '\\':
			buf.WriteString(`\\`)
		case r < 0x20 || r == 0x7f || (r > 0x7f && (unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r))):
			if r > 0xffff {
				r1, r2 := surrogates(r)
				fmt.Fprintf(buf, `\u%04x\u%04x`, r1, r2)
			} else {
				fmt.Fprintf(buf, `\u%04x`, r)
			}
		default:
			buf.WriteRune(r)
		}
	}
	buf.WriteByte('"')
}

func surrogates(r rune) (rune, rune) {
	r -= 0x10000
	return 0xd800 + (r>>10)&0x3ff, 0xdc00 + r&0x3ff
}
