// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDeviceWidgetsFollowTheDescriptors keeps the optional-package table of
// the host build check in step with the descriptors: a widget whose
// descriptor names a package is in the table with that package, and the
// table names no widget a descriptor does not (RT-060, PLX-1233).
// Verifies: RT-060.
func TestDeviceWidgetsFollowTheDescriptors(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "schema", "widgets", "layer*", "*.json"))
	if err != nil || len(files) < 100 {
		t.Fatalf("descriptors: %d files, %v", len(files), err)
	}
	named := map[string]string{}
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // G304: the repository's descriptors.
		if err != nil {
			t.Fatal(err)
		}
		var d struct {
			Type    string `json:"type"`
			Package string `json:"package"`
		}
		if err := json.Unmarshal(data, &d); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if d.Package != "" {
			named[d.Type] = d.Package
		}
	}
	for typ, pkg := range named {
		if use, ok := deviceWidgets[typ]; !ok || use.pkg != pkg {
			t.Errorf("%s: the descriptor names %s, the table has %+v", typ, pkg, use)
		}
	}
	for typ, use := range deviceWidgets {
		// KycCameraCapture is provided by plux_media, which its descriptor
		// does not say (the runtime renders the widget's refusal itself).
		if typ == "KycCameraCapture" {
			continue
		}
		if named[typ] != use.pkg {
			t.Errorf("%s: the table says %q, the descriptor %q", typ, use.pkg, named[typ])
		}
	}
}
