// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/riverqueue/river"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/jobs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/telemetry"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// analyticsEvery is how often the worker rolls events up, applies retention
// and exports (ANL-010, ANL-011): the width of the finest rollup.
const analyticsEvery = 5 * time.Minute

// buildAnalytics assembles ingestion and analytics storage. Events go
// through a Valkey stream to worker consumers when the shared cache is
// Valkey, and are written directly otherwise (ADR-0080).
func buildAnalytics(cfg *config.Config, db *storage.DB, shared cache.Cache, set limits.Set, ids interface{ New() (string, error) }, auditLog *audit.Log, tenants *tenancy.Service, work WorkDeps) (*telemetry.Service, *analytics.Service, *telemetry.Consumer, error) {
	store := work.AnalyticsStore
	if store == nil {
		store = analytics.NewPostgres(db, nil)
	}
	opts := telemetry.Options{
		DB: db, IDs: ids, Limits: set, Store: store,
		Retention: time.Duration(cfg.Telemetry.RetentionDays) * 24 * time.Hour,
	}
	if c, ok := shared.(cache.Counter); ok {
		opts.Counter = c
	}
	streams, queued := shared.(cache.Streams)
	if queued {
		opts.Queue = telemetry.StreamQueue{Streams: streams, MaxLen: int64(cfg.Telemetry.QueueMaxEntries)}
	}
	events, err := telemetry.NewService(opts)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("server: %w", err)
	}
	win, err := analytics.ParseWindow(cfg.Telemetry.MaintenanceWindow.Start, cfg.Telemetry.MaintenanceWindow.End, cfg.Telemetry.TimeZone)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("server: telemetry.maintenanceWindow: %w", err)
	}
	years := cfg.Retention.AuditYears
	svc, err := analytics.New(analytics.Options{
		DB: db, Store: store, Audit: auditLog, IDs: ids, Limits: set, Window: win,
		Defaults:      analytics.Defaults{RawDays: cfg.Telemetry.RetentionDays, AuditDays: years*365 + years/4},
		Organizations: tenants.ForEachOrganization,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("server: %w", err)
	}
	var consumer *telemetry.Consumer
	if queued && cfg.Has(config.RoleWorker) {
		host, _ := os.Hostname()
		consumer = &telemetry.Consumer{Streams: streams, Service: events, Name: host + "-" + strconv.Itoa(os.Getpid()), Log: work.Log}
	}
	return events, svc, consumer, nil
}

// AnalyticsMaintain is the periodic job that readies partitions, rolls
// events up and applies and enforces retention (ANL-010, Q17).
type AnalyticsMaintain struct{}

// Kind names the job.
func (AnalyticsMaintain) Kind() string { return "analytics.maintain" }

// InsertOpts puts the job on the maintenance queue, at most one waiting.
func (AnalyticsMaintain) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.QueueMaintenance, UniqueOpts: river.UniqueOpts{ByPeriod: analyticsEvery}}
}

// AnalyticsExport is the periodic job that sends new rollups to the
// organisations' OTLP destinations (ANL-011).
type AnalyticsExport struct{}

// Kind names the job.
func (AnalyticsExport) Kind() string { return "analytics.export" }

// InsertOpts puts the job on the maintenance queue, at most one waiting.
func (AnalyticsExport) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.QueueMaintenance, UniqueOpts: river.UniqueOpts{ByPeriod: analyticsEvery}}
}

type analyticsMaintainWorker struct {
	river.WorkerDefaults[AnalyticsMaintain]
	svc *analytics.Service
	log *slog.Logger
}

// Work runs one maintenance round; River retries it if any part failed.
func (w *analyticsMaintainWorker) Work(ctx context.Context, _ *river.Job[AnalyticsMaintain]) error {
	err := w.svc.Maintain(ctx)
	if err != nil && w.log != nil {
		w.log.WarnContext(ctx, "analytics maintenance", slog.Any("error", err))
	}
	return err //nolint:wrapcheck // a domain error
}

type analyticsExportWorker struct {
	river.WorkerDefaults[AnalyticsExport]
	svc *analytics.Service
	log *slog.Logger
}

// Work runs one export round; a failing destination is recorded on its
// configuration and retried at the next round, so the job itself succeeds.
func (w *analyticsExportWorker) Work(ctx context.Context, _ *river.Job[AnalyticsExport]) error {
	if err := w.svc.ExportAll(ctx); err != nil && w.log != nil {
		w.log.WarnContext(ctx, "analytics export", slog.Any("error", err))
	}
	return nil
}

// AnalyticsJobs registers the analytics workers and returns their schedule.
func AnalyticsJobs(workers *jobs.Workers, svc *Services, log *slog.Logger) []*river.PeriodicJob {
	if svc.Analytics == nil {
		return nil
	}
	jobs.AddWorker(workers, &analyticsMaintainWorker{svc: svc.Analytics, log: log})
	jobs.AddWorker(workers, &analyticsExportWorker{svc: svc.Analytics, log: log})
	return []*river.PeriodicJob{jobs.Every(analyticsEvery, AnalyticsMaintain{}), jobs.Every(analyticsEvery, AnalyticsExport{})}
}
