// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package tenancy

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// appUsageKeys are the app-scope limits whose current value the server
// measures, in the order the apps grid shows them (STU-025, LIM-005). The
// other app-scope limits bound one request, one invocation, one device or
// one period (rates, timeouts, per-message and per-file sizes of the runtime,
// retention, dev-session and gateway limits), or count something the server
// does not hold per app (translation keys live inside a document's content),
// so they have no single current value and the listing reports them with a
// value of zero. Per-plugin limits are reported as the largest value any
// plugin of the app has.
var appUsageKeys = []limits.Key{
	limits.AppPlugins, limits.ReleaseAppSize, limits.PluginPages, limits.BundlePluginSize,
	limits.PluginAssetBytes, limits.AssetFileSize, limits.AppFunctions, limits.AppLocales, limits.FlagsPerApp,
}

// measureApps measures the app-scope usage of a set of apps in one query,
// under the row-level security of the caller's transaction: an app of another
// organisation is not seen and has no entry.
func measureApps(ctx context.Context, tx pgx.Tx, ids []pgtype.UUID) (map[string]map[limits.Key]int64, error) {
	rows, err := dbgen.New(tx).AppUsage(ctx, ids)
	if err != nil {
		return nil, failure(err, "app")
	}
	out := make(map[string]map[limits.Key]int64, len(rows))
	for _, r := range rows {
		out[storage.ID(r.AppID)] = map[limits.Key]int64{
			limits.AppPlugins:       r.Plugins,
			limits.ReleaseAppSize:   r.ReleaseSize,
			limits.PluginPages:      r.MaxPages,
			limits.BundlePluginSize: r.MaxBundleSize,
			limits.PluginAssetBytes: r.AssetBytes,
			limits.AssetFileSize:    r.MaxAssetFile,
			limits.AppFunctions:     r.Functions,
			limits.AppLocales:       r.Locales,
			limits.FlagsPerApp:      r.Flags,
		}
	}
	return out, nil
}

// Publisher is who created a release (STU-022).
type Publisher struct {
	Kind, ID, Display string
}

// lastPublishers returns, per app, who created its newest release and when.
func lastPublishers(ctx context.Context, q *dbgen.Queries, ids []pgtype.UUID) (map[string]Publisher, error) {
	rows, err := q.LastReleasePublishers(ctx, ids)
	if err != nil {
		return nil, failure(err, "release")
	}
	out := make(map[string]Publisher, len(rows))
	for _, r := range rows {
		out[storage.ID(r.AppID)] = Publisher{Kind: r.CreatedByKind, ID: r.CreatedByID, Display: r.CreatedBy}
	}
	return out, nil
}
