// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/l10n"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const (
	appFile        = "app.json"
	assetIndexFile = "assets/index.json"
	// resultTextNode is a Text node of the fixture's result page.
	resultTextNode = "01a0c450-6c00-702a-8000-000000051336"
	resultPageID   = "01a0c450-6c00-7013-8000-000000024bbd"
)

// localeSection reads the l10n section of a locale from the app bundle.
func localeSection(t *testing.T, res *Result, tag string) *fbs.Locale {
	t.Helper()
	id, err := bundle.LocaleID(tag)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := readAll(t, res)[0].Section(bundle.SectionL10n, id)
	if !ok {
		t.Fatalf("no %s section", tag)
	}
	return fbs.GetRootAsLocale(s.Data, 0)
}

// Verifies: I18N-002, CMP-002.
// Every supported locale's section carries its reduced CLDR data, the
// locale the data came from and Flutter's widget strings; the output is
// deterministic.
func TestLocaleSectionCarriesDataAndFlutterStrings(t *testing.T) {
	t.Parallel()
	res := compileFS(fixture(t))
	if res.Diagnostics.HasErrors() {
		t.Fatal(list(res.Diagnostics))
	}
	for tag, resolved := range map[string]string{"en": "en", "de-DE": "de", "am-ET": "am"} {
		loc := localeSection(t, res, tag)
		if string(loc.TagResolved()) != resolved {
			t.Errorf("%s: tag_resolved = %q", tag, loc.TagResolved())
		}
		data, err := l10n.Parse(loc.DataBytes())
		if err != nil || data.ID() != resolved {
			t.Errorf("%s: data: %v", tag, err)
		}
		var flutter map[string]map[string]string
		if err := json.Unmarshal(loc.FlutterBytes(), &flutter); err != nil {
			t.Fatalf("%s: flutter: %v", tag, err)
		}
		if flutter["material"]["cutButtonLabel"] == "" || flutter["widgets"]["copyButtonLabel"] == "" {
			t.Errorf("%s: the Flutter strings lack the Material or widgets keys", tag)
		}
	}
	de := localeSection(t, res, "de-DE")
	if !bytes.Contains(de.FlutterBytes(), []byte("Ausschneiden")) {
		t.Error("the de-DE Flutter strings are not German")
	}
	again := compileFS(fixture(t))
	if !bytes.Equal(res.App.Data, again.App.Data) {
		t.Error("two compilations differ")
	}
}

// Verifies: I18N-002.
// A calendar the app declares is kept in the data; others are dropped.
func TestLocaleDataKeepsDeclaredCalendars(t *testing.T) {
	t.Parallel()
	calendars := func(m fstest.MapFS) map[string]json.RawMessage {
		res := compileFS(m)
		if res.Diagnostics.HasErrors() {
			t.Fatal(list(res.Diagnostics))
		}
		var doc struct {
			Calendars map[string]json.RawMessage `json:"calendars"`
		}
		if err := json.Unmarshal(localeSection(t, res, "en").DataBytes(), &doc); err != nil {
			t.Fatal(err)
		}
		return doc.Calendars
	}
	if got := calendars(fixture(t)); len(got) != 1 || got["gregorian"] == nil {
		t.Errorf("default calendars = %d", len(got))
	}
	m := fixture(t)
	edit(t, m, appFile, func(doc map[string]any) {
		doc["localisation"] = map[string]any{"calendar": "ethiopic", "secondaryCalendar": "persian"}
	})
	if got := calendars(m); len(got) != 3 || got["ethiopic"] == nil || got["persian"] == nil {
		t.Errorf("declared calendars = %d", len(got))
	}
}

// Verifies: I18N-002.
// A supported locale CLDR has no data for is PLX-1600 at its place in app.json.
func TestUnsupportedLocaleIsRejected(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	edit(t, m, appFile, func(doc map[string]any) {
		doc["supportedLocales"] = append(doc["supportedLocales"].([]any), "xx")
	})
	res := compileFS(m)
	wantDiag(t, res, plxerr.LocaleUnsupported, appFile, "/supportedLocales/3")
}

// overrideFixture adds localeOverrides to the result page's Text node.
func overrideFixture(t *testing.T, overrides string) fstest.MapFS {
	t.Helper()
	m := fixture(t)
	edit(t, m, resultPage, func(doc map[string]any) {
		node := at(t, doc, "root/slots/body/slots/child")
		node["localeOverrides"] = raw(t, overrides)
	})
	return m
}

// Verifies: I18N-004, I18N-012.
// A node's localeOverrides compile into locale override layers keyed by tag.
func TestLocaleOverridesBecomeLayers(t *testing.T) {
	t.Parallel()
	res := compileFS(overrideFixture(t, `{"de-DE": {"data": "Hallo"}, "am-ET": {"data": "ሰላም"}}`))
	if res.Diagnostics.HasErrors() {
		t.Fatal(list(res.Diagnostics))
	}
	nodes, strs := pageNodes(t, readAll(t, res)[1], resultPageID)
	n := nodes[resultTextNode]
	if n == nil || n.OverridesLength() != 2 {
		t.Fatalf("overrides: %v", n)
	}
	var keys []string
	var o fbs.Override
	for i := range n.OverridesLength() {
		n.Overrides(&o, i)
		if o.Kind() != fbs.OverrideKindLocale {
			t.Errorf("override %d is %v", i, o.Kind())
		}
		keys = append(keys, strs[o.Key()])
	}
	if keys[0] != "am-ET" || keys[1] != "de-DE" {
		t.Errorf("keys = %v", keys)
	}
}

// Verifies: I18N-012.
// A locale override naming an unsupported locale, or a prop the widget
// lacks, is PLX-1613; a malformed tag fails the document schema (PLX-1005)
// at the override's own location.
func TestInvalidLocaleOverridesAreRejected(t *testing.T) {
	t.Parallel()
	base := resultPage + ""
	for name, c := range map[string]struct {
		json, ptr string
		code      plxerr.Code
	}{
		"unsupported locale": {`{"fr": {"data": "x"}}`, "/localeOverrides/fr", plxerr.LocaleOverrideInvalid},
		"malformed locale":   {`{"de_DE": {"data": "x"}}`, "/localeOverrides/de_DE", plxerr.InvalidFormat},
		"unknown prop":       {`{"de-DE": {"nope": "x"}}`, "/localeOverrides/de-DE", plxerr.LocaleOverrideInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res := compileFS(overrideFixture(t, c.json))
			found := false
			for _, d := range res.Diagnostics {
				if d.Code == c.code && d.File == base && len(d.Path) >= len(c.ptr) && d.Path[len(d.Path)-len(c.ptr):] == c.ptr {
					found = true
				}
			}
			if !found {
				t.Errorf("no %s ending in %s:\n%s", c.code, c.ptr, list(res.Diagnostics))
			}
		})
	}
}

// assetLocaleFixture gives the logo a German file.
func assetLocaleFixture(t *testing.T, locales string) fstest.MapFS {
	t.Helper()
	m := fixture(t)
	edit(t, m, assetIndexFile, func(doc map[string]any) {
		doc["assets"].([]any)[0].(map[string]any)["locales"] = raw(t, locales)
	})
	logo := m["assets/images/logo.png"].Data
	m["assets/images/logo-de.png"] = &fstest.MapFile{Data: append(bytes.Clone(logo), 0)}
	return m
}

// Verifies: I18N-012.
// An asset's per-locale files are indexed with their own hash and size.
func TestAssetLocaleFilesAreIndexed(t *testing.T) {
	t.Parallel()
	m := assetLocaleFixture(t, `{"de-DE": "images/logo-de.png"}`)
	res := compileFS(m)
	if res.Diagnostics.HasErrors() {
		t.Fatal(list(res.Diagnostics))
	}
	var s bundle.Section
	var ok bool
	for _, sec := range readAll(t, res)[0].Sections {
		if sec.Kind == bundle.SectionAssetsIndex {
			s, ok = sec, true
		}
	}
	if !ok {
		t.Fatal("no asset index")
	}
	var a fbs.Asset
	fbs.GetRootAsAssetIndex(s.Data, 0).Assets(&a, 0)
	if a.LocaleFilesLength() != 1 {
		t.Fatalf("%d locale files", a.LocaleFilesLength())
	}
	var lf fbs.AssetLocaleFile
	a.LocaleFiles(&lf, 0)
	want := sha256.Sum256(m["assets/images/logo-de.png"].Data)
	if string(lf.Tag()) != "de-DE" || !bytes.Equal(lf.HashBytes(), want[:]) || lf.Size() != uint64(len(m["assets/images/logo-de.png"].Data)) {
		t.Errorf("locale file = %s", lf.Tag())
	}
}

// Verifies: I18N-012.
func TestInvalidAssetLocalesAreRejected(t *testing.T) {
	t.Parallel()
	for name, locales := range map[string]string{
		"unsupported locale": `{"fr": "images/logo-de.png"}`,
		"missing file":       `{"de-DE": "images/none.png"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res := compileFS(assetLocaleFixture(t, locales))
			if !res.Diagnostics.HasErrors() {
				t.Fatalf("accepted:\n%s", list(res.Diagnostics))
			}
		})
	}
	res := compileFS(assetLocaleFixture(t, `{"fr": "images/logo-de.png"}`))
	wantDiag(t, res, plxerr.LocaleOverrideInvalid, assetIndexFile, "/assets/0/locales/fr")
}

// Verifies: CMP-032.
// The script of a locale comes from its subtag or its language, and the
// ranges of the app's locales never include another script's.
func TestScriptsOfLocales(t *testing.T) {
	t.Parallel()
	for tag, want := range map[string]string{
		"en": "Latn", "de-DE": "Latn", "am-ET": "Ethi", "ar": "Arab", "zh-Hant-TW": "Hant", "zh": "Hans",
		"sr-Latn": "Latn", "ja": "Jpan", "ru": "Cyrl",
	} {
		if got := scriptOf(tag); got != want {
			t.Errorf("scriptOf(%q) = %q, want %q", tag, got, want)
		}
	}
	covers := func(ranges [][2]rune, r rune) bool {
		for _, p := range ranges {
			if r >= p[0] && r <= p[1] {
				return true
			}
		}
		return false
	}
	ranges := textRanges([]string{"en", "am-ET"})
	if !covers(ranges, 'a') || !covers(ranges, 0x1200) || covers(ranges, 0x0627) {
		t.Error("the ranges of en and am are not Latin and Ethiopic")
	}
}

// Verifies: CMP-032.
// A text font in the asset index is subset to the scripts of the supported
// locales, and the subset is a file of the compilation.
func TestTextFontsAreSubsetToTheLocalesScripts(t *testing.T) {
	t.Parallel()
	font, err := os.ReadFile(filepath.Join("..", "icons", "fonts", "CupertinoIcons.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	m := fixture(t)
	edit(t, m, assetIndexFile, func(doc map[string]any) {
		doc["assets"] = append(doc["assets"].([]any), map[string]any{
			"id": "01a0c450-6c00-7fff-8000-0000000000b1", "key": "ui-font", "file": "fonts/ui.ttf", "mediaType": "font/ttf",
		})
	})
	m["assets/fonts/ui.ttf"] = &fstest.MapFile{Data: font}
	res := compileFS(m)
	if res.Diagnostics.HasErrors() {
		t.Fatal(list(res.Diagnostics))
	}
	var s bundle.Section
	for _, sec := range readAll(t, res)[0].Sections {
		if sec.Kind == bundle.SectionAssetsIndex {
			s = sec
		}
	}
	idx := fbs.GetRootAsAssetIndex(s.Data, 0)
	var a fbs.Asset
	found := false
	for i := range idx.AssetsLength() {
		idx.Assets(&a, i)
		if string(a.Key()) != "ui-font" {
			continue
		}
		found = true
		if a.Size() >= uint64(len(font)) {
			t.Errorf("the font was not subset: %d of %d bytes", a.Size(), len(font))
		}
		var sum [sha256.Size]byte
		copy(sum[:], a.HashBytes())
		if data, ok := res.Files[sum]; !ok || uint64(len(data)) != a.Size() {
			t.Error("the subset font is not a file of the compilation")
		}
	}
	if !found {
		t.Fatal("the font is not indexed")
	}
}

// Verifies: I18N-003.
// A document's camelCase calendar names map to the CLDR keys the locale data
// is keyed by.
func TestCLDRCalendarKeys(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"islamicUmalqura": "islamic-umalqura",
		"ethiopic":        "ethiopic",
		"persian":         "persian",
		"gregorian":       "gregorian",
	} {
		if got := cldrCalendar(in); got != want {
			t.Errorf("cldrCalendar(%q) = %q, want %q", in, got, want)
		}
	}
	m := fixture(t)
	edit(t, m, appFile, func(doc map[string]any) {
		doc["localisation"] = map[string]any{"calendar": "islamicUmalqura"}
	})
	res := compileFS(m)
	if res.Diagnostics.HasErrors() {
		t.Fatal(list(res.Diagnostics))
	}
	var data struct {
		Calendars map[string]json.RawMessage `json:"calendars"`
	}
	if err := json.Unmarshal(localeSection(t, res, "en").DataBytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data.Calendars["islamic-umalqura"] == nil {
		t.Error("the locale data lacks the Umm al-Qura calendar")
	}
}

// Verifies: I18N-003, I18N-011.
// The app's localisation settings reach the app bundle's meta, numbering
// systems sorted by locale; an app without them has no table.
func TestMetaCarriesLocalisation(t *testing.T) {
	t.Parallel()
	meta := func(res *Result) *fbs.Meta {
		if res.Diagnostics.HasErrors() {
			t.Fatal(list(res.Diagnostics))
		}
		for _, s := range readAll(t, res)[0].Sections {
			if s.Kind == bundle.SectionMeta {
				return fbs.GetRootAsMeta(s.Data, 0)
			}
		}
		t.Fatal("no meta section")
		return nil
	}
	if meta(compileFS(fixture(t))).Localisation(nil) != nil {
		t.Error("an app without localisation has a table")
	}
	m := fixture(t)
	edit(t, m, appFile, func(doc map[string]any) {
		doc["localisation"] = map[string]any{
			"calendar":          "ethiopic",
			"secondaryCalendar": "islamicUmalqura",
			"numberingSystem":   map[string]any{"de": "latn", "ar": "native", "am": "native"},
		}
	})
	res := compileFS(m)
	l := meta(res).Localisation(nil)
	if l == nil || string(l.Calendar()) != "ethiopic" || string(l.SecondaryCalendar()) != "islamicUmalqura" {
		t.Fatalf("localisation = %v", l)
	}
	var got []string
	var e fbs.NumberingSystemEntry
	for i := range l.NumberingSystemsLength() {
		if !l.NumberingSystems(&e, i) {
			t.Fatal("entry unreadable")
		}
		got = append(got, string(e.Locale())+"="+string(e.System()))
	}
	if want := []string{"am=native", "ar=native", "de=latn"}; !reflect.DeepEqual(got, want) {
		t.Errorf("numbering systems = %v, want %v", got, want)
	}
	if !bytes.Equal(res.App.Data, compileFS(m).App.Data) {
		t.Error("two compilations differ")
	}
}

// Verifies: I18N-002, ACT-020.
// The locale's flutter strings carry the runtime's own strings as the "plux"
// group, from the locale's parent chain, else the default locale's.
func TestLocaleCarriesRuntimeStrings(t *testing.T) {
	t.Parallel()
	res := compileFS(fixture(t))
	if res.Diagnostics.HasErrors() {
		t.Fatal(list(res.Diagnostics))
	}
	for tag, want := range map[string]string{"en": "Content unavailable", "de-DE": "Inhalt nicht verfügbar", "am-ET": "ይዘቱ አይገኝም"} {
		var flutter map[string]map[string]string
		if err := json.Unmarshal(localeSection(t, res, tag).FlutterBytes(), &flutter); err != nil {
			t.Fatal(err)
		}
		if got := flutter["plux"]["plux.fallback.unavailable"]; got != want {
			t.Errorf("%s: plux.fallback.unavailable = %q, want %q", tag, got, want)
		}
	}
}
