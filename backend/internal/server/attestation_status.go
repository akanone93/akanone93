// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/nightCode42/plux3/backend/internal/jobs"
)

// attestationStatusEvery is how often the worker looks at Google's
// attestation status list. The list says how long it stays fresh, and a
// look within that time costs a cache read, so the interval only bounds how
// late a lost or expiring list is mended.
const attestationStatusEvery = 10 * time.Minute

// AttestationStatusRefresh is the periodic job that keeps Google's Android
// attestation status list in the shared cache (SEC-002).
type AttestationStatusRefresh struct{}

// Kind names the job.
func (AttestationStatusRefresh) Kind() string { return "attestation.status_list" }

// InsertOpts puts the job on the maintenance queue.
func (AttestationStatusRefresh) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.QueueMaintenance}
}

// statusListWorker runs the job.
type statusListWorker struct {
	river.WorkerDefaults[AttestationStatusRefresh]
	svc *Services
	log *slog.Logger
}

// Work fetches the list when it is due. A failure is logged and returned,
// so that River retries it; Android registrations meanwhile follow the
// attestationOutageGrace of their environment (SEC-009).
func (w *statusListWorker) Work(ctx context.Context, _ *river.Job[AttestationStatusRefresh]) error {
	if err := w.svc.AttestationStatus.Refresh(ctx); err != nil {
		if w.log != nil {
			w.log.WarnContext(ctx, "the Android attestation status list could not be refreshed", slog.Any("error", err))
		}
		return err //nolint:wrapcheck // a domain error
	}
	return nil
}

// AttestationStatusJobs registers the refresh worker and returns its
// schedule; nothing is scheduled where the list is an operator's file, which
// every replica reads itself.
func AttestationStatusJobs(workers *jobs.Workers, svc *Services, log *slog.Logger) []*river.PeriodicJob {
	if svc.AttestationStatus == nil || svc.AttestationStatus.FromFile() {
		return nil
	}
	jobs.AddWorker(workers, &statusListWorker{svc: svc, log: log})
	return []*river.PeriodicJob{jobs.Every(attestationStatusEvery, AttestationStatusRefresh{})}
}
