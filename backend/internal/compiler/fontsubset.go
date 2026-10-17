// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"crypto/sha256"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/icons"
)

// scriptRanges are the Unicode blocks of each script, as inclusive pairs of
// code points, keyed by ISO 15924 code (CMP-032). Hans, Hant, Jpan and Kore
// are the CJK combinations. A script's ranges cover its letters, marks and
// digits in full.
var scriptRanges = map[string][][2]rune{
	"Latn": {{0x0020, 0x024F}, {0x1E00, 0x1EFF}, {0x2C60, 0x2C7F}, {0xA720, 0xA7FF}},
	"Cyrl": {{0x0400, 0x052F}, {0x1C80, 0x1C8F}, {0x2DE0, 0x2DFF}, {0xA640, 0xA69F}},
	"Grek": {{0x0370, 0x03FF}, {0x1F00, 0x1FFF}},
	"Arab": {{0x0600, 0x06FF}, {0x0750, 0x077F}, {0x08A0, 0x08FF}, {0xFB50, 0xFDFF}, {0xFE70, 0xFEFF}},
	"Hebr": {{0x0590, 0x05FF}, {0xFB1D, 0xFB4F}},
	"Armn": {{0x0530, 0x058F}, {0xFB13, 0xFB17}},
	"Geor": {{0x10A0, 0x10FF}, {0x1C90, 0x1CBF}, {0x2D00, 0x2D2F}},
	"Ethi": {{0x1200, 0x139F}, {0x2D80, 0x2DDF}, {0xAB00, 0xAB2F}},
	"Cher": {{0x13A0, 0x13FF}, {0xAB70, 0xABBF}},
	"Deva": {{0x0900, 0x097F}, {0xA8E0, 0xA8FF}},
	"Beng": {{0x0980, 0x09FF}},
	"Guru": {{0x0A00, 0x0A7F}},
	"Gujr": {{0x0A80, 0x0AFF}},
	"Orya": {{0x0B00, 0x0B7F}},
	"Taml": {{0x0B80, 0x0BFF}},
	"Telu": {{0x0C00, 0x0C7F}},
	"Knda": {{0x0C80, 0x0CFF}},
	"Mlym": {{0x0D00, 0x0D7F}},
	"Sinh": {{0x0D80, 0x0DFF}},
	"Thai": {{0x0E00, 0x0E7F}},
	"Laoo": {{0x0E80, 0x0EFF}},
	"Mymr": {{0x1000, 0x109F}, {0xA9E0, 0xA9FF}, {0xAA60, 0xAA7F}},
	"Khmr": {{0x1780, 0x17FF}, {0x19E0, 0x19FF}},
	"Mong": {{0x1800, 0x18AF}},
	"Hans": hanRanges,
	"Hant": hanRanges,
	"Jpan": append(slices.Clone(hanRanges), [2]rune{0x3040, 0x30FF}, [2]rune{0x31F0, 0x31FF}),
	"Kore": append(slices.Clone(hanRanges), [2]rune{0x1100, 0x11FF}, [2]rune{0x3130, 0x318F}, [2]rune{0xAC00, 0xD7AF}),
}

// hanRanges are the CJK ideograph blocks and CJK punctuation and forms.
var hanRanges = [][2]rune{{0x2E80, 0x2FDF}, {0x3000, 0x303F}, {0x3400, 0x4DBF}, {0x4E00, 0x9FFF}, {0xF900, 0xFAFF}, {0xFF00, 0xFFEF}, {0x20000, 0x2A6DF}}

// commonRanges are kept for every script: ASCII, Latin-1 punctuation, general
// punctuation and currency symbols, which every UI shows beside any script.
var commonRanges = [][2]rune{{0x0020, 0x007F}, {0x00A0, 0x00BF}, {0x2000, 0x206F}, {0x20A0, 0x20CF}}

// languageScript is the script of each language CLDR has locales for whose
// script is not Latin; every other language is written in Latin.
var languageScript = map[string]string{
	"am": "Ethi", "ti": "Ethi", "ar": "Arab", "fa": "Arab", "ps": "Arab", "sd": "Arab", "ur": "Arab",
	"as": "Beng", "bn": "Beng", "ba": "Cyrl", "be": "Cyrl", "bg": "Cyrl", "cv": "Cyrl", "kk": "Cyrl",
	"ky": "Cyrl", "mk": "Cyrl", "mn": "Cyrl", "ru": "Cyrl", "sr": "Cyrl", "uk": "Cyrl", "chr": "Cher",
	"el": "Grek", "gu": "Gujr", "he": "Hebr", "hi": "Deva", "kok": "Deva", "mr": "Deva", "ne": "Deva",
	"hy": "Armn", "ja": "Jpan", "ka": "Geor", "km": "Khmr", "kn": "Knda", "ko": "Kore", "lo": "Laoo",
	"ml": "Mlym", "my": "Mymr", "shn": "Mymr", "or": "Orya", "pa": "Guru", "si": "Sinh", "ta": "Taml",
	"te": "Telu", "th": "Thai", "yue": "Hant", "zh": "Hans",
}

// scriptOf returns the script the locale tag is written in: its script
// subtag, else the language's script; "" when the tag is empty.
func scriptOf(tag string) string {
	parts := strings.Split(tag, "-")
	if parts[0] == "" {
		return ""
	}
	for _, p := range parts[1:] {
		if len(p) == 4 {
			if _, ok := scriptRanges[p]; ok {
				return p
			}
		}
	}
	if s, ok := languageScript[parts[0]]; ok {
		return s
	}
	return "Latn"
}

// textRanges returns the code point ranges of the scripts of tags, with the
// common ranges: what a text font must cover for the app's locales.
func textRanges(tags []string) [][2]rune {
	scripts := map[string]bool{}
	for _, t := range tags {
		if s := scriptOf(t); s != "" {
			scripts[s] = true
		}
	}
	if len(scripts) == 0 {
		return nil
	}
	out := slices.Clone(commonRanges)
	for _, s := range sortedKeys(scripts) {
		out = append(out, scriptRanges[s]...)
	}
	return out
}

// textFont subsets a TrueType text font to the scripts of the app's locales
// (CMP-032), keeping the result as a file of the compilation. A font that
// cannot be subset (CFF outlines, a damaged table) is indexed as uploaded.
func (u *unit) textFont(data []byte) []byte {
	app := u.project.App.Doc
	ranges := textRanges(append(slices.Clone(app.SupportedLocales), app.DefaultLocale))
	if ranges == nil {
		return data
	}
	sub, err := icons.SubsetCovering(data, ranges)
	if err != nil || len(sub) >= len(data) {
		return data
	}
	u.files[sha256.Sum256(sub)] = sub
	return sub
}
