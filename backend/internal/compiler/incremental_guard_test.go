// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"reflect"
	"slices"
	"testing"
)

// unitFields classifies every field of the compilation unit for incremental
// compilation (CMP-051). Recompile checks and encodes only the pages it is
// given; a field that many pages write and a bundle reads would make the
// bundle depend on pages Recompile did not check, so each field says how it
// is covered:
//
//   - fact: an aggregate over documents that reaches a bundle; the facts
//     type carries it from the Memo, and Recompile declines when the
//     recompiled pages stop contributing to it;
//   - index: built by resolve from all documents on every run, so it is as
//     complete in Recompile as in Compile;
//   - plugin level: written and read by stages that run on the plugin or app
//     documents, which Recompile requires to be unchanged;
//   - report: written by checks and read only to build Result reports, which
//     Recompile does not return;
//   - memo: Recompile's own state;
//   - per page: the page's products, taken from the Memo for a page not in
//     focus.
var unitFields = map[string]string{
	"calendars": "fact", "use": "fact", "features": "fact", "icons": "fact",
	"gateway": "report", "opts": "plugin level", "project": "plugin level", "diags": "plugin level",
	"a11y": "report", "l10n": "report", "focus": "memo", "validating": "memo", "memo": "memo", "next": "memo",
	"sensitiveByOwner": "per page",
	"ids":              "index", "plugins": "index", "pages": "index", "routes": "index", "hostEvents": "index", "tabValues": "index",
	"graphs": "index", "components": "index", "shared": "index", "appGraphs": "index", "appTriggers": "index", "tkeys": "index",
	"tokens": "index", "assetIDs": "index", "natives": "index", "experiments": "index", "segments": "index", "types": "index",
	"graph":      "index",
	"deviceUses": "report", "stored": "report", "collections": "report",
	"files":    "plugin level",
	"appScope": "plugin level", "appState": "plugin level", "appSources": "plugin level", "envs": "plugin level", "exprs": "per page",
	"dataConfigs": "plugin level", "fnSteps": "per page",
	"appOut": "plugin level", "pluginOuts": "plugin level", "app": "plugin level", "outputs": "plugin level",
}

// TestUnitFieldsAreClassified fails when a field is added to unit without
// deciding how Recompile covers it: the cost of forgetting is a bundle that
// differs from Compile's only after some edit (CMP-051, CMP-002).
func TestUnitFieldsAreClassified(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeFor[unit]()
	var have []string
	for i := range typ.NumField() {
		have = append(have, typ.Field(i).Name)
	}
	want := make([]string, 0, len(unitFields))
	for name := range unitFields {
		want = append(want, name)
	}
	slices.Sort(have)
	slices.Sort(want)
	for _, name := range have {
		if _, ok := unitFields[name]; !ok {
			t.Errorf("unit.%s is not classified for incremental compilation: add it to unitFields, and to facts if many pages write it and a bundle reads it", name)
		}
	}
	for _, name := range want {
		if !slices.Contains(have, name) {
			t.Errorf("unitFields names %s, which unit no longer has", name)
		}
	}
	for name, kind := range unitFields {
		if kind == "fact" && !slices.Contains(factFields, name) {
			t.Errorf("unit.%s is a fact but facts does not carry it", name)
		}
	}
}
