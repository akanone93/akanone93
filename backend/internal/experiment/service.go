// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// IDs generates identifiers; the salts of experiments and holdouts are
// drawn from it too, so that no randomness enters except through it.
type IDs interface{ New() (string, error) }

// LimitsFunc returns the limits in force for an app, read in the caller's
// transaction.
type LimitsFunc func(ctx context.Context, tx pgx.Tx, organizationID, appID string) (limits.Set, error)

// ControlWriter writes the flags, experiments and holdout of an
// environment into its channels' control documents, in the caller's
// transaction. *release.Service implements it.
type ControlWriter interface {
	ApplyExperimentation(ctx context.Context, tx pgx.Tx, p auth.Principal, environmentID string, ex release.Experimentation, detail string) error
}

// Documents is the part of the document service that promotion and the
// resolution of segments use. *document.Service implements it.
type Documents interface {
	GetDocument(ctx context.Context, p auth.Principal, appID, pluginID, path string) (document.Document, error)
	Export(ctx context.Context, p auth.Principal, appID, pluginID string) ([]document.File, error)
	Import(ctx context.Context, p auth.Principal, appID, pluginID, session string, files []document.File) (document.Written, error)
	ReleaseLock(ctx context.Context, p auth.Principal, appID, pluginID, session string) error
}

// Publisher starts a publish; *release.Service implements it.
type Publisher interface {
	Publish(ctx context.Context, p auth.Principal, r release.PublishRequest) (release.PublishJob, error)
}

// Options configures a Service.
type Options struct {
	DB    *storage.DB
	Audit *audit.Log
	IDs   IDs
	// Limits reads an app's limits (flags.perApp, experiments.activePerApp,
	// document.jsonDepth, document.stringPropSize, document.fileSize).
	Limits LimitsFunc
	// Control writes the control documents.
	Control ControlWriter
	// Segments resolves segment keys to rules; nil resolves them from the
	// app's draft through Documents.
	Segments SegmentSource
	// Documents and Publisher serve promotion; nil leaves it unavailable.
	Documents Documents
	Publisher Publisher
	// Observations supplies the exposure and metric statistics; nil leaves
	// analysis unavailable.
	Observations Observations
	// Approval decides whether an experiment may start; nil allows every
	// start (ABT-007 is wired by the approval engine).
	Approval ApprovalGate
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service administers feature flags and experiments.
type Service struct {
	o        Options
	now      func() time.Time
	loader   *schema.Loader
	segments SegmentSource
	approval ApprovalGate
}

// New returns the service.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil || o.Limits == nil || o.Control == nil {
		return nil, errors.New("experiment: a database, an audit log, identifiers, limits and a control writer are required")
	}
	validator, err := schema.NewValidator()
	if err != nil {
		return nil, fmt.Errorf("experiment: %w", err)
	}
	s := &Service{o: o, now: o.Now, approval: o.Approval, segments: o.Segments}
	s.loader = schema.NewLoader(validator, schema.DefaultMigrator(), limits.Defaults())
	if s.now == nil {
		s.now = time.Now
	}
	if s.approval == nil {
		s.approval = AllowAll{}
	}
	if s.segments == nil {
		s.segments = draftSegments{docs: o.Documents, loader: s.loader}
	}
	return s, nil
}

// inOrg runs f in a transaction bound to the principal's organisation.
func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// record appends an audit entry in the caller's transaction.
func (s *Service) record(ctx context.Context, tx pgx.Tx, p auth.Principal, e audit.Entry) error {
	e.OrganizationID, e.Actor = p.OrganizationID, p.Actor()
	if _, err := s.o.Audit.Append(ctx, tx, e); err != nil {
		return fmt.Errorf("experiment: %w", err)
	}
	return nil
}

// limitsOf reads the limits of an app in the caller's transaction.
func (s *Service) limitsOf(ctx context.Context, tx pgx.Tx, p auth.Principal, appID string) (limits.Set, error) {
	lim, err := s.o.Limits(ctx, tx, p.OrganizationID, appID)
	if err != nil {
		return limits.Set{}, fmt.Errorf("experiment: %w", err)
	}
	return lim, nil
}

// failure turns a database error into the refusal a caller understands.
func failure(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	}
	return fmt.Errorf("experiment: %s: %w", what, err)
}

// parseID validates an identifier from a caller.
func parseID(s, what string) (pgtype.UUID, error) {
	id, err := storage.UUID(s)
	if err != nil {
		return pgtype.UUID{}, plxerr.New(plxerr.InvalidFormat, "the %s identifier is not valid", what)
	}
	return id, nil
}

// environment resolves an app's environment.
func environment(ctx context.Context, q *dbgen.Queries, appID, envID string) (dbgen.Environment, error) {
	app, err := parseID(appID, "app")
	if err != nil {
		return dbgen.Environment{}, err
	}
	eid, err := parseID(envID, "environment")
	if err != nil {
		return dbgen.Environment{}, err
	}
	env, err := q.GetEnvironment(ctx, eid)
	if err != nil {
		return dbgen.Environment{}, failure(err, "environment")
	}
	if env.AppID != app {
		return dbgen.Environment{}, plxerr.New(plxerr.ResourceNotFound, "no such environment in this app")
	}
	return env, nil
}

// authorizeFlags checks flags.manage on an environment; the first check is
// made before the environment is read, the second against its key (GOV-003).
func authorizeFlags(p auth.Principal, appID string, env *dbgen.Environment) error {
	if env == nil {
		return wrapAuth(p.AuthorizeSomeEnvironment(auth.FlagsManage, appID))
	}
	return wrapAuth(p.AuthorizeEnvironment(auth.FlagsManage, appID, env.Key))
}

// authorizeExperiments checks experiments.manage on an app.
func authorizeExperiments(p auth.Principal, appID string) error {
	return wrapAuth(p.AuthorizeApp(auth.ExperimentsManage, appID))
}

// authorizeRead checks that the principal may read the app.
func authorizeRead(p auth.Principal, appID string) error {
	return wrapAuth(p.AuthorizeApp(auth.AppRead, appID))
}

func wrapAuth(err error) error {
	if err != nil {
		return fmt.Errorf("experiment: %w", err)
	}
	return nil
}

// actorOf converts a stored actor.
func actorOf(kind, id, display string) audit.Actor {
	return audit.Actor{Kind: kind, ID: id, Display: display}
}
