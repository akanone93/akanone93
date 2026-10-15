// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestReadProjectLeavesOutScenarios checks that the files a publish or an
// import sends are the project's documents: plux.json, hidden files, Plux
// Test scenarios and the crates of functions stay local.
// Verifies: TST-001, FN-016.
func TestReadProjectLeavesOutScenarios(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, f := range []string{
		"app.json", "plux.json", ".hidden/x.json",
		"plugins/p/plugin.json",
		"tests/home.scenario.yaml", "tests/deep/flow.scenario.json", "plugins/p/tests/page.scenario.yaml",
		"functions/bank.fee/function.json", "functions/bank.fee/Cargo.toml", "functions/bank.fee/src/lib.rs",
		"functions/bank.fee/target/plux/module.wasm", "functions/bank.fee/target/plux/input.schema.json",
	} {
		path := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		putFile(t, path, "{}")
	}
	files, err := readProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := slices.Sorted(maps.Keys(files))
	if want := []string{"app.json", "functions/bank.fee/function.json", "plugins/p/plugin.json"}; !slices.Equal(got, want) {
		t.Errorf("readProject = %v, want %v", got, want)
	}
}
