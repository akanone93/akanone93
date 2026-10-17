// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package mf

import (
	"fmt"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/l10n"
)

// ArgType is the type a translation key declares for an argument, reduced
// to what a message can use it for.
type ArgType int

// The argument types.
const (
	// TypeNumber is int, double and decimal.
	TypeNumber ArgType = iota + 1
	// TypeString is string and enums.
	TypeString
	// TypeBool is bool.
	TypeBool
	// TypeDate is date.
	TypeDate
	// TypeDateTime is dateTime.
	TypeDateTime
	// TypeOther is every other type (money, lists, maps, ...), which a
	// message cannot use.
	TypeOther
)

// ProblemKind says which check a message failed.
type ProblemKind int

// The problem kinds, one per PLX-1602 to PLX-1605.
const (
	// ProblemUndeclared is an argument the key does not declare (PLX-1602).
	ProblemUndeclared ProblemKind = iota + 1
	// ProblemType is an argument used as a type it is not declared as
	// (PLX-1603).
	ProblemType
	// ProblemCategory is a plural category the locale lacks (PLX-1604).
	ProblemCategory
	// ProblemOtherMissing is a plural or select without an other case
	// (PLX-1605).
	ProblemOtherMissing
)

// Problem is one finding of Check; Offset is a byte offset in the source.
type Problem struct {
	Kind   ProblemKind
	Offset int
	Msg    string
}

// Check verifies the message against the key's declared arguments and the
// plural categories of the locale, and returns every problem in source
// order. A nil loc skips the category check.
func (m *Message) Check(declared map[string]ArgType, loc *l10n.Locale) []Problem {
	c := &checker{declared: declared, loc: loc}
	c.nodes(m.Nodes)
	return c.problems
}

type checker struct {
	declared map[string]ArgType
	loc      *l10n.Locale
	problems []Problem
}

func (c *checker) add(kind ProblemKind, off int, format string, args ...any) {
	c.problems = append(c.problems, Problem{Kind: kind, Offset: off, Msg: fmt.Sprintf(format, args...)})
}

// use checks that the argument is declared with one of the types.
func (c *checker) use(n *Node, as string, ok ...ArgType) {
	t, declared := c.declared[n.Name]
	switch {
	case !declared:
		c.add(ProblemUndeclared, n.Pos, "argument %q is not declared by the key", n.Name)
	case !slices.Contains(ok, t):
		c.add(ProblemType, n.Pos, "argument %q is declared with a type that cannot be used as %s", n.Name, as)
	}
}

func (c *checker) nodes(nodes []Node) {
	for i := range nodes {
		n := &nodes[i]
		switch n.Kind {
		case KindArg:
			c.use(n, "text", TypeNumber, TypeString, TypeBool, TypeDate, TypeDateTime)
		case KindNumber:
			c.use(n, "a number", TypeNumber)
		case KindDate:
			c.use(n, "a date", TypeDate, TypeDateTime)
		case KindTime:
			c.use(n, "a time", TypeDateTime)
		case KindPlural:
			c.use(n, "a plural count", TypeNumber)
			c.cases(n)
		case KindSelect:
			c.use(n, "a select key", TypeString, TypeBool)
			c.cases(n)
		}
	}
}

func (c *checker) cases(n *Node) {
	var allowed []l10n.Category
	if n.Kind == KindPlural && c.loc != nil {
		allowed = c.loc.Categories()
		if n.Ordinal {
			allowed = c.loc.OrdinalCategories()
		}
	}
	hasOther := false
	for i := range n.Cases {
		cs := &n.Cases[i]
		hasOther = hasOther || cs.Selector == "other"
		if allowed != nil && pluralSelectors[cs.Selector] && !slices.Contains(allowed, l10n.Category(cs.Selector)) {
			c.add(ProblemCategory, cs.Pos, "the locale has no plural category %q", cs.Selector)
		}
		c.nodes(cs.Body)
	}
	if !hasOther {
		c.add(ProblemOtherMissing, n.Pos, "argument %q has no other case", n.Name)
	}
}

// SampleArgs returns an argument for each declared name, to render a message
// for its length: the strings are "Sample", numbers 1234.5, and dates
// 2026-03-14 15:09:26.
func SampleArgs(declared map[string]ArgType) map[string]any {
	out := make(map[string]any, len(declared))
	for name, t := range declared {
		switch t {
		case TypeNumber:
			out[name] = 1234.5
		case TypeString:
			out[name] = "Sample"
		case TypeBool:
			out[name] = true
		case TypeDate, TypeDateTime:
			out[name] = l10n.GregorianDate(2026, 3, 14, 15, 9, 26)
		case TypeOther:
		}
	}
	return out
}
