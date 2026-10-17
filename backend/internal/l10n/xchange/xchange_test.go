// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package xchange_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/l10n/xchange"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const (
	idTitle = "01d0c450-6c00-7000-8000-000000000280"
	idUser  = "01d0c450-6c00-7000-8000-000000000281"
	idHint  = "01d0c450-6c00-7000-8000-000000000282"
)

func keys() []xchange.Key {
	return []xchange.Key{
		{ID: idTitle, Key: "login.title", Description: "Heading of the sign-in page", MaxLength: 40},
		{ID: idUser, Key: "login.username", Args: []xchange.Arg{{Name: "count", Type: "int"}}},
		{ID: idHint, Key: "login.hint", Description: "Quotes \"and\" <tags> & ampersands"},
	}
}

func english() xchange.Doc {
	return xchange.Doc{ID: "d-en", Locale: "en", Messages: map[string]string{
		idTitle: "Welcome back", idUser: "Username", idHint: "Use <b>your</b> \"login\" & more\nsecond line",
	}}
}

func german() xchange.Doc {
	return xchange.Doc{ID: "d-de", Locale: "de", Messages: map[string]string{
		idTitle: "Willkommen zurück", idHint: "Nutzen Sie <b>Ihre</b> \"Anmeldung\" & mehr\nzweite Zeile",
	}, States: map[string]string{idTitle: "approved", idHint: "reviewed"}}
}

func empty(locale string) xchange.Doc {
	return xchange.Doc{Locale: locale, Messages: map[string]string{}, States: map[string]string{}}
}

func opts(approver bool) xchange.Options {
	return xchange.Options{MaxBytes: 1 << 20, Approver: approver}
}

func codeOf(t *testing.T, err error) plxerr.Code {
	t.Helper()
	var pe *plxerr.Error
	if !errors.As(err, &pe) {
		t.Fatalf("error %v is not a plxerr", err)
	}
	return pe.Code
}

// Verifies: I18N-009. Export then import returns the same translations in
// every format: into the document it came from nothing changes, and into an
// empty locale the messages and, for an approver, the states are restored.
func TestRoundTrip(t *testing.T) {
	t.Parallel()
	for _, f := range xchange.Formats() {
		t.Run(string(f), func(t *testing.T) {
			t.Parallel()
			data, err := xchange.Export(keys(), english(), german(), f)
			if err != nil {
				t.Fatal(err)
			}
			again, err := xchange.Export(keys(), english(), german(), f)
			if err != nil || string(again) != string(data) {
				t.Fatalf("export is not deterministic: %v", err)
			}

			same, diags, err := xchange.Import(data, f, keys(), german(), english(), opts(true))
			if err != nil {
				t.Fatalf("import: %v\n%s", err, data)
			}
			if len(diags) != 0 {
				t.Fatalf("diagnostics on a round trip: %v", diags)
			}
			if changed := xchange.Changed(german(), same); len(changed) != 0 {
				t.Fatalf("a round trip changed %v", changed)
			}

			fresh, _, err := xchange.Import(data, f, keys(), empty("de"), english(), opts(true))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(fresh.Messages, german().Messages) {
				t.Fatalf("messages = %q, want %q", fresh.Messages, german().Messages)
			}
			if !reflect.DeepEqual(fresh.States, german().States) {
				t.Fatalf("states = %q, want %q", fresh.States, german().States)
			}
		})
	}
}

// Verifies: I18N-009. The XLIFF export has the shape tools expect.
func TestXLIFFExportShape(t *testing.T) {
	t.Parallel()
	data, err := xchange.Export(keys(), english(), german(), xchange.XLIFF)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="de">`,
		`<file id="app">`,
		`<unit id="` + idTitle + `" name="login.title">`,
		`<note id="description" category="description">Heading of the sign-in page</note>`,
		`<note id="maxLength" category="maxLength">40</note>`,
		`<segment state="final">`,
		`<segment state="initial">`,
		`<source>Welcome back</source>`,
		`<target>Willkommen zurück</target>`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("export lacks %q:\n%s", want, text)
		}
	}
	// The key without a translation has no target.
	unit := text[strings.Index(text, `id="`+idUser):]
	unit = unit[:strings.Index(unit, "</unit>")]
	if strings.Contains(unit, "<target>") {
		t.Errorf("an untranslated key has a target:\n%s", unit)
	}
}

// Verifies: I18N-009. Documents with a DOCTYPE or ENTITY declaration, the
// vector of XXE and entity-expansion attacks, are refused whole.
func TestXLIFFRefusesDeclarations(t *testing.T) {
	t.Parallel()
	body := `<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="de"><file id="app">` +
		`<unit id="` + idTitle + `" name="login.title"><segment><source>Welcome back</source><target>&xxe;</target></segment></unit></file></xliff>`
	for name, doc := range map[string]string{
		"external entity": `<?xml version="1.0"?><!DOCTYPE xliff [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>` + body,
		"internal entity": `<?xml version="1.0"?><!DOCTYPE xliff [<!ENTITY xxe "boom">]>` + body,
		"lower case":      `<!doctype xliff>` + body,
		"entity only":     `<!ENTITY xxe "x">` + body,
		"in a comment":    body + `<!-- <!DOCTYPE x> -->`,
		"undeclared":      body,
	} {
		_, _, err := xchange.Import([]byte(doc), xchange.XLIFF, keys(), german(), english(), opts(true))
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if code := codeOf(t, err); code != plxerr.TranslationImportInvalid {
			t.Errorf("%s: code %s, want PLX-1620", name, code)
		}
	}
}

// Verifies: I18N-009. A file that is not XLIFF 2.0, not UTF-8, ill-formed,
// or holds markup a message cannot, applies nothing.
func TestXLIFFRefusesInvalid(t *testing.T) {
	t.Parallel()
	const head = `<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="de"><file id="app">`
	const tail = `</file></xliff>`
	unit := func(inner string) string {
		return head + `<unit id="` + idTitle + `" name="login.title">` + inner + `</unit>` + tail
	}
	for name, doc := range map[string]string{
		"not xml":       `hello`,
		"empty":         ``,
		"wrong root":    `<html/>`,
		"xliff 1.2":     `<xliff xmlns="urn:oasis:names:tc:xliff:document:1.2" version="1.2"><file/></xliff>`,
		"no namespace":  `<xliff version="2.0" srcLang="en"><file/></xliff>`,
		"wrong version": strings.Replace(head+tail, `version="2.0"`, `version="2.1"`, 1),
		"latin1":        `<?xml version="1.0" encoding="ISO-8859-1"?>` + head + tail,
		"unterminated":  head + `<unit id="a">`,
		"unknown ent":   unit(`<segment><source>a</source><target>&nbsp;</target></segment>`),
		"nested markup": unit(`<segment><source>a</source><target>x <ph id="1"/> y</target></segment>`),
		"two segments":  unit(`<segment><source>a</source><target>b</target></segment><segment><source>c</source><target>d</target></segment>`),
		"bad state":     unit(`<segment state="bogus"><source>a</source><target>b</target></segment>`),
		"no identity":   head + `<unit><segment><source>a</source><target>b</target></segment></unit>` + tail,
		"wrong locale":  strings.Replace(unit(`<segment><source>a</source><target>b</target></segment>`), `trgLang="de"`, `trgLang="fr"`, 1),
		"deep":          head + strings.Repeat("<group>", 40) + strings.Repeat("</group>", 40) + tail,
	} {
		_, _, err := xchange.Import([]byte(doc), xchange.XLIFF, keys(), german(), english(), opts(true))
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if code := codeOf(t, err); code != plxerr.TranslationImportInvalid {
			t.Errorf("%s: code %s, want PLX-1620", name, code)
		}
	}
}

// Verifies: I18N-009. A tool's own additions, such as comments, processing
// instructions, CDATA and unknown elements, are tolerated.
func TestXLIFFToleratesToolOutput(t *testing.T) {
	t.Parallel()
	doc := `<?xml version="1.0" encoding="UTF-8"?><?tool x?>
<!-- generated -->
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="de">
 <file id="f1"><group id="g"><unit id="` + idUser + `" name="login.username">
  <originalData><data id="d1">ignored</data></originalData>
  <segment state="translated"><source>Username</source><target><![CDATA[Benutzer <name>]]></target></segment>
 </unit></group></file></xliff>`
	got, diags, err := xchange.Import([]byte(doc), xchange.XLIFF, keys(), empty("de"), english(), opts(false))
	if err != nil || len(diags) != 0 {
		t.Fatalf("err %v diags %v", err, diags)
	}
	if got.Messages[idUser] != "Benutzer <name>" || got.State(idUser) != "translated" {
		t.Fatalf("got %q %q", got.Messages[idUser], got.State(idUser))
	}
}

// Verifies: I18N-009. A key the app does not declare is reported, skipped,
// and does not stop the others.
func TestImportUnknownKey(t *testing.T) {
	t.Parallel()
	csv := "keyId,key,target\n" +
		"01d0c450-6c00-7000-8000-0000000002ff,ghost.key,Geist\n" +
		",login.username,Benutzername\n" +
		idHint + ",login.title,mismatch\n"
	got, diags, err := xchange.Import([]byte(csv), xchange.CSV, keys(), german(), english(), opts(false))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %v", diags)
	}
	for _, d := range diags {
		if d.Code != plxerr.TranslationImportKeyUnknown {
			t.Errorf("code %s, want PLX-1621", d.Code)
		}
	}
	if got.Messages[idUser] != "Benutzername" {
		t.Errorf("the known key was not applied: %q", got.Messages)
	}
	if got.Messages[idHint] != german().Messages[idHint] {
		t.Errorf("a key whose id and name disagree was applied")
	}
	if _, ok := got.Messages["01d0c450-6c00-7000-8000-0000000002ff"]; ok {
		t.Errorf("an unknown key was applied")
	}
}

// Verifies: I18N-009, I18N-008. A translation made from another source text
// is stored as translated with a PLX-1622 warning, even when an approver
// imports it as approved.
func TestImportSourceChanged(t *testing.T) {
	t.Parallel()
	csv := "keyId,source,target,state\n" + idTitle + ",Welcome,Willkommen zurück,approved\n"
	got, diags, err := xchange.Import([]byte(csv), xchange.CSV, keys(), german(), english(), opts(true))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 || diags[0].Code != plxerr.TranslationImportSourceChanged {
		t.Fatalf("diagnostics = %v", diags)
	}
	if got.State(idTitle) != "translated" {
		t.Errorf("state = %q, want translated", got.State(idTitle))
	}
}

// Verifies: I18N-008, I18N-009. An import never sets approved for a caller
// without translations.approve, and an approver may.
func TestImportApprovedNeedsApprover(t *testing.T) {
	t.Parallel()
	csv := "keyId,target,state\n" + idUser + ",Benutzer,approved\n" + idTitle + ",Willkommen!,approved\n" + idHint + ",\"" + strings.ReplaceAll(german().Messages[idHint], "\"", "\"\"") + "\",approved\n"

	plain, plainDiags, err := xchange.Import([]byte(csv), xchange.CSV, keys(), german(), english(), opts(false))
	if err != nil {
		t.Fatal(err)
	}
	if plain.State(idUser) != "translated" {
		t.Errorf("new text: state %q, want translated", plain.State(idUser))
	}
	if plain.State(idTitle) != "translated" {
		t.Errorf("changed approved text: state %q, want translated", plain.State(idTitle))
	}
	if plain.State(idHint) != "reviewed" {
		t.Errorf("unchanged text: state %q, want the stored reviewed", plain.State(idHint))
	}
	for id := range plain.States {
		if plain.States[id] == "approved" {
			t.Errorf("%s approved without translations.approve", id)
		}
	}

	downgrades := 0
	for _, d := range plainDiags {
		if d.Code == plxerr.TranslationImportDowngraded {
			downgrades++
			if !strings.Contains(d.Message, "3 translation(s)") {
				t.Errorf("warning %q does not name the count", d.Message)
			}
		}
	}
	if downgrades != 1 {
		t.Errorf("%d downgrade warnings, want one", downgrades)
	}

	approver, approverDiags, err := xchange.Import([]byte(csv), xchange.CSV, keys(), german(), english(), opts(true))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range approverDiags {
		if d.Code == plxerr.TranslationImportDowngraded {
			t.Errorf("an approver is warned: %s", d.Message)
		}
	}
	for _, id := range []string{idUser, idTitle, idHint} {
		if approver.State(id) != "approved" {
			t.Errorf("approver: %s state %q", id, approver.State(id))
		}
	}
}

// Verifies: I18N-008. Changed text without a stated state becomes translated;
// unchanged text keeps its state; the input document is never modified.
func TestImportStates(t *testing.T) {
	t.Parallel()
	before := german()
	csv := "keyId,target\n" + idUser + ",Benutzername\n" + idTitle + "," + before.Messages[idTitle] + "\n"
	got, _, err := xchange.Import([]byte(csv), xchange.CSV, keys(), before, english(), opts(false))
	if err != nil {
		t.Fatal(err)
	}
	if got.State(idUser) != "translated" || got.State(idTitle) != "approved" {
		t.Errorf("states = %q", got.States)
	}
	if !reflect.DeepEqual(before, german()) {
		t.Errorf("Import modified its input")
	}
	if want := []string{idUser}; !reflect.DeepEqual(xchange.Changed(before, got), want) {
		t.Errorf("changed = %v, want %v", xchange.Changed(before, got), want)
	}
}

// Verifies: I18N-009. A file larger than translation.importSize is refused
// before it is parsed.
func TestImportSizeBound(t *testing.T) {
	t.Parallel()
	csv := []byte("keyId,target\n" + idUser + ",Benutzer\n")
	_, _, err := xchange.Import(csv, xchange.CSV, keys(), german(), english(), xchange.Options{MaxBytes: int64(len(csv)) - 1})
	if err == nil || codeOf(t, err) != plxerr.LimitExceeded {
		t.Fatalf("err = %v, want PLX-1320", err)
	}
	if _, _, err := xchange.Import(csv, xchange.CSV, keys(), german(), english(), xchange.Options{MaxBytes: int64(len(csv))}); err != nil {
		t.Fatalf("a file at the limit: %v", err)
	}
	if _, _, err := xchange.Import(csv, xchange.CSV, keys(), german(), english(), xchange.Options{}); err == nil {
		t.Fatal("no limit was accepted")
	}
}

// Verifies: I18N-009. A file for another locale applies nothing.
func TestImportLocaleMismatch(t *testing.T) {
	t.Parallel()
	for name, doc := range map[string]struct {
		f    xchange.Format
		data string
	}{
		"arb":  {xchange.ARB, `{"@@locale":"fr","login.username":"x"}`},
		"json": {xchange.JSON, `{"kind":"translations","locale":"fr","messages":{"` + idUser + `":"x"}}`},
	} {
		_, _, err := xchange.Import([]byte(doc.data), doc.f, keys(), german(), english(), opts(true))
		if err == nil || codeOf(t, err) != plxerr.TranslationImportInvalid {
			t.Errorf("%s: err = %v, want PLX-1620", name, err)
		}
	}
}

// Verifies: I18N-009. Malformed ARB, JSON and CSV are PLX-1620.
func TestImportMalformed(t *testing.T) {
	t.Parallel()
	for name, doc := range map[string]struct {
		f    xchange.Format
		data string
	}{
		"arb array":        {xchange.ARB, `[]`},
		"arb number":       {xchange.ARB, `{"login.title":1}`},
		"arb meta":         {xchange.ARB, `{"login.title":"x","@login.title":"nope"}`},
		"arb bad state":    {xchange.ARB, `{"login.title":"x","@login.title":{"x-plux-state":"bogus"}}`},
		"arb duplicate":    {xchange.ARB, `{"login.title":"x","login.title":"y"}`},
		"arb truncated":    {xchange.ARB, `{"login.title":`},
		"json wrong kind":  {xchange.JSON, `{"kind":"pages","messages":{}}`},
		"json no messages": {xchange.JSON, `{"kind":"translations"}`},
		"json bad state":   {xchange.JSON, `{"messages":{"` + idUser + `":"x"},"states":{"` + idUser + `":"done"}}`},
		"json deep":        {xchange.JSON, strings.Repeat(`{"a":`, 100) + `1` + strings.Repeat(`}`, 100)},
		"csv empty":        {xchange.CSV, ``},
		"csv no target":    {xchange.CSV, "keyId,source\n" + idUser + ",a\n"},
		"csv no identity":  {xchange.CSV, "source,target\na,b\n"},
		"csv bad state":    {xchange.CSV, "keyId,target,state\n" + idUser + ",a,done\n"},
		"csv bad quote":    {xchange.CSV, "keyId,target\n" + idUser + ",\"a\n"},
		"csv double key":   {xchange.CSV, "keyId,target\n" + idUser + ",a\n" + idUser + ",b\n"},
		"csv double col":   {xchange.CSV, "keyId,target,target\n"},
	} {
		_, _, err := xchange.Import([]byte(doc.data), doc.f, keys(), german(), english(), opts(true))
		if err == nil || codeOf(t, err) != plxerr.TranslationImportInvalid {
			t.Errorf("%s: err = %v, want PLX-1620", name, err)
		}
	}
}

// Verifies: I18N-009. Spreadsheet formulas are neutralised in a CSV export
// and the protection is exact on import.
func TestCSVFormulaProtection(t *testing.T) {
	t.Parallel()
	texts := []string{"=1+1", "+cmd", "-5 %", "@SUM(A1)", "'quoted", "''two", "'=x", "plain", "a=b", "\tTab", "Zeile\nzwei, \"mit\" Komma"}
	target := empty("de")
	ks := make([]xchange.Key, len(texts))
	for i, text := range texts {
		id := "id-" + string(rune('a'+i))
		ks[i] = xchange.Key{ID: id, Key: "k" + string(rune('a'+i))}
		target.Messages[id] = text
	}
	data, err := xchange.Export(ks, empty("en"), target, xchange.CSV)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		for _, bad := range []string{",=", ",+", ",-5", ",@"} {
			if strings.Contains(line, bad) {
				t.Errorf("unprotected cell in %q", line)
			}
		}
	}
	got, _, err := xchange.Import(data, xchange.CSV, ks, empty("de"), empty("en"), opts(true))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Messages, target.Messages) {
		t.Fatalf("round trip: %q, want %q", got.Messages, target.Messages)
	}
}

// Verifies: I18N-009. A spreadsheet's byte order mark and column order do
// not matter.
func TestCSVTolerance(t *testing.T) {
	t.Parallel()
	data := "\xEF\xBB\xBFtarget,key,extra\nBenutzer,login.username,ignored\n"
	got, _, err := xchange.Import([]byte(data), xchange.CSV, keys(), empty("de"), english(), opts(false))
	if err != nil || got.Messages[idUser] != "Benutzer" {
		t.Fatalf("got %q, err %v", got.Messages, err)
	}
}

// Verifies: I18N-009. The ARB export carries the key, its metadata and its
// placeholders, and a flutter-style file without Plux metadata imports by key
// name.
func TestARB(t *testing.T) {
	t.Parallel()
	data, err := xchange.Export(keys(), english(), german(), xchange.ARB)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"@@locale": "de"`, `"login.title": "Willkommen zurück"`, `"@login.title": {`, `"description": "Heading of the sign-in page"`,
		`"x-plux-id": "` + idTitle + `"`, `"x-plux-state": "approved"`, `"placeholders"`, `"type": "int"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("export lacks %q:\n%s", want, data)
		}
	}
	got, _, err := xchange.Import([]byte(`{"@@locale":"de","@@last_modified":"x","login.username":"Benutzer"}`), xchange.ARB, keys(), empty("de"), english(), opts(false))
	if err != nil || got.Messages[idUser] != "Benutzer" {
		t.Fatalf("got %q, err %v", got.Messages, err)
	}
}

// Verifies: I18N-009. The JSON export is the translations document.
func TestJSONExport(t *testing.T) {
	t.Parallel()
	data, err := xchange.Export(keys(), english(), german(), xchange.JSON)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "id": "d-de",
  "kind": "translations",
  "locale": "de",
  "messages": {
    "` + idTitle + `": "Willkommen zurück",
    "` + idHint + `": "Nutzen Sie <b>Ihre</b> \"Anmeldung\" & mehr\nzweite Zeile"
  },
  "schemaVersion": "1.0.0",
  "states": {
    "` + idTitle + `": "approved",
    "` + idHint + `": "reviewed"
  }
}
`
	// encoding/json sorts the map keys; the sorted order is the contract.
	if string(data) != want {
		t.Errorf("export:\n%s\nwant:\n%s", data, want)
	}
}

// Verifies: I18N-009. Export refuses a missing locale and an unknown format.
func TestExportErrors(t *testing.T) {
	t.Parallel()
	if _, err := xchange.Export(keys(), xchange.Doc{}, german(), xchange.CSV); err == nil {
		t.Error("no source locale was accepted")
	}
	if _, err := xchange.Export(keys(), english(), german(), "odt"); err == nil {
		t.Error("an unknown format was accepted")
	}
	if _, err := xchange.ParseFormat("odt"); err == nil {
		t.Error("ParseFormat accepted odt")
	}
	if f, err := xchange.ParseFormat("xliff"); err != nil || f != xchange.XLIFF {
		t.Errorf("ParseFormat(xliff) = %v, %v", f, err)
	}
}

func seedFiles(t testing.TB, f xchange.Format) [][]byte {
	t.Helper()
	data, err := xchange.Export(keys(), english(), german(), f)
	if err != nil {
		t.Fatal(err)
	}
	return [][]byte{data}
}

// fuzzImport is the property every format must keep: an import never panics
// and never invents a key, and what it returns stays within the declared keys
// and the valid states.
func fuzzImport(t *testing.T, f xchange.Format, data []byte) {
	t.Helper()
	declared := map[string]bool{idTitle: true, idUser: true, idHint: true}
	for _, approver := range []bool{false, true} {
		before := german()
		got, _, err := xchange.Import(data, f, keys(), before, english(), xchange.Options{MaxBytes: 1 << 20, Approver: approver})
		if !reflect.DeepEqual(before, german()) {
			t.Fatal("Import modified its input")
		}
		if err != nil {
			var pe *plxerr.Error
			if !errors.As(err, &pe) {
				t.Fatalf("error without a code: %v", err)
			}
			continue
		}
		for id := range got.Messages {
			if !declared[id] {
				t.Fatalf("import invented the key %q", id)
			}
		}
		for id, s := range got.States {
			if !declared[id] || (s == "approved" && !approver && german().States[id] != "approved") {
				t.Fatalf("state %q of %q", s, id)
			}
		}
	}
}

func FuzzImportXLIFF(f *testing.F) {
	for _, s := range seedFiles(f, xchange.XLIFF) {
		f.Add(s)
	}
	f.Add([]byte(`<?xml version="1.0"?><!DOCTYPE x [<!ENTITY a "b">]><xliff/>`))
	f.Add([]byte(`<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en"><file id="a"><unit id="` + idUser + `"><segment state="final"><source>a</source><target>&amp;&lt;</target></segment></unit></file></xliff>`))
	f.Add([]byte(`<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0"><file><group><unit name="login.title"><segment><target><![CDATA[x]]></target></segment></unit></group></file></xliff>`))
	f.Fuzz(func(t *testing.T, data []byte) { fuzzImport(t, xchange.XLIFF, data) })
}

func FuzzImportCSV(f *testing.F) {
	for _, s := range seedFiles(f, xchange.CSV) {
		f.Add(s)
	}
	f.Add([]byte("keyId,target\n" + idUser + ",\"quoted, \"\"text\"\"\"\n"))
	f.Add([]byte("\xEF\xBB\xBFkey,target,state\nlogin.title,x,approved\n"))
	f.Add([]byte("target\n\"unterminated"))
	f.Fuzz(func(t *testing.T, data []byte) { fuzzImport(t, xchange.CSV, data) })
}

func FuzzImportARB(f *testing.F) {
	for _, s := range seedFiles(f, xchange.ARB) {
		f.Add(s)
	}
	f.Add([]byte(`{"@@locale":"de","login.title":"x","@login.title":{"x-plux-id":"` + idTitle + `","x-plux-state":"approved","x-plux-source":"Welcome back"}}`))
	f.Add([]byte(`{"login.username":"a","@login.username":[]}`))
	f.Add([]byte(`{"@@locale":1}`))
	f.Fuzz(func(t *testing.T, data []byte) { fuzzImport(t, xchange.ARB, data) })
}

func FuzzImportJSON(f *testing.F) {
	for _, s := range seedFiles(f, xchange.JSON) {
		f.Add(s)
	}
	f.Add([]byte(`{"kind":"translations","locale":"de","messages":{"` + idUser + `":"x"},"states":{"` + idUser + `":"approved"}}`))
	f.Fuzz(func(t *testing.T, data []byte) { fuzzImport(t, xchange.JSON, data) })
}
