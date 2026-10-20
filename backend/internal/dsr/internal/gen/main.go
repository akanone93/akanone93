// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Command gen writes docs/operations/personal-data.md from the
// personal-data map of package dsr (go:generate in package dsr; SEC-162). The
// map is the single source: nothing else lists where personal data lives.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nightCode42/plux3/backend/internal/dsr"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen <output.md>")
		os.Exit(2)
	}
	if err := write(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "dsr gen:", err)
		os.Exit(1)
	}
}

// write renders the map into path, creating its directory.
func write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(dsr.Render()), 0o644); err != nil { //nolint:gosec // a committed document, not a secret
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
