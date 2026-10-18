// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Translation workflow (I18N-008, ADR-0068). A translations document is an
// app-level document saved with app.manage under the draft's lock, or with
// translations.edit under the lock of its locale. A caller with only
// translations.edit changes only messages and states; the approved state
// needs translations.approve, and a text change without it resets the
// key to translated.

// Review states of a message, as the document spells them.
const (
	stateNew        = string(schema.TranslationStateNew)
	stateTranslated = string(schema.TranslationStateTranslated)
	stateApproved   = string(schema.TranslationStateApproved)
)

// translatorOnly reports whether a principal may edit translations on an
// app but cannot manage the app: such a caller touches nothing else.
func translatorOnly(p auth.Principal, appID string) bool {
	return !p.HoldsOnApp(auth.AppManage, appID) && p.HoldsOnApp(auth.TranslationsEdit, appID)
}

// guardTranslator refuses a write of the app-level draft that a caller
// holding only translations.edit may not make: anything but a save of a
// translations document, or, when onlySave is false, any change at all
// (PLX-8093). Everyone else passes to the ordinary permission check.
func guardTranslator(p auth.Principal, appID, pluginID, path string, onlySave bool) error {
	if pluginID != "" || !translatorOnly(p, appID) {
		return nil
	}
	if onlySave && scopeOf(path) != "" {
		return nil
	}
	return plxerr.New(plxerr.TranslationDocumentForbidden,
		"translations.edit lets you save the messages and states of translations/<locale>.json and nothing else; %s needs app.manage", path)
}

// reviewTranslations checks a prepared translations document against the
// stored one and the caller's permissions, and returns the change to
// write: the caller with only translations.edit changes messages and
// states only (PLX-8093); approving, or changing an approved text, needs
// translations.approve (PLX-8092); a changed text without it is saved as
// translated.
func (s *Service) reviewTranslations(ctx context.Context, q *dbgen.Queries, d draft, p auth.Principal, lim limits.Set, c change) (change, error) {
	depth := int(lim.Get(limits.DocumentJSONDepth))
	parsed, err := jcs.Parse(c.content, depth)
	if err != nil {
		return change{}, fmt.Errorf("document: read %s: %w", c.path, err)
	}
	next, ok := parsed.(map[string]any)
	if !ok {
		return change{}, plxerr.New(plxerr.InvalidStructure, "%s is not a JSON object", c.path)
	}
	prev, found, err := s.storedTree(ctx, q, d, c.path, depth)
	if err != nil {
		return change{}, err
	}
	if translatorOnly(p, d.app) {
		if !found {
			return change{}, plxerr.New(plxerr.TranslationDocumentForbidden, "%s does not exist; creating a locale needs app.manage", c.path)
		}
		if field, differs := otherField(prev, next); differs {
			return change{}, plxerr.New(plxerr.TranslationDocumentForbidden,
				"translations.edit lets you change messages and states only; %q differs in %s", field, c.path)
		}
	}
	adjusted, err := reviewStates(p, d.app, prev, next)
	if err != nil || !adjusted {
		return c, err
	}
	data, err := jcs.Marshal(next)
	if err != nil {
		return change{}, plxerr.Wrap(plxerr.InvalidStructure, err, "the adjusted document is not JSON")
	}
	again, _, err := s.prepare(d, lim, c.path, data, c.ifRevision)
	return again, err
}

// storedTree reads the stored document at a path as a JSON object; found
// is false when there is none.
func (s *Service) storedTree(ctx context.Context, q *dbgen.Queries, d draft, path string, depth int) (map[string]any, bool, error) {
	row, err := q.GetDocument(ctx, dbgen.GetDocumentParams{DraftID: d.row.ID, Path: path})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("document: read %s: %w", path, err)
	}
	content, err := s.blob(ctx, q, d.row.OrganizationID, row.Sha256)
	if err != nil {
		return nil, false, err
	}
	tree, err := jcs.Parse(content, depth)
	if err != nil {
		return nil, false, fmt.Errorf("document: read %s: %w", path, err)
	}
	obj, _ := tree.(map[string]any)
	return obj, obj != nil, nil
}

// otherField returns a top-level property other than messages and states
// that differs between two documents.
func otherField(prev, next map[string]any) (string, bool) {
	keys := make([]string, 0, len(prev)+len(next))
	for k := range prev {
		keys = append(keys, k)
	}
	for k := range next {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range slices.Compact(keys) {
		if k == "messages" || k == "states" {
			continue
		}
		a, aok := prev[k]
		b, bok := next[k]
		if aok != bok || !reflect.DeepEqual(a, b) {
			return k, true
		}
	}
	return "", false
}

// reviewPass is one application of the status rules to a save.
type reviewPass struct {
	p           auth.Principal
	appID       string
	approver    bool
	oldMessages map[string]any
	oldStates   map[string]any
	messages    map[string]any
	states      map[string]any
	adjusted    bool
	stepped     bool
}

// reviewStates applies the status rules to the states of next, given the
// stored document prev (nil for a new one). It reports whether it changed
// next.
func reviewStates(p auth.Principal, appID string, prev, next map[string]any) (bool, error) {
	r := &reviewPass{p: p, appID: appID, approver: p.HoldsOnApp(auth.TranslationsApprove, appID)}
	r.oldMessages, _ = prev["messages"].(map[string]any)
	r.oldStates, _ = prev["states"].(map[string]any)
	r.messages, _ = next["messages"].(map[string]any)
	r.states, _ = next["states"].(map[string]any)

	r.keepUnsaidStates()
	ids := make([]string, 0, len(r.messages)+len(r.states))
	for id := range r.messages {
		ids = append(ids, id)
	}
	for id := range r.states {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range slices.Compact(ids) {
		if err := r.review(id); err != nil {
			return false, err
		}
	}
	if r.adjusted {
		if len(r.states) == 0 {
			delete(next, "states")
		} else {
			next["states"] = r.states
		}
	}
	return r.adjusted, nil
}

// keepUnsaidStates keeps a state left out of the save: only an explicit
// state, or a changed text, alters it. The state of a message the save
// removes goes with it.
func (r *reviewPass) keepUnsaidStates() {
	for id, state := range r.oldStates {
		if _, said := r.states[id]; said {
			continue
		}
		if _, kept := r.messages[id]; !kept {
			continue
		}
		r.setState(id, state)
	}
}

// setState records a state for a key, creating the states object if needed.
func (r *reviewPass) setState(id string, state any) {
	if r.states == nil {
		r.states = map[string]any{}
	}
	r.states[id] = state
	r.adjusted = true
}

// review applies the rules to one key: approving, or changing an approved
// message, needs the approve permission; any other change by a non-approver
// leaves the message translated, or drops the state of a removed one.
func (r *reviewPass) review(id string) error {
	was := stateOf(r.oldStates, id)
	now := stateOf(r.states, id)
	before, hadBefore := r.oldMessages[id]
	after, hasAfter := r.messages[id]
	changed := hadBefore != hasAfter || !reflect.DeepEqual(before, after)
	switch {
	case now == stateApproved && (was != stateApproved || changed):
		if !r.approver {
			return plxerr.New(plxerr.TranslationStateForbidden,
				"approving %s, or changing the text of an approved message, needs translations.approve", id)
		}
		if !r.stepped {
			if err := authorize(r.p, auth.TranslationsApprove, r.appID); err != nil {
				return err
			}
			r.stepped = true
		}
	case changed && !r.approver:
		switch {
		case !hasAfter:
			if _, ok := r.states[id]; ok {
				delete(r.states, id)
				r.adjusted = true
			}
		case now != stateTranslated:
			r.setState(id, stateTranslated)
		}
	}
	return nil
}

// stateOf returns the review state of a key; a key with none is new.
func stateOf(states map[string]any, id string) string {
	if s, ok := states[id].(string); ok {
		return s
	}
	return stateNew
}
