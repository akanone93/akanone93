// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// databaseConfig is the configuration of a `database` data source: a
// watched query over a local collection that lists bind to and that
// updates when the collection changes (DB-006).
type databaseConfig struct {
	Collection string          `json:"collection"`
	Filter     json.RawMessage `json:"filter"`
	// Params are bound in the source's scope, like a REST source's, and a
	// comparison takes one as its value with "param" (DB-006): a page
	// watches the record its parameter names.
	Params     map[string]json.RawMessage `json:"params"`
	OrderBy    string                     `json:"orderBy"`
	Descending bool                       `json:"descending"`
	Limit      *int64                     `json:"limit"`
	Offset     *int64                     `json:"offset"`
}

// filterOps are the operators of a filter's comparisons; the null tests
// take no value.
var filterOps = []string{"eq", "ne", "lt", "le", "gt", "ge", "in", "contains", "startsWith", "isNull", "notNull"}

// maxFilterDepth bounds the nesting of and, or and not.
const maxFilterDepth = 8

// badFunc reports a problem at a pointer below the source's config.
type badFunc func(sub, format string, args ...any)

// checkDatabaseSource validates a database source's configuration against
// the collection it names and requires the db feature (DB-006).
func (u *unit) checkDatabaseSource(pl *plugin, s *schema.DataSource, file, ptr string) {
	cptr := ptr + "/config"
	u.useRevision(dbFeature, dbRuntimes, 1, vctx{file: file, ptr: ptr, pl: pl})
	bad := func(sub, format string, args ...any) {
		u.report(plxerr.DatabaseSourceInvalid, file, cptr+sub, format, args...)
	}
	if len(s.Config) == 0 {
		bad("", "database source %q has no config", s.Name)
		return
	}
	var c databaseConfig
	dec := json.NewDecoder(bytes.NewReader(s.Config))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		bad("", "database source %q: %v", s.Name, err)
		return
	}
	if te, err := parseTypeExpr(s.Type); err == nil && te.name != "list" {
		u.report(plxerr.DatabaseSourceInvalid, file, ptr+"/type", "database source %q returns records: its type is a list, not %s", s.Name, s.Type)
	}
	col := u.collectionOf(pl, c.Collection)
	if col == nil {
		bad("/collection", "no collection is named %q", c.Collection)
		return
	}
	kinds := map[string]string{}
	for _, f := range col.Fields {
		kinds[f.Name] = f.Type
	}
	if len(c.Filter) > 0 {
		var v any
		if err := json.Unmarshal(c.Filter, &v); err != nil {
			bad("/filter", "the filter is not JSON: %v", err)
		} else {
			f := filterCheck{fields: kinds, params: u.paramTypes(file, cptr, c.Params), bad: func(p, msg string) { bad(p, "%s", msg) }}
			f.check(v, 0, "/filter")
			if f.bound {
				u.useRevision(dbFeature, dbRuntimes, 2, vctx{file: file, ptr: cptr + "/filter", pl: pl})
			}
		}
	}
	u.checkDatabaseOrder(&c, col, kinds, bad)
}

// checkDatabaseOrder checks a database source's sort, limit and offset.
func (u *unit) checkDatabaseOrder(c *databaseConfig, col *schema.Collection, kinds map[string]string, bad badFunc) {
	if c.OrderBy != "" {
		t, ok := kinds[c.OrderBy]
		switch {
		case !ok:
			bad("/orderBy", "collection %q has no field %q", col.Key, c.OrderBy)
		case scalarKind(t) == "":
			bad("/orderBy", "field %q (%s) cannot be sorted", c.OrderBy, t)
		}
	}
	if c.Descending && c.OrderBy == "" {
		bad("/descending", "descending needs orderBy")
	}
	if max := u.opts.Limits.Get(limits.DBQueryRows); c.Limit != nil && (*c.Limit < 0 || *c.Limit > max) {
		bad("/limit", "limit %d is outside 0 to db.queryRows = %d", *c.Limit, max)
	}
	if c.Offset != nil && *c.Offset < 0 {
		bad("/offset", "offset %d is negative", *c.Offset)
	}
}

// scalarKind names how the database stores a field type, "" for types it
// cannot sort or order: lists, maps, objects and money.
func scalarKind(t string) string {
	te, err := parseTypeExpr(t)
	if err != nil {
		return ""
	}
	switch te.name {
	case "string", "date", "dateTime", "color", "asset", "route", "decimal":
		return "string"
	case "int", "duration":
		return "int"
	case "double":
		return "real"
	case "bool":
		return "bool"
	}
	return ""
}

// databaseConfigValue is a database source's configuration as the bundle
// carries it: literal, but for its params, which are bindings evaluated in
// the source's scope when the watch starts.
func (u *unit) databaseConfigValue(pl *plugin, s *schema.DataSource, file, ptr string, sc *scope) *value {
	if !hasExpr(s.Config) {
		return u.inferred(literalCtx(pl, file, ptr+"/config"), s.Config)
	}
	cfg, ok := decodeJSON(s.Config)
	if !ok {
		return nil
	}
	return u.infer(vctx{file: file, ptr: ptr + "/config", scope: sc, pl: pl, code: plxerr.DatabaseSourceInvalid}, cfg)
}

// paramTypes returns the type of each of a database source's params, from
// their compiled bindings; a literal param is typed by its JSON value.
func (u *unit) paramTypes(file, cptr string, params map[string]json.RawMessage) map[string]string {
	out := map[string]string{}
	for name, raw := range params {
		if e, ok := u.exprs[file+"#"+cptr+plxerr.Pointer("params", name)]; ok {
			if e.typ != nil {
				out[name] = e.typ.String()
			}
			continue
		}
		var v any
		if json.Unmarshal(raw, &v) != nil {
			continue
		}
		switch x := v.(type) {
		case string:
			out[name] = "string"
		case bool:
			out[name] = "bool"
		case float64:
			out[name] = "double"
			if x == float64(int64(x)) {
				out[name] = "int"
			}
		}
	}
	return out
}

// filterCheck checks a filter tree: known fields, operators, and values or
// params of the field's type.
type filterCheck struct {
	fields map[string]string
	// params maps each of the source's params to its type.
	params map[string]string
	bad    func(ptr, msg string)
	// bound is set when a comparison takes a param.
	bound bool
}

// check checks one node of the filter tree.
func (f *filterCheck) check(v any, depth int, ptr string) {
	m, ok := v.(map[string]any)
	if !ok {
		f.bad(ptr, "a filter is an object")
		return
	}
	if depth > maxFilterDepth {
		f.bad(ptr, fmt.Sprintf("filters nest at most %d levels", maxFilterDepth))
		return
	}
	for _, k := range []string{"and", "or"} {
		if list, has := m[k]; has {
			items, isList := list.([]any)
			if !isList {
				f.bad(ptr+"/"+k, k+" takes a list of filters")
				return
			}
			for i, it := range items {
				f.check(it, depth+1, fmt.Sprintf("%s/%s/%d", ptr, k, i))
			}
			return
		}
	}
	if not, has := m["not"]; has {
		f.check(not, depth+1, ptr+"/not")
		return
	}
	if name, has := m["param"]; has {
		f.checkParam(m, name, ptr)
		return
	}
	checkComparison(m, f.fields, ptr, f.bad)
}

// boundOps are the operators a param can supply the value of.
var boundOps = []string{"eq", "ne", "lt", "le", "gt", "ge", "in", "contains", "startsWith"}

// boundFields are the field types compared with a param: the database
// stores them as they are bound.
var boundFields = []string{"string", "int", "double", "bool"}

// checkParam checks a comparison whose value is the source's param `name`:
// a string, int, double or bool field, an operator that takes a value, and
// a param of the field's type (a list of it for in).
func (f *filterCheck) checkParam(m map[string]any, name any, ptr string) {
	field, _ := m["field"].(string)
	op, _ := m["op"].(string)
	param, isString := name.(string)
	t, known := f.fields[field]
	te, err := parseTypeExpr(t)
	switch {
	case !known:
		f.bad(ptr+"/field", fmt.Sprintf("no field %q in the collection", field))
		return
	case !slices.Contains(boundOps, op):
		f.bad(ptr+"/op", fmt.Sprintf("operator %q takes no param; use one of %v", op, boundOps))
		return
	case !isString || m["value"] != nil:
		f.bad(ptr+"/param", "a comparison takes either a value or the name of a param")
		return
	case err != nil || te.elem != nil || !slices.Contains(boundFields, te.name):
		f.bad(ptr+"/field", fmt.Sprintf("field %q (%s) is not compared with a param: bind %v fields", field, t, boundFields))
		return
	}
	pt, declared := f.params[param]
	want := te.name
	if op == "in" {
		want = "list<" + te.name + ">"
	}
	switch {
	case !declared:
		f.bad(ptr+"/param", fmt.Sprintf("the source has no param %q", param))
	case strings.TrimSuffix(pt, "?") != want:
		f.bad(ptr+"/param", fmt.Sprintf("param %q is a %s; field %q needs a %s", param, pt, field, want))
	default:
		f.bound = true
	}
}

// checkComparison checks one comparison of a filter.
func checkComparison(m map[string]any, types map[string]string, ptr string, bad func(ptr, msg string)) {
	field, _ := m["field"].(string)
	op, _ := m["op"].(string)
	t, known := types[field]
	switch {
	case !known:
		bad(ptr+"/field", fmt.Sprintf("no field %q in the collection", field))
		return
	case !slices.Contains(filterOps, op):
		bad(ptr+"/op", fmt.Sprintf("operator %q is not one of %v", op, filterOps))
		return
	}
	kind := scalarKind(t)
	ordered := op == "lt" || op == "le" || op == "gt" || op == "ge"
	text := op == "contains" || op == "startsWith"
	if (ordered && kind == "") || (text && kind != "string") {
		bad(ptr+"/op", fmt.Sprintf("%s does not apply to %s (%s)", op, field, t))
		return
	}
	if op == "isNull" || op == "notNull" {
		return
	}
	values := []any{m["value"]}
	if op == "in" {
		list, isList := m["value"].([]any)
		if !isList {
			bad(ptr+"/value", "in takes a list")
			return
		}
		values = list
	}
	for _, val := range values {
		nullOK := val == nil && (op == "eq" || op == "ne")
		if kind != "" && !valueFits(val, kind) && !nullOK {
			bad(ptr+"/value", fmt.Sprintf("the value compared with %s is not a %s", field, t))
			return
		}
	}
}

// valueFits reports whether a JSON value is of a scalar kind.
func valueFits(v any, kind string) bool {
	switch kind {
	case "string":
		_, ok := v.(string)
		return ok
	case "int":
		f, ok := v.(float64)
		return ok && f == float64(int64(f))
	case "real":
		_, ok := v.(float64)
		return ok
	case "bool":
		_, ok := v.(bool)
		return ok
	}
	return false
}
