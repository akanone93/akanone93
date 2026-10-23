// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package incremental

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/schema"
)

// stateEdit changes the state of the calculator page of the fixture,
// compiles it and returns the verdict of the page (DEV-010).
func stateEdit(t *testing.T, c *Cache, m fstest.MapFS, f func(state []any) []any) PageVerdict {
	t.Helper()
	edit(t, m, calculatorPage, func(doc map[string]any) { doc["state"] = f(doc["state"].([]any)) })
	res := incr(t, c, m)
	if res.App == nil {
		t.Fatalf("the edit does not compile: %v", diagLines(res.Diagnostics))
	}
	mustMatch(t, res, full(m))
	for _, v := range res.Pages {
		if v.File == calculatorPage {
			return v
		}
	}
	t.Fatalf("no verdict for the calculator page in %+v", res.Pages)
	return PageVerdict{}
}

// TestVerdictStateSchemaUnchanged checks that a page whose state schema is
// unchanged is compatible, and a changed default is not a change of schema
// (DEV-010, CMP-051).
func TestVerdictStateSchemaUnchanged(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	c, _ := newCached(t, m)
	setMax(t, m, calculatorPage)
	res := incr(t, c, m)
	if len(res.Pages) != 1 || !res.Pages[0].Compatible || len(res.Pages[0].Incompatible) != 0 {
		t.Fatalf("verdicts %+v", res.Pages)
	}
	v := stateEdit(t, c, m, func(state []any) []any {
		state[1].(map[string]any)["default"] = 24
		return state
	})
	if !v.Compatible {
		t.Errorf("a changed default is incompatible: %v", v.Incompatible)
	}
}

// TestVerdictScopeChangedOrRemoved checks that a scope whose declaration
// changed, or that was removed, is listed as incompatible and the others
// are kept (DEV-010).
func TestVerdictScopeChangedOrRemoved(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	c, _ := newCached(t, m)
	note := map[string]any{"id": "01d0ffff-6c00-7000-8000-000000000001", "name": "note", "type": "string", "default": ""}
	v := stateEdit(t, c, m, func(state []any) []any { return append(state, note) })
	if !v.Compatible || len(v.Incompatible) != 0 {
		t.Errorf("a new scope is incompatible: %v", v.Incompatible)
	}
	v = stateEdit(t, c, m, func(state []any) []any {
		state[3].(map[string]any)["persistence"] = "session"
		state[1].(map[string]any)["persistence"] = "session"
		return state
	})
	if v.Compatible || !slices.Equal(v.Incompatible, []string{"page.months", "page.note"}) {
		t.Errorf("changed persistence: %+v", v)
	}
	v = stateEdit(t, c, m, func(state []any) []any {
		state[3].(map[string]any)["type"] = "int"
		state[3].(map[string]any)["default"] = 0
		return state
	})
	if v.Compatible || !slices.Equal(v.Incompatible, []string{"page.note"}) {
		t.Errorf("changed type: %+v", v)
	}
	v = stateEdit(t, c, m, func(state []any) []any { return state[:3] })
	if v.Compatible || !slices.Equal(v.Incompatible, []string{"page.note"}) {
		t.Errorf("removed scope: %+v", v)
	}
}

// TestVerdictOf covers the verdict's rules directly: renames, sensitivity
// and computed state change a scope, defaults and descriptions do not.
func TestVerdictOf(t *testing.T) {
	t.Parallel()
	yes := true
	entry := func(id, name, typ string) schema.StateEntry {
		return schema.StateEntry{ID: id, Name: name, Type: typ}
	}
	base := &schema.PageDocument{ID: "p", Key: "k", State: []schema.StateEntry{entry("a", "a", "int"), entry("b", "b", "int"), entry("c", "c", "int"), entry("d", "d", "int")}}
	now := &schema.PageDocument{ID: "p", Key: "k", State: []schema.StateEntry{
		entry("a", "renamed", "int"),
		{ID: "b", Name: "b", Type: "int", Sensitive: &yes},
		{ID: "c", Name: "c", Type: "int", Computed: &schema.Expr{}},
		{ID: "d", Name: "d", Type: "int", Description: "documented", Default: []byte("3")},
		entry("e", "fresh", "int"),
	}}
	v := verdictOf(base, now, "f.page.json")
	if v.Compatible || !slices.Equal(v.Incompatible, []string{"page.b", "page.c", "page.renamed"}) {
		t.Errorf("verdict %+v", v)
	}
	if v := verdictOf(nil, now, "f.page.json"); !v.Compatible {
		t.Errorf("a page without an earlier version has no state to lose: %+v", v)
	}
}
