// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: PXL-001, PXL-002.
// A prop bound to an expression types its literals against the prop: map
// markers built from a list, a centre written as a map literal and a
// timeline step's state chosen between two enum members compile; a centre
// missing a field is refused where it is written.
func TestPropsTypeLiteralsByContext(t *testing.T) {
	t.Parallel()
	ok := `[{"id": "%ID%", "type": "MapView", "props": {
		"semanticLabel": "Places", "center": {"$expr": "{\"lat\": 52.5, \"lng\": 13.4}"},
		"markers": {"$expr": "[1.0, 2.0].map(x, {\"id\": string(x), \"position\": {\"lat\": x, \"lng\": x}, \"title\": null})"}}},
	  {"id": "%ID%", "type": "StatusTimeline", "props": {"steps": [
		{"label": "Sent", "state": "done"},
		{"label": "Review", "state": {"$expr": "size([1]) > 0 ? \"current\" : \"done\""}}]}}]`
	project := func(kids string) fstest.MapFS {
		m := a11yProject(t, "standard", "#000000", "#FFFFFF", kids)
		edit(t, m, "app.json", func(doc map[string]any) { doc["minRuntimeVersion"] = "0.6.0" })
		return m
	}
	res := compileFS(project(ok))
	if res.Diagnostics.HasErrors() {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	bad := `[{"id": "%ID%", "type": "MapView", "props": {"semanticLabel": "Places", "center": {"$expr": "{\"lat\": 52.5}"}}}]`
	res = compileFS(project(bad))
	found := false
	for _, d := range res.Diagnostics {
		found = found || d.Code == plxerr.PXLTypeMismatch
	}
	if !found {
		t.Errorf("a centre without lng: %s", list(res.Diagnostics))
	}
}
