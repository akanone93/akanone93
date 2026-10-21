// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/breakglass"
	"github.com/nightCode42/plux3/backend/internal/freeze"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Emergency is the break-glass service as the release service uses it
// (GOV-024); *breakglass.Service implements it.
type Emergency interface {
	// Invoke records an emergency action in the transaction of the action.
	Invoke(ctx context.Context, tx pgx.Tx, p auth.Principal, in breakglass.Invocation) (breakglass.Record, error)
	// Alert tells the approvers once the action has committed.
	Alert(ctx context.Context, rec breakglass.Record)
}

var _ Emergency = (*breakglass.Service)(nil)

// ChangeOption changes how a kill switch, a forced rollback or a rollback is
// made.
type ChangeOption func(*changeOptions)

// changeOptions are the options of one change.
type changeOptions struct {
	breakGlass    bool
	justification string
}

// WithBreakGlass makes the change an emergency action: it needs
// breakglass.invoke on the environment, skips the approval and the change
// freeze, and opens a post-hoc review (GOV-024). An empty justification is
// refused with PLX-8005.
func WithBreakGlass(justification string) ChangeOption {
	return func(o *changeOptions) { o.breakGlass, o.justification = true, justification }
}

// emergency describes the break-glass invocation a change would make, or is
// nil for an ordinary change.
type emergency struct {
	action        breakglass.Action
	subject       string
	contentHash   string
	justification string
}

// emergencyOf returns the invocation the options ask for, or nil.
func emergencyOf(opts []ChangeOption, action breakglass.Action, subject, contentHash string) *emergency {
	var o changeOptions
	for _, apply := range opts {
		apply(&o)
	}
	if !o.breakGlass {
		return nil
	}
	return &emergency{action: action, subject: subject, contentHash: contentHash, justification: o.justification}
}

// gateChange is made in the transaction of a change to an environment: an
// emergency change is recorded, and an ordinary change to a production
// environment is held back by a change freeze (GOV-025). Automatic changes of
// the server itself, such as the revert of a failing rollout, are never held.
// It returns the invocation to alert once the transaction commits.
func (s *Service) gateChange(ctx context.Context, tx pgx.Tx, p auth.Principal, appID string, env dbgen.Environment, em *emergency) (*breakglass.Record, error) {
	if em != nil {
		return s.invoke(ctx, tx, p, appID, env, em)
	}
	if !env.Production || p.Kind == auth.KindSystem {
		return nil, nil
	}
	return nil, freeze.Check(ctx, s.o.Freezes, tx, env.Key) //nolint:wrapcheck // a refusal for the caller
}

// invoke records an emergency action.
func (s *Service) invoke(ctx context.Context, tx pgx.Tx, p auth.Principal, appID string, env dbgen.Environment, em *emergency) (*breakglass.Record, error) {
	if s.o.BreakGlass == nil {
		return nil, plxerr.New(plxerr.PreconditionFailed, "break-glass is not available on this server")
	}
	rec, err := s.o.BreakGlass.Invoke(ctx, tx, p, breakglass.Invocation{
		AppID: appID, EnvironmentID: storage.ID(env.ID), EnvironmentKey: env.Key, Production: env.Production,
		Action: em.action, Subject: em.subject, ContentHash: em.contentHash, Justification: em.justification,
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return &rec, nil
}

// alert tells the approvers of an emergency action that has committed.
func (s *Service) alert(ctx context.Context, rec *breakglass.Record) {
	if rec != nil && s.o.BreakGlass != nil {
		s.o.BreakGlass.Alert(ctx, *rec)
	}
}

// emergencyHash is the content hash of a kill-switch change.
func emergencyHash(parts ...string) string {
	raw := make([][]byte, len(parts))
	for i, part := range parts {
		raw[i] = []byte(part)
	}
	return approval.HashOf(raw...)
}
