// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	fbs "github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/l10n"
	"github.com/nightCode42/plux3/backend/internal/l10n/flutterdata"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// localeBlob is what a locale's section carries besides its messages: the
// reduced CLDR data, Flutter's widget strings and the CLDR locale the data
// came from (I18N-002, ADR-0066).
type localeBlob struct {
	data, flutter []byte
	resolved      string
}

// noteCalendar records a calendar a node's prop names as a literal, so the
// locale data keeps it (I18N-003).
func (u *unit) noteCalendar(raw map[string]json.RawMessage) {
	for _, name := range []string{"calendar", "secondaryCalendar"} {
		var id string
		if json.Unmarshal(raw[name], &id) == nil && id != "" {
			if u.calendars == nil {
				u.calendars = map[string]bool{}
			}
			u.calendars[id] = true
		}
	}
}

// calendarsUsed lists the calendars the app declares or its nodes name,
// sorted, without gregorian (which every locale keeps).
func (u *unit) calendarsUsed(app *schema.AppDocument) []string {
	set := map[string]bool{}
	for c := range u.calendars {
		set[c] = true
	}
	if l := app.Localisation; l != nil {
		set[string(l.Calendar)] = true
		set[string(l.SecondaryCalendar)] = true
	}
	delete(set, "")
	delete(set, string(schema.CalendarGregorian))
	return sortedKeys(set)
}

// localeBlobs builds the data of every locale the app supports or
// translates, by the tag as written. A supported locale CLDR has no data for
// is PLX-1600 at its place in app.json.
func (u *unit) localeBlobs(app *schema.AppDocument) map[string]*localeBlob {
	opts := l10n.ReduceOptions{Calendars: u.calendarsUsed(app)}
	out := map[string]*localeBlob{}
	build := func(tag string) *localeBlob {
		if b, ok := out[tag]; ok {
			return b
		}
		var b *localeBlob
		data, resolved, err := l10n.Reduce(tag, opts)
		if err == nil {
			b = &localeBlob{data: data, resolved: resolved, flutter: u.flutterStrings(tag, app.DefaultLocale)}
		}
		out[tag] = b
		return b
	}
	for i, tag := range app.SupportedLocales {
		if build(tag) == nil {
			u.report(plxerr.LocaleUnsupported, "app.json", plxerr.Pointer("supportedLocales", strconv.Itoa(i)),
				"the locale %q has no CLDR data in this release", tag)
		}
	}
	for _, t := range u.project.Translations {
		if t.Doc != nil {
			build(t.Doc.Locale)
		}
	}
	return out
}

// flutterStrings returns Flutter's widget strings for tag as compact JSON:
// the first locale on tag's CLDR parent chain Flutter has, else the same for
// the app's default locale, else nil.
func (u *unit) flutterStrings(tag, fallback string) []byte {
	for _, t := range []string{tag, fallback} {
		chain, err := l10n.Chain(t)
		if err != nil {
			continue
		}
		for _, c := range chain {
			raw, err := flutterdata.Strings(c)
			if err != nil {
				if errors.Is(err, flutterdata.ErrNoStrings) {
					continue
				}
				return nil
			}
			var buf bytes.Buffer
			if json.Compact(&buf, raw) != nil {
				return nil
			}
			return buf.Bytes()
		}
	}
	return nil
}

// finishLocale ends a locale section with the tag, the messages vector and
// the locale's data, and returns its encoded bytes.
func (u *unit) finishLocale(b *flatbuffers.Builder, tag string, messages flatbuffers.UOffsetT, blob *localeBlob) []byte {
	var data, flutter, resolved flatbuffers.UOffsetT
	if blob != nil {
		data = b.CreateByteVector(blob.data)
		if len(blob.flutter) > 0 {
			flutter = b.CreateByteVector(blob.flutter)
		}
		resolved = b.CreateString(blob.resolved)
	}
	t := b.CreateString(tag)
	fbs.LocaleStart(b)
	fbs.LocaleAddTag(b, t)
	fbs.LocaleAddMessages(b, messages)
	addOptional(b, data, fbs.LocaleAddData)
	addOptional(b, flutter, fbs.LocaleAddFlutter)
	addOptional(b, resolved, fbs.LocaleAddTagResolved)
	return finish(b, fbs.LocaleEnd(b), bundle.SectionL10n)
}

// localeOverrides checks a node's per-locale prop overrides and appends
// them as locale override layers, sorted by tag (I18N-012). A tag that is
// not a supported locale, or a prop the node's shape rejects, is PLX-1613
// (a malformed tag already fails the document schema).
func (u *unit) localeOverrides(n *node, sh *shape, c vctx) {
	lo := n.doc.LocaleOverrides
	for _, tag := range sortedKeys(lo) {
		ptr := n.ptr + "/localeOverrides" + plxerr.Pointer(tag)
		if !u.releaseLocale(tag) {
			u.report(plxerr.LocaleOverrideInvalid, c.file, ptr, "%q is not a canonical tag of a supported locale", tag)
			continue
		}
		saved := u.diags
		u.diags = nil
		props := u.checkProps(n, sh, lo[tag], c, "localeOverrides/"+tag, false)
		found := u.diags
		u.diags = saved
		if len(found) > 0 {
			u.report(plxerr.LocaleOverrideInvalid, c.file, ptr, "the override for %q is invalid: %s", tag, found[0].Message)
			continue
		}
		n.overrides = append(n.overrides, &override{kind: fbs.OverrideKindLocale, key: tag, props: props})
	}
}
