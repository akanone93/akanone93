// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"errors"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// DraftFiles reads the current state of an app's draft and of the draft of
// each of its plugins as one file set in the Git layout, with the app's
// asset files as they are now: what a dev session compiles (DEV-010). Unlike
// FreezeDraft it keeps nothing; a draft that was never written contributes
// nothing. It runs in the caller's transaction, which is bound to the
// draft's organisation.
func (s *Service) DraftFiles(ctx context.Context, tx pgx.Tx, appID string) (fstest.MapFS, error) {
	app, err := parseID(appID, "app")
	if err != nil {
		return nil, err
	}
	q := dbgen.New(tx)
	plugins, err := q.ListAllPlugins(ctx, app)
	if err != nil {
		return nil, failure(err, "plugin")
	}
	owners := []pgtype.UUID{{}}
	for _, pl := range plugins {
		owners = append(owners, pl.ID)
	}
	snapshots := make([]string, 0, len(owners))
	for _, plugin := range owners {
		d, err := q.GetDraft(ctx, dbgen.GetDraftParams{AppID: app, PluginID: plugin})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, failure(err, "draft")
		}
		if d.Revision == 0 {
			continue
		}
		snap, err := q.SnapshotAtRevision(ctx, dbgen.SnapshotAtRevisionParams{DraftID: d.ID, Revision: d.Revision})
		if err != nil {
			return nil, failure(err, "snapshot")
		}
		snapshots = append(snapshots, storage.ID(snap.ID))
	}
	return s.SourceFiles(ctx, tx, appID, snapshots)
}
