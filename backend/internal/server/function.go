// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/jobs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
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
		return nil, nil //nolint:nilnil // the installation runs no functions
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

// serverSecrets reads the secrets a function declares from the
// environment's server-held values (ADR-0061, ADR-0064).
type serverSecrets struct{ r *gateway.ConfigResolver }

// Secret returns the text of a server-bound value of the environment.
func (s serverSecrets) Secret(ctx context.Context, organizationID, appID, environmentID, name string) (string, bool, error) {
	return s.r.ServerValue(ctx, organizationID, appID, environmentID, name) //nolint:wrapcheck // a domain error
}

// buildFunctionHost assembles the api role's side of the function runner's
// host service, or returns nil where the process is not the api role or
// has no function service. The grants' secret is created on first use and
// shared by every replica (ADR-0064).
func buildFunctionHost(ctx context.Context, cfg *config.Config, db *storage.DB, crypter signing.Crypter, svc *function.Service, gw *GatewayParts) (*function.Host, *function.Grants, error) {
	if svc == nil || !cfg.Has(config.RoleAPI) {
		return nil, nil, nil
	}
	grants, err := function.OpenGrants(ctx, db, crypter, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("server: %w", err)
	}
	o := function.HostOptions{Service: svc, Grants: grants}
	if gw != nil && gw.Resolver != nil {
		o.Secrets = serverSecrets{r: gw.Resolver}
	}
	host, err := function.NewHost(o)
	if err != nil {
		return nil, nil, fmt.Errorf("server: %w", err)
	}
	return host, grants, nil
}

// hostEnvelopeBytes is the room, beyond the largest function input, that a
// host call needs for its framing.
const hostEnvelopeBytes = 1 << 20

// registerFunctionHost mounts FunctionHostService on the internal
// listener, where only a role with a certificate of the internal CA can
// reach it (SEC-043). Nothing is mounted without a host.
func (s *Server) registerFunctionHost(h *api.Handlers) {
	if h.FunctionHosting == nil {
		return
	}
	opts := connect.WithHandlerOptions(
		connect.WithInterceptors(s.InternalInterceptors()...),
		connect.WithReadMaxBytes(int(min(s.limits.Get(limits.FunctionInputSize)+hostEnvelopeBytes, 1<<31-1))),
	)
	path, handler := fnpbconnect.NewFunctionHostServiceHandler(h.FunctionHost(), opts)
	s.RegisterInternal(path, handler)
}
