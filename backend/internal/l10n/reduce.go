// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// Chain returns the canonical locales to try for tag, most specific first
// (Resolve's chain): de-AT gives de-AT, de.
func Chain(tag string) ([]string, error) {
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	want, err := Canonicalize(tag)
	if err != nil {
		return nil, err
	}
	return idx.chain(want), nil
}

// ReduceOptions say what a bundle's locale data keeps (I18N-002).
type ReduceOptions struct {
	// Calendars are the calendar systems to keep besides gregorian, which
	// is always kept.
	Calendars []string
	// Currencies are the ISO 4217 codes whose names to keep; nil keeps
	// every currency. The digits of every currency are always kept.
	Currencies []string
}

// keepKeys deletes the keys of m that are not in keep.
func keepKeys(m map[string]any, keep []string) {
	for k := range m {
		if !slices.Contains(keep, k) {
			delete(m, k)
		}
	}
}

// Reduce returns the canonical JSON of the data of the best embedded locale
// for tag, reduced to what an app uses, and the CLDR locale it came from.
// The numbers, plurals, date formats, territory names and currency digits
// are kept whole; calendars and currency names are filtered by o, and the
// plural forms of currency names, which neither engine reads, are dropped
// (over a quarter of the bytes of a launch locale, downloaded on every
// device's first launch). The
// output is compact with sorted keys, so it depends on its inputs only
// (CMP-002). It returns ErrNoLocale when no locale on the chain has data.
func Reduce(tag string, o ReduceOptions) (data []byte, resolved string, err error) {
	idx, err := loadIndex()
	if err != nil {
		return nil, "", err
	}
	id, err := idx.resolve(tag, idx.locales, "")
	if err != nil {
		return nil, "", err
	}
	src, err := dataFS.ReadFile("data/" + id + ".json")
	if err != nil {
		return nil, "", fmt.Errorf("%w: %s: %w", ErrNoLocale, id, err)
	}
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, "", fmt.Errorf("%w: %s: %w", ErrInvalidData, id, err)
	}
	if cals, ok := doc["calendars"].(map[string]any); ok {
		keepKeys(cals, append([]string{"gregorian"}, o.Calendars...))
	}
	if cur, ok := doc["currencies"].(map[string]any); ok {
		if o.Currencies != nil {
			keepKeys(cur, o.Currencies)
		}
		for _, names := range cur {
			if m, ok := names.(map[string]any); ok {
				delete(m, "plural")
			}
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return nil, "", fmt.Errorf("%w: %s: %w", ErrInvalidData, id, err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), id, nil
}
