// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/bundle"
	"github.com/nightCode42/plux3/backend/internal/bundle/fbs"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const unknownMessageKey = "01a0c450-6c00-7fff-8000-0000000009ff"

// withValidatorMessage gives the calculator page a form whose one validator
// carries message.
func withValidatorMessage(t *testing.T, m fstest.MapFS, message any) {
	t.Helper()
	edit(t, m, calculatorPage, func(doc map[string]any) {
		doc["forms"] = []any{map[string]any{
			"id":   "01a0c450-6c00-7fff-8000-0000000009a1",
			"name": "contact",
			"fields": []any{map[string]any{
				"name": "email", "type": "string", "initial": "",
				"validators": []any{map[string]any{"kind": "required", "message": message}},
			}},
		}}
	})
}

// Verifies: I18N-001. A validator message that is a translation reference
// compiles to a message reference, adds a reference-graph edge, and does not
// warn as literal text.
func TestValidatorMessageReference(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	withValidatorMessage(t, m, map[string]any{"$t": keyLoansTitle})
	res := compileFS(m)
	if len(res.Diagnostics) != 0 {
		t.Fatalf("diagnostics:\n%s", list(res.Diagnostics))
	}
	if !slices.ContainsFunc(res.Graph.Edges, func(e Edge) bool {
		return e.Kind == EdgeUsesTranslation && e.To == keyLoansTitle && e.File == calculatorPage
	}) {
		t.Error("no translation edge from the validator message")
	}
	found := false
	for _, b := range readAll(t, res) {
		for _, s := range b.Sections {
			if s.Kind != bundle.SectionPage {
				continue
			}
			pg := fbs.GetRootAsPage(s.Data, 0)
			var f fbs.Form
			if pg.FormsLength() == 0 || !pg.Forms(&f, 0) {
				continue
			}
			var fd fbs.FormField
			var v fbs.FormValidator
			f.Fields(&fd, 0)
			fd.Validators(&v, 0)
			ref := v.MessageRef(nil)
			if ref == nil || ref.Kind() != fbs.ValueKindTranslation || v.Message() != 0 {
				t.Fatalf("validator message not encoded as a reference")
			}
			found = true
		}
	}
	if !found {
		t.Error("the form is not in the bundle")
	}
}

// Verifies: I18N-001. A reference to a key the project does not declare is
// the unresolved-reference diagnostic.
func TestValidatorMessageUnknownKey(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	withValidatorMessage(t, m, map[string]any{"$t": unknownMessageKey})
	res := compileFS(m)
	wantDiag(t, res, plxerr.UnresolvedReference, calculatorPage, "/forms/0/fields/0/validators/0/message/$t")
}

// Verifies: I18N-001, I18N-007. A literal validator message in an app with
// two locales warns (PLX-1609); a referenced one does not.
func TestLiteralValidatorMessageWarns(t *testing.T) {
	t.Parallel()
	m := fixture(t)
	withValidatorMessage(t, m, "Enter your email")
	res := compileFS(m)
	wantDiag(t, res, plxerr.LiteralTextUntranslated, calculatorPage, "/forms/0/fields/0/validators/0/message")
	if res.Diagnostics.HasErrors() {
		t.Errorf("a literal must only warn:\n%s", list(res.Diagnostics))
	}
}
