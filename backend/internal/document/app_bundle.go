// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// AppBundleChanged reports whether the app's own documents (app.json, the
// theme, the translations: the app bundle, plugin key "") changed after its
// newest published version, or whether no version was published yet while
// the draft has content. A release of plugins that read a new translation key
// or theme token needs that bundle too, so a release request includes it
// (STU-100, BND-002).
func AppBundleChanged(ctx context.Context, q *dbgen.Queries, app pgtype.UUID) (bool, error) {
	draft, err := q.GetDraft(ctx, dbgen.GetDraftParams{AppID: app, PluginID: pgtype.UUID{}})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, failure(err, "draft")
	}
	latest, err := q.LatestVersion(ctx, dbgen.LatestVersionParams{AppID: app, PluginID: pgtype.UUID{}})
	if errors.Is(err, pgx.ErrNoRows) {
		return draft.Revision > 0, nil
	}
	if err != nil {
		return false, failure(err, "version")
	}
	return draft.UpdatedAt.Time.After(latest.CreatedAt.Time), nil
}

// AppUnpublishedChanges is AppBundleChanged for a person who may read the
// app's plugins: the app canvas and the publish flow show it.
func (s *Service) AppUnpublishedChanges(ctx context.Context, p auth.Principal, appID string) (bool, error) {
	var out bool
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.resolve(ctx, q, p, appID, "")
		if err != nil {
			return err
		}
		if err := authorize(p, auth.PluginRead, appID); err != nil {
			return err
		}
		out, err = AppBundleChanged(ctx, q, d.row.AppID)
		return err
	})
	return out, err
}
