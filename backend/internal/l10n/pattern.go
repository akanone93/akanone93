// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"fmt"
	"sort"
	"strings"
)

type partKind uint8

const (
	partText partKind = iota
	partCurrency
	partPercent
	partPermille
	partMinus
	partPlus
)

type part struct {
	kind partKind
	text string
}

// affix is the literal text and symbols before or after a number.
type affix []part

// pattern is a compiled CLDR number pattern such as "#,##0.00 ¤;(#,##0.00 ¤)".
type pattern struct {
	prefix, suffix       affix
	negPrefix, negSuffix affix
	hasNeg               bool
	minInt               int
	minFrac, maxFrac     int
	primary, secondary   int // grouping sizes; 0 means no grouping
	scale                int // power of ten the value is multiplied by: 2 for a percent
}

// numberSystem holds a numbering system's symbols and compiled patterns.
type numberSystem struct {
	symbols    map[string]string
	decimal    *pattern
	percent    *pattern
	currency   *pattern
	accounting *pattern
	compact    []compactRow // ascending by exp
}

// compactRow is the short-compact patterns of one magnitude, by plural
// category.
type compactRow struct {
	exp      int // power of ten of the magnitude
	patterns map[string]*pattern
}

func compileNumberSystem(r rawNumberSystem) (*numberSystem, error) {
	ns := &numberSystem{symbols: r.Symbols}
	for _, k := range []string{"decimal", "group", "minusSign", "percentSign", "plusSign"} {
		if ns.symbols[k] == "" {
			return nil, fmt.Errorf("symbol %s is missing", k)
		}
	}
	for _, p := range []struct {
		src string
		dst **pattern
	}{{r.Decimal, &ns.decimal}, {r.Percent, &ns.percent}, {r.Currency, &ns.currency}, {r.Accounting, &ns.accounting}} {
		c, err := compilePattern(p.src)
		if err != nil {
			return nil, err
		}
		*p.dst = c
	}
	for key, byCat := range r.CompactShort {
		exp := len(key) - 1
		if key == "" || strings.Trim(key, "0") != "1" || key[0] != '1' {
			return nil, fmt.Errorf("compact magnitude %q is not a power of ten", key)
		}
		row := compactRow{exp: exp, patterns: make(map[string]*pattern, len(byCat))}
		for cat, src := range byCat {
			c, err := compilePattern(src)
			if err != nil {
				return nil, err
			}
			row.patterns[cat] = c
		}
		ns.compact = append(ns.compact, row)
	}
	sort.Slice(ns.compact, func(i, j int) bool { return ns.compact[i].exp < ns.compact[j].exp })
	return ns, nil
}

// compilePattern parses a CLDR number pattern. The negative subpattern after
// ";" is kept for its affixes only.
func compilePattern(src string) (*pattern, error) {
	pos, neg, hasNeg := cutUnquoted(src, ';')
	p := &pattern{hasNeg: hasNeg}
	var err error
	if p.prefix, p.suffix, err = parseHalf(pos, p, true); err != nil {
		return nil, fmt.Errorf("pattern %q: %w", src, err)
	}
	if hasNeg {
		if p.negPrefix, p.negSuffix, err = parseHalf(neg, p, false); err != nil {
			return nil, fmt.Errorf("pattern %q: %w", src, err)
		}
	}
	for _, a := range []affix{p.prefix, p.suffix} {
		for _, pt := range a {
			switch pt.kind {
			case partPercent:
				p.scale = 2
			case partPermille:
				p.scale = 3
			}
		}
	}
	return p, nil
}

// cutUnquoted splits s at the first sep outside single quotes.
func cutUnquoted(s string, sep rune) (before, after string, found bool) {
	quoted := false
	for i, r := range s {
		switch {
		case r == '\'':
			quoted = !quoted
		case r == sep && !quoted:
			return s[:i], s[i+len(string(sep)):], true
		}
	}
	return s, "", false
}

// parseHalf splits one subpattern into prefix, number core and suffix. When
// setCore is set it also records the core's digit counts in p.
func parseHalf(s string, p *pattern, setCore bool) (prefix, suffix affix, err error) {
	runes := []rune(s)
	start, end := -1, -1
	quoted := false
	for i, r := range runes {
		switch {
		case r == '\'':
			quoted = !quoted
		case !quoted && (r == '#' || r == '0' || r == ',' || r == '.'):
			if start < 0 {
				start = i
			}
			end = i + 1
		}
	}
	if start < 0 {
		return nil, nil, fmt.Errorf("no digit placeholder")
	}
	if setCore {
		if err := parseCore(string(runes[start:end]), p); err != nil {
			return nil, nil, err
		}
	}
	if prefix, err = parseAffix(string(runes[:start])); err != nil {
		return nil, nil, err
	}
	if suffix, err = parseAffix(string(runes[end:])); err != nil {
		return nil, nil, err
	}
	return prefix, suffix, nil
}

func parseCore(core string, p *pattern) error {
	ip, fp, _ := strings.Cut(core, ".")
	p.minInt = strings.Count(ip, "0")
	p.minFrac = strings.Count(fp, "0")
	p.maxFrac = len(fp)
	if strings.Contains(fp, ",") || strings.Contains(fp, ".") {
		return fmt.Errorf("grouping in the fraction")
	}
	groups := strings.Split(ip, ",")
	if len(groups) > 1 {
		p.primary = len(groups[len(groups)-1])
		p.secondary = p.primary
		if len(groups) > 2 {
			p.secondary = len(groups[len(groups)-2])
		}
		if p.primary == 0 || p.secondary == 0 {
			return fmt.Errorf("empty grouping")
		}
	}
	return nil
}

// symbolKind returns the part kind of a percent, per-mille, minus or plus
// sign in an affix.
func symbolKind(r rune) partKind {
	switch r {
	case '%':
		return partPercent
	case '‰':
		return partPermille
	case '-':
		return partMinus
	}
	return partPlus
}

// readQuotedRunes appends the text of the quote that opens at runes[i] to text
// and returns the index of its closing quote; false when it is unterminated.
func readQuotedRunes(runes []rune, i int, text *strings.Builder) (int, bool) {
	j := i + 1
	for j < len(runes) {
		if runes[j] == '\'' {
			if j+1 < len(runes) && runes[j+1] == '\'' {
				text.WriteRune('\'') // '' inside a quote is an apostrophe
				j += 2
				continue
			}
			break
		}
		text.WriteRune(runes[j])
		j++
	}
	if j == len(runes) {
		return 0, false
	}
	return j, true
}

func parseAffix(s string) (affix, error) {
	var out affix
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			out = append(out, part{kind: partText, text: text.String()})
			text.Reset()
		}
	}
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case '\'':
			if i+1 < len(runes) && runes[i+1] == '\'' {
				text.WriteRune('\'')
				i++
				continue
			}
			end, ok := readQuotedRunes(runes, i, &text)
			if !ok {
				return nil, fmt.Errorf("unterminated quote")
			}
			i = end
		case '¤':
			flush()
			for i+1 < len(runes) && runes[i+1] == '¤' {
				i++
			}
			out = append(out, part{kind: partCurrency})
		case '%', '‰', '-', '+':
			flush()
			out = append(out, part{kind: symbolKind(r)})
		default:
			text.WriteRune(r)
		}
	}
	flush()
	return out, nil
}
