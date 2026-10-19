// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"strconv"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/l10n"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// releaseLocale reports whether tag is a canonical BCP 47 tag of one of the
// app's supported locales: what a locale override or a per-locale asset may
// name (I18N-012).
func (u *unit) releaseLocale(tag string) bool {
	canon, err := l10n.Canonicalize(tag)
	if err != nil || canon != tag {
		return false
	}
	for _, s := range u.project.App.Doc.SupportedLocales {
		if c, err := l10n.Canonicalize(s); err == nil && c == tag {
			return true
		}
	}
	return false
}

// checkAssetLocales checks the per-locale files of asset i: each names a
// locale of the release and a file that exists and holds no executable code
// (I18N-012, PLX-1613, SEC-054).
func (u *unit) checkAssetLocales(file string, i int, e schema.AssetEntry) {
	for _, tag := range sortedKeys(e.Locales) {
		ptr := plxerr.Pointer("assets", strconv.Itoa(i), "locales", tag)
		if !u.releaseLocale(tag) {
			u.report(plxerr.LocaleOverrideInvalid, file, ptr, "%q is not a canonical tag of a supported locale", tag)
			continue
		}
		data, ok := u.project.AssetFiles[e.Locales[tag]]
		if !ok {
			u.report(plxerr.LocaleOverrideInvalid, file, ptr, "the file %s for locale %q does not exist", e.Locales[tag], tag)
			continue
		}
		if format := bundle.ExecutableFormat(data); format != "" {
			u.report(plxerr.ExecutableContent, file, ptr, "%s holds %s code", e.Locales[tag], format)
		}
	}
}

// localeFiles returns the per-locale files of an asset that passed
// checkAssetLocales, sorted by tag.
func (u *unit) localeFiles(a *schema.AssetEntry) []localeFile {
	var out []localeFile
	for _, tag := range sortedKeys(a.Locales) {
		data, ok := u.project.AssetFiles[a.Locales[tag]]
		if ok && u.releaseLocale(tag) {
			out = append(out, localeFile{tag: tag, data: data})
		}
	}
	return out
}
