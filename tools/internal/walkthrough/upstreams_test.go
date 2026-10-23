// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"
)

// Verifies: SEC-031.
// The Plux Bank project's gateway data sources, and the base URLs the
// upstreams registered for them take, are read from the project itself.
func TestProjectSources(t *testing.T) {
	sources, values, err := projectSources(filepath.Join("..", "..", "..", "schema", "testdata", "documents", "plux_bank"))
	if err != nil {
		t.Fatal(err)
	}
	if got := slices.Sorted(maps.Keys(sources)); !slices.Equal(got, []string{"accounts", "api", "notifications", "payments", "transactions"}) {
		t.Errorf("sources = %v", got)
	}
	ops := sources["payments"].operations
	if len(ops) != 1 || ops[0]["name"] != "transfer" || ops[0]["pathPrefix"] != "/transfers" || ops[0]["transaction"] == nil {
		t.Errorf("payments operations = %v; the transfer must be registered as a transaction", ops)
	}
	if values["production"][sources["api"].baseVariable] == "" {
		t.Errorf("no production base URL for api: %v", values)
	}
}
