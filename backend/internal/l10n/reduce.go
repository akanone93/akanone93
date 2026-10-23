// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
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
	// Territories are the territory codes whose names to keep; nil keeps
	// every territory, an empty list none.
	Territories []string
	// Skeletons are the date and time skeletons the app formats with; nil
	// keeps every available format of every calendar. For each skeleton the
	// available format FormatDate would pick in each kept calendar is kept,
	// so the result of a listed skeleton is unchanged. A skeleton that does
	// not parse keeps every format. The date and time styles are always kept.
	Skeletons []string
}

// trimFormats deletes the available formats of a calendar that none of the
// skeletons selects.
func trimFormats(cal map[string]any, skeletons []string) {
	af, ok := cal["availableFormats"].(map[string]any)
	if !ok {
		return
	}
	avail := make([]availableFormat, 0, len(af))
	for key := range af {
		sk, err := parseSkeleton(key)
		if err != nil {
			return
		}
		avail = append(avail, availableFormat{key: key, fields: sk})
	}
	sort.Slice(avail, func(i, j int) bool { return avail[i].key < avail[j].key })
	keep := map[string]bool{}
	for _, s := range skeletons {
		want, err := parseSkeleton(s)
		if err != nil {
			return
		}
		date, tm := want.split()
		for _, part := range []skeleton{date, tm} {
			if len(part) == 0 {
				continue
			}
			if i, ok := bestFormat(avail, part); ok {
				keep[avail[i].key] = true
			}
		}
	}
	for key := range af {
		if !keep[key] {
			delete(af, key)
		}
	}
}

// keepKeys deletes the keys of m that are not in keep.
func keepKeys(m map[string]any, keep []string) {
	for k := range m {
		if !slices.Contains(keep, k) {
			delete(m, k)
		}
	}
}

// reduceDoc filters a locale's decoded data in place as Reduce describes.
func reduceDoc(doc map[string]any, o ReduceOptions) {
	if cals, ok := doc["calendars"].(map[string]any); ok {
		keepKeys(cals, append([]string{"gregorian"}, o.Calendars...))
		for _, c := range cals {
			if m, ok := c.(map[string]any); ok && o.Skeletons != nil {
				trimFormats(m, o.Skeletons)
			}
		}
	}
	if terr, ok := doc["territories"].(map[string]any); ok && o.Territories != nil {
		keepKeys(terr, o.Territories)
	}
	cur, ok := doc["currencies"].(map[string]any)
	if !ok {
		return
	}
	if o.Currencies != nil {
		keepKeys(cur, o.Currencies)
	}
	for _, names := range cur {
		if m, ok := names.(map[string]any); ok {
			delete(m, "plural")
		}
	}
}

// Reduce returns the canonical JSON of the data of the best embedded locale
// for tag, reduced to what an app uses, and the CLDR locale it came from.
// The numbers, plurals, date and time styles and currency digits are kept
// whole; calendars, currency names, territory names and the skeleton formats
// are filtered by o, and the
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
	reduceDoc(doc, o)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return nil, "", fmt.Errorf("%w: %s: %w", ErrInvalidData, id, err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), id, nil
}
