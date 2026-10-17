// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package mf

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/l10n"
)

// ErrArgument is wrapped by evaluation errors about an argument's value.
var ErrArgument = errors.New("invalid message argument")

// Format evaluates the message in the locale. Arguments are string (also a
// number written as text where a number is needed), bool,
// int, int64, float64, l10n.Decimal (a number; its fraction digits count for
// plural rules) or l10n.CalendarDate (a Gregorian date and time of day). An
// argument that is missing renders as nothing. A node that cannot be
// evaluated, such as a plural over a string, renders as nothing and the
// first such error is returned beside the rest of the text, so a message
// always has a rendering.
func (m *Message) Format(loc *l10n.Locale, args map[string]any) (string, error) {
	e := &evaluator{loc: loc, args: args}
	var b strings.Builder
	e.nodes(&b, m.Nodes, nil)
	return b.String(), e.err
}

type evaluator struct {
	loc  *l10n.Locale
	args map[string]any
	err  error
}

func (e *evaluator) fail(format string, args ...any) {
	if e.err == nil {
		e.err = fmt.Errorf(format, args...)
	}
}

// pound is what # writes in a plural: the number less the offset.
type pound struct{ text string }

func (e *evaluator) nodes(b *strings.Builder, nodes []Node, p *pound) {
	for i := range nodes {
		n := &nodes[i]
		switch n.Kind {
		case KindText:
			b.WriteString(n.Text)
		case KindPound:
			if p != nil {
				b.WriteString(p.text)
			}
		case KindArg:
			e.plain(b, n)
		case KindNumber:
			e.number(b, n)
		case KindDate, KindTime:
			e.date(b, n)
		case KindPlural:
			e.plural(b, n, p)
		case KindSelect:
			e.selectCase(b, n, p)
		}
	}
}

// toDecimal converts a numeric argument.
func toDecimal(v any) (l10n.Decimal, bool) {
	switch x := v.(type) {
	case l10n.Decimal:
		return x, true
	case int:
		return l10n.DecimalFromInt(int64(x)), true
	case int64:
		return l10n.DecimalFromInt(x), true
	case float64:
		d, err := l10n.ParseDecimal(strconv.FormatFloat(x, 'f', -1, 64))
		return d, err == nil
	case string: // PXL passes every argument of t() as a string
		d, err := l10n.ParseDecimal(x)
		return d, err == nil
	}
	return l10n.Decimal{}, false
}

func (e *evaluator) plain(b *strings.Builder, n *Node) {
	v, ok := e.args[n.Name]
	if !ok || v == nil {
		return
	}
	switch x := v.(type) {
	case string:
		b.WriteString(x)
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case l10n.CalendarDate:
		e.date(b, &Node{Kind: KindDate, Name: n.Name})
	default:
		if d, ok := toDecimal(v); ok {
			s, err := e.loc.FormatNumber(d, l10n.NumberOptions{})
			if err != nil {
				e.fail("argument %q: %w", n.Name, err)
			}
			b.WriteString(s)
			return
		}
		e.fail("%w: %q has an unsupported type", ErrArgument, n.Name)
	}
}

func (e *evaluator) number(b *strings.Builder, n *Node) {
	v, ok := e.args[n.Name]
	if !ok || v == nil {
		return
	}
	d, ok := toDecimal(v)
	if !ok {
		e.fail("%w: %q is not a number", ErrArgument, n.Name)
		return
	}
	var s string
	var err error
	switch {
	case n.Style == "integer":
		s, err = e.loc.FormatNumber(d, l10n.NumberOptions{Fraction: &l10n.FractionDigits{}})
	case n.Style == "percent":
		s, err = e.loc.FormatPercent(d, l10n.NumberOptions{})
	case strings.HasPrefix(n.Style, "::"):
		s, err = e.skeleton(d, n.Style[2:])
	default:
		s, err = e.loc.FormatNumber(d, l10n.NumberOptions{})
	}
	if err != nil {
		e.fail("argument %q: %w", n.Name, err)
	}
	b.WriteString(s)
}

func (e *evaluator) skeleton(d l10n.Decimal, src string) (string, error) {
	sk, err := parseNumberSkeleton(src)
	if err != nil {
		return "", err
	}
	var frac *l10n.FractionDigits
	if sk.fractionSet {
		frac = &l10n.FractionDigits{Min: sk.min, Max: sk.max}
	}
	switch {
	case sk.compact:
		return e.loc.FormatCompact(d, l10n.NumberOptions{NoGrouping: sk.noGroup})
	case sk.percent:
		return e.loc.FormatPercent(d, l10n.NumberOptions{Fraction: frac, NoGrouping: sk.noGroup})
	case sk.currency != "":
		return e.loc.FormatMoney(d, sk.currency, l10n.MoneyOptions{Fraction: frac, NoGrouping: sk.noGroup})
	}
	return e.loc.FormatNumber(d, l10n.NumberOptions{Fraction: frac, NoGrouping: sk.noGroup})
}

func (e *evaluator) date(b *strings.Builder, n *Node) {
	v, ok := e.args[n.Name]
	if !ok || v == nil {
		return
	}
	d, ok := v.(l10n.CalendarDate)
	if !ok {
		e.fail("%w: %q is not a date", ErrArgument, n.Name)
		return
	}
	var opts l10n.DateOptions
	switch {
	case strings.HasPrefix(n.Style, "::"):
		opts.Skeleton = n.Style[2:]
	case n.Kind == KindDate && n.Style == "":
		opts.DateStyle = "medium"
	case n.Kind == KindDate:
		opts.DateStyle = n.Style
	case n.Style == "":
		opts.TimeStyle = "medium"
	default:
		opts.TimeStyle = n.Style
	}
	s, err := e.loc.FormatDate(d, "gregorian", opts)
	if err != nil {
		e.fail("argument %q: %w", n.Name, err)
	}
	b.WriteString(s)
}

func (e *evaluator) selectCase(b *strings.Builder, n *Node, p *pound) {
	v, ok := e.args[n.Name]
	if !ok || v == nil {
		return
	}
	var key string
	switch x := v.(type) {
	case string:
		key = x
	case bool:
		key = strconv.FormatBool(x)
	default:
		e.fail("%w: %q is not a string", ErrArgument, n.Name)
		return
	}
	for i := range n.Cases {
		if n.Cases[i].Selector == key {
			e.nodes(b, n.Cases[i].Body, p)
			return
		}
	}
	for i := range n.Cases {
		if n.Cases[i].Selector == "other" {
			e.nodes(b, n.Cases[i].Body, p)
			return
		}
	}
}

func (e *evaluator) plural(b *strings.Builder, n *Node, outer *pound) {
	v, ok := e.args[n.Name]
	if !ok || v == nil {
		return
	}
	d, ok := toDecimal(v)
	if !ok {
		e.fail("%w: %q is not a number", ErrArgument, n.Name)
		return
	}
	shifted, err := subtract(d, n.Offset)
	if err != nil {
		e.fail("argument %q: %w", n.Name, err)
		return
	}
	text, err := e.loc.FormatNumber(shifted, l10n.NumberOptions{})
	if err != nil {
		e.fail("argument %q: %w", n.Name, err)
	}
	inner := &pound{text: text}
	if whole, ok := wholeNumber(d); ok {
		for i := range n.Cases {
			if sel, isExact := strings.CutPrefix(n.Cases[i].Selector, "="); isExact && canonical(sel) == whole {
				e.nodes(b, n.Cases[i].Body, inner)
				return
			}
		}
	}
	var cat l10n.Category
	if n.Ordinal {
		cat = e.loc.PluralOrdinal(shifted)
	} else {
		cat = e.loc.PluralCardinal(shifted)
	}
	for _, want := range []string{string(cat), "other"} {
		for i := range n.Cases {
			if n.Cases[i].Selector == want {
				e.nodes(b, n.Cases[i].Body, inner)
				return
			}
		}
	}
}

// canonical strips leading zeros of a digit string.
func canonical(digits string) string {
	if t := strings.TrimLeft(digits, "0"); t != "" {
		return t
	}
	return "0"
}

// wholeNumber returns the digits of d when it is a non-negative integer
// value, 1.0 included.
func wholeNumber(d l10n.Decimal) (string, bool) {
	s := d.String()
	if strings.HasPrefix(s, "-") {
		return "", false
	}
	ints, frac, _ := strings.Cut(s, ".")
	if strings.Trim(frac, "0") != "" {
		return "", false
	}
	return canonical(ints), true
}

// subtract returns d less a whole offset, keeping d's fraction digits.
func subtract(d l10n.Decimal, offset int64) (l10n.Decimal, error) {
	if offset == 0 {
		return d, nil
	}
	s := d.String()
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	ints, frac, _ := strings.Cut(s, ".")
	u, ok := new(big.Int).SetString(ints+frac, 10)
	if !ok {
		return d, fmt.Errorf("%w: %q", ErrArgument, d.String())
	}
	if neg {
		u.Neg(u)
	}
	shift := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(frac))), nil)
	u.Sub(u, shift.Mul(shift, big.NewInt(offset)))
	out := new(big.Int).Abs(u).String()
	if len(out) <= len(frac) {
		out = strings.Repeat("0", len(frac)-len(out)+1) + out
	}
	text := out[:len(out)-len(frac)]
	if frac != "" {
		text += "." + out[len(out)-len(frac):]
	}
	if u.Sign() < 0 {
		text = "-" + text
	}
	return l10n.ParseDecimal(text)
}
