// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
)

// copyL10nProject copies the conformance project into a temporary directory.
func copyL10nProject(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(projectDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(projectDir, p)
		data, err := os.ReadFile(p) //nolint:gosec // G304: the repository's fixture.
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // G301: a temporary project.
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// readJSON reads a document of a project.
func readJSON(t *testing.T, dir, rel string) map[string]any {
	t.Helper()
	doc, err := readDocument(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// calculator is the page the tests add literal text to.
const calculator = "plugins/loans/pages/calculator.page.json"

// addLiterals appends two Text nodes with literal text to the column of the
// calculator page, one with a nested decoration.
func addLiterals(t *testing.T, dir string) {
	t.Helper()
	doc := readJSON(t, dir, calculator)
	col := doc["root"].(map[string]any)["slots"].(map[string]any)["body"].(map[string]any)["slots"].(map[string]any)["child"].(map[string]any)
	col["children"] = append(col["children"].([]any),
		map[string]any{
			"id": "01a0c450-6c00-7fff-8000-0000000004a1", "type": "Text", "testId": "greeting",
			"props": map[string]any{"data": "Don't {panic}", "semanticsLabel": "Greeting"},
		},
		map[string]any{
			"id": "01a0c450-6c00-7fff-8000-0000000004a2", "type": "TextFormField",
			"props": map[string]any{"decoration": map[string]any{"labelText": "Your name", "hintText": "Ada"}},
		})
	if err := writeDocument(filepath.Join(dir, filepath.FromSlash(calculator)), doc); err != nil {
		t.Fatal(err)
	}
}

// Verifies: I18N-007.
// plux l10n extract moves literal text into new keys, is idempotent, and
// leaves a project that compiles without PLX-1609.
func TestL10nExtract(t *testing.T) {
	t.Parallel()
	dir := copyL10nProject(t)
	addLiterals(t, dir)
	before, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(calculator))) //nolint:gosec // G304: a temporary project.
	if n := countWarnings(t, dir, plxerr.LiteralTextUntranslated); n != 4 {
		t.Fatalf("the test project has %d literals, want 4", n)
	}

	code, out, stderr := cli(t, t.TempDir(), "l10n", "extract", "--dir", dir, "--dry-run")
	if code != exitOK || !strings.Contains(out, "Would extract 4 texts from 1 documents") {
		t.Fatalf("dry run: %d\n%s%s", code, out, stderr)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(calculator))); string(after) != string(before) { //nolint:gosec // G304: a temporary project.
		t.Fatal("a dry run changed the page")
	}

	code, out, stderr = cli(t, t.TempDir(), "l10n", "extract", "--dir", dir)
	if code != exitOK || !strings.Contains(out, "Extracted 4 texts from 1 documents") ||
		!strings.Contains(out, "loans.calculator.greeting-data") {
		t.Fatalf("extract: %d\n%s%s", code, out, stderr)
	}

	keys := readJSON(t, dir, keysDocument)["keys"].([]any)
	if len(keys) != 7+4 {
		t.Fatalf("keys.json holds %d keys, want 11", len(keys))
	}
	en := readJSON(t, dir, "translations/en.json")["messages"].(map[string]any)
	byName := map[string]string{}
	for _, k := range keys[7:] {
		m := k.(map[string]any)
		id := m["id"].(string)
		u, err := uuid7.Parse(id)
		if err != nil || u.Version() != 7 {
			t.Errorf("key %v has no UUIDv7: %v", m["key"], err)
		}
		byName[m["key"].(string)], _ = en[id].(string)
	}
	want := map[string]string{
		"loans.calculator.greeting-data":           "Don''t '{'panic'}'",
		"loans.calculator.greeting-semanticslabel": "Greeting",
		"loans.calculator.your-name":               "Your name",
		"loans.calculator.ada":                     "Ada",
	}
	for name, text := range want {
		got, ok := byName[name]
		if !ok {
			t.Errorf("no key %s in %v", name, byName)
		}
		if got != text {
			t.Errorf("message of %s = %q, want %q", name, got, text)
		}
	}

	if n := countWarnings(t, dir, plxerr.LiteralTextUntranslated); n != 0 {
		t.Errorf("%d literals remain after extraction", n)
	}
	res := compiler.Compile(os.DirFS(dir), compiler.DefaultOptions())
	if res.Diagnostics.HasErrors() {
		t.Fatalf("the extracted project does not compile:\n%v", res.Diagnostics)
	}

	snapshot := map[string][]byte{}
	for _, rel := range []string{calculator, keysDocument, "translations/en.json"} {
		snapshot[rel], _ = os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))) //nolint:gosec // G304: a temporary project.
	}
	code, out, _ = cli(t, t.TempDir(), "l10n", "extract", "--dir", dir)
	if code != exitOK || !strings.Contains(out, "No literal text") {
		t.Errorf("second run: %d %q", code, out)
	}
	for rel, was := range snapshot {
		if now, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))); string(now) != string(was) { //nolint:gosec // G304: a temporary project.
			t.Errorf("the second run changed %s", rel)
		}
	}
}

// countWarnings compiles the project in dir and counts diagnostics with a
// code.
func countWarnings(t *testing.T, dir string, code plxerr.Code) int {
	t.Helper()
	res := compiler.Compile(os.DirFS(dir), compiler.DefaultOptions())
	n := 0
	for _, d := range res.Diagnostics {
		if d.Code == code {
			n++
		}
	}
	return n
}

// Verifies: I18N-007.
// plux l10n extract --json lists what it moved.
func TestL10nExtractJSON(t *testing.T) {
	t.Parallel()
	dir := copyL10nProject(t)
	addLiterals(t, dir)
	code, out, _ := cli(t, t.TempDir(), "l10n", "extract", "--dir", dir, "--dry-run", "--json")
	var got struct {
		DryRun    bool        `json:"dryRun"`
		Extracted []extracted `json:"extracted"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != exitOK || !got.DryRun || len(got.Extracted) != 4 {
		t.Fatalf("%d %v\n%s", code, err, out)
	}
	if got.Extracted[0].File != calculator || !strings.Contains(got.Extracted[0].Path, "/props/") {
		t.Errorf("first = %+v", got.Extracted[0])
	}
}

// Verifies: I18N-002, I18N-010.
// plux l10n pseudo writes the two pseudo-locale documents, keeps their
// identifiers, and the project still compiles with them.
func TestL10nPseudo(t *testing.T) {
	t.Parallel()
	dir := copyL10nProject(t)
	code, out, stderr := cli(t, t.TempDir(), "l10n", "pseudo", "--dir", dir)
	if code != exitOK || !strings.Contains(out, "Wrote translations/en-XA.json (7 messages).") ||
		!strings.Contains(out, "Wrote translations/ar-XB.json (7 messages).") {
		t.Fatalf("pseudo: %d\n%s%s", code, out, stderr)
	}
	const title = "01a0c450-6c00-7002-8000-000000003dde"
	xa := readJSON(t, dir, "translations/en-XA.json")
	if xa["locale"] != "en-XA" || xa["kind"] != "translations" {
		t.Errorf("en-XA = %v", xa)
	}
	if got := xa["messages"].(map[string]any)[title]; !strings.HasPrefix(got.(string), "[") || strings.Contains(got.(string), "Kredit") {
		t.Errorf("en-XA title = %q", got)
	}
	xb := readJSON(t, dir, "translations/ar-XB.json")
	if got := xb["messages"].(map[string]any)[title].(string); !strings.HasPrefix(got, "‮") || !strings.HasSuffix(got, "‬") {
		t.Errorf("ar-XB title = %q", got)
	}
	first, _ := os.ReadFile(filepath.Join(dir, "translations", "en-XA.json")) //nolint:gosec // G304: a temporary project.
	if code, _, _ := cli(t, t.TempDir(), "l10n", "pseudo", "-C", dir); code != exitOK {
		t.Fatal("second run failed")
	}
	if again, _ := os.ReadFile(filepath.Join(dir, "translations", "en-XA.json")); string(again) != string(first) { //nolint:gosec // G304: a temporary project.
		t.Error("the second run changed en-XA.json")
	}
	res := compiler.Compile(os.DirFS(dir), compiler.DefaultOptions())
	if res.Diagnostics.HasErrors() {
		t.Errorf("the project with pseudo-locale documents does not compile:\n%v", res.Diagnostics)
	}
}

// Verifies: I18N-002.
func TestL10nLocalCommandsReportUsageAndMissingProjects(t *testing.T) {
	t.Parallel()
	if code, _, _ := cli(t, t.TempDir(), "l10n", "pseudo", "extra"); code != exitUsage {
		t.Errorf("a stray argument: %d, want usage", code)
	}
	empty := t.TempDir()
	if code, _, _ := cli(t, t.TempDir(), "l10n", "extract", "--dir", empty); code == exitOK {
		t.Error("a directory without app.json must fail")
	}
	if code, _, _ := cli(t, t.TempDir(), "l10n", "nonsense"); code != exitUsage {
		t.Error("an unknown subcommand must be a usage error")
	}
}
