// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package approval

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/freeze"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// IDs generates the identifiers of the rows the engine writes.
type IDs interface{ New() (string, error) }

// ProfileFunc answers which security profile governs an environment (an
// app when environmentID is empty); the default policies follow it.
type ProfileFunc func(ctx context.Context, appID, environmentID string) (settings.Profile, error)

// Options configures a Service.
type Options struct {
	DB    *storage.DB
	Audit *audit.Log
	IDs   IDs
	// Signer signs decision records with SignerKey. Only the worker role
	// holds one (L-3, ADR-0006); nil leaves records for the worker to sign.
	Signer    signing.Signer
	SignerKey string
	// Log receives the failures that must not fail a change, such as a
	// notification that could not be queued; nil uses the default logger.
	Log *slog.Logger
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service is the approval engine.
type Service struct {
	o         Options
	now       func() time.Time
	validator *schema.Validator
	reviewers map[schema.PolicyResource]Reviewer
	profile   ProfileFunc
	notifier  notify.Publisher
	freezes   freeze.Guard
}

// New returns the engine.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil {
		return nil, errors.New("approval: a database, an audit log and an identifier source are required")
	}
	if o.Signer != nil && o.SignerKey == "" {
		return nil, errors.New("approval: a signer needs the reference of the key it signs with")
	}
	validator, err := schema.NewValidator()
	if err != nil {
		return nil, fmt.Errorf("approval: %w", err)
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	return &Service{o: o, now: now, validator: validator, reviewers: map[schema.PolicyResource]Reviewer{}}, nil
}

// Use binds the services the engine reads and tells, which are built after
// the services that need the engine. profile resolves an environment's
// security profile; without it every environment is standard. notifier
// receives "approval requested" for each request opened; nil publishes
// nothing, and a notification that fails never fails a change. Call it before
// the engine is used.
func (s *Service) Use(profile ProfileFunc, notifier notify.Publisher) {
	s.profile, s.notifier = profile, notifier
}

// UseFreezes binds the guard that holds production changes back inside a
// change-freeze window (GOV-025); without one nothing is held. Call it before
// the engine is used.
func (s *Service) UseFreezes(g freeze.Guard) { s.freezes = g }

// Register makes r the reviewer of a resource type (GOV-023). The services
// that supply evidence need the engine to be built, and the engine needs
// them, so they register after both exist and before the engine is used.
func (s *Service) Register(resource schema.PolicyResource, r Reviewer) {
	s.reviewers[resource] = r
}

// inOrg runs f in a transaction of the principal's organisation.
func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// inTenant runs f in a transaction of an organisation, for work no
// principal started.
func (s *Service) inTenant(ctx context.Context, organizationID string, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: organizationID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// besideLockTimeout bounds how long a transaction opened beside a caller's
// waits for a lock. The caller waits for it, so a lock the caller holds would
// block both for ever; the bound turns a caller that broke the rule of
// Require into an error.
const besideLockTimeout = "5s"

// beside runs f in a transaction of an organisation that is separate from
// the caller's, which is waiting for it (see Require).
func (s *Service) beside(ctx context.Context, organizationID string, f func(context.Context, pgx.Tx) error) error {
	return s.inTenant(ctx, organizationID, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '"+besideLockTimeout+"'"); err != nil {
			return fmt.Errorf("approval: bound the lock wait: %w", err)
		}
		return f(ctx, tx)
	})
}

// profileOf resolves the security profile that decides the default policies
// of an artifact: the one it names, else its environment's, else, for a change
// that belongs to an app but no environment, the strictest of the app's
// environments.
func (s *Service) profileOf(ctx context.Context, q *dbgen.Queries, a Artifact) (settings.Profile, error) {
	switch {
	case a.Profile != "":
		return settings.Profile(a.Profile), nil
	case s.profile == nil || a.AppID == "":
		return settings.Standard, nil
	case a.Environment.ID != "":
		return s.profileOfEnvironment(ctx, a.AppID, a.Environment.ID)
	}
	app, err := scopeUUID(a.AppID)
	if err != nil {
		return "", err
	}
	envs, err := q.ListApprovalAppEnvironments(ctx, app)
	if err != nil {
		return "", fmt.Errorf("approval: list the environments: %w", err)
	}
	strictest := settings.Standard
	for _, env := range envs {
		profile, err := s.profileOfEnvironment(ctx, a.AppID, storage.ID(env))
		if err != nil {
			return "", err
		}
		if profileRank(profile) > profileRank(strictest) {
			strictest = profile
		}
	}
	return strictest, nil
}

// profileOfEnvironment asks the bound resolver.
func (s *Service) profileOfEnvironment(ctx context.Context, appID, environmentID string) (settings.Profile, error) {
	profile, err := s.profile(ctx, appID, environmentID)
	if err != nil {
		return "", fmt.Errorf("approval: resolve the security profile: %w", err)
	}
	return profile, nil
}

// profileRank orders the profiles by strictness.
func profileRank(p settings.Profile) int {
	switch p {
	case settings.Maximum:
		return 2
	case settings.Strict:
		return 1
	}
	return 0
}

// failure turns a missing row into a not-found refusal.
func failure(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	}
	return fmt.Errorf("approval: %s: %w", what, err)
}
