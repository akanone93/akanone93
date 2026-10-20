// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Command gen writes docs/operations/egress.md from the registry of
// destinations (go:generate in package httpx; SEC-163).
package main

import (
	"fmt"
	"os"

	"github.com/nightCode42/plux3/backend/internal/httpx"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen <egress.md>")
		os.Exit(2)
	}
	if err := os.WriteFile(os.Args[1], []byte(httpx.RenderEgressList()), 0o644); err != nil { //nolint:gosec // a committed document
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(2)
	}
}
