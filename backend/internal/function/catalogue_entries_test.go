// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"testing"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// Verifies: FN-006, STU-100.
// A catalogue lists every answer it can give, sorted, and a set built from
// the list answers the same: that is how the render worker compiles against
// the catalogue of an environment.
func TestCatalogueListsItsAnswers_FN_006(t *testing.T) {
	t.Parallel()
	c := &catalogue{
		deviceFunctions: true,
		latest:          map[string]uint32{"loans.quote": 3, "app.format": 1},
		aliases:         map[string]map[string]uint32{"loans.quote": {"stable": 2}},
		versions: map[versionKey]compiler.FunctionInfo{
			{"loans.quote", 3}: {Placement: schema.PlacementServer, Version: 3},
			{"loans.quote", 2}: {Placement: schema.PlacementServer, Version: 2},
			{"app.format", 1}:  {Placement: schema.PlacementDevice, Version: 1},
		},
	}
	got := c.FunctionEntries()
	var keys []string
	for _, e := range got {
		keys = append(keys, e.Name+"@"+e.Alias)
	}
	want := []string{"app.format@", "loans.quote@", "loans.quote@stable"}
	if len(keys) != len(want) || keys[0] != want[0] || keys[1] != want[1] || keys[2] != want[2] {
		t.Fatalf("entries = %v, want %v", keys, want)
	}
	set := compiler.NewFunctionSet(got, c.DeviceFunctionsEnabled())
	for _, e := range got {
		want, _ := c.Function(e.Name, e.Alias)
		if have, ok := set.Function(e.Name, e.Alias); !ok || have.Version != want.Version || have.Placement != want.Placement {
			t.Errorf("%s@%s = %+v, %v; want %+v", e.Name, e.Alias, have, ok, want)
		}
	}
	if _, ok := set.Function("loans.quote", "other"); ok || !set.DeviceFunctionsEnabled() {
		t.Error("the set answers for an alias the catalogue does not have, or lost deviceFunctions")
	}
}
