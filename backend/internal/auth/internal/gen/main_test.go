// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// generate runs the generator into a fresh directory and returns the
// TypeScript it wrote.
func generate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ts := filepath.Join(dir, "permissions.gen.ts")
	err := run("../../permissions.json", "../../roles.json", filepath.Join(dir, "c.go"), filepath.Join(dir, "p.md"), ts)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	b, err := os.ReadFile(ts) //nolint:gosec // G304: a path under t.TempDir.
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}

// TestCatalogueTypeScriptDeterministic checks that the TypeScript catalogue
// for Studio is byte-identical across runs and carries the catalogue (GOV-002).
func TestCatalogueTypeScriptDeterministic(t *testing.T) {
	first, second := generate(t), generate(t)
	if first != second {
		t.Fatal("two runs wrote different TypeScript")
	}
	for _, want := range []string{
		"DO NOT EDIT.", "export type PermissionName =", `| "a11y.waive"`,
		"export const permissions = [", "export const roles = [", "as const satisfies readonly PermissionDefinition[];",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}
