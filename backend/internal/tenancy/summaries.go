// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package tenancy

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// EnvironmentRelease is the release an environment serves.
type EnvironmentRelease struct {
	Environment string
	Sequence    int64
	PublishedAt time.Time
}

// Usage is a limit with what an app uses of it.
type Usage struct {
	Key   string
	Used  int64
	Limit int64
}

// AppSummary is what the apps grid shows of one app (plan §4.3 #14).
type AppSummary struct {
	App             App
	Releases        []EnvironmentRelease
	LastPublishedAt time.Time
	// LastPublisher is who created the newest release; nil when the app was
	// never published.
	LastPublisher *Publisher
	ActiveDevices int64
	PluginCount   int64
	// CrashFreeRate is the share of active devices without a fatal error in
	// the last seven days; CrashFreeTrend its change against the seven days
	// before.
	CrashFreeRate  float64
	CrashFreeTrend float64
	Limits         []Usage
}

// ListAppSummaries returns one summary per app the principal may read, in
// key order after afterKey, in a bounded number of queries: one for the
// apps with their counts, one for the live releases. The page is capped at
// app.summariesPerCall (LIM-001); next is the key to continue after, or ""
// at the end.
func (s *Service) ListAppSummaries(ctx context.Context, p auth.Principal, afterKey string, size int32) (out []AppSummary, next string, err error) {
	allowed := p.AllowedApps()
	var only []pgtype.UUID
	if allowed != nil {
		if len(allowed) == 0 {
			return nil, "", authorize(p, auth.AppRead)
		}
		for _, id := range allowed {
			only = append(only, storage.MustUUID(id))
		}
	}
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		org, err := s.Effective(ctx, tx, p.OrganizationID, "", "")
		if err != nil {
			return err
		}
		cap := int32(org.Get(limits.AppSummariesPerCall)) //nolint:gosec // a registry limit
		if size <= 0 || size > cap {
			size = cap
		}
		q := dbgen.New(tx)
		rows, err := q.SummariseApps(ctx, dbgen.SummariseAppsParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), OnlyIds: only, ExceptIds: excludedApps(p),
			AfterKey: afterKey, PageSize: size + 1,
		})
		if err != nil {
			return failure(err, "app")
		}
		if int32(len(rows)) > size { //nolint:gosec // bounded by the page size
			rows = rows[:size]
			next = rows[size-1].Key
		}
		out, err = s.summaries(ctx, tx, p, q, rows)
		return err
	})
	return out, next, err
}

// summaries adds the live releases, the last publisher, the measured usage and the
// effective limits to the rows, each in one query for the whole page.
func (s *Service) summaries(ctx context.Context, tx pgx.Tx, p auth.Principal, q *dbgen.Queries, rows []dbgen.SummariseAppsRow) ([]AppSummary, error) {
	ids := make([]pgtype.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	live, err := q.LiveReleases(ctx, ids)
	if err != nil {
		return nil, failure(err, "release")
	}
	byApp := map[string][]EnvironmentRelease{}
	for _, l := range live {
		id := storage.ID(l.AppID)
		byApp[id] = append(byApp[id], EnvironmentRelease{Environment: l.EnvironmentKey, Sequence: l.ReleaseSequence, PublishedAt: timeOf(l.PublishedAt)})
	}
	used, err := measureApps(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	who, err := lastPublishers(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	out := make([]AppSummary, 0, len(rows))
	for _, r := range rows {
		id := storage.ID(r.ID)
		set, err := s.Effective(ctx, tx, p.OrganizationID, id, "")
		if err != nil {
			return nil, err
		}
		rate, trend := crashFree(r.ActiveDevices, r.CrashedDevices, r.CrashedDevicesBefore)
		var usage []Usage
		for _, k := range appUsageKeys {
			usage = append(usage, Usage{Key: string(k), Used: used[id][k], Limit: set.Get(k)})
		}
		var publisher *Publisher
		if w, ok := who[id]; ok {
			publisher = &w
		}
		out = append(out, AppSummary{
			App: appOf(appRow(r)), Releases: byApp[id], LastPublishedAt: timeOf(r.LastPublishedAt),
			ActiveDevices: r.ActiveDevices, PluginCount: r.PluginCount, CrashFreeRate: rate, CrashFreeTrend: trend,
			LastPublisher: publisher, Limits: usage,
		})
	}
	return out, nil
}

// crashFree returns the crash-free rate of the last seven days and its
// change against the seven days before. The same active devices are the
// denominator of both periods, because the earlier set of active devices is
// not kept; with no active device the rate is 1 and the trend 0.
func crashFree(active, crashed, before int64) (rate, trend float64) {
	if active <= 0 {
		return 1, 0
	}
	share := func(n int64) float64 { return 1 - float64(min(n, active))/float64(active) }
	return share(crashed), share(crashed) - share(before)
}

func timeOf(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

// appRow is the app part of a summary row.
func appRow(r dbgen.SummariseAppsRow) dbgen.App {
	return dbgen.App{
		ID: r.ID, OrganizationID: r.OrganizationID, Key: r.Key, Name: r.Name, DefaultPluginKey: r.DefaultPluginKey,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, DeletedAt: r.DeletedAt, Description: r.Description,
		IconAssetID: r.IconAssetID, OwningTeamID: r.OwningTeamID, Tags: r.Tags, ArchivedAt: r.ArchivedAt,
	}
}
