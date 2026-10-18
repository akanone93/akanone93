// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pxl

import (
	"fmt"
	"math"
	"strconv"

	"github.com/nightCode42/plux3/backend/internal/l10n"
	"github.com/nightCode42/plux3/backend/internal/pxl/decimal"
)

// LocaleContext is what the format and l10n groups need from the user's
// locale (PXL-006): the CLDR data to format with and, for t, the lookup of
// translated messages. The zero value is unusable; pass nil to evaluate
// without a locale, where those functions fail with ErrorUnsupported.
type LocaleContext struct {
	// Locale formats numbers, money and dates and selects plural forms.
	Locale *l10n.Locale
	// Translate returns the message of a translation key formatted with the
	// arguments, or false when the key has no message.
	Translate func(key string, args map[string]string) (string, bool)
}

// EvalIn is Eval with the locale the format and l10n groups use.
func (p *Program) EvalIn(inputs map[string]Value, lim Limits, loc *LocaleContext) (Value, error) {
	m := &vm{p: p, lim: lim, inputs: inputs, loc: loc, stack: make([]Value, 0, min(p.MaxStack, len(p.Code))), locals: make([]any, p.Locals)}
	v, err := m.run()
	if err != nil {
		return nil, err
	}
	return v, nil
}

// locale returns the locale of the evaluation or an unsupported error.
func (m *vm) locale() (*l10n.Locale, *EvalError) {
	if m.loc == nil || m.loc.Locale == nil {
		return nil, evalErr(ErrorUnsupported, "the evaluation has no locale")
	}
	return m.loc.Locale, nil
}

// l10nErr maps an engine error to an evaluation error.
func l10nErr(err error) *EvalError {
	return evalErr(ErrorInvalidArgument, "%v", err)
}

// decimalOf converts a number argument for the engine.
func decimalOf(v Value) (l10n.Decimal, *EvalError) {
	var s string
	switch x := v.(type) {
	case int64:
		return l10n.DecimalFromInt(x), nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return l10n.Decimal{}, evalErr(ErrorInvalidArgument, "%v cannot be formatted", x)
		}
		s = strconv.FormatFloat(x, 'f', -1, 64)
	case decimal.Decimal:
		s = x.String()
	default:
		return l10n.Decimal{}, evalErr(ErrorInvalidProgram, "not a number")
	}
	d, err := l10n.ParseDecimal(s)
	if err != nil {
		return l10n.Decimal{}, l10nErr(err)
	}
	return d, nil
}

// formatNumberFn formats a number argument at index 0, with fraction digits
// from argument 1 when given.
func formatNumberFn(m *vm, a []Value) (Value, *EvalError) {
	loc, e := m.locale()
	if e != nil {
		return nil, e
	}
	d, e := decimalOf(a[0])
	if e != nil {
		return nil, e
	}
	var opts l10n.NumberOptions
	if len(a) > 1 {
		n := arg[int64](a, 1)
		if n < 0 || n > 20 {
			return nil, evalErr(ErrorInvalidArgument, "fraction digits %d outside 0 to 20", n)
		}
		opts.Fraction = &l10n.FractionDigits{Min: int(n), Max: int(n)}
	}
	s, err := loc.FormatNumber(d, opts)
	if err != nil {
		return nil, l10nErr(err)
	}
	return s, nil
}

// formatWith builds a one-number formatter.
func formatWith(f func(*l10n.Locale, l10n.Decimal) (string, error)) builtinFn {
	return func(m *vm, a []Value) (Value, *EvalError) {
		loc, e := m.locale()
		if e != nil {
			return nil, e
		}
		d, e := decimalOf(a[0])
		if e != nil {
			return nil, e
		}
		s, err := f(loc, d)
		if err != nil {
			return nil, l10nErr(err)
		}
		return s, nil
	}
}

// calendarDate converts a date and, when withTime, a date-time with its own
// UTC offset to the engine's Gregorian fields.
func calendarDate(v Value) (d l10n.CalendarDate, zone string, hasTime bool) {
	var days, secs int64
	switch x := v.(type) {
	case Date:
		days = int64(x)
	case DateTime:
		local := x.Millis + int64(x.Offset)*msPerMinute
		days = floorDiv(local, msPerDay)
		secs = (local - days*msPerDay) / msPerSecond
		hasTime = true
		sign, off := '+', int64(x.Offset)
		if off < 0 {
			sign, off = '-', -off
		}
		zone = fmt.Sprintf("GMT%c%02d:%02d", sign, off/60, off%60)
	}
	y, mo, dd := civilFromDays(days)
	return l10n.GregorianDate(int(y), int(mo), int(dd), int(secs/3600), int(secs/60%60), int(secs%60)), zone, hasTime
}

// formatDateFn implements the four format.date overloads.
func formatDateFn(m *vm, a []Value) (Value, *EvalError) {
	loc, e := m.locale()
	if e != nil {
		return nil, e
	}
	style := "medium"
	if len(a) > 1 {
		style = arg[string](a, 1)
	}
	d, zone, hasTime := calendarDate(a[0])
	opts := l10n.DateOptions{DateStyle: style, ZoneName: zone}
	if hasTime {
		opts.TimeStyle = style
	}
	s, err := loc.FormatDate(d, "gregorian", opts)
	if err != nil {
		return nil, l10nErr(err)
	}
	return s, nil
}

// localeFns implements the format and l10n groups on the locale of the
// evaluation (PXL-006, I18N-001, I18N-011).
func localeFns() map[string]builtinFn {
	num := formatWith
	percent := num(func(l *l10n.Locale, d l10n.Decimal) (string, error) { return l.FormatPercent(d, l10n.NumberOptions{}) })
	compact := num(func(l *l10n.Locale, d l10n.Decimal) (string, error) { return l.FormatCompact(d, l10n.NumberOptions{}) })
	return map[string]builtinFn{
		"format.number(int)":              formatNumberFn,
		"format.number(double)":           formatNumberFn,
		"format.number(decimal)":          formatNumberFn,
		"format.number(double,int)":       formatNumberFn,
		"format.number(decimal,int)":      formatNumberFn,
		"format.percent(double)":          percent,
		"format.percent(decimal)":         percent,
		"format.compact(int)":             compact,
		"format.compact(double)":          compact,
		"format.compact(decimal)":         compact,
		"format.date(date)":               formatDateFn,
		"format.date(dateTime)":           formatDateFn,
		"format.date(date,DateStyle)":     formatDateFn,
		"format.date(dateTime,DateStyle)": formatDateFn,
		"format.money(money)": func(m *vm, a []Value) (Value, *EvalError) {
			loc, e := m.locale()
			if e != nil {
				return nil, e
			}
			mv := arg[Money](a, 0)
			d, e := decimalOf(mv.Amount)
			if e != nil {
				return nil, e
			}
			s, err := loc.FormatMoney(d, mv.Currency, l10n.MoneyOptions{})
			if err != nil {
				return nil, l10nErr(err)
			}
			return s, nil
		},
		"t(string)":                    translateFn,
		"t(string,map<string,string>)": translateFn,
		"plural(int,map<string,string>)": func(m *vm, a []Value) (Value, *EvalError) {
			loc, e := m.locale()
			if e != nil {
				return nil, e
			}
			forms := arg[Map](a, 1)
			cat := string(loc.PluralCardinal(l10n.DecimalFromInt(arg[int64](a, 0))))
			for _, k := range []string{cat, "other"} {
				if s, ok := forms[k].(string); ok {
					return s, nil
				}
			}
			return nil, evalErr(ErrorInvalidArgument, "no form for the plural category %q and no other form", cat)
		},
	}
}

// translateFn implements t(key) and t(key, args).
func translateFn(m *vm, a []Value) (Value, *EvalError) {
	if m.loc == nil || m.loc.Translate == nil {
		return nil, evalErr(ErrorUnsupported, "the evaluation has no translations")
	}
	args := map[string]string{}
	if len(a) > 1 {
		for k, v := range arg[Map](a, 1) {
			s, _ := v.(string)
			args[k] = s
		}
	}
	key := arg[string](a, 0)
	s, ok := m.loc.Translate(key, args)
	if !ok {
		return nil, evalErr(ErrorInvalidArgument, "translation %q is missing", key)
	}
	return s, nil
}
