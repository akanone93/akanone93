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

// reviewStates applies the status rules to the states of next, given the
// stored document prev (nil for a new one). It reports whether it changed
// next.
func reviewStates(p auth.Principal, appID string, prev, next map[string]any) (bool, error) {
	oldMessages, _ := prev["messages"].(map[string]any)
	oldStates, _ := prev["states"].(map[string]any)
	messages, _ := next["messages"].(map[string]any)
	states, _ := next["states"].(map[string]any)
	approver := p.HoldsOnApp(auth.TranslationsApprove, appID)

	ids := make([]string, 0, len(messages)+len(states))
	for id := range messages {
		ids = append(ids, id)
	}
	for id := range states {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	adjusted, stepped := false, false
	for _, id := range slices.Compact(ids) {
		was := stateOf(oldStates, id)
		now := stateOf(states, id)
		before, hadBefore := oldMessages[id]
		after, hasAfter := messages[id]
		changed := hadBefore != hasAfter || !reflect.DeepEqual(before, after)
		switch {
		case now == stateApproved && (was != stateApproved || changed):
			if !approver {
				return false, plxerr.New(plxerr.TranslationStateForbidden,
					"approving %s, or changing the text of an approved message, needs translations.approve", id)
			}
			if !stepped {
				if err := authorize(p, auth.TranslationsApprove, appID); err != nil {
					return false, err
				}
				stepped = true
			}
		case changed && !approver:
			switch {
			case !hasAfter:
				if _, ok := states[id]; ok {
					delete(states, id)
					adjusted = true
				}
			case now != stateTranslated:
				if states == nil {
					states = map[string]any{}
				}
				states[id] = stateTranslated
				adjusted = true
			}
		}
	}
	if adjusted {
		if len(states) == 0 {
			delete(next, "states")
		} else {
			next["states"] = states
		}
	}
	return adjusted, nil
}

// stateOf returns the review state of a key; a key with none is new.
func stateOf(states map[string]any, id string) string {
	if s, ok := states[id].(string); ok {
		return s
	}
	return stateNew
}
