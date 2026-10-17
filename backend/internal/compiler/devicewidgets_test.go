// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// addWidget appends a node to the body of the gallery's material page and
// returns the JSON pointer of its type.
func addWidget(t *testing.T, m fstest.MapFS, node map[string]any) string {
	t.Helper()
	var ptr string
	edit(t, m, galleryPage, func(doc map[string]any) {
		body := at(t, doc, "root/slots/body/slots/child")
		kids, _ := body["children"].([]any)
		node["id"] = "01c0c450-6c00-7000-8a2c-0000000fe001"
		body["children"] = append(kids, node)
		ptr = "/root/slots/body/slots/child/children/" + strconv.Itoa(len(kids)) + "/type"
	})
	return ptr
}

// undeclared lists the device-API diagnostics at a pointer.
func undeclared(res *Result, ptr string) int {
	n := 0
	for _, d := range res.Diagnostics {
		if d.Code == plxerr.DeviceCapabilityUndeclared && d.File == galleryPage && d.Path == ptr {
			n++
		}
	}
	return n
}

// TestDeviceWidgetsNeedTheirCapability checks that a widget needing a
// device API is held to the plugin's declaration like an action, that a
// map needs location only when it shows the user, and that widgets of an
// optional package are recorded for the host build check.
// Verifies: SEC-080, RT-060.
func TestDeviceWidgetsNeedTheirCapability(t *testing.T) {
	t.Parallel()
	only := func(f string) bool { return f == galleryPage }
	t.Run("scanner without camera", func(t *testing.T) {
		t.Parallel()
		m := project(t, widgetsDir)
		ptr := addWidget(t, m, map[string]any{"type": "QrScanner"})
		wantDiag(t, compileFS(m), plxerr.DeviceCapabilityUndeclared, galleryPage, ptr)
	})
	t.Run("scanner with camera is recorded", func(t *testing.T) {
		t.Parallel()
		m := project(t, widgetsDir)
		ptr := addWidget(t, m, map[string]any{"type": "QrScanner"})
		apis := []any{"haptics", "files", "camera"}
		capabilities(t, m, map[string]any{"deviceApis": apis, "networkDomains": []any{"example.com"}}, map[string]any{"deviceApis": apis})
		res := compileFS(m)
		if undeclared(res, ptr) != 0 {
			t.Errorf("unexpected:\n%s", list(res.Diagnostics))
		}
		uses := DeviceUses(res, only)
		var got []DeviceUse
		for _, u := range uses {
			if u.Action == "QrScanner" {
				got = append(got, u)
			}
		}
		if len(got) != 1 || got[0].Package != "plux_scanner" || got[0].Path != ptr {
			t.Fatalf("uses = %+v", uses)
		}
		without := schema.NativeCatalogueDocument{Packages: []string{"plux_media"}}
		found := false
		for _, d := range HostBuildLacksPackages(res, only, "1.0.0+1", &without) {
			found = found || (d.Code == plxerr.HostBuildLacksPackage && d.Path == ptr)
		}
		if !found {
			t.Error("a build without plux_scanner is not warned")
		}
	})
	t.Run("map shows the user", func(t *testing.T) {
		t.Parallel()
		for name, v := range map[string]any{"literal": true, "bound": map[string]any{"$expr": "true"}} {
			m := project(t, widgetsDir)
			ptr := addWidget(t, m, map[string]any{"type": "MapView", "props": map[string]any{"showUserLocation": v}})
			if res := compileFS(m); undeclared(res, ptr) != 1 {
				t.Errorf("%s: want one PLX-1231 at %s\n%s", name, ptr, list(res.Diagnostics))
			}
		}
	})
	t.Run("map without the user", func(t *testing.T) {
		t.Parallel()
		m := project(t, widgetsDir)
		ptr := addWidget(t, m, map[string]any{"type": "MapView", "props": map[string]any{"showUserLocation": false}})
		if res := compileFS(m); undeclared(res, ptr) != 0 {
			t.Errorf("unexpected:\n%s", list(res.Diagnostics))
		}
	})
}
