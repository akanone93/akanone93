// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// syncAppPlugins makes the app document's plugins list name exactly the app's live plugins, in the transaction that
// created, deleted or restored one (SCH-006, STU-034). The listed order is kept; a plugin that is not listed yet
// follows the plugin `after` when it is listed (a duplicate sits next to its source), and otherwise goes last. The
// layout check refuses a project whose list and plugin directories disagree (PLX-1020), so a plugin made in Studio
// used to leave the app invalid until someone edited app.json by hand.
//
// The write is a snapshot of the app's draft made by the server for the person who changed the plugins; it is not
// subject to the app draft's editing lock, because the list belongs to the plugin set, which the caller already has
// the right to change. An editor holding app.json at the old revision replays its edits on the new one (SRV-030).
// An app document that does not parse is left alone: its own diagnostics say what is wrong, and a list cannot be
// merged into a document that cannot be read. An app with no live plugin keeps its list, as the schema needs one
// entry; the layout check then names the missing plugin.
func (s *Service) syncAppPlugins(ctx context.Context, tx pgx.Tx, p auth.Principal, appID, after string) error {
	q := dbgen.New(tx)
	d, err := s.resolve(ctx, q, p, appID, "")
	if err != nil {
		return err
	}
	if _, err := q.GetDraftForUpdate(ctx, d.row.ID); err != nil {
		return failure(err, "draft")
	}
	d.tx = tx
	existing, err := q.GetDocumentForUpdate(ctx, dbgen.GetDocumentForUpdateParams{DraftID: d.row.ID, Path: appDocument})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("document: read %s: %w", appDocument, err)
	}
	content, err := s.blob(ctx, q, d.row.OrganizationID, existing.Sha256)
	if err != nil {
		return err
	}
	doc, listed, readable := readAppPlugins(content)
	if !readable {
		return nil
	}
	rows, err := q.ListAllPlugins(ctx, d.row.AppID)
	if err != nil {
		return failure(err, "plugin")
	}
	live := make([]string, 0, len(rows))
	for _, r := range rows {
		live = append(live, r.Key)
	}
	next := reconcilePlugins(listed, live, after)
	if len(next) == 0 || slices.Equal(next, listed) {
		return nil
	}
	doc["plugins"], _ = json.Marshal(next)
	updated, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("document: write %s: %w", appDocument, err)
	}
	lim, err := s.limitsFor(ctx, tx, d)
	if err != nil {
		return err
	}
	c, _, err := s.prepare(d, lim, appDocument, updated, existing.Revision)
	if err != nil {
		// The rest of app.json is invalid (its own diagnostics say why); the plugin change still stands.
		return nil //nolint:nilerr // an unreadable app document is reported by validation, not by this write
	}
	_, err = s.apply(ctx, q, tx, d, p, []change{c}, reasonWrite)
	return err
}

// readAppPlugins reads an app document as members and its plugins list; false when it cannot be read.
func readAppPlugins(content []byte) (map[string]json.RawMessage, []string, bool) {
	var doc map[string]json.RawMessage
	if json.Unmarshal(content, &doc) != nil {
		return nil, nil, false
	}
	var listed []string
	if raw, ok := doc["plugins"]; ok && json.Unmarshal(raw, &listed) != nil {
		return nil, nil, false
	}
	return doc, listed, true
}

// reconcilePlugins keeps the listed keys that are live, in their order, and adds each live key that is missing: after
// `after` when it is listed, otherwise at the end in key order.
func reconcilePlugins(listed, live []string, after string) []string {
	out := make([]string, 0, len(live))
	for _, k := range listed {
		if slices.Contains(live, k) && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	var missing []string
	for _, k := range live {
		if !slices.Contains(out, k) {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return out
	}
	if i := slices.Index(out, after); after != "" && i >= 0 {
		return slices.Concat(out[:i+1], missing, out[i+1:])
	}
	return append(out, missing...)
}

// appIDOfPlugin is the app of a plugin row, deleted or not.
func appIDOfPlugin(ctx context.Context, q *dbgen.Queries, pluginID string) (string, error) {
	row, err := q.GetPluginIncludingDeleted(ctx, storage.MustUUID(pluginID))
	if err != nil {
		return "", failure(err, "plugin")
	}
	return storage.ID(row.AppID), nil
}
