// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/jobs"
	"github.com/nightCode42/plux3/backend/internal/releasereq"
	"github.com/nightCode42/plux3/backend/internal/renderclient"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// renderDeferral is how long a render job waits when its organisation already
// has render.visualDiffJobsPerOrganization jobs running.
const renderDeferral = 30 * time.Second

// renderQueue enqueues visual-diff render jobs (O-9).
type renderQueue struct{ q *JobQueue }

// Enqueue adds a render job on its own queue in the caller's transaction.
func (r renderQueue) Enqueue(ctx context.Context, tx pgx.Tx, job releasereq.RenderJob) error {
	return r.q.insert(ctx, tx, job, jobs.QueueRender)
}

// renderRunner is what the worker needs of the release request service.
type renderRunner interface {
	RunRender(ctx context.Context, job releasereq.RenderJob) error
	FailRender(ctx context.Context, job releasereq.RenderJob, attempts int) error
}

// renderWorker runs releasereq.render jobs. It runs only in a process given a
// renderer; the renderer itself runs in the render worker image, which holds
// no database credentials and no signing capability (L-4).
type renderWorker struct {
	river.WorkerDefaults[releasereq.RenderJob]
	runner renderRunner
	log    *slog.Logger
}

// Work runs one job. A job over its organisation's limit waits in the queue;
// after the last attempt a failure also fails the job, so that none stays
// running for ever.
func (w *renderWorker) Work(ctx context.Context, job *river.Job[releasereq.RenderJob]) error {
	err := w.runner.RunRender(ctx, job.Args)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, releasereq.ErrDeferred):
		return river.JobSnooze(renderDeferral) //nolint:wrapcheck // the queue reads this value
	case job.Attempt < job.MaxAttempts:
		return err //nolint:wrapcheck // a domain error
	}
	w.log.ErrorContext(ctx, "a render job gave up", slog.String("job", job.Args.JobID), slog.Int("attempts", job.Attempt), slog.String("error", err.Error()))
	if failErr := w.runner.FailRender(ctx, job.Args, job.Attempt); failErr != nil {
		return errors.Join(err, failErr)
	}
	return err //nolint:wrapcheck // a domain error
}

// buildReleaseRequests assembles the release request service, which needs the
// release service for builds and releases.
func (k *serviceKit) buildReleaseRequests(svc *Services) *releasereq.Service {
	if svc.Releases == nil {
		return nil
	}
	var functions releasereq.Functions
	if svc.Functions != nil {
		functions = svc.Functions
	}
	return releasereq.New(releasereq.Options{
		Functions: functions,
		DB:        k.db, IDs: k.gen, Builds: releasereq.ReleaseBuilds{Releases: svc.Releases}, Jobs: renderQueue{k.work.Queue},
		Objects: k.work.Objects, Sources: releasereq.ReleaseSources{Releases: svc.Releases}, Releaser: releasereq.ReleaseMaker{Releases: svc.Releases}, Approvals: k.approvals, Renderer: k.work.Renderer,
		Limits: func(ctx context.Context, tx pgx.Tx, organizationID string) (limits.Set, error) {
			return svc.Tenancy.Effective(ctx, tx, organizationID, "", "") //nolint:wrapcheck // a domain error
		},
	})
}

// attachRenderer gives the worker role its route to the render worker over the
// mutual TLS of server.internalTLS, where renderWorker.address is set (O-9,
// SEC-043). Without it the process runs no render jobs, and a requested
// visual diff stays queued.
func attachRenderer(cfg *config.Config, deps *WorkDeps) error {
	if cfg.RenderWorker.Address == "" || deps.Renderer != nil {
		return nil
	}
	t := cfg.Server.InternalTLS
	httpClient, err := httpx.NewInternalClient(httpx.InternalTLSOptions{CAFile: t.CAFile, CertFile: t.CertFile, KeyFile: t.KeyFile}, renderclient.Timeout+time.Minute)
	if err != nil {
		return fmt.Errorf("server: render worker client: %w", err)
	}
	client, err := renderclient.New(renderclient.Options{HTTP: httpClient, URL: cfg.RenderWorker.Address})
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}
	deps.Renderer = client
	return nil
}
