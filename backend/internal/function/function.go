// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// IDs generates identifiers for the rows this package writes.
type IDs interface {
	New() (string, error)
}

// Enqueuer adds a build job in the caller's transaction, so that the build
// and its job commit together (SRV-024).
type Enqueuer interface {
	Enqueue(ctx context.Context, tx pgx.Tx, job BuildJob) error
}

// Options configures a Service.
type Options struct {
	DB      *storage.DB
	Audit   *audit.Log
	Objects objects.Store
	IDs     IDs
	// Jobs enqueues builds; the api role sets it.
	Jobs Enqueuer
	// Builder compiles a source on the builder role. Nil refuses to run a
	// build, as on the api role.
	Builder Builder
	// Signer signs artifact statements; only the worker role is given one
	// (SRV-052). Nil refuses to run a build.
	Signer ArtifactSigner
	// Limits returns the limits in force for an app (LIM-001).
	Limits func(ctx context.Context, tx pgx.Tx, organizationID, appID string) (limits.Set, error)
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service stores functions and administers them. It is safe for
// concurrent use: it holds no state beyond its options, and the database
// serialises the deploys and the key-value writes of one function.
type Service struct{ o Options }

// NewService returns the function service.
func NewService(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.Objects == nil || o.IDs == nil || o.Limits == nil {
		return nil, errors.New("function: the service needs a database, an audit log, an object store, identifiers and limits")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{o: o}, nil
}

// Placement says where a function runs (FN-008).
type Placement string

// The placements.
const (
	PlacementServer Placement = "server"
	PlacementDevice Placement = "device"
)

// Valid reports whether p is a placement.
func (p Placement) Valid() bool { return p == PlacementServer || p == PlacementDevice }

// BuildState is the progress of a build (FN-016).
type BuildState string

// The states of a build.
const (
	BuildQueued    BuildState = "queued"
	BuildRunning   BuildState = "running"
	BuildSucceeded BuildState = "succeeded"
	BuildFailed    BuildState = "failed"
)

// nameRE matches a dotted function name, as the FunctionGrant of a plugin
// document does.
var nameRE = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*(\.[a-z][a-zA-Z0-9]*)+$`)

// aliasRE matches an alias, as the FunctionGrant of a plugin document does.
var aliasRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Bounds of names, fixed by the plugin document's schema (FN-006).
const (
	maxNameLength  = 128
	maxAliasLength = 32
)

// Source is an uploaded source archive (FN-016).
type Source struct {
	ID            string
	AppID         string
	EnvironmentID string
	Name          string
	ObjectKey     string
	SHA256        []byte
	Size          int64
	CreatedAt     time.Time
}

// TestResult is the outcome of one stored test case (FN-031).
type TestResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

// Dependency is one vendored crate of a build (FN-005).
type Dependency struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Checksum string `json:"checksum"`
}

// Provenance records how a module was built (FN-005). BuildTime is
// recorded, never embedded in the module (FN-019). SDKSHA256 is the hash of
// the pinned SDK crate the build linked, which no lock file records
// (ADR-0063).
type Provenance struct {
	SourceSHA256    string       `json:"sourceSha256"`
	SDKSHA256       string       `json:"sdkSha256"`
	Dependencies    []Dependency `json:"dependencies"`
	Toolchain       string       `json:"toolchain"`
	WasmOptVersion  string       `json:"wasmOptVersion"`
	BuildTime       time.Time    `json:"buildTime"`
	BuilderIdentity string       `json:"builderIdentity"`
}

// Build is one build of a source (FN-016).
type Build struct {
	ID            string
	SourceID      string
	AppID         string
	EnvironmentID string
	Name          string
	// Placement is the one asked for until the build succeeds, then the
	// module's own.
	Placement   Placement
	State       BuildState
	Diagnostics plxerr.Diagnostics
	TestResults []TestResult
	// ModuleSHA256 is set once the build succeeded.
	ModuleSHA256 []byte
	ABIVersion   int32
	Provenance   *Provenance
	// Signed is true when the artifact statement carries a signature.
	Signed     bool
	KeyID      string
	CreatedAt  time.Time
	FinishedAt time.Time
}

// Version is one immutable version of a function (FN-006).
type Version struct {
	AppID         string
	EnvironmentID string
	Name          string
	Version       int32
	Placement     Placement
	ModuleKey     string
	ModuleSHA256  []byte
	ABIVersion    int32
	InputSchema   string
	OutputSchema  string
	Provenance    Provenance
	Statement     []byte
	Signature     []byte
	KeyID         string
	TestsPassed   bool
	BuildID       string
	CreatedAt     time.Time
}

// Alias points a name at a version (FN-006).
type Alias struct {
	Name    string
	Version int32
}

// Function is a function with its aliases and latest version.
type Function struct {
	Latest  Version
	Aliases []Alias
}

// Resolved is what a release records of a function it references
// (FN-006): the version an alias or the latest version stands for, the
// SHA-256 of its module and its placement.
type Resolved struct {
	Name         string
	Version      int32
	ModuleSHA256 []byte
	Placement    Placement
	// TestsPassed is false for a version whose stored test cases failed
	// (FN-031): a release refuses it.
	TestsPassed bool
}

// inOrg runs f in a transaction bound to the principal's organisation.
func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.inTenant(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f)
}

// inTenant runs f in a transaction bound to a tenant.
func (s *Service) inTenant(ctx context.Context, t storage.Tenant, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, t, f) //nolint:wrapcheck // InTx wraps its own failures
}

// newID generates an identifier.
func (s *Service) newID() (string, error) {
	id, err := s.o.IDs.New()
	if err != nil {
		return "", fmt.Errorf("function: %w", err)
	}
	return id, nil
}

// authorize is Principal.AuthorizeApp with the refusal marked as this
// package's.
func authorize(p auth.Principal, want auth.Permission, appID string) error {
	if err := p.AuthorizeApp(want, appID); err != nil {
		return fmt.Errorf("function: %w", err)
	}
	return nil
}

// record appends an audit entry in the transaction of the change.
func (s *Service) record(ctx context.Context, tx pgx.Tx, p auth.Principal, action audit.Action, kind, target, detail string) error {
	_, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: action,
		TargetKind: kind, TargetID: target, Detail: detail,
	})
	if err != nil {
		return fmt.Errorf("function: %w", err)
	}
	return nil
}

// failure turns a database error into the refusal a caller understands.
func failure(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	}
	return fmt.Errorf("function: %s: %w", what, err)
}

// parseID validates an identifier from a caller.
func parseID(s, what string) (pgtype.UUID, error) {
	u, err := storage.UUID(s)
	if err != nil {
		return pgtype.UUID{}, plxerr.New(plxerr.InvalidFormat, "the %s identifier is not valid", what)
	}
	return u, nil
}

// environmentOf reads an environment of an app.
func environmentOf(ctx context.Context, q *dbgen.Queries, appID, environmentID string) (dbgen.Environment, error) {
	app, err := storage.UUID(appID)
	if err != nil {
		return dbgen.Environment{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	id, err := storage.UUID(environmentID)
	if err != nil {
		return dbgen.Environment{}, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
	}
	env, err := q.GetEnvironment(ctx, id)
	if err != nil {
		return dbgen.Environment{}, failure(err, "environment")
	}
	if env.AppID != app {
		return dbgen.Environment{}, plxerr.New(plxerr.ResourceNotFound, "no such environment in this app")
	}
	return env, nil
}

// createdBy returns the principal's user for a created_by column, or
// NULL for a principal that is not a person.
func createdBy(p auth.Principal) pgtype.UUID {
	if p.UserID == "" {
		return pgtype.UUID{}
	}
	return storage.MustUUID(p.UserID)
}
