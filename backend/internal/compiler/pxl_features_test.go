// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestPxlFeatureRuntimesCoverEveryGroup_BND_008 checks that every PXL
// standard-library group beyond core names the runtime it first shipped in,
// so that an expression using it under an older minRuntimeVersion is a
// problem in Studio (PLX-1119) or raises the plugin's required features
// (PLX-1120). The format, l10n and calendar groups had no entry, so a
// draft using them compiled clean and the phone refused it (build log #257).
// Verifies: BND-008.
func TestPxlFeatureRuntimesCoverEveryGroup_BND_008(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", "pxl", "stdlib.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lib struct {
		Groups []struct {
			Name    string `json:"name"`
			Feature string `json:"feature"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(raw, &lib); err != nil {
		t.Fatal(err)
	}
	if len(lib.Groups) == 0 {
		t.Fatal("the standard library lists no groups")
	}
	for _, g := range lib.Groups {
		if g.Name == "core" {
			continue
		}
		if runtimes := pxlFeatureRuntimes[g.Feature]; len(runtimes) == 0 {
			t.Errorf("group %s (%s) names no runtime in pxlFeatureRuntimes", g.Name, g.Feature)
		}
	}
}
