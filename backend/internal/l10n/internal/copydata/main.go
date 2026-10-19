// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Command copydata copies the locale files of schema/l10n/cldr into the
// package's data/ directory, where go:embed can reach them (go:generate in
// package l10n). The copy is byte for byte; files the source no longer has
// are removed.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: copydata <source dir> <destination dir>")
		os.Exit(2)
	}
	if err := copyJSON(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "copydata:", err)
		os.Exit(2)
	}
}

// copyJSON replaces the *.json files of dst with those of src.
func copyJSON(src, dst string) error {
	files, err := filepath.Glob(filepath.Join(src, "*.json"))
	if err != nil {
		return fmt.Errorf("list %s: %w", src, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("%s has no JSON file", src)
	}
	if err := os.MkdirAll(dst, 0o750); err != nil { //nolint:gosec // G703: the developer names the destination.
		return fmt.Errorf("create %s: %w", dst, err)
	}
	old, err := filepath.Glob(filepath.Join(dst, "*.json"))
	if err != nil {
		return fmt.Errorf("list %s: %w", dst, err)
	}
	for _, f := range old {
		if err := os.Remove(f); err != nil { //nolint:gosec // G703: a file the tool lists under the destination.
			return fmt.Errorf("remove %s: %w", f, err)
		}
	}
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // G304: a file the tool lists under the source.
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		if err := os.WriteFile(filepath.Join(dst, filepath.Base(f)), b, 0o600); err != nil { //nolint:gosec // G703: the developer names the destination.
			return fmt.Errorf("write %s: %w", f, err)
		}
	}
	return nil
}
