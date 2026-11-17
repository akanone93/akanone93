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

// Verifies: WGT-001.
// An optional widget prop bound to an expression may be null, which leaves
// it unset (a field's forced error while there is none); a required prop
// may not.
func TestOptionalPropsTakeNull(t *testing.T) {
	t.Parallel()
	field := `[{"id": "%ID%", "type": "TextFormField", "props": {"forceErrorText": {"$expr": "size([1]) > 1 ? \"Wrong\" : null"}},
		"semantics": {"label": "Name"}}]`
	res := compileFS(a11yProject(t, "standard", "#000000", "#FFFFFF", field))
	if res.Diagnostics.HasErrors() {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	text := `[{"id": "%ID%", "type": "Text", "props": {"data": {"$expr": "size([1]) > 1 ? \"Hello\" : null"}}}]`
	res = compileFS(a11yProject(t, "standard", "#000000", "#FFFFFF", text))
	found := false
	for _, d := range res.Diagnostics {
		found = found || d.Code == plxerr.PropTypeMismatch
	}
	if !found {
		t.Errorf("a required prop bound to a nullable expression:\n%s", list(res.Diagnostics))
	}
}

// Verifies: THM-005.
// An icon chosen by an expression is admitted when the plugin declares the
// icons its expressions may choose; their glyphs join the plugin's font.
// Without the declaration, or with an icon its set lacks, it is refused.
func TestDynamicIcons(t *testing.T) {
	t.Parallel()
	node := `[{"id": "%ID%", "type": "Icon", "props": {"icon": {"$expr": "device.darkMode ? {\"name\": \"school\"} : {\"name\": \"home\"}"},
		"semanticLabel": "Kind"}}]`
	declare := func(icons ...string) func(map[string]any) {
		return func(doc map[string]any) {
			list := make([]any, len(icons))
			for i, name := range icons {
				list[i] = map[string]any{"name": name}
			}
			doc["dynamicIcons"] = list
		}
	}
	m := a11yProject(t, "standard", "#000000", "#FFFFFF", node)
	res := compileFS(m)
	if !anyDiag(res, plxerr.UnknownIcon) {
		t.Errorf("an undeclared computed icon:\n%s", list(res.Diagnostics))
	}
	m = a11yProject(t, "standard", "#000000", "#FFFFFF", node)
	edit(t, m, "plugins/loans/plugin.json", declare("school", "home"))
	res = compileFS(m)
	if res.Diagnostics.HasErrors() {
		t.Fatalf("declared computed icons:\n%s", list(res.Diagnostics))
	}
	m = a11yProject(t, "standard", "#000000", "#FFFFFF", node)
	edit(t, m, "plugins/loans/plugin.json", declare("school", "no_such_icon_at_all"))
	if !anyDiag(compileFS(m), plxerr.UnknownIcon) {
		t.Error("a declared icon its set lacks is refused")
	}
}

func anyDiag(res *Result, code plxerr.Code) bool {
	for _, d := range res.Diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}
