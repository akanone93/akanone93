// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"slices"
	"strconv"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// The key-value store's types (DB-009): a kvGet of a literal key reads
// the type the plugin's kvSet steps of that key write, as the action
// reference says ("fixed by the first kvSet for the key within the
// plugin"). Without it a value could be written and never read back as
// itself (build log row 134).

// kvTypes are the types a plugin's kvSet steps write, by literal key; ""
// marks a key written with two types, which a kvGet then reads untyped.
type kvTypes map[string]string

// note records that a kvSet writes typ to key.
func (k kvTypes) note(key, typ string) {
	if prev, ok := k[key]; ok && prev != typ {
		k[key] = ""
		return
	}
	k[key] = typ
}

// kvPrepass records, before a plugin's graphs are typed, what each kvSet
// writes when it is known without the step's own graph: a literal's type,
// or an expression's that reads only the plugin's roots (state, app,
// flags), compiled quietly in the plugin's scope. The others are recorded
// as their graphs are typed (kvTyped), for the kvGets typed after them.
func (t *typer) kvPrepass(pl *plugin, base *scope) {
	pl.kv = kvTypes{}
	for _, g := range slices.Concat(pl.graphs, pl.inline) {
		for _, st := range g.steps {
			key := literalString(st.Input["key"])
			if st.Action != "kvSet" || key == "" {
				continue
			}
			if typ := t.quietType(st.Input["value"], base); typ != "" {
				pl.kv.note(key, typ)
			}
		}
	}
}

// quietType is the type of a kvSet's value known in the plugin's scope:
// a scalar literal's, or an expression's that compiles there; "" for any
// other. Nothing is reported: the value is checked in its own graph.
func (t *typer) quietType(raw json.RawMessage, base *scope) string {
	src, ok := bareExpr(raw)
	if !ok {
		return primitiveOfRaw(raw)
	}
	env, err := t.u.env(base)
	if err != nil {
		return ""
	}
	_, typ, diags := pxl.Compile(src, env, base.options(t.opts), plxerr.Location{})
	if diags.HasErrors() || typ == nil {
		return ""
	}
	return nonNull(typ.String())
}

// kvTyped records what the kvSet steps of a graph just typed write.
func (t *typer) kvTyped(g *graph) {
	if g.plugin == nil || g.plugin.kv == nil {
		return
	}
	for i, st := range g.steps {
		key := literalString(st.Input["key"])
		if st.Action != "kvSet" || key == "" {
			continue
		}
		ptr := g.ptr + plxerr.Pointer("steps", strconv.Itoa(i), "input", "value")
		if e, ok := t.u.exprs[g.file+"#"+ptr]; ok && e.typ != nil {
			g.plugin.kv.note(key, nonNull(e.typ.String()))
		} else if !ok {
			if typ := primitiveOfRaw(st.Input["value"]); typ != "" {
				g.plugin.kv.note(key, typ)
			}
		}
	}
}

// kvOutput is the output type of a kvGet step: the type the plugin's
// kvSet steps write to its literal key, nullable, as the key may hold
// nothing; "" when no kvSet of the key is known yet or two disagree.
func (t *typer) kvOutput(g *graph, st schema.Step) string {
	key := literalString(st.Input["key"])
	if g.plugin == nil || key == "" {
		return ""
	}
	typ := g.plugin.kv[key]
	te, err := parseTypeExpr(typ)
	if typ == "" || err != nil {
		return ""
	}
	if !t.u.types.known(g.plugin, te.name) && !hasKey(t.u.types.base, te.name) && te.elem == nil {
		return ""
	}
	te.nullable = true
	return te.String()
}

// nonNull is a type without its nullability: a kvSet of null removes
// nothing and stores no type, so a nullable expression writes its type.
func nonNull(typ string) string {
	te, err := parseTypeExpr(typ)
	if err != nil {
		return typ
	}
	te.nullable = false
	return te.String()
}

// primitiveOfRaw is the type of a scalar JSON literal, "" for any other.
func primitiveOfRaw(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return primitiveOf(v)
}
