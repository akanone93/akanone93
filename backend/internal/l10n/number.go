// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FractionDigits bounds the number of fraction digits shown: the value is
// rounded, half to even, to Max digits and trailing zeros are dropped down to
// Min.
type FractionDigits struct{ Min, Max int }

// maxFractionDigits is the most fraction digits a request may ask for.
const maxFractionDigits = 20

func (f *FractionDigits) validate() error {
	if f.Min < 0 || f.Max < f.Min || f.Max > maxFractionDigits {
		return fmt.Errorf("%w: fraction digits %d..%d", ErrInvalidOption, f.Min, f.Max)
	}
	return nil
}

// NumberOptions tune FormatNumber, FormatPercent and FormatCompact.
type NumberOptions struct {
	// Fraction overrides the pattern's fraction digits; nil keeps them.
	Fraction *FractionDigits
	// NumberingSystem is "" or "latn" for Latin digits (the default), or
	// "default" or the locale's own system id (such as "arab") for its native
	// digits.
	NumberingSystem string
	// NoGrouping turns the grouping separator off.
	NoGrouping bool
}

// CurrencyDisplay selects how a currency is written.
type CurrencyDisplay int

// The currency displays.
const (
	// DisplaySymbol writes the locale's symbol ($, €, US$).
	DisplaySymbol CurrencyDisplay = iota
	// DisplayNarrow writes the narrow symbol ($ for every dollar).
	DisplayNarrow
	// DisplayCode writes the ISO 4217 code.
	DisplayCode
)

// MoneyOptions tune FormatMoney.
type MoneyOptions struct {
	Display CurrencyDisplay
	// Accounting writes negative amounts in the locale's accounting style,
	// such as (€1.00).
	Accounting bool
	// Fraction overrides the currency's ISO 4217 digits.
	Fraction        *FractionDigits
	NumberingSystem string
	NoGrouping      bool
}

// layout is everything one formatting call needs besides the number.
type layout struct {
	pat      *pattern
	sysID    string
	ns       *numberSystem
	min, max int
	grouping bool
	currency string // what ¤ is replaced by; "" outside money
	// minGrouping, when set, raises the locale's minimum grouping digits:
	// compact notation groups only from five digits.
	minGrouping int
	isMoney     bool
}

// FormatNumber formats x with the locale's decimal pattern: grouping
// (including secondary grouping), fraction digits rounded half to even, and
// the locale's separators (I18N-001).
func (l *Locale) FormatNumber(x Decimal, opts NumberOptions) (string, error) {
	id, ns, err := l.system(opts.NumberingSystem)
	if err != nil {
		return "", err
	}
	ly := layout{pat: ns.decimal, sysID: id, ns: ns, grouping: !opts.NoGrouping}
	if err := ly.fractions(opts.Fraction); err != nil {
		return "", err
	}
	return l.render(ly, x), nil
}

// FormatPercent formats x, a fraction (0.25), as a percentage ("25%").
func (l *Locale) FormatPercent(x Decimal, opts NumberOptions) (string, error) {
	id, ns, err := l.system(opts.NumberingSystem)
	if err != nil {
		return "", err
	}
	ly := layout{pat: ns.percent, sysID: id, ns: ns, grouping: !opts.NoGrouping}
	if err := ly.fractions(opts.Fraction); err != nil {
		return "", err
	}
	return l.render(ly, x), nil
}

func (ly *layout) fractions(f *FractionDigits) error {
	ly.min, ly.max = ly.pat.minFrac, ly.pat.maxFrac
	if f == nil {
		return nil
	}
	if err := f.validate(); err != nil {
		return err
	}
	ly.min, ly.max = f.Min, f.Max
	return nil
}

// FormatMoney formats amount in a currency (I18N-011): the currency's ISO
// 4217 digits unless overridden, the locale's symbol, its placement and
// spacing, and its accounting style on request. 1234.56 EUR is "1.234,56 €"
// in de and 1234.56 USD is "$1,234.56" in en.
func (l *Locale) FormatMoney(amount Decimal, currency string, opts MoneyOptions) (string, error) {
	if len(currency) != 3 || strings.Trim(currency, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return "", fmt.Errorf("%w: %q", ErrInvalidCurrency, currency)
	}
	id, ns, err := l.system(opts.NumberingSystem)
	if err != nil {
		return "", err
	}
	names := l.currencies[currency]
	var shown string
	switch opts.Display {
	case DisplaySymbol:
		shown = names.Symbol
		if shown == "" {
			shown = currency
		}
	case DisplayNarrow:
		shown = names.Narrow
		if shown == "" {
			shown = names.Symbol
		}
		if shown == "" {
			shown = currency
		}
	case DisplayCode:
		shown = currency
	default:
		return "", fmt.Errorf("%w: currency display %d", ErrUnsupported, opts.Display)
	}
	ly := layout{pat: ns.currency, sysID: id, ns: ns, grouping: !opts.NoGrouping, currency: shown, isMoney: true}
	if opts.Accounting {
		ly.pat = ns.accounting
	}
	digits := l.CurrencyDigits(currency)
	ly.min, ly.max = digits, digits
	if opts.Fraction != nil {
		if err := opts.Fraction.validate(); err != nil {
			return "", err
		}
		ly.min, ly.max = opts.Fraction.Min, opts.Fraction.Max
	}
	return l.render(ly, amount), nil
}

// FormatCompact formats x in the short compact notation ("1.2K", "3 Mio.",
// "12万"): the magnitude's pattern is chosen by the plural category of the
// rounded number, which keeps two significant digits below ten and whole
// numbers above.
func (l *Locale) FormatCompact(x Decimal, opts NumberOptions) (string, error) {
	id, ns, err := l.system(opts.NumberingSystem)
	if err != nil {
		return "", err
	}
	if opts.Fraction != nil {
		return "", fmt.Errorf("%w: compact notation fixes its own fraction digits", ErrInvalidOption)
	}
	scaled, exp, cur := ns.compactScale(x.abs())
	pat := ns.decimal
	if exp > 0 {
		cat := l.cardinal.category(operands{d: scaled, c: exp})
		row := ns.compactRow(cur.intLen() - 1)
		if row == nil {
			return "", fmt.Errorf("%w: compact patterns", ErrInvalidData)
		}
		pat = row.patterns[string(cat)]
		if pat == nil {
			pat = row.patterns[string(Other)]
		}
	}
	scaled.neg = x.neg
	ly := layout{pat: pat, sysID: id, ns: ns, grouping: !opts.NoGrouping, min: 0, max: len(scaled.frac), minGrouping: 2}
	return l.render(ly, scaled), nil
}

// compactScale scales cur to the compact magnitude it belongs to and rounds
// it. It returns the scaled number, the power of ten removed and the number
// the magnitude was finally chosen for: a number that rounds up to the next
// magnitude is scaled again, once.
func (ns *numberSystem) compactScale(cur Decimal) (scaled Decimal, exp int, chosen Decimal) {
	for attempt := 0; attempt < 2; attempt++ {
		row := ns.compactRow(cur.intLen() - 1)
		exp = 0
		if row != nil {
			if p := row.patterns[string(Other)]; p != nil && (len(p.prefix) > 0 || len(p.suffix) > 0) {
				exp = row.exp - (p.minInt - 1)
			}
		}
		scaled = cur.shift(-exp)
		if scaled.intLen() >= 2 {
			scaled = scaled.roundFrac(0)
		} else {
			scaled = scaled.roundSignificant(2).trimFrac(0)
		}
		if exp > 0 && scaled.intLen() > row.patterns[string(Other)].minInt {
			cur = scaled.shift(exp) // rounded up to the next magnitude
			continue
		}
		break
	}
	return scaled, exp, cur
}

// compactRow returns the row for the largest magnitude not above 10^exp, or
// nil when the number is below the smallest.
func (ns *numberSystem) compactRow(exp int) *compactRow {
	var best *compactRow
	for i := range ns.compact {
		if ns.compact[i].exp <= exp {
			best = &ns.compact[i]
		}
	}
	return best
}

// render writes x with the layout: rounding, grouping, symbols, digits and
// the pattern's affixes.
func (l *Locale) render(ly layout, x Decimal) string {
	p := ly.pat
	v := x.abs().shift(p.scale).roundFrac(ly.max).trimFrac(ly.min)
	ints := v.ints
	if len(ints) < p.minInt {
		ints = strings.Repeat("0", p.minInt-len(ints)) + ints
	}
	decSym, groupSym := ly.ns.symbols["decimal"], ly.ns.symbols["group"]
	if ly.isMoney {
		if s := ly.ns.symbols["currencyDecimal"]; s != "" {
			decSym = s
		}
		if s := ly.ns.symbols["currencyGroup"]; s != "" {
			groupSym = s
		}
	}
	mg := max(l.minGrouping, ly.minGrouping, 1)
	if ly.grouping && p.primary > 0 && len(ints) >= p.primary+mg {
		ints = group(ints, p.primary, p.secondary, groupSym)
	}
	core := ints
	if v.frac != "" {
		core += decSym + v.frac
	}
	core = l.mapDigits(core, ly.sysID)

	prefix, suffix := p.prefix, p.suffix
	var lead string
	if x.neg {
		if p.hasNeg {
			prefix, suffix = p.negPrefix, p.negSuffix
		} else {
			lead = ly.ns.symbols["minusSign"]
		}
	}
	var b strings.Builder
	b.WriteString(lead)
	l.writeAffix(&b, prefix, ly, true)
	b.WriteString(core)
	l.writeAffix(&b, suffix, ly, false)
	return b.String()
}

// group inserts sep into the integer digits: the first group from the right
// has primary digits, the others secondary.
func group(ints string, primary, secondary int, sep string) string {
	var parts []string
	end := len(ints)
	size := primary
	for end > size {
		parts = append(parts, ints[end-size:end])
		end -= size
		size = secondary
	}
	parts = append(parts, ints[:end])
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, sep)
}

// writeCurrency writes the currency text cur; when it touches the number it
// gets the locale's currency spacing on the number's side.
func (l *Locale) writeCurrency(b *strings.Builder, cur string, touches, isPrefix bool) {
	switch {
	case touches && isPrefix:
		b.WriteString(cur)
		if r, _ := utf8.DecodeLastRuneInString(cur); !unicode.IsSymbol(r) && !unicode.IsSpace(r) {
			b.WriteString(l.spacingAfter)
		}
	case touches:
		if r, _ := utf8.DecodeRuneInString(cur); !unicode.IsSymbol(r) && !unicode.IsSpace(r) {
			b.WriteString(l.spacingBefore)
		}
		b.WriteString(cur)
	default:
		b.WriteString(cur)
	}
}

// writeAffix writes a prefix or suffix. A currency sign touching the number
// gets the locale's currency spacing when the currency text ends (before the
// number) or starts (after it) with a letter-like character.
func (l *Locale) writeAffix(b *strings.Builder, a affix, ly layout, isPrefix bool) {
	for i, pt := range a {
		switch pt.kind {
		case partText:
			b.WriteString(pt.text)
		case partCurrency:
			cur := ly.currency
			if cur == "" {
				continue
			}
			l.writeCurrency(b, cur, (isPrefix && i == len(a)-1) || (!isPrefix && i == 0), isPrefix)
		case partPercent:
			b.WriteString(ly.ns.symbols["percentSign"])
		case partPermille:
			b.WriteString(ly.ns.symbols["perMille"])
		case partMinus:
			b.WriteString(ly.ns.symbols["minusSign"])
		case partPlus:
			b.WriteString(ly.ns.symbols["plusSign"])
		}
	}
}
