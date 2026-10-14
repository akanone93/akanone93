// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
)

// PrepareTimeout bounds how long a deploy or a promotion waits for the
// runner to fetch, verify and compile a module (FN-023). The compile of a
// small module takes tens of milliseconds; the bound only keeps an
// unreachable runner from holding the request.
const PrepareTimeout = 10 * time.Second

// The results the prepare metric distinguishes.
const (
	PrepareOK     = "ok"
	PrepareFailed = "failed"
)

// Preparer asks the runner to compile a version's module ahead of its first
// invocation, so that the first call after a deploy finds it compiled
// (FN-010, FN-023). Implementations must be safe for concurrent use.
type Preparer interface {
	Prepare(ctx context.Context, organizationID string, v Version) error
}

// PrepareMetrics counts the requests to prepare a module by result
// (PrepareOK or PrepareFailed). The series carries no app, user or
// function.
type PrepareMetrics interface {
	Prepared(result string)
}

// ConnectPreparer prepares modules over the internal Connect service
// (ADR-0064). It issues the grant the runner presents to fetch the module,
// scoped to the version like the grant of an invocation. It is safe for
// concurrent use.
type ConnectPreparer struct {
	Client fnpbconnect.FunctionRunnerServiceClient
	Grants *Grants
}

var _ Preparer = ConnectPreparer{}

// Prepare calls FunctionRunnerService.Prepare for the version's module.
func (p ConnectPreparer) Prepare(ctx context.Context, organizationID string, v Version) error {
	digest := HexSHA256(v.ModuleSHA256)
	grant, _, err := p.Grants.Issue(GrantClaims{
		OrganizationID: organizationID, AppID: v.AppID, EnvironmentID: v.EnvironmentID, Function: v.Name,
		Version: v.Version, ModuleSHA256: digest,
	}, PrepareTimeout)
	if err != nil {
		return fmt.Errorf("function: %w", err)
	}
	if _, err := p.Client.Prepare(ctx, connect.NewRequest(&fnpb.PrepareRequest{ModuleSha256: digest, Grant: string(grant)})); err != nil {
		return fmt.Errorf("function: prepare a module: %w", err)
	}
	return nil
}

// prepare asks the runner to compile a server-placed version once the
// transaction that stored it committed. It is best effort: a failure is
// logged and counted and never fails the deploy, because the first
// invocation fetches and compiles the module itself. The call is bounded by
// PrepareTimeout and runs on the caller's goroutine, so none outlives the
// request; it is detached from the request's cancellation, so a client that
// hangs up after the commit does not lose the warm-up.
func (s *Service) prepare(ctx context.Context, organizationID string, v Version) {
	if s.o.Preparer == nil || v.Placement != PlacementServer {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), PrepareTimeout)
	defer cancel()
	err := s.o.Preparer.Prepare(ctx, organizationID, v)
	result := PrepareOK
	if err != nil {
		result = PrepareFailed
		if s.o.Log != nil {
			s.o.Log.WarnContext(ctx, "preparing a function module failed; the first invocation compiles it",
				slog.String("function", v.Name), slog.Int("version", int(v.Version)), slog.String("error", err.Error()))
		}
	}
	if s.o.PrepareMetrics != nil {
		s.o.PrepareMetrics.Prepared(result)
	}
}
