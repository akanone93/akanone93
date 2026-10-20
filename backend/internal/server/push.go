// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/jobs"
	"github.com/nightCode42/plux3/backend/internal/push"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// pushTimeout bounds one request to a push provider.
const pushTimeout = 15 * time.Second

// pushQueue enqueues silent-push jobs on the control queue, so that a push
// does not wait behind a publish (NFR-024).
type pushQueue struct{ q *JobQueue }

func (p pushQueue) Enqueue(ctx context.Context, tx pgx.Tx, job push.SendJob) error {
	return p.q.insert(ctx, tx, job, jobs.QueueControl)
}

// buildPush assembles the silent-push service (SYN-060, Q22). Its HTTP
// client refuses private destinations, and the service itself is off until
// the installation turns it on.
func buildPush(cfg *config.Config, db *storage.DB, set limits.Set, crypter signing.Crypter, ten *tenancy.Service, work WorkDeps) (*push.Service, error) {
	svc, err := push.New(push.Options{
		DB: db, Crypter: crypter, Secrets: ten, Jobs: pushQueue{work.Queue}, Enabled: cfg.Push.Enabled,
		HTTP: httpx.NewClient(httpx.Options{Timeout: pushTimeout}), Limits: set, Log: work.Log,
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return svc, nil
}

// controlNotifier adapts the push service to the release service's
// ControlNotifier.
type controlNotifier struct{ push *push.Service }

// NotifyControl queues the silent pushes of a newly signed control document.
func (n controlNotifier) NotifyControl(ctx context.Context, tx pgx.Tx, c release.ControlNotice) error {
	return n.push.NotifyControl(ctx, tx, push.Notice{ //nolint:wrapcheck // a domain error
		OrganizationID: c.OrganizationID, AppID: c.AppID, EnvironmentID: c.EnvironmentID, Version: c.Version,
	})
}

// pushWorker sends the silent push to a page of an environment's devices.
type pushWorker struct {
	river.WorkerDefaults[push.SendJob]
	svc *Services
}

// Work sends one page.
func (w *pushWorker) Work(ctx context.Context, job *river.Job[push.SendJob]) error {
	return w.svc.Push.Send(ctx, job.Args) //nolint:wrapcheck // a domain error
}
