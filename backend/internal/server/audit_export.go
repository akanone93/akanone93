// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/jobs"
)

// auditExportInterval is how often the worker looks for audit entries to
// deliver. A destination that is behind keeps going within one run.
const auditExportInterval = 15 * time.Second

// AuditExport is the periodic job that delivers the audit chains to their
// SIEM destinations (SEC-143).
type AuditExport struct{}

// Kind names the job.
func (AuditExport) Kind() string { return "audit.export" }

// InsertOpts puts the job on the maintenance queue.
func (AuditExport) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.QueueMaintenance}
}

// auditExportWorker runs the job. It exists only in the worker role, which
// holds no signing key the export could use and none it needs (L-4).
type auditExportWorker struct {
	river.WorkerDefaults[AuditExport]
	svc     *Services
	edition edition.Provider
	log     *slog.Logger
}

// Work makes one delivery pass. Two passes may overlap: a destination is
// locked by the pass that is delivering to it.
func (w *auditExportWorker) Work(ctx context.Context, _ *river.Job[AuditExport]) error {
	if w.edition != nil {
		ctx = edition.WithProvider(ctx, w.edition)
	}
	return w.svc.AuditExporter.Run(ctx) //nolint:wrapcheck // a domain error
}

// AuditExportJobs registers the export worker and returns its schedule; a
// process without an exporter registers and schedules nothing.
func AuditExportJobs(workers *jobs.Workers, svc *Services, provider edition.Provider, log *slog.Logger) []*river.PeriodicJob {
	if svc.AuditExporter == nil {
		return nil
	}
	jobs.AddWorker(workers, &auditExportWorker{svc: svc, edition: provider, log: log})
	return []*river.PeriodicJob{jobs.Every(auditExportInterval, AuditExport{})}
}
