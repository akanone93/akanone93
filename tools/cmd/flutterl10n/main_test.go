// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Verifies: I18N-002.
// The tool merges the ARB families per CLDR locale, drops metadata and skips
// locales CLDR lacks.
func TestDeriveMergesAndFilters(t *testing.T) {
	dir := t.TempDir()
	arb := filepath.Join(dir, "sdk", "packages", "flutter_localizations", "lib", "src", "l10n")
	write(t, filepath.Join(arb, "material_de.arb"), `{"@@x":1,"@cut":{"d":"x"},"cut":"Ausschneiden","scriptCategory":"English-like"}`)
	write(t, filepath.Join(arb, "widgets_de.arb"), `{"cut":"W"}`)
	write(t, filepath.Join(arb, "material_zh_Hant.arb"), `{"cut":"剪下"}`)
	write(t, filepath.Join(arb, "material_xx.arb"), `{"cut":"?"}`)
	write(t, filepath.Join(dir, "cldr", "index.json"), `{"locales":["de","zh-Hant"]}`)
	out := filepath.Join(dir, "out")
	var so, se bytes.Buffer
	if code := run([]string{"-sdk", filepath.Join(dir, "sdk"), "-cldr", filepath.Join(dir, "cldr"), "-out", out}, &so, &se); code != 0 {
		t.Fatalf("exit %d: %s", code, se.String())
	}
	got, err := os.ReadFile(filepath.Join(out, "de.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"material\": {\n    \"cut\": \"Ausschneiden\",\n    \"scriptCategory\": \"English-like\"\n  },\n  \"widgets\": {\n    \"cut\": \"W\"\n  }\n}\n"
	if string(got) != want {
		t.Errorf("de.json = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(out, "zh-Hant.json")); err != nil {
		t.Errorf("zh-Hant.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "xx.json")); err == nil {
		t.Error("xx has no CLDR data but was written")
	}
}

// Verifies: I18N-002.
func TestUsageAndMissingInput(t *testing.T) {
	var so, se bytes.Buffer
	if code := run(nil, &so, &se); code != 2 || !strings.Contains(se.String(), "usage") {
		t.Errorf("no flags: %d %q", code, se.String())
	}
	if code := run([]string{"-sdk", t.TempDir(), "-cldr", t.TempDir(), "-out", t.TempDir()}, &so, &se); code != 2 {
		t.Errorf("empty inputs: %d", code)
	}
}
