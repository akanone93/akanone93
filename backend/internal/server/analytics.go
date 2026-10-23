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

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/jobs"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
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
func buildAnalytics(cfg *config.Config, db *storage.DB, shared cache.Cache, set limits.Set, ids interface{ New() (string, error) }, auditLog *audit.Log, tenants *tenancy.Service, notifier *notify.Service, crypter signing.Crypter, work WorkDeps) (*telemetry.Service, *analytics.Service, *telemetry.Consumer, error) {
	store := work.AnalyticsStore
	if store == nil {
		store = analytics.NewPostgres(db, nil)
	}
	win, err := analytics.ParseWindow(cfg.Telemetry.MaintenanceWindow.Start, cfg.Telemetry.MaintenanceWindow.End, cfg.Telemetry.TimeZone)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("server: telemetry.maintenanceWindow: %w", err)
	}
	years := cfg.Retention.AuditYears
	aopts := analytics.Options{
		DB: db, Store: store, Audit: auditLog, IDs: ids, Limits: set, Window: win,
		Defaults:      analytics.Defaults{RawDays: cfg.Telemetry.RetentionDays, AuditDays: years*365 + years/4},
		Organizations: tenants.ForEachOrganization, Objects: work.Objects,
		Exporter: work.AnalyticsExporter, Crypter: crypter,
	}
	if notifier != nil {
		aopts.Notify = notifier
	}
	svc, err := analytics.New(aopts)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("server: %w", err)
	}
	opts := telemetry.Options{
		DB: db, IDs: ids, Limits: set, Store: store, Errors: svc,
		Retention: time.Duration(cfg.Telemetry.RetentionDays) * 24 * time.Hour,
	}
	opts.Changes = telemetry.PostgresControlChanges{DB: db}
	if work.Metrics != nil {
		opts.Propagation = work.Metrics
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

// AnalyticsAlerts is the periodic job that evaluates alert rules against
// the rollups (ANL-030). River's unique period makes one evaluation per
// minute across all workers.
type AnalyticsAlerts struct{}

// Kind names the job.
func (AnalyticsAlerts) Kind() string { return "analytics.alerts" }

// InsertOpts puts the job on the maintenance queue, at most one waiting.
func (AnalyticsAlerts) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.QueueMaintenance, UniqueOpts: river.UniqueOpts{ByPeriod: analytics.AlertEvery}}
}

type analyticsAlertsWorker struct {
	river.WorkerDefaults[AnalyticsAlerts]
	svc *analytics.Service
	log *slog.Logger
}

// Work evaluates the rules once; a rule that fails is retried at the next
// minute, so the job itself succeeds.
func (w *analyticsAlertsWorker) Work(ctx context.Context, _ *river.Job[AnalyticsAlerts]) error {
	if err := w.svc.EvaluateAlerts(ctx); err != nil && w.log != nil {
		w.log.WarnContext(ctx, "analytics alerts", slog.Any("error", err))
	}
	return nil
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
	svc     *analytics.Service
	edition edition.Provider
	log     *slog.Logger
}

// Work runs one export round, the OTLP destinations and then the enterprise
// Parquet and webhook exports; a failing destination is recorded on its
// configuration and retried at the next round, so the job itself succeeds.
// The licence is put on the context because a job does not carry it.
func (w *analyticsExportWorker) Work(ctx context.Context, _ *river.Job[AnalyticsExport]) error {
	if err := w.svc.ExportAll(ctx); err != nil && w.log != nil {
		w.log.WarnContext(ctx, "analytics export", slog.Any("error", err))
	}
	if w.edition != nil {
		ctx = edition.WithProvider(ctx, w.edition)
	}
	if err := w.svc.ExportFiles(ctx); err != nil && w.log != nil {
		w.log.WarnContext(ctx, "analytics file export", slog.Any("error", err))
	}
	return nil
}

// AnalyticsJobs registers the analytics workers and returns their schedule.
func AnalyticsJobs(workers *jobs.Workers, svc *Services, provider edition.Provider, log *slog.Logger) []*river.PeriodicJob {
	if svc.Analytics == nil {
		return nil
	}
	jobs.AddWorker(workers, &analyticsMaintainWorker{svc: svc.Analytics, log: log})
	jobs.AddWorker(workers, &analyticsExportWorker{svc: svc.Analytics, edition: provider, log: log})
	jobs.AddWorker(workers, &analyticsAlertsWorker{svc: svc.Analytics, log: log})
	return []*river.PeriodicJob{
		jobs.Every(analyticsEvery, AnalyticsMaintain{}), jobs.Every(analyticsEvery, AnalyticsExport{}),
		jobs.Every(analytics.AlertEvery, AnalyticsAlerts{}),
	}
}
