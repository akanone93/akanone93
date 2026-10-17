// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/l10n"
	"github.com/nightCode42/plux3/backend/internal/l10n/mf"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const (
	messageKeys = "translations/keys.json"
	messageEN   = "translations/en.json"
	messageDE   = "translations/de-DE.json"
	// messageKey is a key the test adds, which no document uses.
	messageKey = "01a0c450-6c00-7fff-8000-0000000000a1"
)

// withMessage adds a key with arguments and a limit, and sets its English
// message.
func withMessage(t *testing.T, text string, maxLength int) fstest.MapFS {
	t.Helper()
	m := fixture(t)
	edit(t, m, messageKeys, func(doc map[string]any) {
		k := map[string]any{"id": messageKey, "key": "test.message"}
		k["args"] = raw(t, `[{"name": "n", "type": "int"}, {"name": "who", "type": "string"}, {"name": "price", "type": "money"}]`)
		if maxLength > 0 {
			k["maxLength"] = maxLength
		}
		doc["keys"] = append(doc["keys"].([]any), k)
	})
	edit(t, m, messageEN, func(doc map[string]any) { at(t, doc, "messages")[messageKey] = text })
	return m
}

// Verifies: I18N-001.
func TestMessagesAreCheckedAgainstTheirKey(t *testing.T) {
	t.Parallel()
	ptr := "/messages/" + messageKey
	cases := []struct {
		name string
		text string
		max  int
		code plxerr.Code
	}{
		{"syntax", "{n, plural, one {x}", 0, plxerr.MessageSyntaxInvalid},
		{"undeclared argument", "Hello {nobody}", 0, plxerr.MessageArgumentUndeclared},
		{"plural over a string", "{who, plural, other {x}}", 0, plxerr.MessageArgumentTypeMismatch},
		{"money is not a message argument", "{price}", 0, plxerr.MessageArgumentTypeMismatch},
		{"category the locale lacks", "{n, plural, few {x} other {y}}", 0, plxerr.PluralCategoryInvalid},
		{"ordinal category the locale lacks", "{n, selectordinal, many {x} other {y}}", 0, plxerr.PluralCategoryInvalid},
		{"other missing", "{n, plural, one {x}}", 0, plxerr.PluralOtherMissing},
		{"select without other", "{who, select, a {x}}", 0, plxerr.PluralOtherMissing},
		{"too long", "A rather long label {who}", 10, plxerr.TranslationTooLong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			res := compileFS(withMessage(t, c.text, c.max))
			wantDiag(t, res, c.code, messageEN, ptr)
		})
	}
}

// Verifies: I18N-001.
func TestValidMessageIsEncodedIntoTheBundle(t *testing.T) {
	t.Parallel()
	const text = "{n, plural, one {# thing} other {# things}} for {who}"
	res := compileFS(withMessage(t, text, 60))
	for _, d := range res.Diagnostics {
		if d.Severity == plxerr.SeverityError {
			t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
		}
	}
}

// Verifies: I18N-001, I18N-006.
func TestMissingTranslationIsAWarning(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	edit(t, m, messageDE, func(doc map[string]any) { delete(at(t, doc, "messages"), "01a0c450-6c00-7003-8000-000000005ccd") })
	res := compileFS(m)
	wantDiag(t, res, plxerr.TranslationMissing, messageDE, "/messages")
	if res.Diagnostics.HasErrors() {
		t.Errorf("a missing translation must not be an error:\n%s", list(res.Diagnostics))
	}
}

// Verifies: I18N-001.
func TestLocaleSectionCarriesTheAST(t *testing.T) {
	t.Parallel()
	const text = "{n, plural, one {# thing} other {# things}}"
	res := compileFS(withMessage(t, text, 0))
	id, err := bundle.LocaleID("en")
	if err != nil {
		t.Fatal(err)
	}
	s, ok := readAll(t, res)[0].Section(bundle.SectionL10n, id)
	if !ok {
		t.Fatal("no en section")
	}
	loc := fbs.GetRootAsLocale(s.Data, 0)
	key := uuidBytes(messageKey)
	hi, lo := uuidHalves(key)
	found := false
	for i := range loc.MessagesLength() {
		var m fbs.Message
		loc.Messages(&m, i)
		k := m.Key(nil)
		if k.Hi() != hi || k.Lo() != lo {
			continue
		}
		found = true
		if string(m.Text()) != text {
			t.Errorf("text = %q", m.Text())
		}
		ast, err := mf.Decode(m.AstBytes())
		if err != nil {
			t.Fatal(err)
		}
		en, err := l10n.Load("en")
		if err != nil {
			t.Fatal(err)
		}
		got, err := ast.Format(en, map[string]any{"n": 3})
		if err != nil || got != "3 things" {
			t.Errorf("ast renders %q, %v", got, err)
		}
	}
	if !found {
		t.Fatal("message not in the section")
	}
}
