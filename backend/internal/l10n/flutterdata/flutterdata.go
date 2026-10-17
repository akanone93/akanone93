// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package flutterdata embeds Flutter's widget strings per locale
// (schema/l10n/flutter, I18N-002, ADR-0066 § Flutter's widget strings), for
// the compiler to carry in the app bundle. The files are a byte-for-byte
// copy made by go generate.
package flutterdata

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:generate go run ../internal/copydata ../../../../schema/l10n/flutter .

//go:embed *.json
var files embed.FS

// ErrNoStrings is a locale Flutter has no widget strings for.
var ErrNoStrings = errors.New("flutterdata: no widget strings for the locale")

// Strings returns the JSON of Flutter's widget strings for the exact CLDR
// locale tag (no fallback; the caller walks the parent chain).
func Strings(tag string) ([]byte, error) {
	if strings.ContainsAny(tag, "/\\") {
		return nil, fmt.Errorf("%w: %q", ErrNoStrings, tag)
	}
	b, err := files.ReadFile(tag + ".json")
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrNoStrings, tag)
	}
	return b, nil
}

// Locales lists the locales with widget strings, sorted.
func Locales() []string {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(out)
	return out
}
