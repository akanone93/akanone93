// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/buildclient"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/jobs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// functionQueue enqueues function builds (FN-016).
type functionQueue struct{ q *JobQueue }

// Enqueue adds a build job on the function queue in the caller's
// transaction.
func (f functionQueue) Enqueue(ctx context.Context, tx pgx.Tx, job function.BuildJob) error {
	return f.q.insert(ctx, tx, job, jobs.QueueFunction)
}

// functionBuildWorker runs function.build jobs.
// FN-016: after the last attempt the build is failed, so that none stays
// running for ever.
type functionBuildWorker struct {
	river.WorkerDefaults[function.BuildJob]
	runner buildRunner
	log    *slog.Logger
}

// buildRunner is what the worker needs of the function service.
type buildRunner interface {
	RunBuild(ctx context.Context, job function.BuildJob) error
	FailBuild(ctx context.Context, job function.BuildJob, attempts int) error
}

// Work runs one build; on the job's last attempt a failure also fails the
// build with a diagnostic.
func (w *functionBuildWorker) Work(ctx context.Context, job *river.Job[function.BuildJob]) error {
	err := w.runner.RunBuild(ctx, job.Args)
	if err == nil || job.Attempt < job.MaxAttempts {
		return err //nolint:wrapcheck // a domain error
	}
	w.log.ErrorContext(ctx, "a function build gave up", slog.String("build", job.Args.BuildID),
		slog.Int("attempts", job.Attempt), slog.String("error", err.Error()))
	if failErr := w.runner.FailBuild(ctx, job.Args, job.Attempt); failErr != nil {
		return errors.Join(err, failErr)
	}
	return err //nolint:wrapcheck // a domain error
}

// buildFunctions assembles the function service, or returns nil when the
// installation has no object store to keep sources and modules in. The
// service has no Builder and no ArtifactSigner here: the worker role that
// runs builds is given them where it is assembled.
func buildFunctions(db *storage.DB, work WorkDeps, log *audit.Log, ten *tenancy.Service, gen interface{ New() (string, error) }) (*function.Service, error) {
	if work.Objects == nil {
		return nil, nil
	}
	var signer function.ArtifactSigner
	if work.Builder != nil && work.Signer != nil {
		// FN-009: only the worker holds a signing.Signer, so only it can
		// sign the statement of a build.
		s, err := buildclient.NewSigner(work.Signer, work.FunctionKey)
		if err != nil {
			return nil, fmt.Errorf("server: %w", err)
		}
		signer = s
	}
	svc, err := function.NewService(function.Options{
		DB: db, Audit: log, Objects: work.Objects, IDs: gen, Jobs: functionQueue{work.Queue},
		Builder: work.Builder, Signer: signer,
		Limits: func(ctx context.Context, tx pgx.Tx, organizationID, appID string) (limits.Set, error) {
			return ten.Effective(ctx, tx, organizationID, appID, "") //nolint:wrapcheck // a domain error
		},
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return svc, nil
}
