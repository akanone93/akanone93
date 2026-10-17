// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package mf

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// maxDepth bounds the nesting of arguments in one message.
const maxDepth = 16

// ErrSyntax is wrapped by every *SyntaxError.
var ErrSyntax = errors.New("invalid message syntax")

// SyntaxError is a malformed message; Offset is the byte offset in the
// source where the problem was found.
type SyntaxError struct {
	Offset int
	Msg    string
}

func (e *SyntaxError) Error() string { return fmt.Sprintf("%s at offset %d", e.Msg, e.Offset) }

// Unwrap returns ErrSyntax.
func (e *SyntaxError) Unwrap() error { return ErrSyntax }

// pluralSelectors are the selector words of plural and selectordinal.
var pluralSelectors = map[string]bool{"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true}

var (
	numberStyles = map[string]bool{"integer": true, "percent": true}
	dateStyles   = map[string]bool{"short": true, "medium": true, "long": true, "full": true}
)

type parser struct {
	src string
	pos int
}

// Parse parses ICU MessageFormat: literal text with apostrophe quoting
// (an apostrophe quotes only before { or }, or # inside a plural; two
// apostrophes are one), simple arguments, number, date and time arguments
// with a style or "::" skeleton, plural with offset and =n, selectordinal,
// select, and # (I18N-001).
func Parse(src string) (*Message, error) {
	p := &parser{src: src}
	nodes, err := p.message(0, false)
	if err != nil {
		return nil, err
	}
	if p.pos < len(src) { // stopped at an unmatched }
		return nil, &SyntaxError{Offset: p.pos, Msg: "unmatched }"}
	}
	return &Message{Nodes: nodes}, nil
}

func (p *parser) done() bool { return p.pos >= len(p.src) }

func (p *parser) fail(off int, format string, args ...any) error {
	return &SyntaxError{Offset: off, Msg: fmt.Sprintf(format, args...)}
}

// message parses up to an unmatched } (not consumed) or the end.
func (p *parser) message(depth int, inPlural bool) ([]Node, error) {
	var nodes []Node
	var text strings.Builder
	start := p.pos
	flush := func() {
		if text.Len() > 0 {
			nodes = append(nodes, Node{Kind: KindText, Text: text.String(), Pos: start})
			text.Reset()
		}
	}
	for !p.done() {
		c := p.src[p.pos]
		switch {
		case c == '}':
			flush()
			return nodes, nil
		case c == '{':
			flush()
			n, err := p.argument(depth+1, inPlural)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, n)
			start = p.pos
		case c == '#' && inPlural:
			flush()
			nodes = append(nodes, Node{Kind: KindPound, Pos: p.pos})
			p.pos++
			start = p.pos
		case c == '\'':
			if text.Len() == 0 {
				start = p.pos
			}
			p.quote(&text, inPlural)
		default:
			if text.Len() == 0 {
				start = p.pos
			}
			r, w := utf8.DecodeRuneInString(p.src[p.pos:])
			text.WriteRune(r)
			p.pos += w
		}
	}
	flush()
	return nodes, nil
}

// quote handles an apostrophe: ” is one, ' before { } (or # in a plural)
// quotes up to the next single apostrophe, and any other ' is literal.
func (p *parser) quote(text *strings.Builder, inPlural bool) {
	p.pos++
	if p.done() {
		text.WriteByte('\'')
		return
	}
	c := p.src[p.pos]
	if c == '\'' {
		text.WriteByte('\'')
		p.pos++
		return
	}
	if c != '{' && c != '}' && (c != '#' || !inPlural) {
		text.WriteByte('\'')
		return
	}
	for !p.done() {
		c := p.src[p.pos]
		if c == '\'' {
			p.pos++
			if !p.done() && p.src[p.pos] == '\'' {
				text.WriteByte('\'')
				p.pos++
				continue
			}
			return
		}
		r, w := utf8.DecodeRuneInString(p.src[p.pos:])
		text.WriteRune(r)
		p.pos += w
	}
}

func (p *parser) skip() {
	for !p.done() && strings.IndexByte(" \t\n\r", p.src[p.pos]) >= 0 {
		p.pos++
	}
}

// word reads up to whitespace or one of , { }.
func (p *parser) word() string {
	start := p.pos
	for !p.done() && strings.IndexByte(" \t\n\r,{}", p.src[p.pos]) < 0 {
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *parser) expect(c byte) error {
	if p.done() || p.src[p.pos] != c {
		return p.fail(p.pos, "%q expected", c)
	}
	p.pos++
	return nil
}

// argument parses {name}, {name, type} or {name, type, ...} at the {.
func (p *parser) argument(depth int, inPlural bool) (Node, error) {
	if depth > maxDepth {
		return Node{}, p.fail(p.pos, "arguments nested deeper than %d", maxDepth)
	}
	n := Node{Pos: p.pos}
	p.pos++ // {
	p.skip()
	nameAt := p.pos
	n.Name = p.word()
	if n.Name == "" {
		return n, p.fail(nameAt, "argument name expected")
	}
	p.skip()
	if !p.done() && p.src[p.pos] == '}' {
		p.pos++
		n.Kind = KindArg
		return n, nil
	}
	if err := p.expect(','); err != nil {
		return n, err
	}
	p.skip()
	typeAt := p.pos
	typ := p.word()
	p.skip()
	switch typ {
	case "number", "date", "time":
		n.Kind = map[string]Kind{"number": KindNumber, "date": KindDate, "time": KindTime}[typ]
		return n, p.styled(&n)
	case "plural", "selectordinal", "select":
		if err := p.expect(','); err != nil {
			return n, err
		}
		return n, p.branches(&n, typ, depth, inPlural)
	case "":
		return n, p.fail(typeAt, "argument type expected")
	}
	return n, p.fail(typeAt, "unknown argument type %q", typ)
}

// styled parses the optional style of number, date and time and the }.
func (p *parser) styled(n *Node) error {
	if !p.done() && p.src[p.pos] == '}' {
		p.pos++
		return nil
	}
	if err := p.expect(','); err != nil {
		return err
	}
	p.skip()
	at := p.pos
	end := strings.IndexByte(p.src[p.pos:], '}')
	if end < 0 {
		return p.fail(len(p.src), "} expected")
	}
	style := strings.TrimSpace(p.src[p.pos : p.pos+end])
	p.pos += end + 1
	switch {
	case strings.HasPrefix(style, "::"):
		skel := strings.Join(strings.Fields(style[2:]), " ")
		if skel == "" {
			return p.fail(at, "empty skeleton")
		}
		if n.Kind == KindNumber {
			if _, err := parseNumberSkeleton(skel); err != nil {
				return p.fail(at, "%v", err)
			}
		}
		style = "::" + skel
	case n.Kind == KindNumber && numberStyles[style], n.Kind != KindNumber && dateStyles[style]:
	default:
		return p.fail(at, "unknown style %q", style)
	}
	n.Style = style
	return nil
}

// branches parses the cases of plural, selectordinal and select.
func (p *parser) branches(n *Node, typ string, depth int, inPlural bool) error {
	switch typ {
	case "plural", "selectordinal":
		n.Kind, n.Ordinal = KindPlural, typ == "selectordinal"
	default:
		n.Kind = KindSelect
	}
	p.skip()
	if n.Kind == KindPlural && strings.HasPrefix(p.src[p.pos:], "offset:") {
		p.pos += len("offset:")
		p.skip()
		at := p.pos
		w := p.word()
		off, err := strconv.ParseInt(w, 10, 32)
		if err != nil || off < 0 {
			return p.fail(at, "offset must be a non-negative integer")
		}
		n.Offset = off
		p.skip()
	}
	seen := map[string]bool{}
	for !p.done() && p.src[p.pos] != '}' {
		at := p.pos
		sel := p.word()
		if sel == "" {
			return p.fail(at, "selector expected")
		}
		if n.Kind == KindPlural {
			if err := checkPluralSelector(sel); err != nil {
				return p.fail(at, "%v", err)
			}
		}
		if seen[sel] {
			return p.fail(at, "duplicate selector %q", sel)
		}
		seen[sel] = true
		p.skip()
		if err := p.expect('{'); err != nil {
			return err
		}
		body, err := p.message(depth, n.Kind == KindPlural || inPlural)
		if err != nil {
			return err
		}
		if err := p.expect('}'); err != nil {
			return err
		}
		n.Cases = append(n.Cases, Case{Selector: sel, Body: body, Pos: at})
		p.skip()
	}
	return p.expect('}')
}

// checkPluralSelector accepts =n and the CLDR category words.
func checkPluralSelector(sel string) error {
	if rest, ok := strings.CutPrefix(sel, "="); ok {
		if rest == "" || strings.Trim(rest, "0123456789") != "" || len(rest) > 9 {
			return fmt.Errorf("%q is not =n with a non-negative integer", sel)
		}
		return nil
	}
	if !pluralSelectors[sel] {
		return fmt.Errorf("%q is not a plural category", sel)
	}
	return nil
}
