// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"fmt"
	"strconv"
	"strings"
)

// Category is a CLDR plural category.
type Category string

// The plural categories, in the order rules are evaluated.
const (
	Zero  Category = "zero"
	One   Category = "one"
	Two   Category = "two"
	Few   Category = "few"
	Many  Category = "many"
	Other Category = "other"
)

var categoryOrder = []Category{Zero, One, Two, Few, Many}

// relation is one "operand [% mod] (=|!=) ranges" test of a plural rule.
type relation struct {
	operand byte
	mod     int64
	negate  bool
	ranges  [][2]int64
}

// rule is a condition in disjunctive normal form: any of its conjunctions.
type rule [][]relation

// ruleSet maps a category to its compiled rule; "other" has none.
type ruleSet map[Category]rule

// compileRules parses the rules of one plural type, written in CLDR's plural
// rule syntax (without samples). It runs once, when a locale loads.
func compileRules(src map[string]string) (ruleSet, error) {
	out := make(ruleSet, len(src))
	for name, text := range src {
		cat := Category(name)
		switch cat {
		case Zero, One, Two, Few, Many:
		default:
			return nil, fmt.Errorf("%w: unknown plural category %q", ErrInvalidData, name)
		}
		r, err := parseRule(text)
		if err != nil {
			return nil, fmt.Errorf("%w: plural rule %q: %w", ErrInvalidData, text, err)
		}
		out[cat] = r
	}
	return out, nil
}

func parseRule(text string) (rule, error) {
	var r rule
	var and []relation
	toks := strings.Fields(text)
	for i := 0; i < len(toks); {
		rel, n, err := parseRelation(toks[i:])
		if err != nil {
			return nil, err
		}
		and = append(and, rel)
		i += n
		if i == len(toks) {
			break
		}
		switch toks[i] {
		case "and":
		case "or":
			r = append(r, and)
			and = nil
		default:
			return nil, fmt.Errorf("unexpected %q", toks[i])
		}
		i++
		if i == len(toks) {
			return nil, fmt.Errorf("rule ends after %q", toks[i-1])
		}
	}
	if len(and) == 0 {
		return nil, fmt.Errorf("empty rule")
	}
	return append(r, and), nil
}

// parseRelation parses one relation from the front of toks and returns the
// number of tokens it used.
func parseRelation(toks []string) (relation, int, error) {
	var rel relation
	if len(toks[0]) != 1 || strings.IndexByte("nivwftce", toks[0][0]) < 0 {
		return rel, 0, fmt.Errorf("unknown operand %q", toks[0])
	}
	rel.operand = toks[0][0]
	i := 1
	if i < len(toks) && toks[i] == "%" {
		if i+1 >= len(toks) {
			return rel, 0, fmt.Errorf("missing modulus")
		}
		m, err := strconv.ParseInt(toks[i+1], 10, 64)
		if err != nil || m <= 0 {
			return rel, 0, fmt.Errorf("bad modulus %q", toks[i+1])
		}
		rel.mod = m
		i += 2
	}
	if i+1 >= len(toks) {
		return rel, 0, fmt.Errorf("incomplete relation")
	}
	switch toks[i] {
	case "=":
	case "!=":
		rel.negate = true
	default:
		return rel, 0, fmt.Errorf("unknown operator %q", toks[i])
	}
	for _, part := range strings.Split(toks[i+1], ",") {
		lo, hi, isRange := strings.Cut(part, "..")
		a, err := strconv.ParseInt(lo, 10, 64)
		if err != nil {
			return rel, 0, fmt.Errorf("bad range %q", part)
		}
		b := a
		if isRange {
			if b, err = strconv.ParseInt(hi, 10, 64); err != nil || b < a {
				return rel, 0, fmt.Errorf("bad range %q", part)
			}
		}
		rel.ranges = append(rel.ranges, [2]int64{a, b})
	}
	return rel, i + 2, nil
}

// operands are the CLDR plural operands of a number.
type operands struct {
	d Decimal
	// c is the compact decimal exponent (operands c and e); 0 for a plain number.
	c int
}

// value returns the operand's value, reduced modulo mod when mod > 0, and
// whether it is an integer (a fraction never matches an integer range).
func (o operands) value(op byte, mod int64) (int64, bool) {
	d := o.d
	trimmed := strings.TrimRight(d.frac, "0")
	switch op {
	case 'n':
		if trimmed != "" {
			return 0, false
		}
		return digitsValue(d.intDigits(), mod), true
	case 'i':
		return digitsValue(d.intDigits(), mod), true
	case 'v':
		return int64(len(d.frac)), true
	case 'w':
		return int64(len(trimmed)), true
	case 'f':
		return digitsValue(d.frac, mod), true
	case 't':
		return digitsValue(trimmed, mod), true
	default: // c, e
		v := int64(o.c)
		if mod > 0 {
			v %= mod
		}
		return v, true
	}
}

func (r relation) matches(o operands) bool {
	v, isInt := o.value(r.operand, r.mod)
	in := false
	if isInt {
		for _, rg := range r.ranges {
			if v >= rg[0] && v <= rg[1] {
				in = true
				break
			}
		}
	}
	return in != r.negate
}

func (r rule) matches(o operands) bool {
	for _, and := range r {
		ok := true
		for _, rel := range and {
			if !rel.matches(o) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// category returns the first category, in CLDR order, whose rule matches.
func (rs ruleSet) category(o operands) Category {
	for _, c := range categoryOrder {
		if r, ok := rs[c]; ok && r.matches(o) {
			return c
		}
	}
	return Other
}

// PluralCardinal returns the cardinal plural category of n in the locale
// (I18N-001). The fraction digits of n count: 1 is "one" in English, 1.0 is
// "other".
func (l *Locale) PluralCardinal(n Decimal) Category {
	return l.cardinal.category(operands{d: n.abs()})
}

// PluralOrdinal returns the ordinal plural category of n in the locale.
func (l *Locale) PluralOrdinal(n Decimal) Category {
	return l.ordinal.category(operands{d: n.abs()})
}

// Categories lists the cardinal categories the locale uses, in CLDR order,
// "other" last.
func (l *Locale) Categories() []Category {
	var out []Category
	for _, c := range categoryOrder {
		if _, ok := l.cardinal[c]; ok {
			out = append(out, c)
		}
	}
	return append(out, Other)
}

// OrdinalCategories lists the ordinal categories the locale uses, in CLDR
// order, "other" last.
func (l *Locale) OrdinalCategories() []Category {
	var out []Category
	for _, c := range categoryOrder {
		if _, ok := l.ordinal[c]; ok {
			out = append(out, c)
		}
	}
	return append(out, Other)
}
