// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pxl

import "sync"

// Currencies says which currencies a compiled expression can produce, so a
// bundle can carry the names of only the currencies its app uses (I18N-002,
// ADR-0066 Revision 2026-10-18). It is recorded by the compiler, not
// encoded.
type Currencies struct {
	// Codes are the currencies the expression names as literals, sorted: the
	// currency of a constant money value and a literal currency argument of
	// money.
	Codes []string
	// Dynamic is true when a money value comes from outside the expression
	// (a state path, a parameter, a data field) or when money gets a currency
	// that is not a literal, so its currency is unknown at compile time.
	Dynamic bool
}

// moneyOverloads are the ids of the overloads of money(x, currency).
var moneyOverloads = sync.OnceValue(func() map[int]bool {
	out := map[int]bool{}
	for _, def := range stdOverloads {
		if def.name == "money" {
			out[def.id] = true
		}
	}
	return out
})

// currenciesOf walks the typed tree of an expression.
func currenciesOf(root *ir) Currencies {
	codes := map[string]bool{}
	var c Currencies
	var walk func(n *ir)
	walk = func(n *ir) {
		switch {
		case n.kind == irConst:
			constCurrencies(n.val, codes)
			return
		case n.kind == irCall && moneyOverloads()[n.id]:
			if len(n.args) == 2 && n.args[1].kind == irConst {
				if code, ok := n.args[1].val.(string); ok {
					codes[code] = true
				} else {
					c.Dynamic = true
				}
			} else {
				c.Dynamic = true
			}
		case n.kind != irLocal && mentionsMoney(n.typ, map[*NamedType]bool{}) && !anyMentionsMoney(n.args) && !anyMentionsMoney(n.keys):
			c.Dynamic = true // money from a read, not derived from money
		}
		for _, a := range n.args {
			walk(a)
		}
		for _, k := range n.keys {
			walk(k)
		}
	}
	walk(root)
	c.Codes = sortedKeys(codes)
	return c
}

// constCurrencies records the currencies of the money values in a constant.
func constCurrencies(v Value, out map[string]bool) {
	switch x := v.(type) {
	case Money:
		out[x.Currency] = true
	case List:
		for _, e := range x {
			constCurrencies(e, out)
		}
	case Map:
		for _, e := range x {
			constCurrencies(e, out)
		}
	}
}

// mentionsMoney reports whether values of type t can hold money.
func mentionsMoney(t *Type, seen map[*NamedType]bool) bool {
	if t == nil {
		return false
	}
	if t.kind == KindMoney {
		return true
	}
	if t.elem != nil && mentionsMoney(t.elem, seen) {
		return true
	}
	if n := t.named; n != nil && !seen[n] {
		seen[n] = true
		for _, f := range n.Fields {
			if mentionsMoney(f, seen) {
				return true
			}
		}
	}
	return false
}

// anyMentionsMoney reports whether the type of one of nodes can hold money.
func anyMentionsMoney(nodes []*ir) bool {
	for _, a := range nodes {
		if mentionsMoney(a.typ, map[*NamedType]bool{}) {
			return true
		}
	}
	return false
}
