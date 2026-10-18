// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package xchange exports and imports the translations of one locale in the
// formats translation tools use: XLIFF 2.0, ARB, JSON (the Plux translations
// document itself) and CSV (I18N-009, ADR-0068).
//
// The package is pure: it reads and writes bytes and documents and touches no
// store. An import file is untrusted input. It is bounded by the
// translation.importSize limit before anything parses it, XML with a document
// type declaration or entity declaration is refused, no external reference is
// ever resolved, and a file that is not valid for its format applies nothing
// (PLX-1620).
package xchange

import (
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// Format is a translation file format.
type Format string

// The supported formats.
const (
	XLIFF Format = "xliff"
	ARB   Format = "arb"
	JSON  Format = "json"
	CSV   Format = "csv"
)

// Formats lists the supported formats in a stable order.
func Formats() []Format { return []Format{XLIFF, ARB, JSON, CSV} }

// ParseFormat reads the name of a format.
func ParseFormat(s string) (Format, error) {
	for _, f := range Formats() {
		if string(f) == s {
			return f, nil
		}
	}
	return "", plxerr.New(plxerr.InvalidEnumValue, "%q is not a translation format; use xliff, arb, json or csv", clip(s))
}

// Extension is the file extension of a format, with its dot.
func (f Format) Extension() string {
	switch f {
	case XLIFF:
		return ".xlf"
	case ARB:
		return ".arb"
	case CSV:
		return ".csv"
	default:
		return ".json"
	}
}

// Arg is a placeholder of a translation key.
type Arg struct {
	Name string
	Type string
}

// Key is a translation key as the keys document declares it.
type Key struct {
	ID          string
	Key         string
	Description string
	// MaxLength is zero when the key has no limit.
	MaxLength int
	Args      []Arg
}

// Doc is the part of a translations document an exchange reads and writes:
// the messages of one locale by key identifier and their review states. A
// message without a state is new.
type Doc struct {
	// ID is the document's identifier; empty for a locale that has no
	// document yet.
	ID       string
	Locale   string
	Messages map[string]string
	States   map[string]string
}

// Clone returns a deep copy.
func (d Doc) Clone() Doc {
	c := d
	c.Messages = make(map[string]string, len(d.Messages))
	maps.Copy(c.Messages, d.Messages)
	c.States = make(map[string]string, len(d.States))
	maps.Copy(c.States, d.States)
	return c
}

// State returns the review state of a key; a key with none is new.
func (d Doc) State(id string) string {
	if s, ok := d.States[id]; ok && s != "" {
		return s
	}
	return stateNew
}

// Options are the bounds and the privileges of an import.
type Options struct {
	// MaxBytes is the largest file accepted: translation.importSize.
	MaxBytes int64
	// Approver is whether the caller holds translations.approve. Without
	// it, an import never sets the approved state.
	Approver bool
}

// Review states, as the translations document spells them.
const (
	stateNew        = string(schema.TranslationStateNew)
	stateTranslated = string(schema.TranslationStateTranslated)
	stateReviewed   = string(schema.TranslationStateReviewed)
	stateApproved   = string(schema.TranslationStateApproved)
)

func validState(s string) bool { return schema.TranslationState(s).Valid() }

// exportSchemaVersion is the document schema version an exported JSON
// document declares (SCH-000).
const exportSchemaVersion = "1.0.0"

// Export writes the translations of target, with the source texts of source,
// for the keys, in a format. The output is deterministic.
func Export(keys []Key, source, target Doc, f Format) ([]byte, error) {
	if source.Locale == "" || target.Locale == "" {
		return nil, plxerr.New(plxerr.InvalidFormat, "an export needs a source and a target locale")
	}
	sorted := sortedKeys(keys)
	switch f {
	case XLIFF:
		return exportXLIFF(sorted, source, target)
	case ARB:
		return exportARB(sorted, source, target)
	case JSON:
		return exportJSON(target)
	case CSV:
		return exportCSV(sorted, source, target)
	}
	return nil, plxerr.New(plxerr.InvalidEnumValue, "%q is not a translation format", clip(string(f)))
}

// Import applies the translations in data to a copy of target and returns
// it with what the import found. It never writes into target.
//
// A key the file names that the keys document does not declare is reported
// (PLX-1621) and skipped. A translation whose source text differs from the
// current source is stored as translated and reported (PLX-1622). The
// approved state is set only when o.Approver is true; otherwise a file that
// asks for it changes a translation to translated, and leaves an unchanged
// one as it was.
func Import(data []byte, f Format, keys []Key, target, source Doc, o Options) (Doc, plxerr.Diagnostics, error) {
	if o.MaxBytes <= 0 {
		return Doc{}, nil, errors.New("xchange: Options.MaxBytes must be the translation.importSize limit")
	}
	if int64(len(data)) > o.MaxBytes {
		return Doc{}, nil, &plxerr.Error{
			Code:    plxerr.LimitExceeded,
			Message: "the file is larger than translation.importSize allows",
			Details: map[string]string{"limit": "translation.importSize"},
		}
	}
	var (
		file importFile
		err  error
	)
	switch f {
	case XLIFF:
		file, err = parseXLIFF(data)
	case ARB:
		file, err = parseARB(data)
	case JSON:
		file, err = parseJSON(data)
	case CSV:
		file, err = parseCSV(data)
	default:
		return Doc{}, nil, plxerr.New(plxerr.InvalidEnumValue, "%q is not a translation format", clip(string(f)))
	}
	if err != nil {
		return Doc{}, nil, err
	}
	return merge(file, keys, target, source, o)
}

// importFile is what a parser read: the locale the file declares for its
// translations, if any, and one entry per translated unit.
type importFile struct {
	locale  string
	entries []entry
}

// entry is one translated unit of an import file.
type entry struct {
	// id and key identify the translation key; either may be empty.
	id, key string
	// source is the source text the translation was made from, if the
	// format carries it.
	source    string
	hasSource bool
	target    string
	hasTarget bool
	// state is a Plux review state, or empty when the file names none.
	state string
}

// invalid is the error of a file that is not valid for its format.
func invalid(format string, args ...any) error {
	return plxerr.New(plxerr.TranslationImportInvalid, format, args...)
}

// merge applies the entries of a parsed file.
func merge(file importFile, keys []Key, target, source Doc, o Options) (Doc, plxerr.Diagnostics, error) {
	if file.locale != "" && target.Locale != "" && file.locale != target.Locale {
		return Doc{}, nil, invalid("the file is for locale %s, not %s", clip(file.locale), target.Locale)
	}
	byID := make(map[string]Key, len(keys))
	byName := make(map[string]Key, len(keys))
	for _, k := range keys {
		byID[k.ID] = k
		byName[k.Key] = k
	}
	out := target.Clone()
	var diags plxerr.Diagnostics
	seen := make(map[string]bool, len(file.entries))
	var downgraded []string
	for _, e := range file.entries {
		k, ok := resolve(e, byID, byName)
		if !ok {
			diags = append(diags, plxerr.NewDiagnostic(plxerr.TranslationImportKeyUnknown, location(target.Locale, e.id, e.key),
				"the file translates %s, which the app does not declare", clip(firstNonEmpty(e.key, e.id))))
			continue
		}
		if seen[k.ID] {
			return Doc{}, nil, invalid("the file translates key %s more than once", clip(k.Key))
		}
		seen[k.ID] = true
		if !e.hasTarget || e.target == "" {
			continue
		}
		if e.state == stateApproved && !o.Approver {
			downgraded = append(downgraded, k.Key)
		}
		if d, changed := applyEntry(out, e, k, target.Locale, source, o); changed {
			diags = append(diags, d)
		}
	}
	if len(downgraded) > 0 {
		diags = append(diags, plxerr.NewDiagnostic(plxerr.TranslationImportDowngraded, plxerr.Location{File: "translations/" + target.Locale + ".json"},
			"%s", downgradeMessage(downgraded)))
	}
	diags.Sort()
	return out, diags, nil
}

// applyEntry stores the translation of e for key k in out and returns the
// diagnostic for a source text that changed since it was translated, with
// true when there is one.
func applyEntry(out Doc, e entry, k Key, locale string, source Doc, o Options) (plxerr.Diagnostic, bool) {
	before, had := out.Messages[k.ID]
	changed := !had || before != e.target
	current := source.Messages[k.ID]
	sourceChanged := e.hasSource && current != "" && e.source != current
	state := decide(e.state, out.State(k.ID), changed, sourceChanged, o.Approver)
	var diag plxerr.Diagnostic
	if sourceChanged {
		diag = plxerr.NewDiagnostic(plxerr.TranslationImportSourceChanged, location(locale, k.ID, k.Key),
			"the source text of %s changed since it was translated; the translation is stored as translated", clip(k.Key))
	}
	out.Messages[k.ID] = e.target
	if state == stateNew {
		delete(out.States, k.ID)
	} else {
		out.States[k.ID] = state
	}
	return diag, sourceChanged
}

// maxDowngradeKeys is how many keys the downgrade warning names.
const maxDowngradeKeys = 10

// downgradeMessage says how many entries asked for approved or final status
// without the right to set it, naming the first few.
func downgradeMessage(keys []string) string {
	sort.Strings(keys)
	listed := keys[:min(len(keys), maxDowngradeKeys)]
	for i, k := range listed {
		listed[i] = clip(k)
	}
	more := ""
	if len(keys) > len(listed) {
		more = fmt.Sprintf(" and %d more", len(keys)-len(listed))
	}
	return fmt.Sprintf("%d translation(s) ask for approved or final status, which needs the approver permission; they are stored as translated: %s%s",
		len(keys), strings.Join(listed, ", "), more)
}

// decide is the review state a translated unit ends in. A changed source
// text always leaves the unit translated. Approved needs an approver; an
// unprivileged request for it counts as no request.
func decide(requested, stored string, changed, sourceChanged, approver bool) string {
	if sourceChanged {
		return stateTranslated
	}
	if requested == stateApproved && approver {
		return stateApproved
	}
	if requested == stateApproved || requested == stateNew {
		requested = ""
	}
	switch {
	case requested != "":
		return requested
	case changed:
		return stateTranslated
	}
	return stored
}

// resolve finds the key an entry names: by identifier when it has one,
// otherwise by key name. An identifier and a name that disagree name no key.
func resolve(e entry, byID, byName map[string]Key) (Key, bool) {
	if e.id != "" {
		k, ok := byID[e.id]
		if ok && e.key != "" && e.key != k.Key {
			return Key{}, false
		}
		return k, ok
	}
	k, ok := byName[e.key]
	return k, ok
}

// Changed lists the identifiers of the keys whose message or state differs
// between two documents, sorted.
func Changed(before, after Doc) []string {
	seen := map[string]bool{}
	for _, d := range []Doc{before, after} {
		for id := range d.Messages {
			seen[id] = true
		}
		for id := range d.States {
			seen[id] = true
		}
	}
	var out []string
	for id := range seen {
		bm, bok := before.Messages[id]
		am, aok := after.Messages[id]
		if bok != aok || bm != am || before.State(id) != after.State(id) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func sortedKeys(keys []Key) []Key {
	out := append([]Key(nil), keys...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// location is where a finding about a key is: its message in the locale's
// translations document.
func location(locale, id, key string) plxerr.Location {
	return plxerr.Location{
		File: "translations/" + locale + ".json",
		Path: "/messages/" + pointerToken(firstNonEmpty(id, key)),
	}
}

// pointerToken escapes a JSON Pointer reference token (RFC 6901) and
// shortens it for a message.
func pointerToken(s string) string {
	s = strings.ReplaceAll(clip(s), "~", "~0")
	return strings.ReplaceAll(s, "/", "~1")
}

// maxEcho is how much of a file's text a message repeats.
const maxEcho = 64

// clip shortens untrusted text for a message, never inside a code point.
func clip(s string) string {
	if len(s) <= maxEcho {
		return strings.ToValidUTF8(s, "�")
	}
	cut := maxEcho
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.ToValidUTF8(s[:cut], "�") + "..."
}
