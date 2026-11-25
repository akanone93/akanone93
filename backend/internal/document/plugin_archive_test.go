// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"strings"
	"testing"
)

// A taken asset name gets the plugin's key and then a number before its
// extension; a taken key name moves under the plugin's key, numbered and
// within the longest name when that is taken too.
func TestFreeNames(t *testing.T) {
	t.Parallel()
	taken := map[string]bool{"images/logo.png": true, "images/logo-cards.png": true}
	if got := freeName("images/logo.png", "cards", taken); got != "images/logo-cards-2.png" {
		t.Errorf("freeName: %s", got)
	}
	if got := freeName("images/new.png", "cards", taken); got != "images/new.png" {
		t.Errorf("a free name: %s", got)
	}
	keys := map[string]bool{"greeting": true, "cards.greeting": true}
	if got := freeKeyName("greeting", "cards", keys); got != "cards.greeting-2" {
		t.Errorf("freeKeyName: %s", got)
	}
	long := strings.Repeat("a", 127)
	keys[long] = true
	if got := freeKeyName(long, "cards", keys); len(got) > maxKeyName || got == long || keys[got] {
		t.Errorf("a long key name: %q", got)
	}
}

// The identifiers a document names are read in one pass, any case.
func TestUUIDsIn(t *testing.T) {
	t.Parallel()
	got := map[string]bool{}
	uuidsIn([]byte(`{"$t":"01a0c450-6c00-7031-8000-000000031001","x":"01A0C450-6C00-7031-8000-00000003100F","y":"01a0c450-6c00-7031-8000-00000003100"}`), got)
	if len(got) != 2 || !got["01a0c450-6c00-7031-8000-000000031001"] || !got["01A0C450-6C00-7031-8000-00000003100F"] {
		t.Errorf("uuidsIn: %v", got)
	}
}

// Only the keys, messages, review states and assets a plugin names are
// cut from the app-level documents, with an asset's per-locale files.
func TestSharedParts(t *testing.T) {
	t.Parallel()
	const used, unused, logo, logoDE, other = "01a0c450-6c00-7031-8000-000000031001", "01a0c450-6c00-7031-8000-000000031002",
		"01a0c450-6c00-7031-8000-000000031003", "01a0c450-6c00-7031-8000-000000031004", "01a0c450-6c00-7031-8000-000000031005"
	own := []File{{Path: "plugins/a/plugin.json", Content: []byte(`{"icon":{"$asset":"` + logo + `"},"t":{"$t":"` + used + `"}}`)}}
	shared := []File{
		{Path: keysPath, Content: []byte(`{"keys":[{"id":"` + used + `","key":"a"},{"id":"` + unused + `","key":"b"}]}`)},
		{Path: "translations/de.json", Content: []byte(`{"locale":"de","messages":{"` + used + `":"A","` + unused + `":"B"},"states":{"` + unused + `":"new"}}`)},
		{Path: indexPath, Content: []byte(`{"assets":[{"id":"` + logo + `","file":"logo.png","locales":{"de":"logo-de.png"}},` +
			`{"id":"` + logoDE + `","file":"logo-de.png"},{"id":"` + other + `","file":"other.png"}]}`)},
	}
	parts, files, err := sharedParts(own, shared, 64)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, p := range parts {
		joined += p.Path + "=" + string(p.Content) + "\n"
	}
	for _, absent := range []string{unused, other, `"B"`} {
		if strings.Contains(joined, absent) {
			t.Errorf("the parts carry %s:\n%s", absent, joined)
		}
	}
	if !files["assets/logo.png"] || !files["assets/logo-de.png"] || files["assets/other.png"] || !strings.Contains(joined, logoDE) {
		t.Errorf("files %v\n%s", files, joined)
	}
}
