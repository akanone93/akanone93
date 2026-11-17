// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// dbDir is the conformance project of the local database, whose bundles
// the runtime's database tests run.
var dbDir = filepath.Join("..", "..", "..", "schema", "testdata", "documents", "db")

const dbPlugin = "plugins/todo/plugin.json"

// TestDBGoldenBundles pins the bundles of the database project byte for
// byte and checks that they need db.v1, first in runtime 0.3.0, and carry
// the collections with their versions and migrations.
// Verifies: CMP-002, QA-003, DB-004, DB-005, DB-006, BND-008.
func TestDBGoldenBundles(t *testing.T) {
	t.Parallel()
	res := Compile(os.DirFS(dbDir), DefaultOptions())
	onlyRaised(t, res)
	readAll(t, res)
	for _, b := range append([]*Bundle{res.App}, res.Plugins...) {
		checkGolden(t, filepath.Join(goldenRoot, "db", b.Key+".pxb"), b.Data)
		if !slices.Contains(b.Features, "db.v1") {
			t.Errorf("bundle %s features %v lack db.v1", b.Key, b.Features)
		}
	}
	todo, err := StoredCollections(res.Plugins[0].Data)
	if err != nil || len(todo.Collections) != 1 {
		t.Fatalf("plugin collections %+v, %v", todo, err)
	}
	c := todo.Collections[0]
	if c.Key != "tasks" || c.Version != 2 || len(c.Fields) != 5 || len(c.Indexes) != 2 ||
		len(c.Migrations) != 1 || c.Migrations[0].Rename["title"] != "name" {
		t.Errorf("tasks %+v", c)
	}
	shared, err := StoredCollections(res.App.Data)
	if err != nil || len(shared.Collections) != 1 || shared.Collections[0].Key != "labels" || shared.Collections[0].Version != 1 {
		t.Errorf("app collections %+v, %v", shared, err)
	}
}

// TestDatabaseSourceChecks checks the configuration of watched queries.
// Verifies: DB-006.
func TestDatabaseSourceChecks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		config map[string]any
		ptr    string
	}{
		{"unknown collection", map[string]any{"collection": "nope"}, "/dataSources/0/config/collection"},
		{"unknown field in a filter", map[string]any{"collection": "tasks", "filter": map[string]any{"field": "nope", "op": "eq", "value": 1}}, "/dataSources/0/config/filter/field"},
		{"operator that does not apply", map[string]any{"collection": "tasks", "filter": map[string]any{"field": "done", "op": "contains", "value": "x"}}, "/dataSources/0/config/filter/op"},
		{"value of the wrong type", map[string]any{"collection": "tasks", "filter": map[string]any{"field": "rank", "op": "gt", "value": "high"}}, "/dataSources/0/config/filter/value"},
		{"in without a list", map[string]any{"collection": "tasks", "filter": map[string]any{"field": "rank", "op": "in", "value": 3}}, "/dataSources/0/config/filter/value"},
		{"nested value of the wrong type", map[string]any{"collection": "tasks", "filter": map[string]any{"and": []any{map[string]any{"field": "done", "op": "eq", "value": 1}}}}, "/dataSources/0/config/filter/and/0/value"},
		{"sort by an unknown field", map[string]any{"collection": "tasks", "orderBy": "nope"}, "/dataSources/0/config/orderBy"},
		{"descending without a sort", map[string]any{"collection": "tasks", "descending": true}, "/dataSources/0/config/descending"},
		{"limit over db.queryRows", map[string]any{"collection": "tasks", "limit": 100000000}, "/dataSources/0/config/limit"},
		{"negative offset", map[string]any{"collection": "tasks", "offset": -1}, "/dataSources/0/config/offset"},
		{"unknown property", map[string]any{"collection": "tasks", "extra": true}, "/dataSources/0/config"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			m := project(t, dbDir)
			edit(t, m, dbPlugin, func(doc map[string]any) {
				doc["dataSources"].([]any)[0].(map[string]any)["config"] = c.config
			})
			wantDiag(t, compileFS(m), plxerr.DatabaseSourceInvalid, dbPlugin, c.ptr)
		})
	}
	t.Run("a source that is not a list", func(t *testing.T) {
		t.Parallel()
		m := project(t, dbDir)
		edit(t, m, dbPlugin, func(doc map[string]any) {
			doc["dataSources"].([]any)[0].(map[string]any)["type"] = "Task"
			doc["dataSources"].([]any)[0].(map[string]any)["mock"] = map[string]any{"id": "a", "title": "t", "done": false, "rank": 1, "note": nil}
		})
		wantDiag(t, compileFS(m), plxerr.DatabaseSourceInvalid, dbPlugin, "/dataSources/0/type")
	})
}

// TestDatabaseSourceParams binds a filter's comparison to one of the
// source's params, a literal or a binding of the declaring scope, and
// requires db.v2 for it.
// Verifies: DB-006, BND-008.
func TestDatabaseSourceParams(t *testing.T) {
	t.Parallel()
	bound := func(params map[string]any, filter map[string]any) func(map[string]any) {
		return func(doc map[string]any) {
			doc["dataSources"].([]any)[0].(map[string]any)["config"] = map[string]any{
				"collection": "tasks", "params": params, "filter": filter,
			}
		}
	}
	for name, params := range map[string]map[string]any{
		"a literal param": {"want": "a"},
		"a bound param":   {"want": map[string]any{"$expr": `"a" + "b"`}},
		"a list for in":   {"want": map[string]any{"$expr": `["a", "b"]`}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := project(t, dbDir)
			op := "eq"
			if name == "a list for in" {
				op = "in"
			}
			edit(t, m, dbPlugin, bound(params, map[string]any{"field": "id", "op": op, "param": "want"}))
			res := compileFS(m)
			for _, d := range res.Diagnostics {
				if d.Severity == plxerr.SeverityError {
					t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
				}
			}
			if !slices.Contains(res.Plugins[0].Features, "db.v2") {
				t.Errorf("features %v lack db.v2", res.Plugins[0].Features)
			}
		})
	}
	for _, c := range []struct {
		name   string
		params map[string]any
		filter map[string]any
		ptr    string
	}{
		{"an undeclared param", map[string]any{"want": "a"}, map[string]any{"field": "id", "op": "eq", "param": "other"}, "/param"},
		{"a param of another type", map[string]any{"want": 3}, map[string]any{"field": "id", "op": "eq", "param": "want"}, "/param"},
		{"a single value for in", map[string]any{"want": "a"}, map[string]any{"field": "id", "op": "in", "param": "want"}, "/param"},
		{"both a value and a param", map[string]any{"want": "a"}, map[string]any{"field": "id", "op": "eq", "param": "want", "value": "a"}, "/param"},
		{"a null test with a param", map[string]any{"want": "a"}, map[string]any{"field": "note", "op": "isNull", "param": "want"}, "/op"},
		{"an unknown field", map[string]any{"want": "a"}, map[string]any{"field": "nope", "op": "eq", "param": "want"}, "/field"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			m := project(t, dbDir)
			edit(t, m, dbPlugin, bound(c.params, c.filter))
			wantDiag(t, compileFS(m), plxerr.DatabaseSourceInvalid, dbPlugin, "/dataSources/0/config/filter"+c.ptr)
		})
	}
}

// TestQueryOutputIsTyped reads a dbQuery step's output in the next step:
// the list of its collection's records, with their fields (DB-007).
// Verifies: DB-007.
func TestQueryOutputIsTyped(t *testing.T) {
	t.Parallel()
	const page = "plugins/todo/pages/list.page.json"
	for src, code := range map[string]plxerr.Code{
		`first(steps.q.output ?? [])?.title ?? ""`: 0,
		`string(size(steps.q.output ?? []))`:       0,
		`first(steps.q.output ?? [])?.nothing`:     plxerr.PXLUnknownField,
	} {
		m := project(t, dbDir)
		edit(t, m, page, func(doc map[string]any) {
			at(t, doc, "root/slots/body/children/1/events")["onPressed"] = raw(t, `{"steps": [
				{"id": "q", "action": "dbQuery", "input": {"collection": "tasks", "where": {"$expr": "!record.done"}}, "next": "say"},
				{"id": "say", "action": "showSnackbar", "input": {"message": {"$expr": `+strconv.Quote(src)+`}}}]}`)
		})
		res := compileFS(m)
		if code == 0 {
			for _, d := range res.Diagnostics {
				if d.Severity == plxerr.SeverityError {
					t.Errorf("%s:\n%s", src, list(res.Diagnostics))
					break
				}
			}
			continue
		}
		wantDiag(t, res, code, page, "/root/slots/body/children/1/events/onPressed/steps/1/input/message")
	}
}

// A readConfig step's output has the type its `type` input names, and a
// step without one has no output to read (DAT-090); naming the type needs
// the runtime that checks it (PLX-6074).
func TestConfigOutputIsTyped(t *testing.T) {
	t.Parallel()
	const page = "plugins/todo/pages/list.page.json"
	for _, c := range []struct {
		typ, src string
		code     plxerr.Code
	}{
		{`"decimal"`, `string(steps.r.output ?? 0d)`, 0},
		{`"int"`, `string((steps.r.output ?? 0) + 1)`, 0},
		{`"bool"`, `(steps.r.output ?? false) ? "on" : "off"`, 0},
		{`"string"`, `steps.r.output ?? ""`, 0},
		{`"decimal"`, `steps.r.output ?? ""`, plxerr.PXLTypeMismatch},
		{`null`, `string(steps.r.output)`, plxerr.PXLUnknownField},
	} {
		m := project(t, dbDir)
		edit(t, m, page, func(doc map[string]any) {
			input := `{"key": "rate"}`
			if c.typ != "null" {
				input = `{"key": "rate", "type": ` + c.typ + `}`
			}
			at(t, doc, "root/slots/body/children/1/events")["onPressed"] = raw(t, `{"steps": [
				{"id": "r", "action": "readConfig", "input": `+input+`, "next": "say"},
				{"id": "say", "action": "showSnackbar", "input": {"message": {"$expr": `+strconv.Quote(c.src)+`}}}]}`)
		})
		res := compileFS(m)
		if c.code != 0 {
			wantDiag(t, res, c.code, page, "/root/slots/body/children/1/events/onPressed/steps/1/input/message")
			continue
		}
		for _, d := range res.Diagnostics {
			if d.Severity == plxerr.SeverityError {
				t.Errorf("%s %s:\n%s", c.typ, c.src, list(res.Diagnostics))
				break
			}
		}
		if !slices.Contains(res.Plugins[0].Features, "enum.ConfigValueType.v1") {
			t.Errorf("%s: features %v lack enum.ConfigValueType.v1", c.typ, res.Plugins[0].Features)
		}
	}
}
