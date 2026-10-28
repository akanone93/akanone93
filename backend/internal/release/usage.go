// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Usage measures what a plugin uses of its limits (LIM-005): its pages, the
// size of its newest bundle and the bytes of its golden baselines. It is
// registered with tenancy, whose plugin limit listing reports it; tenancy
// measures the app-scope limits itself.
func (*Service) Usage(ctx context.Context, tx pgx.Tx, appID, pluginID string) (map[limits.Key]int64, error) {
	q := dbgen.New(tx)
	app, err := parseID(appID, "app")
	if err != nil {
		return nil, err
	}
	out := map[limits.Key]int64{}
	if pluginID == "" {
		// The app's usage is measured by tenancy, in one query for a page of apps.
		return out, nil
	}
	plugin, err := parseID(pluginID, "plugin")
	if err != nil {
		return nil, err
	}
	if out[limits.PluginPages], err = q.CountPages(ctx, dbgen.CountPagesParams{AppID: app, PluginID: plugin}); err != nil {
		return nil, failure(err, "page")
	}
	v, err := q.LatestVersion(ctx, dbgen.LatestVersionParams{AppID: app, PluginID: plugin})
	switch {
	case err == nil:
		out[limits.BundlePluginSize] = v.BundleSize
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, failure(err, "version")
	}
	if out[limits.GoldenBaselineBytesPerPlugin], err = q.PluginGoldenBytes(ctx, dbgen.PluginGoldenBytesParams{AppID: app, ID: plugin}); err != nil {
		return nil, failure(err, "golden baseline")
	}
	return out, nil
}
