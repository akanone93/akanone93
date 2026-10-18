// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: I18N-001, I18N-007.
// plux l10n extract moves a literal validator message into a key and leaves
// a reference in its place.
func TestL10nExtractValidatorMessage(t *testing.T) {
	t.Parallel()
	dir := copyL10nProject(t)
	doc := readJSON(t, dir, calculator)
	doc["forms"] = []any{map[string]any{
		"id":   "01a0c450-6c00-7fff-8000-0000000009a1",
		"name": "contact",
		"fields": []any{map[string]any{
			"name": "email", "type": "string", "initial": "",
			"validators": []any{map[string]any{"kind": "required", "message": "Enter your email"}},
		}},
	}}
	if err := writeDocument(filepath.Join(dir, filepath.FromSlash(calculator)), doc); err != nil {
		t.Fatal(err)
	}
	if n := countWarnings(t, dir, plxerr.LiteralTextUntranslated); n != 1 {
		t.Fatalf("the project has %d literals, want 1", n)
	}

	code, out, stderr := cli(t, t.TempDir(), "l10n", "extract", "--dir", dir)
	if code != exitOK || !strings.Contains(out, "Extracted 1 texts from 1 documents") {
		t.Fatalf("extract: %d\n%s%s", code, out, stderr)
	}
	after := readJSON(t, dir, calculator)
	form := after["forms"].([]any)[0].(map[string]any)
	v := form["fields"].([]any)[0].(map[string]any)["validators"].([]any)[0].(map[string]any)
	ref, ok := v["message"].(map[string]any)
	if !ok || ref["$t"] == nil {
		t.Fatalf("the message is %v, want a translation reference", v["message"])
	}
	en := readJSON(t, dir, "translations/en.json")["messages"].(map[string]any)
	if en[ref["$t"].(string)] != "Enter your email" {
		t.Errorf("default message = %v", en[ref["$t"].(string)])
	}
	if n := countWarnings(t, dir, plxerr.LiteralTextUntranslated); n != 0 {
		t.Errorf("%d literals remain after extraction", n)
	}
}
