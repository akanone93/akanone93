// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/l10n"
	"github.com/nightCode42/plux3/backend/internal/l10n/mf"
	"github.com/nightCode42/plux3/backend/internal/pxl"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// What an app's locale data keeps (I18N-002, ADR-0066 Revision 2026-10-18).
//
// A bundle's l10n section carries the CLDR names of only the currencies,
// territories and date skeletons the app uses. The rule for each category is
// the same and conservative: the app's documents are searched for every
// reference whose value is known at compile time, and when a reference is
// dynamic (an expression or a typed value whose content only exists at run
// time), or when pages embed native widgets of the host app, which can read
// the locale data, the whole category is kept. A trimmed bundle therefore never lacks a name that
// a literal reference needs, and a missing name still falls back at run time
// to the ISO code (currencies) or the region code (territories).
//
//   - Currencies: the literal currency of a money constant or of
//     money(x, "XXX") in a PXL expression, a literal currency prop of
//     AmountInput and CurrencyPicker, and the `::currency/XXX` number
//     skeletons of messages. Dynamic: a money value read from state, a
//     parameter or a data field, a money call with a computed currency, any
//     declared type that mentions money, a computed currency prop, and a
//     CurrencyPicker without a literal list of currencies.
//   - Territories: CountryPicker and PhoneInput list every territory, and
//     nothing else names one, so a project that uses neither keeps none.
//   - Skeletons: the skeletons of the date and time arguments of messages
//     and the ones the runtime's own Flutter localizations use
//     (runtimeSkeletons); the date and time styles are always kept.

// runtimeSkeletons are the date skeletons packages/plux_flutter asks the
// locale data for besides an app's own (flutter_localizations.dart); a
// test pins the list to that file.
var runtimeSkeletons = []string{"EEEEE", "LLLL", "MMMEd", "MMMd", "y", "yMMMM", "yMMMMEEEEd", "yMMMd", "yMd"}

// localeUse collects what the app's documents reference of the locale data.
type localeUse struct {
	currencies map[string]bool
	// dynamicCurrency is set by a currency reference that is not a literal.
	dynamicCurrency bool
	// territories is set by a widget that lists territories.
	territories bool
}

// addCurrency records a literal currency.
func (l *localeUse) addCurrency(code string) {
	if l.currencies == nil {
		l.currencies = map[string]bool{}
	}
	l.currencies[code] = true
}

// noteProgram records the currencies of a compiled expression.
func (u *unit) noteProgram(p *pxl.Program) {
	if p == nil {
		return
	}
	for _, c := range p.Currencies.Codes {
		u.use.addCurrency(c)
	}
	u.use.dynamicCurrency = u.use.dynamicCurrency || p.Currencies.Dynamic
}

// noteLocaleUse records the locale data a node's widget and props reference.
func (u *unit) noteLocaleUse(doc *schema.Node) {
	switch doc.Type {
	case "CountryPicker", "PhoneInput":
		u.use.territories = true
	}
	layers := []map[string]json.RawMessage{doc.Props}
	for _, tag := range sortedKeys(doc.LocaleOverrides) {
		layers = append(layers, doc.LocaleOverrides[tag])
	}
	if r := doc.Responsive; r != nil {
		layers = append(layers, r.Medium, r.Expanded)
	}
	if raw, ok := doc.Props["currencies"]; doc.Type == "CurrencyPicker" && (!ok || string(raw) == "null") {
		u.use.dynamicCurrency = true // without a list it offers every currency
	}
	for _, props := range layers {
		switch doc.Type {
		case "AmountInput":
			u.noteCurrencyProp(props, "currency")
		case "CurrencyPicker":
			u.noteCurrencyProp(props, "value")
			u.noteCurrencyProp(props, "currencies")
		}
	}
}

// noteCurrencyProp records the currency a prop names as a literal string or
// list of strings; a prop of another form is dynamic.
func (u *unit) noteCurrencyProp(props map[string]json.RawMessage, name string) {
	raw, ok := props[name]
	if !ok || string(raw) == "null" {
		return
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		u.use.addCurrency(one)
		return
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		for _, c := range many {
			u.use.addCurrency(c)
		}
		return
	}
	u.use.dynamicCurrency = true
}

// moneyDeclared reports whether a document declares a type that mentions
// money: values of it come from outside the documents, so their currency is
// not known.
func moneyDeclared(v any) bool {
	switch x := v.(type) {
	case string:
		if !strings.Contains(x, "money") {
			return false
		}
		te, err := parseTypeExpr(x)
		if err != nil {
			return false
		}
		found := false
		te.names(func(n string) { found = found || n == "money" })
		return found
	case []any:
		return slices.ContainsFunc(x, moneyDeclared)
	case map[string]any:
		for k, e := range x {
			if k != "$expr" && moneyDeclared(e) {
				return true
			}
		}
	}
	return false
}

// docMentionsMoney reports whether the JSON form of a document declares a
// type that mentions money; a document that is nil or cannot be encoded does
// not.
func docMentionsMoney(doc any) bool {
	raw, err := json.Marshal(doc)
	if err != nil {
		return false
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return false
	}
	return moneyDeclared(v)
}

// declaresMoney reports whether any document of the project declares a type
// that mentions money. Translations are not searched: a message cannot
// format a money argument.
func (u *unit) declaresMoney() bool {
	p := u.project
	docs := []any{p.App.Doc}
	for i := range p.Components {
		docs = append(docs, p.Components[i].Doc)
	}
	for i := range p.Templates {
		docs = append(docs, p.Templates[i].Doc)
	}
	for i := range p.Functions {
		docs = append(docs, p.Functions[i].Doc)
	}
	for i := range p.Plugins {
		pl := &p.Plugins[i]
		docs = append(docs, pl.Doc)
		for j := range pl.Pages {
			docs = append(docs, pl.Pages[j].Doc)
		}
		for j := range pl.Components {
			docs = append(docs, pl.Components[j].Doc)
		}
		for j := range pl.Graphs {
			docs = append(docs, pl.Graphs[j].Doc)
		}
	}
	return slices.ContainsFunc(docs, docMentionsMoney)
}

// hasNativeSlots reports whether the host app registers native widgets that
// pages embed, whose Dart code may read any of the locale data.
func (u *unit) hasNativeSlots() bool {
	n := u.project.NativeCatalogue
	return n != nil && n.Doc != nil && len(n.Doc.Slots) > 0
}

// messageUse returns the date skeletons and the currencies the messages of
// all translations format with, sorted. A message that does not parse is
// reported elsewhere and adds nothing.
func (u *unit) messageUse() (skeletons, currencies []string) {
	sk, cur := map[string]bool{}, map[string]bool{}
	for i := range u.project.Translations {
		t := &u.project.Translations[i]
		if t.Doc == nil {
			continue
		}
		for _, k := range sortedKeys(t.Doc.Messages) {
			if m, err := mf.Parse(t.Doc.Messages[k]); err == nil {
				formatsUsed(m.Nodes, sk, cur)
			}
		}
	}
	return sortedKeys(sk), sortedKeys(cur)
}

// formatsUsed adds the date skeletons and the currencies the nodes of a
// message, and of its cases, format with to sk and cur.
func formatsUsed(nodes []mf.Node, sk, cur map[string]bool) {
	for i := range nodes {
		n := &nodes[i]
		style, skeleton := strings.CutPrefix(n.Style, "::")
		switch {
		case !skeleton:
		case n.Kind == mf.KindDate || n.Kind == mf.KindTime:
			sk[style] = true
		case n.Kind == mf.KindNumber:
			for _, tok := range strings.Fields(style) {
				if code, ok := strings.CutPrefix(tok, "currency/"); ok {
					cur[code] = true
				}
			}
		}
		for _, c := range n.Cases {
			formatsUsed(c.Body, sk, cur)
		}
	}
}

// reduceOptions say which locale data the app's bundle keeps: its calendars
// and, unless a reference is dynamic or native code is present, only the
// currencies, territories and date skeletons it uses.
func (u *unit) reduceOptions(app *schema.AppDocument) l10n.ReduceOptions {
	o := l10n.ReduceOptions{Calendars: u.calendarsUsed(app)}
	if u.hasNativeSlots() {
		return o
	}
	skeletons, currencies := u.messageUse()
	o.Skeletons = sortedUnique(append(slices.Clone(runtimeSkeletons), skeletons...))
	if !u.use.territories {
		o.Territories = []string{}
	}
	if u.use.dynamicCurrency || u.declaresMoney() {
		return o
	}
	for c := range u.use.currencies {
		currencies = append(currencies, c)
	}
	o.Currencies = sortedUnique(currencies)
	return o
}

// sortedUnique returns s sorted without duplicates, never nil.
func sortedUnique(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return append([]string{}, slices.Compact(out)...)
}
