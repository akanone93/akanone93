// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/l10n"
	"github.com/nightCode42/plux3/backend/internal/l10n/literals"
	"github.com/nightCode42/plux3/backend/internal/l10n/mf"
	"github.com/nightCode42/plux3/backend/internal/l10n/pseudo"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// l10nState collects what the localisation report says: which messages each
// locale has, which are approved and which the release leaves out.
type l10nState struct {
	locales map[string]*localeStat
	// unused are the IDs of the keys no document references, sorted.
	unused []string
	// uses are the keys each plugin's documents reference, by plugin key;
	// "" holds those of the app's own documents.
	uses map[string]map[string]bool
}

// localeStat is what one locale's translations document holds.
type localeStat struct {
	present, approved, leftOut map[string]bool
	// pseudo is true for a generated pseudo-locale, whose present set is
	// the messages it generated.
	pseudo bool
}

// stat returns the statistics of a locale, created on first use.
func (s *l10nState) stat(tag string) *localeStat {
	if s.locales == nil {
		s.locales = map[string]*localeStat{}
	}
	st := s.locales[tag]
	if st == nil {
		st = &localeStat{present: map[string]bool{}, approved: map[string]bool{}, leftOut: map[string]bool{}}
		s.locales[tag] = st
	}
	return st
}

// localisation runs the localisation checks of the policy stage: pseudo-
// locales refused in production (PLX-1614), literal text in translatable
// props (PLX-1609) and unused translation keys (PLX-1608).
func (u *unit) localisation() {
	app := u.project.App.Doc
	if u.pseudoRequested(app) && u.opts.Production {
		u.report(plxerr.PseudoLocaleInProduction, "app.json", "/localisation/pseudoLocales",
			"pseudo-locales are for development; this build targets a production environment and ships none")
	}
	for _, n := range u.allNodes() {
		if u.inFocus(n.owner.file()) {
			u.translatableLiterals(n)
		}
	}
	for _, pl := range u.plugins {
		for _, pg := range pl.pages {
			if u.inFocus(pg.file) {
				u.translatableTitle(pg)
				u.translatableMessages(pg.file, pg.doc.Forms)
			}
		}
	}
	comps := make([]*component, 0, len(u.components))
	for _, c := range u.components {
		comps = append(comps, c)
	}
	slices.SortFunc(comps, func(a, b *component) int { return strings.Compare(a.file, b.file) })
	for _, c := range comps {
		if c.doc != nil && u.inFocus(c.file) {
			u.translatableMessages(c.file, c.doc.Forms)
		}
	}
	if u.focus == nil {
		u.unusedKeys()
	}
}

// pseudoRequested reports whether the app turns the pseudo-locales on.
func (*unit) pseudoRequested(app *schema.AppDocument) bool {
	l := app.Localisation
	return l != nil && l.PseudoLocales != nil && *l.PseudoLocales
}

// requireApproved reports whether releases use approved translations only
// (I18N-008).
func requireApproved(app *schema.AppDocument) bool {
	l := app.Localisation
	return l != nil && l.RequireApprovedTranslations != nil && *l.RequireApprovedTranslations
}

// translatableLiterals warns about a literal string in a translatable prop
// of a node (PLX-1609, I18N-007).
func (u *unit) translatableLiterals(n *node) {
	if n.doc == nil || len(u.project.App.Doc.SupportedLocales) < 2 {
		return // an app in one language has nothing to translate
	}
	for _, l := range literals.Node(nodeJSON(n.doc)) {
		u.report(plxerr.LiteralTextUntranslated, n.owner.file(), n.ptr+plxerr.Pointer(l.Path...),
			"%s holds literal text; move it into a translation key (plux l10n extract)", l.Prop)
	}
}

// translatableTitle warns about a literal page title (PLX-1609, I18N-007).
func (u *unit) translatableTitle(pg *page) {
	if pg.doc == nil || len(u.project.App.Doc.SupportedLocales) < 2 {
		return
	}
	var title any
	if json.Unmarshal(pg.doc.Title, &title) != nil {
		return
	}
	for _, l := range literals.Title(map[string]any{"title": title}) {
		u.report(plxerr.LiteralTextUntranslated, pg.file, plxerr.Pointer(l.Path...),
			"%s holds literal text; move it into a translation key (plux l10n extract)", l.Prop)
	}
}

// translatableMessages warns about a literal validator message of a page's
// or component's forms (PLX-1609, I18N-001, I18N-007).
func (u *unit) translatableMessages(file string, forms []schema.Form) {
	if len(forms) == 0 || len(u.project.App.Doc.SupportedLocales) < 2 {
		return
	}
	raw, err := json.Marshal(map[string]any{"forms": forms})
	if err != nil {
		return
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return
	}
	for _, l := range literals.Forms(doc) {
		u.report(plxerr.LiteralTextUntranslated, file, plxerr.Pointer(l.Path...),
			"%s holds literal text; move it into a translation key (plux l10n extract)", l.Prop)
	}
}

// nodeJSON decodes the parts of a node document the literal finder reads.
func nodeJSON(d *schema.Node) map[string]any {
	m := map[string]any{"type": d.Type}
	props := make(map[string]any, len(d.Props))
	for name, raw := range d.Props {
		var v any
		if json.Unmarshal(raw, &v) == nil {
			props[name] = v
		}
	}
	m["props"] = props
	if s := d.Semantics; s != nil {
		sem := map[string]any{}
		for name, raw := range map[string]json.RawMessage{"label": s.Label, "hint": s.Hint} {
			var v any
			if len(raw) > 0 && json.Unmarshal(raw, &v) == nil {
				sem[name] = v
			}
		}
		m["semantics"] = sem
	}
	return m
}

// pxlTranslation matches t("key") and t('key', ...) in PXL source.
var pxlTranslation = regexp.MustCompile(`\bt\(\s*(?:"([^"]+)"|'([^']+)')`)

// unusedKeys warns about each key that has a message in the default locale
// and that no page, component, action, trigger or PXL expression of the app
// references (PLX-1608, I18N-007). It also records which plugin uses which
// key, for the localisation report.
func (u *unit) unusedKeys() {
	tk := u.project.TranslationKeys
	if tk == nil || tk.Doc == nil {
		return
	}
	used := u.markKeyUses(tk)
	u.warnUnusedKeys(tk, used)
}

// markKeyUses returns the IDs of the keys something references, and records
// in u.l10n.uses which plugin references which key.
func (u *unit) markKeyUses(tk *schema.Loaded[schema.TranslationKeysDocument]) map[string]bool {
	used := map[string]bool{}
	u.l10n.uses = map[string]map[string]bool{}
	owner := u.pluginOwner()
	u.eachKeyUse(tk, func(id, file string) {
		used[id] = true
		who := owner(file)
		if u.l10n.uses[who] == nil {
			u.l10n.uses[who] = map[string]bool{}
		}
		u.l10n.uses[who][id] = true
	})
	return used
}

// eachKeyUse calls mark with the ID of every translation key something
// references and the file that references it.
func (u *unit) eachKeyUse(tk *schema.Loaded[schema.TranslationKeysDocument], mark func(id, file string)) {
	for _, e := range u.graph.Edges {
		if e.Kind == EdgeUsesTranslation {
			mark(e.To, e.File)
		}
	}
	byName := make(map[string]string, len(tk.Doc.Keys))
	for _, k := range tk.Doc.Keys {
		byName[k.Key] = k.ID
	}
	for _, e := range u.exprs {
		for _, m := range pxlTranslation.FindAllStringSubmatch(e.src, -1) {
			if id, ok := byName[m[1]+m[2]]; ok {
				mark(id, e.file)
			}
		}
	}
	for _, n := range u.allNodes() {
		if len(n.doc.LocaleOverrides) > 0 {
			u.markIDsIn(n.doc.LocaleOverrides, n.owner.file(), tk.Doc.Keys, mark)
		}
	}
	u.markIDsIn(u.project.App.Doc, "app.json", tk.Doc.Keys, mark)
}

// pluginOwner returns a function naming the plugin whose directory holds a
// file, or "" for a file of the app itself.
func (u *unit) pluginOwner() func(file string) string {
	dirs := make([]string, len(u.plugins))
	for i, pl := range u.plugins {
		dirs[i] = pl.file[:strings.LastIndex(pl.file, "/")+1]
	}
	return func(file string) string {
		for i, d := range dirs {
			if d != "" && strings.HasPrefix(file, d) {
				return u.plugins[i].key
			}
		}
		return ""
	}
}

// warnUnusedKeys reports each key with a default-locale message that is not
// in used, and records it for the localisation report.
func (u *unit) warnUnusedKeys(tk *schema.Loaded[schema.TranslationKeysDocument], used map[string]bool) {
	def := u.defaultMessages()
	for i, k := range tk.Doc.Keys {
		if used[k.ID] {
			continue
		}
		if _, ok := def[k.ID]; !ok {
			continue
		}
		u.l10n.unused = append(u.l10n.unused, k.ID)
		u.report(plxerr.TranslationKeyUnused, tk.Source.File, plxerr.Pointer("keys", strconv.Itoa(i)),
			"translation key %q has a message in %s and nothing references it", k.Key, u.project.App.Doc.DefaultLocale)
	}
	slices.Sort(u.l10n.unused)
}

// markIDsIn marks every key whose ID appears in the JSON form of v, for
// documents whose references the reference graph does not record.
func (*unit) markIDsIn(v any, file string, keys []schema.TranslationKey, mark func(id, file string)) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	for _, k := range keys {
		if bytes.Contains(raw, []byte(`"`+k.ID+`"`)) {
			mark(k.ID, file)
		}
	}
}

// defaultMessages returns the messages of the default locale by key ID, or
// nil when the app has no translations document for it.
func (u *unit) defaultMessages() map[string]string {
	for _, t := range u.project.Translations {
		if t.Doc != nil && t.Doc.Locale == u.project.App.Doc.DefaultLocale {
			return t.Doc.Messages
		}
	}
	return nil
}

// approvedOnly returns the keys of a translations document the release
// uses and the number it leaves out, when the app requires approved
// translations (PLX-1610, I18N-008). The default locale always ships.
func (u *unit) approvedOnly(doc *schema.TranslationsDocument, keys []string) (kept []string, leftOut int) {
	app := u.project.App.Doc
	st := u.l10n.stat(doc.Locale)
	for _, k := range keys {
		st.present[k] = true
		if doc.States[k] == schema.TranslationStateApproved {
			st.approved[k] = true
		}
	}
	if !requireApproved(app) || doc.Locale == app.DefaultLocale {
		return keys, 0
	}
	for _, k := range keys {
		if st.approved[k] {
			kept = append(kept, k)
			continue
		}
		st.leftOut[k] = true
		leftOut++
	}
	return kept, leftOut
}

// reportLeftOut reports once per locale that unapproved translations are
// left out of the release (PLX-1610).
func (u *unit) reportLeftOut(doc *schema.TranslationsDocument, file string, left, total int) {
	if left == 0 {
		return
	}
	u.report(plxerr.TranslationNotApproved, file, "/states",
		"%d of %d translations in %s are not approved and are left out of the release; the app falls back to %s",
		left, total, doc.Locale, u.project.App.Doc.DefaultLocale)
}

// pseudoSections encodes the pseudo-locales generated from the default
// locale's messages (PLX-1614, I18N-002): en-XA with the CLDR data of en
// and ar-XB with that of ar, so the second lays out right to left. Nothing
// is generated in a production build; policy has reported it.
func (u *unit) pseudoSections(o *out) {
	app := u.project.App.Doc
	if !u.pseudoRequested(app) || u.opts.Production {
		return
	}
	def := u.defaultMessages()
	if def == nil {
		return
	}
	opts := u.reduceOptions(app)
	for _, tag := range pseudo.Tags() {
		if u.hasTranslations(tag) {
			continue // `plux l10n pseudo` wrote the project's own document
		}
		base := "en"
		if tag == pseudo.RTL {
			base = "ar"
		}
		id, err := bundle.LocaleID(tag)
		if err != nil {
			continue
		}
		var blob *localeBlob
		if data, resolved, err := l10n.Reduce(base, opts); err == nil {
			blob = &localeBlob{data: data, resolved: resolved, flutter: u.flutterStrings(base, app.DefaultLocale)}
		}
		made := pseudo.Messages(tag, def)
		ids := sortedKeys(made)
		slices.SortFunc(ids, func(a, c string) int {
			x, y := uuidBytes(a), uuidBytes(c)
			return bytes.Compare(x[:], y[:])
		})
		b := flatbuffers.NewBuilder(1024)
		offs := make([]flatbuffers.UOffsetT, 0, len(ids))
		st := u.l10n.stat(tag)
		st.pseudo = true
		for _, k := range ids {
			var ast []byte
			if m, err := mf.Parse(made[k]); err == nil {
				ast = m.Encode()
			}
			offs = append(offs, messageTable(b, k, made[k], ast))
			st.present[k] = true
		}
		o.add(bundle.SectionL10n, id, u.finishLocale(b, tag, offsetVector(b, offs), blob))
	}
}

// generatedPseudo lists the pseudo-locales this build generated, sorted, so
// the app bundle's supported locales name them (I18N-002). A release that
// lists one cannot be promoted into production (PLX-3064).
func (u *unit) generatedPseudo() []string {
	var tags []string
	for tag, st := range u.l10n.locales {
		if st.pseudo {
			tags = append(tags, tag)
		}
	}
	slices.Sort(tags)
	return tags
}

// hasTranslations reports whether the project has a translations document
// for the locale.
func (u *unit) hasTranslations(tag string) bool {
	for _, t := range u.project.Translations {
		if t.Doc != nil && t.Doc.Locale == tag {
			return true
		}
	}
	return false
}

// messageTable writes one message of a locale section: its key, text and
// optional pre-parsed AST.
func messageTable(b *flatbuffers.Builder, key, text string, ast []byte) flatbuffers.UOffsetT {
	t := b.CreateString(text)
	var a flatbuffers.UOffsetT
	if ast != nil {
		a = b.CreateByteVector(ast)
	}
	fbs.MessageStart(b)
	hi, lo := uuidHalves(uuidBytes(key))
	fbs.MessageAddKey(b, fbs.CreateUuid(b, hi, lo))
	fbs.MessageAddText(b, t)
	addOptional(b, a, fbs.MessageAddAst)
	return fbs.MessageEnd(b)
}
