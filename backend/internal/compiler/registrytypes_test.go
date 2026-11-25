// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl"
)

// Verifies: SCH-010.
// A declared type's field may be of a registry value type or enum, such
// as a LatLng (build log row 143); a state entry still may not be.
func TestDeclaredTypesUseRegistryTypes(t *testing.T) {
	t.Parallel()
	m := project(t, dbDir)
	edit(t, m, dbPlugin, func(doc map[string]any) {
		doc["types"] = raw(t, `[{"name": "AidStation", "fields": [
			{"name": "name", "type": "string"},
			{"name": "at", "type": "LatLng"},
			{"name": "markers", "type": "list<MapMarker>"},
			{"name": "align", "type": "TextAlign?"}]}]`)
		doc["state"] = raw(t, `[{"id": "01e0c450-6c00-7000-8000-0000000006f1", "name": "station", "type": "AidStation",
			"default": {"name": "Km 21", "at": {"lat": 52.5, "lng": 13.4}, "markers": [], "align": "center"}}]`)
	})
	res := compileFS(m)
	for _, d := range res.Diagnostics {
		if d.Severity == plxerr.SeverityError {
			t.Fatalf("a declared type with registry fields:\n%s", list(res.Diagnostics))
		}
	}

	bad := project(t, dbDir)
	edit(t, bad, dbPlugin, func(doc map[string]any) {
		doc["types"] = raw(t, `[{"name": "Device", "fields": [{"name": "info", "type": "PluxDevice"}]}]`)
	})
	wantDiag(t, compileFS(bad), plxerr.UnknownType, dbPlugin, "/types/0/fields/0/type")
}

// Inside a declared object the runtime reads data: a registry value type
// is an object keyed by field name, an optional field may be left out,
// and a registry enum is its member's name, as a declared one's.
func TestRegistryValuesInDataAreByName(t *testing.T) {
	t.Parallel()
	u := &unit{envs: map[string]*pxl.Env{}, opts: DefaultOptions()}
	u.types = &universe{base: baseTypes(), app: map[string]pxl.TypeSpec{}, plugin: map[*plugin]map[string]pxl.TypeSpec{}}
	data := vctx{data: true, code: plxerr.ValueTypeMismatch}
	v := u.check(data, map[string]any{"lat": json.Number("52.5"), "lng": json.Number("13.4")}, &texpr{name: "LatLng"})
	if v == nil || v.kind != fbs.ValueKindObject || len(v.entries) != 2 || v.entries[0].key != "lat" || v.entries[1].key != "lng" {
		t.Fatalf("a LatLng in data: %+v, %v", v, u.diags)
	}
	marker := u.check(data, map[string]any{"id": "a", "position": map[string]any{"lat": json.Number("1"), "lng": json.Number("2")}}, &texpr{name: "MapMarker"})
	if marker == nil || len(marker.entries) != 2 {
		t.Fatalf("a MapMarker without its optional fields: %+v, %v", marker, u.diags)
	}
	align := u.check(data, "center", &texpr{name: "TextAlign"})
	if align == nil || align.kind != fbs.ValueKindEnum || align.s != "center" {
		t.Fatalf("a TextAlign in data: %+v", align)
	}
}
