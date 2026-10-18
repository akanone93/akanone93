// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"slices"
)

// L10nReport is the localisation report of a compilation (I18N-008,
// ADR-0068): per locale and plugin, how many translation keys the
// translations cover, how many are approved, which are missing and which a
// release that requires approval leaves out. It is deterministic: the same
// project always yields the same report.
type L10nReport struct {
	// DefaultLocale is the app's default locale, whose texts always ship.
	DefaultLocale string `json:"defaultLocale"`
	// RequireApproved is true when the app requires approved translations.
	RequireApproved bool `json:"requireApproved"`
	// Locales has one entry per locale and plugin, sorted by locale, with
	// pseudo-locales last, then by plugin.
	Locales []L10nCoverage `json:"locales"`
	// UnusedKeys are the names of the keys the default locale translates
	// and nothing references (PLX-1608), sorted.
	UnusedKeys []string `json:"unusedKeys"`
}

// L10nCoverage is the coverage of one locale over the keys one plugin's
// documents reference, or over all keys of the app.
type L10nCoverage struct {
	Locale string `json:"locale"`
	// Plugin is the plugin's key; empty for all the keys of the app.
	Plugin string `json:"plugin"`
	// Pseudo is true for a generated pseudo-locale (I18N-010).
	Pseudo bool `json:"pseudo"`
	// Keys is how many keys are in scope.
	Keys int `json:"keys"`
	// Translated counts the keys that have a message in the locale,
	// shipped or not.
	Translated int `json:"translated"`
	// Approved counts the translated keys whose state is approved.
	Approved int `json:"approved"`
	// Missing counts the keys with no message in the locale.
	Missing int `json:"missing"`
	// LeftOut counts the messages the release leaves out because they are
	// not approved (PLX-1610).
	LeftOut int `json:"leftOutUnapproved"`
	// MissingKeys and LeftOutKeys name those keys; they are listed on the
	// entry of the whole app only.
	MissingKeys []string `json:"missingKeys,omitempty"`
	LeftOutKeys []string `json:"leftOutKeys,omitempty"`
}

// reportScope is the set of keys one report entry covers.
type reportScope struct {
	plugin string
	keys   map[string]bool
}

// l10nReport builds the report, or nil when the app has no translation keys.
func (u *unit) l10nReport() *L10nReport {
	tk := u.project.TranslationKeys
	if tk == nil || tk.Doc == nil || u.l10n.uses == nil {
		return nil
	}
	app := u.project.App.Doc
	names := make(map[string]string, len(tk.Doc.Keys))
	all := make(map[string]bool, len(tk.Doc.Keys))
	for _, k := range tk.Doc.Keys {
		names[k.ID] = k.Key
		all[k.ID] = true
	}
	scopes := u.reportScopes(all)
	order := sortedKeys(u.reportLocales())
	slices.SortStableFunc(order, func(a, b string) int {
		return boolInt(u.l10n.stat(a).pseudo) - boolInt(u.l10n.stat(b).pseudo)
	})
	r := &L10nReport{DefaultLocale: app.DefaultLocale, RequireApproved: requireApproved(app), Locales: []L10nCoverage{}, UnusedKeys: []string{}}
	for _, tag := range order {
		st := u.l10n.stat(tag)
		for _, sc := range scopes {
			r.Locales = append(r.Locales, coverageOf(tag, st, sc, names))
		}
	}
	for _, id := range u.l10n.unused {
		r.UnusedKeys = append(r.UnusedKeys, names[id])
	}
	slices.Sort(r.UnusedKeys)
	return r
}

// reportScopes lists the scopes of the report: the whole app (all keys),
// then each plugin that uses keys.
func (u *unit) reportScopes(all map[string]bool) []reportScope {
	scopes := []reportScope{{"", all}}
	for _, pl := range u.plugins {
		if keys := u.l10n.uses[pl.key]; len(keys) > 0 {
			scopes = append(scopes, reportScope{pl.key, keys})
		}
	}
	return scopes
}

// reportLocales is the set of locale tags the report covers: the default,
// the supported ones and any with a translation file.
func (u *unit) reportLocales() map[string]bool {
	app := u.project.App.Doc
	tags := map[string]bool{app.DefaultLocale: true}
	for _, tag := range app.SupportedLocales {
		tags[tag] = true
	}
	for tag := range u.l10n.locales {
		tags[tag] = true
	}
	return tags
}

// coverageOf counts how the keys of one scope stand in one locale. The key
// lists are filled on the entry of the whole app only.
func coverageOf(tag string, st *localeStat, sc reportScope, names map[string]string) L10nCoverage {
	c := L10nCoverage{Locale: tag, Plugin: sc.plugin, Pseudo: st.pseudo, Keys: len(sc.keys)}
	for _, id := range sortedKeys(sc.keys) {
		switch {
		case !st.present[id]:
			c.Missing++
			if sc.plugin == "" {
				c.MissingKeys = append(c.MissingKeys, names[id])
			}
			continue
		case st.leftOut[id]:
			c.LeftOut++
			if sc.plugin == "" {
				c.LeftOutKeys = append(c.LeftOutKeys, names[id])
			}
		}
		c.Translated++
		if st.approved[id] {
			c.Approved++
		}
	}
	slices.Sort(c.MissingKeys)
	slices.Sort(c.LeftOutKeys)
	return c
}

// boolInt is 1 for true and 0 for false.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
