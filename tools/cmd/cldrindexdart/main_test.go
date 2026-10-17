// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

// Verifies: I18N-003.
func TestRender(t *testing.T) {
	out, err := render([]byte(`{"parentLocales":{"b":"und","a":"en"},"likelyScripts":{"zh-TW":"Hant"}}`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Index(s, "'a': 'en'") > strings.Index(s, "'b': 'und'") {
		t.Fatal("keys are not sorted")
	}
	if !strings.Contains(s, "'zh-TW': 'Hant'") || !strings.Contains(s, "cldrLocaleIndex") {
		t.Fatalf("missing content:\n%s", s)
	}
}

// Verifies: I18N-003, CI-003.
func TestCommittedFileIsCurrent(t *testing.T) {
	in, err := os.ReadFile("../../../schema/l10n/cldr/index.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := render(in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../../packages/plux_flutter/lib/src/l10n/cldr_index.g.dart")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("cldr_index.g.dart is stale; run make gen")
	}
}
