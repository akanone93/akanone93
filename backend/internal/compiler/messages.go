// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"errors"
	"slices"
	"unicode/utf8"

	"github.com/nightCode42/plux3/backend/internal/l10n"
	"github.com/nightCode42/plux3/backend/internal/l10n/mf"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
)

// messageArgTypes reduces the declared arguments of a translation key to the
// types a message can use them as.
func (u *unit) messageArgTypes(key *schema.TranslationKey) map[string]mf.ArgType {
	out := make(map[string]mf.ArgType, len(key.Args))
	for _, f := range key.Args {
		out[f.Name] = mf.TypeOther
		t, err := parseTypeExpr(f.Type)
		if err != nil || t.elem != nil {
			continue
		}
		switch t.name {
		case "int", "double", "decimal":
			out[f.Name] = mf.TypeNumber
		case "string":
			out[f.Name] = mf.TypeString
		case "bool":
			out[f.Name] = mf.TypeBool
		case "date":
			out[f.Name] = mf.TypeDate
		case "dateTime":
			out[f.Name] = mf.TypeDateTime
		default:
			if !primitives[t.name] && u.isEnum(t.name) {
				out[f.Name] = mf.TypeString
			}
		}
	}
	return out
}

// isEnum reports whether the app declares the named enum.
func (u *unit) isEnum(name string) bool {
	if u.types == nil {
		return false
	}
	return len(u.types.app[name].Enum) > 0
}

// checkedMessage is a message that passed the checks of I18N-001.
type checkedMessage struct {
	ast []byte
}

// checkMessage parses and checks the message of translation key id in the
// locale and returns its AST, or nil after reporting PLX-1601 to PLX-1607 at
// the message's JSON pointer in file. defaultLocale is true for the app's
// default locale, whose rendering with sample arguments must fit the key's
// maxLength.
func (u *unit) checkMessage(file, ptr, id, text string, loc *l10n.Locale, defaultLocale bool) *checkedMessage {
	m, err := mf.Parse(text)
	if err != nil {
		u.report(plxerr.MessageSyntaxInvalid, file, ptr, "%v", err)
		return nil
	}
	key := u.tkeys[id]
	if key == nil {
		return &checkedMessage{ast: m.Encode()} // an undeclared key is reported where it is used
	}
	declared := u.messageArgTypes(key)
	failed := false
	for _, p := range m.Check(declared, loc) {
		failed = true
		code := map[mf.ProblemKind]plxerr.Code{
			mf.ProblemUndeclared:   plxerr.MessageArgumentUndeclared,
			mf.ProblemType:         plxerr.MessageArgumentTypeMismatch,
			mf.ProblemCategory:     plxerr.PluralCategoryInvalid,
			mf.ProblemOtherMissing: plxerr.PluralOtherMissing,
		}[p.Kind]
		u.report(code, file, ptr, "translation %q at offset %d: %s", key.Key, p.Offset, p.Msg)
	}
	if failed {
		return nil
	}
	if defaultLocale && key.MaxLength != nil && loc != nil {
		shown, err := m.Format(loc, mf.SampleArgs(declared))
		if err != nil && !errors.Is(err, mf.ErrArgument) {
			shown = text
		}
		if n := int64(utf8.RuneCountInString(shown)); n > *key.MaxLength {
			u.report(plxerr.TranslationTooLong, file, ptr, "translation %q renders %d characters with sample arguments, over its maxLength %d", key.Key, n, *key.MaxLength)
		}
	}
	return &checkedMessage{ast: m.Encode()}
}

// reportMissingTranslations warns about every key that a supported locale
// lacks (PLX-1606).
func (u *unit) reportMissingTranslations(tag, file, base string, have map[string]string) {
	ids := make([]string, 0, len(u.tkeys))
	for id := range u.tkeys {
		if _, ok := have[id]; !ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		u.report(plxerr.TranslationMissing, file, base, "translation key %q has no message in %s", u.tkeys[id].Key, tag)
	}
}
