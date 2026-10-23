// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/rollout"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// buildRollouts assembles the rollout service and makes the release service
// ask it which release a device receives (REL-050 to REL-053). It returns nil
// where there is no release service. Health gates read the analytics rollups
// and are left unevaluated without them.
func buildRollouts(db *storage.DB, log *audit.Log, ids interface{ New() (string, error) }, ten *tenancy.Service,
	releases *release.Service, analyticsService *analytics.Service, notifier *notify.Service, logger *slog.Logger,
) (*rollout.Service, error) {
	if releases == nil {
		return nil, nil
	}
	o := rollout.Options{
		DB: db, Audit: log, IDs: ids, Releases: releases, Log: logger,
		Limits: func(ctx context.Context, tx pgx.Tx, org, app string) (limits.Set, error) {
			return ten.Effective(ctx, tx, org, app, "") //nolint:wrapcheck // a domain error
		},
	}
	if analyticsService != nil {
		o.Signals = rollout.AnalyticsSignals{Store: analyticsService.Rollups()}
	}
	if notifier != nil {
		o.Notify = notifier
	}
	svc, err := rollout.NewService(o)
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	releases.UseRollouts(svc)
	return svc, nil
}
