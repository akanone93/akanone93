// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package literals finds the literal text in the translatable props of a
// node document (I18N-007): the text the compiler warns about (PLX-1609) and
// `plux l10n extract` moves into translation keys. A prop or value-type
// field is translatable when its descriptor says so; the registry is the
// only list.
package literals

import (
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/nightCode42/plux3/backend/internal/schema/registry"
)

// Literal is a literal string in a translatable position.
type Literal struct {
	// Path locates the string from the node: object keys and list indices,
	// such as props, decoration, labelText.
	Path []string
	// Prop is the name of the widget prop the string belongs to, or
	// "semantics.label" or "semantics.hint".
	Prop string
	// Text is the string.
	Text string
}

// Node returns the literals of a node document decoded as generic JSON: the
// translatable props of its widget, and its semantics label and hint. The
// result follows the document order of the props sorted by name, so it is
// deterministic. A node whose type is not a widget has none.
func Node(node map[string]any) []Literal {
	var out []Literal
	typ, _ := node["type"].(string)
	if w, ok := registry.LookupWidget(typ); ok {
		props, _ := node["props"].(map[string]any)
		for _, p := range w.Props {
			v, present := props[p.Name]
			if !present {
				continue
			}
			walk(p.Type, v, p.Translatable, []string{"props", p.Name}, p.Name, &out)
		}
	}
	if sem, ok := node["semantics"].(map[string]any); ok {
		for _, name := range []string{"label", "hint"} {
			if s, ok := sem[name].(string); ok && Translatable(s) {
				out = append(out, Literal{Path: []string{"semantics", name}, Prop: "semantics." + name, Text: s})
			}
		}
	}
	return out
}

// Translatable reports whether a string is text a person reads: it holds at
// least one letter. Punctuation, digits and symbols alone ("-", "$", "12")
// are not translated.
func Translatable(s string) bool {
	return strings.IndexFunc(s, unicode.IsLetter) >= 0
}

// walk follows a value along its type expression and collects the strings
// of translatable positions.
func walk(typ string, v any, marked bool, path []string, prop string, out *[]Literal) {
	typ = strings.TrimSuffix(typ, "?")
	switch x := v.(type) {
	case string:
		if marked && typ == "string" && Translatable(x) {
			*out = append(*out, Literal{Path: append([]string(nil), path...), Prop: prop, Text: x})
		}
	case []any:
		elem, ok := inner(typ, "list<")
		if !ok {
			return
		}
		for i, it := range x {
			walk(elem, it, marked, append(path, itoa(i)), prop, out)
		}
	case map[string]any:
		if isBinding(x) {
			return
		}
		if elem, ok := inner(typ, "map<"); ok {
			if _, value, found := strings.Cut(elem, ","); found {
				for _, k := range sortedKeys(x) {
					walk(value, x[k], marked, append(path, k), prop, out)
				}
			}
			return
		}
		vt, ok := registry.LookupValueType(typ)
		if !ok {
			return
		}
		for _, f := range vt.Fields {
			if fv, present := x[f.Name]; present {
				walk(f.Type, fv, f.Translatable, append(path, f.Name), prop, out)
			}
		}
	}
}

// isBinding reports whether an object is a binding, an expression, a
// translation reference, a token or an asset, none of them literal text.
func isBinding(m map[string]any) bool {
	for k := range m {
		if strings.HasPrefix(k, "$") {
			return true
		}
	}
	return false
}

// inner returns the type argument of prefix<...>.
func inner(typ, prefix string) (string, bool) {
	if strings.HasPrefix(typ, prefix) && strings.HasSuffix(typ, ">") {
		return typ[len(prefix) : len(typ)-1], true
	}
	return "", false
}

// itoa formats a list index.
func itoa(i int) string { return strconv.Itoa(i) }

// sortedKeys returns the keys of m in order.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
