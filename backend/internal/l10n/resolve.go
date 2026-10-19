// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// index is the part of index.json that locale resolution needs.
type index struct {
	locales       []string
	have          map[string]bool
	parents       map[string]string
	likelyScripts map[string]string
}

func loadIndex() (*index, error) {
	b, err := dataFS.ReadFile("data/index.json")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidData, err)
	}
	var raw struct {
		Locales       []string          `json:"locales"`
		ParentLocales map[string]string `json:"parentLocales"`
		LikelyScripts map[string]string `json:"likelyScripts"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("%w: index.json: %w", ErrInvalidData, err)
	}
	idx := &index{locales: raw.Locales, have: map[string]bool{}, parents: raw.ParentLocales, likelyScripts: raw.LikelyScripts}
	for _, l := range raw.Locales {
		idx.have[l] = true
	}
	return idx, nil
}

// languageAliases are the deprecated language subtags BCP 47 canonicalisation
// replaces.
var languageAliases = map[string]string{
	"iw": "he", "in": "id", "ji": "yi", "jw": "jv", "mo": "ro", "tl": "fil", "nb": "no",
}

// Canonicalize returns the canonical form of a BCP 47 tag: language in lower
// case, script in title case, region in upper case, "_" read as "-",
// deprecated language subtags replaced. Variants and extensions are dropped:
// they do not select locale data.
func Canonicalize(tag string) (string, error) {
	parts := strings.Split(strings.ReplaceAll(tag, "_", "-"), "-")
	bad := func() (string, error) { return "", fmt.Errorf("%w: %q", ErrInvalidTag, tag) }
	if slices.Contains(parts, "") {
		return bad()
	}
	lang := strings.ToLower(parts[0])
	if (len(lang) != 2 && len(lang) != 3) || !isAlpha(lang) || lang == "und" {
		return bad()
	}
	if a, ok := languageAliases[lang]; ok {
		lang = a
	}
	out := []string{lang}
	rest := parts[1:]
	if len(rest) > 0 && len(rest[0]) == 4 && isAlpha(rest[0]) {
		out = append(out, strings.ToUpper(rest[0][:1])+strings.ToLower(rest[0][1:]))
		rest = rest[1:]
	}
	if len(rest) > 0 && ((len(rest[0]) == 2 && isAlpha(rest[0])) || (len(rest[0]) == 3 && allDigits(rest[0]))) {
		out = append(out, strings.ToUpper(rest[0]))
		rest = rest[1:]
	}
	for _, p := range rest {
		if len(p) == 1 && isAlnum(p) {
			break // an extension: ignored with everything after it
		}
		if len(p) < 2 || len(p) > 8 || !isAlnum(p) {
			return bad()
		}
	}
	return strings.Join(out, "-"), nil
}

func isAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i] | 0x20
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

func isAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i] | 0x20
		if (c < 'a' || c > 'z') && (s[i] < '0' || s[i] > '9') {
			return false
		}
	}
	return true
}

// chain returns the locales to try for a canonical tag, most specific first:
// the tag with its likely script (zh-TW is zh-Hant-TW), then each CLDR
// parent (explicit in parentLocales, else the tag minus its last subtag).
func (idx *index) chain(tag string) []string {
	parts := strings.Split(tag, "-")
	if len(parts) == 2 && len(parts[1]) != 4 {
		if s, ok := idx.likelyScripts[tag]; ok {
			tag = parts[0] + "-" + s + "-" + parts[1]
		}
	}
	var out []string
	for tag != "" {
		out = append(out, tag)
		if p, ok := idx.parents[tag]; ok {
			if p == "root" {
				break
			}
			tag = p
			continue
		}
		i := strings.LastIndexByte(tag, '-')
		if i < 0 {
			break
		}
		tag = tag[:i]
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (idx *index) resolve(tag string, supported []string, fallback string) (string, error) {
	want, err := Canonicalize(tag)
	if err != nil {
		return "", err
	}
	sup := make([]string, 0, len(supported))
	for _, s := range supported {
		c, err := Canonicalize(s)
		if err != nil {
			return "", fmt.Errorf("supported locale: %w", err)
		}
		sup = append(sup, c)
	}
	chain := idx.chain(want)
	for _, c := range chain {
		if contains(sup, c) {
			return c, nil
		}
	}
	// A supported locale that is a refinement of a locale on the chain
	// (de-DE for a request for de) comes next.
	for _, c := range chain {
		for _, s := range sup {
			if contains(idx.chain(s), c) {
				return s, nil
			}
		}
	}
	if fallback == "" {
		return "", fmt.Errorf("%w: %q", ErrNoLocale, tag)
	}
	return Canonicalize(fallback)
}

// Resolve picks the locale to use for a requested tag out of the locales an
// app supports: the tag canonicalised (BCP 47), then each locale on its CLDR
// parent chain (de-AT, de), then a supported locale that refines one of
// them, then fallback. The result is the canonical form of a supported
// locale or of fallback. With an empty fallback and no match it returns
// ErrNoLocale.
func Resolve(tag string, supported []string, fallback string) (string, error) {
	idx, err := loadIndex()
	if err != nil {
		return "", err
	}
	return idx.resolve(tag, supported, fallback)
}
