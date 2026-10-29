// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package releasereq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// DefaultChannel is the channel of a request that names none.
const DefaultChannel = "production"

var channelForm = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

// IDs generates identifiers for the rows this package writes.
type IDs interface{ New() (string, error) }

// Builds reads the publish jobs a request froze (SRV-050). The service never
// sees a job it was not given, and reads nothing but its state, version and
// diagnostics.
type Builds interface {
	Build(ctx context.Context, p auth.Principal, appID, jobID string) (Build, error)
}

// Releaser makes the app release of a finished request and promotes it
// (REL-002, REL-004). Promotion goes through the approval engine and the
// step-up of SEC-103 as it always does.
type Releaser interface {
	// Create makes an app release of the built versions, one per plugin key,
	// in an environment, and returns its sequence.
	Create(ctx context.Context, p auth.Principal, appID, environmentID string, versions map[string]int64, notes string) (int64, error)
	// Promote points a channel at the release.
	Promote(ctx context.Context, p auth.Principal, appID string, sequence int64, environmentID, channel string) error
}

// RenderQueue enqueues render jobs in the caller's transaction, so that a job
// is either recorded and queued or neither.
type RenderQueue interface {
	Enqueue(ctx context.Context, tx pgx.Tx, job RenderJob) error
}

// Sources reads the two projects of a visual diff (O-9): what a release of the
// built versions would hold, and what the channel serves now.
type Sources interface {
	// Projects returns them as file sets in the Git layout. before is nil
	// when the channel serves nothing yet. built maps plugin keys to the
	// versions the request built.
	Projects(ctx context.Context, tx pgx.Tx, appID, environmentID, channel string, built map[string]int64) (before, after fs.FS, err error)
}

// Functions supplies the catalogue of the functions deployed in an
// environment, the one a release build compiles against (FN-006). *function.Service
// is one.
type Functions interface {
	Catalogue(ctx context.Context, organizationID, appID, environmentID string) (compiler.FunctionCatalogue, error)
}

// Options are what the service needs. Approvals, Releaser and Jobs may be nil
// where an installation lacks them: no policy is asked, a request cannot be
// completed, and no diff can be queued.
type Options struct {
	DB      *storage.DB
	IDs     IDs
	Builds  Builds
	Jobs    RenderQueue
	Objects objects.Store
	// Releaser completes a request.
	Releaser Releaser
	// Approvals decides whether the release needs approval (GOV-020); nil
	// requires nothing.
	Approvals approval.Gate
	// Sources reads the projects a visual diff compares; nil leaves them out
	// of the renderer's input, for tests with a fake renderer.
	Sources Sources
	// Functions supplies the function catalogue of a visual diff's
	// environment; nil renders with no functions deployed.
	Functions Functions
	// Renderer renders the visual diffs. It is held only by the worker role
	// that runs render jobs, in the render worker's isolation (L-4).
	Renderer Renderer
	// Limits are the installation's limits (LIM-001) for an organisation;
	// nil means the defaults.
	Limits func(ctx context.Context, tx pgx.Tx, organizationID string) (limits.Set, error)
}

// Service stores release requests and runs their visual-diff jobs.
type Service struct{ o Options }

// New returns a service.
func New(o Options) *Service { return &Service{o: o} }

// Selected is a plugin in a request and why it is there.
type Selected struct {
	PluginKey string `json:"pluginKey"`
	// Reason is "changed" for a plugin with unpublished changes and
	// "dependency" for one added because another needs it (REL-003).
	Reason string `json:"reason"`
}

// Build is one plugin's publish job as the request last saw it.
type Build struct {
	PluginKey    string   `json:"pluginKey"`
	JobID        string   `json:"jobId"`
	State        string   `json:"state"`
	Version      int64    `json:"version"`
	BundleSHA256 string   `json:"bundleSha256"`
	Errors       int      `json:"errors"`
	Warnings     []string `json:"warnings"`
}

// Request is a release request.
type Request struct {
	ID, AppID, EnvironmentID, Channel string
	State                             State
	Hold                              Hold
	HoldReason                        string
	Revision                          int64
	Plugins                           []Selected
	Builds                            []Build
	Acknowledged                      []string
	// ContentHash is the SHA-256 of what the builds produced, in hex.
	ContentHash                   string
	ChangelogNotes, Label         string
	RolloutPlan                   []byte
	ApprovalRequestID             string
	ApprovedHash                  string
	ReleaseSequence               int64
	CreatedBy, CreatedByName      string
	CreatedAt, UpdatedAt, EndedAt time.Time
}

func authorize(p auth.Principal, want auth.Permission, appID string) error {
	if err := p.AuthorizeApp(want, appID); err != nil {
		return fmt.Errorf("releasereq: %w", err)
	}
	return nil
}

func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

func failure(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	}
	return fmt.Errorf("releasereq: %s: %w", what, err)
}

// uniqueViolation reports whether err is PostgreSQL's unique_violation.
func uniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func ids(appID, id string) (app, req pgtype.UUID, err error) {
	app, err = storage.UUID(appID)
	if err != nil {
		return app, req, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	req, err = storage.UUID(id)
	if err != nil {
		return app, req, plxerr.New(plxerr.InvalidFormat, "the release request identifier is not valid")
	}
	return app, req, nil
}

// requestOf converts a row.
func requestOf(r dbgen.ReleaseRequest) (Request, error) {
	out := Request{
		ID: storage.ID(r.ID), AppID: storage.ID(r.AppID), EnvironmentID: storage.ID(r.EnvironmentID), Channel: r.Channel,
		State: State(r.State), Hold: Hold(r.Hold), HoldReason: r.HoldReason, Revision: r.Revision,
		Acknowledged: r.Acknowledged, ContentHash: r.ContentHash, ChangelogNotes: r.ChangelogNotes, Label: r.Label,
		RolloutPlan: r.RolloutPlan, ApprovalRequestID: r.ApprovalRequestID, ApprovedHash: r.ApprovedHash,
		CreatedBy: r.CreatedBy, CreatedByName: r.CreatedByName, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time, EndedAt: r.EndedAt.Time,
	}
	if r.ReleaseSequence != nil {
		out.ReleaseSequence = *r.ReleaseSequence
	}
	if err := json.Unmarshal(r.Plugins, &out.Plugins); err != nil {
		return Request{}, fmt.Errorf("releasereq: stored plugins: %w", err)
	}
	if err := json.Unmarshal(r.Builds, &out.Builds); err != nil {
		return Request{}, fmt.Errorf("releasereq: stored builds: %w", err)
	}
	return out, nil
}

// updateParams converts a request to the statement that stores it.
func updateParams(r Request) (dbgen.UpdateReleaseRequestParams, error) {
	id, err := storage.UUID(r.ID)
	if err != nil {
		return dbgen.UpdateReleaseRequestParams{}, fmt.Errorf("releasereq: %w", err)
	}
	app, err := storage.UUID(r.AppID)
	if err != nil {
		return dbgen.UpdateReleaseRequestParams{}, fmt.Errorf("releasereq: %w", err)
	}
	env, err := storage.UUID(r.EnvironmentID)
	if err != nil {
		return dbgen.UpdateReleaseRequestParams{}, fmt.Errorf("releasereq: %w", err)
	}
	plugins, err := json.Marshal(nonNil(r.Plugins))
	if err != nil {
		return dbgen.UpdateReleaseRequestParams{}, fmt.Errorf("releasereq: %w", err)
	}
	builds, err := json.Marshal(nonNil(r.Builds))
	if err != nil {
		return dbgen.UpdateReleaseRequestParams{}, fmt.Errorf("releasereq: %w", err)
	}
	out := dbgen.UpdateReleaseRequestParams{
		ID: id, AppID: app, EnvironmentID: env, Channel: r.Channel, State: string(r.State), Hold: string(r.Hold), HoldReason: r.HoldReason,
		Plugins: plugins, Builds: builds, Acknowledged: nonNil(r.Acknowledged), ContentHash: r.ContentHash,
		ChangelogNotes: r.ChangelogNotes, Label: r.Label, RolloutPlan: r.RolloutPlan,
		ApprovalRequestID: r.ApprovalRequestID, ApprovedHash: r.ApprovedHash,
	}
	if r.ReleaseSequence != 0 {
		seq := r.ReleaseSequence
		out.ReleaseSequence = &seq
	}
	if !r.EndedAt.IsZero() {
		out.EndedAt = storage.Timestamp(r.EndedAt)
	}
	return out, nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// update runs f on a request under the row's lock and stores what f leaves.
// A request that has ended takes no change, unless f says it may (a
// cancellation of a cancelled request is a replay).
func (s *Service) update(ctx context.Context, p auth.Principal, appID, id string, f func(context.Context, pgx.Tx, *Request) error) (Request, error) {
	app, req, err := ids(appID, id)
	if err != nil {
		return Request{}, err
	}
	var out Request
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetReleaseRequestForUpdate(ctx, dbgen.GetReleaseRequestForUpdateParams{ID: req, AppID: app})
		if err != nil {
			return failure(err, "release request")
		}
		r, err := requestOf(row)
		if err != nil {
			return err
		}
		before := r
		if err := f(ctx, tx, &r); err != nil {
			return err
		}
		out = r
		if sameStored(before, r) {
			out = before
			return nil
		}
		params, err := updateParams(r)
		if err != nil {
			return err
		}
		stored, err := q.UpdateReleaseRequest(ctx, params)
		switch {
		case uniqueViolation(err):
			return plxerr.New(plxerr.ResourceExists, "the app, environment and channel already have an open release request")
		case err != nil:
			return failure(err, "release request")
		}
		out, err = requestOf(stored)
		return err
	})
	return out, err
}

// sameStored reports whether two states of a request store identically, so a
// replay writes nothing and does not raise the revision.
func sameStored(a, b Request) bool {
	pa, errA := updateParams(a)
	pb, errB := updateParams(b)
	if errA != nil || errB != nil {
		return false
	}
	return pa.EnvironmentID == pb.EnvironmentID && pa.Channel == pb.Channel && pa.State == pb.State && pa.Hold == pb.Hold &&
		pa.HoldReason == pb.HoldReason && string(pa.Plugins) == string(pb.Plugins) && string(pa.Builds) == string(pb.Builds) &&
		slices.Equal(pa.Acknowledged, pb.Acknowledged) && pa.ContentHash == pb.ContentHash && pa.ChangelogNotes == pb.ChangelogNotes &&
		pa.Label == pb.Label && string(pa.RolloutPlan) == string(pb.RolloutPlan) && pa.ApprovalRequestID == pb.ApprovalRequestID &&
		pa.ApprovedHash == pb.ApprovedHash && ptrEqual(pa.ReleaseSequence, pb.ReleaseSequence) && pa.EndedAt == pb.EndedAt
}

func ptrEqual(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func ended(r Request) error {
	return plxerr.New(plxerr.ReleaseRequestEnded, "the release request is %s", r.State)
}
