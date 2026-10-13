// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
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

// buildFunctions assembles the function service, or returns nil when the
// installation has no object store to keep sources and modules in. The
// service has no Builder and no ArtifactSigner here: the worker role that
// runs builds is given them where it is assembled.
func buildFunctions(db *storage.DB, work WorkDeps, log *audit.Log, ten *tenancy.Service, gen interface{ New() (string, error) }) (*function.Service, error) {
	if work.Objects == nil {
		return nil, nil
	}
	svc, err := function.NewService(function.Options{
		DB: db, Audit: log, Objects: work.Objects, IDs: gen, Jobs: functionQueue{work.Queue},
		Limits: func(ctx context.Context, tx pgx.Tx, organizationID, appID string) (limits.Set, error) {
			return ten.Effective(ctx, tx, organizationID, appID, "") //nolint:wrapcheck // a domain error
		},
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return svc, nil
}
