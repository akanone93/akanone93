// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package l10n is Plux's localisation engine: BCP 47 locale resolution along
// the CLDR parent chain, plural rules, and number, percent, compact, money
// and date formatting over the locale data of schema/l10n/cldr (ADR-0066,
// I18N-001, I18N-002, I18N-011). It is a pure library: it reads only the data
// embedded in it or handed to Parse, and its output depends on nothing else,
// so the compiler, the server, the CLI and the Dart runtime agree on it
// byte for byte (QA-003).
package l10n

//go:generate go run ./internal/copydata ../../../schema/l10n/cldr data

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

//go:embed data/*.json
var dataFS embed.FS

// The errors the package returns, to be tested with errors.Is.
var (
	// ErrInvalidTag is a locale tag that is not well-formed BCP 47.
	ErrInvalidTag = errors.New("l10n: invalid locale tag")
	// ErrNoLocale is a locale with no data and no fallback.
	ErrNoLocale = errors.New("l10n: no data for locale")
	// ErrInvalidDecimal is a string that is not a decimal number.
	ErrInvalidDecimal = errors.New("l10n: invalid decimal")
	// ErrInvalidData is locale data that does not have the expected shape.
	ErrInvalidData = errors.New("l10n: invalid locale data")
	// ErrUnsupported is a request the engine does not implement: an unknown
	// calendar, numbering system or style, or a pattern letter it lacks.
	ErrUnsupported = errors.New("l10n: unsupported")
	// ErrInvalidCurrency is a currency that is not an ISO 4217 code.
	ErrInvalidCurrency = errors.New("l10n: invalid currency")
	// ErrInvalidOption is an option out of range.
	ErrInvalidOption = errors.New("l10n: invalid option")
)

// Locale is the formatting data of one CLDR locale. It is immutable after
// Load or Parse and safe for concurrent use.
type Locale struct {
	id             string
	rtl            bool
	defaultSystem  string
	digits         map[string]string // numbering system -> its ten digits
	minGrouping    int
	spacingBefore  string
	spacingAfter   string
	systems        map[string]*numberSystem
	currencies     map[string]currencyNames
	currencyDigits map[string]int
	territories    map[string]string
	calendars      map[string]*calendar
	cardinal       ruleSet
	ordinal        ruleSet
}

// rawLocale is the JSON shape of a locale file.
type rawLocale struct {
	Locale    string `json:"locale"`
	Direction string `json:"direction"`
	Numbering struct {
		Default string `json:"default"`
		Digits  string `json:"digits"`
	} `json:"numbering"`
	MinimumGroupingDigits int `json:"minimumGroupingDigits"`
	CurrencySpacing       struct {
		Before string `json:"before"`
		After  string `json:"after"`
	} `json:"currencySpacing"`
	Numbers        map[string]rawNumberSystem `json:"numbers"`
	Currencies     map[string]currencyNames   `json:"currencies"`
	CurrencyDigits map[string]int             `json:"currencyDigits"`
	Territories    map[string]string          `json:"territories"`
	Calendars      map[string]rawCalendar     `json:"calendars"`
	Plurals        struct {
		Cardinal map[string]string `json:"cardinal"`
		Ordinal  map[string]string `json:"ordinal"`
	} `json:"plurals"`
}

type rawNumberSystem struct {
	Symbols      map[string]string            `json:"symbols"`
	Decimal      string                       `json:"decimal"`
	Percent      string                       `json:"percent"`
	Currency     string                       `json:"currency"`
	Accounting   string                       `json:"accounting"`
	CompactShort map[string]map[string]string `json:"compactShort"`
}

// currencyNames are the display strings of one currency; Symbol and Narrow
// default to the code, Name to the code.
type currencyNames struct {
	Name   string            `json:"name"`
	Symbol string            `json:"symbol"`
	Narrow string            `json:"narrow"`
	Plural map[string]string `json:"plural"`
}

// Load returns the data of the best embedded locale for tag, following
// Resolve's chain (de-DE loads de). It returns ErrNoLocale when no locale on
// the chain has data.
func Load(tag string) (*Locale, error) {
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	id, err := idx.resolve(tag, idx.locales, "")
	if err != nil {
		return nil, err
	}
	b, err := dataFS.ReadFile("data/" + id + ".json")
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrNoLocale, id, err)
	}
	return Parse(b)
}

// Locales lists the locales with embedded data, sorted.
func Locales() ([]string, error) {
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	out := append([]string(nil), idx.locales...)
	sort.Strings(out)
	return out, nil
}

// Parse builds a Locale from the JSON of a locale file, for data that does
// not come from the embedded set (the bundle's LocaleData).
func Parse(data []byte) (*Locale, error) {
	var raw rawLocale
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidData, err)
	}
	if raw.Locale == "" || len(raw.Numbers) == 0 || len(raw.Calendars) == 0 {
		return nil, fmt.Errorf("%w: locale file is incomplete", ErrInvalidData)
	}
	l := &Locale{
		id:             raw.Locale,
		rtl:            raw.Direction == "rtl",
		defaultSystem:  raw.Numbering.Default,
		digits:         map[string]string{"latn": "0123456789"},
		minGrouping:    raw.MinimumGroupingDigits,
		spacingBefore:  raw.CurrencySpacing.Before,
		spacingAfter:   raw.CurrencySpacing.After,
		systems:        make(map[string]*numberSystem, len(raw.Numbers)),
		currencies:     raw.Currencies,
		currencyDigits: raw.CurrencyDigits,
		territories:    raw.Territories,
		calendars:      make(map[string]*calendar, len(raw.Calendars)),
	}
	if l.defaultSystem != "latn" {
		if len([]rune(raw.Numbering.Digits)) != 10 {
			return nil, fmt.Errorf("%w: numbering system %q has no ten digits", ErrInvalidData, l.defaultSystem)
		}
		l.digits[l.defaultSystem] = raw.Numbering.Digits
	}
	for id, rs := range raw.Numbers {
		ns, err := compileNumberSystem(rs)
		if err != nil {
			return nil, fmt.Errorf("%w: numbers %s: %w", ErrInvalidData, id, err)
		}
		l.systems[id] = ns
	}
	if l.systems["latn"] == nil || l.systems[l.defaultSystem] == nil {
		return nil, fmt.Errorf("%w: numbers lack latn or the default system", ErrInvalidData)
	}
	for id, rc := range raw.Calendars {
		c, err := compileCalendar(rc)
		if err != nil {
			return nil, fmt.Errorf("%w: calendar %s: %w", ErrInvalidData, id, err)
		}
		l.calendars[id] = c
	}
	var err error
	if l.cardinal, err = compileRules(raw.Plurals.Cardinal); err != nil {
		return nil, err
	}
	if l.ordinal, err = compileRules(raw.Plurals.Ordinal); err != nil {
		return nil, err
	}
	return l, nil
}

// ID returns the CLDR locale the data is for (de for a request for de-DE).
func (l *Locale) ID() string { return l.id }

// IsRTL reports whether the locale's layout direction is right to left.
func (l *Locale) IsRTL() bool { return l.rtl }

// DefaultNumberingSystem returns the locale's own numbering system, such as
// latn or arabext. Formatting uses latn unless asked for another.
func (l *Locale) DefaultNumberingSystem() string { return l.defaultSystem }

// TerritoryName returns the locale's name for an ISO 3166 region code.
func (l *Locale) TerritoryName(code string) (string, bool) {
	n, ok := l.territories[code]
	return n, ok
}

// CurrencyName returns the locale's name for a currency, falling back to the
// code itself.
func (l *Locale) CurrencyName(code string) string {
	if n := l.currencies[code].Name; n != "" {
		return n
	}
	return code
}

// CurrencyDigits returns the ISO 4217 minor-unit digits of a currency.
func (l *Locale) CurrencyDigits(code string) int {
	if d, ok := l.currencyDigits[code]; ok {
		return d
	}
	return 2
}

// system returns the number system for an option value: "" and "latn" are
// Latin digits, "default" or the locale's own id its native digits.
func (l *Locale) system(opt string) (id string, ns *numberSystem, err error) {
	switch opt {
	case "", "latn":
		id = "latn"
	case "default":
		id = l.defaultSystem
	default:
		id = opt
	}
	ns = l.systems[id]
	if ns == nil {
		return "", nil, fmt.Errorf("%w: numbering system %q for locale %s", ErrUnsupported, opt, l.id)
	}
	return id, ns, nil
}

// mapDigits replaces the ASCII digits of s by those of a numbering system.
func (l *Locale) mapDigits(s, system string) string {
	d := l.digits[system]
	if system == "latn" || d == "" {
		return s
	}
	runes := []rune(d)
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			r = runes[r-'0']
		}
		out = append(out, r)
	}
	return string(out)
}
