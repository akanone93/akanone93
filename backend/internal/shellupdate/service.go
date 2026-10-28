// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package shellupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/codegen"
	"github.com/nightCode42/plux3/backend/internal/generator"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// The states of a project job.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// The project formats: the whole project as a zip, or a patch holding what
// changed since the last generation (a Git push is not offered; see the S8.1
// hand-off note).
const (
	FormatZip   = "zip"
	FormatPatch = "patch"
)

// IDs generates identifiers for the rows this package writes.
type IDs interface{ New() (string, error) }

// Job is the queue entry of a generation job.
type Job struct {
	OrganizationID string `json:"organizationId"`
	AppID          string `json:"appId"`
	JobID          string `json:"jobId"`
}

// Kind names the job type for the queue.
func (Job) Kind() string { return "project.generate" }

// Enqueuer adds a generation job in the caller's transaction.
type Enqueuer interface {
	Enqueue(ctx context.Context, tx pgx.Tx, job Job) error
}

// KeyReader returns an environment's root public keys (SEC-051).
type KeyReader func(ctx context.Context, organizationID, environmentID string) ([]codegen.RootKey, error)

// Options are what the service needs.
type Options struct {
	DB    *storage.DB
	Audit *audit.Log
	IDs   IDs
	// Queue enqueues jobs; nil makes GenerateProject refuse.
	Queue Enqueuer
	Keys  KeyReader
	// Objects holds the asset files, so an icon can be checked and read; nil
	// refuses an icon.
	Objects objects.Store
	// Endpoint is the server's public base URL, compiled into the project.
	Endpoint string
	// Limits are the installation's limits (LIM-001); zero means the defaults.
	Limits limits.Set
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service stores shell settings and runs generation jobs.
type Service struct {
	o   Options
	now func() time.Time
}

// New returns a service.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil {
		return nil, errors.New("shellupdate: a database, audit log and identifiers are required")
	}
	if o.Limits.IsZero() {
		o.Limits = limits.Defaults()
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{o: o, now: now}, nil
}

// Status compares the settings with those the project was generated from.
type Status struct {
	Current     generator.ShellSpec
	Generated   *generator.ShellSpec
	GeneratedAt time.Time
	Changes     []Change
}

// ProjectJob is a generation job.
type ProjectJob struct {
	ID, AppID, Environment, Channel, Format, Status string
	ErrorCode, ErrorMessage, SHA256                 string
	Size                                            int64
	CreatedAt, FinishedAt                           time.Time
}

func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

func authorize(p auth.Principal, want auth.Permission, appID string) error {
	if err := p.AuthorizeApp(want, appID); err != nil {
		return fmt.Errorf("shellupdate: %w", err)
	}
	return nil
}

func failure(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	}
	return fmt.Errorf("shellupdate: %s: %w", what, err)
}

// defaults are the settings of an app that has stored none: derived from its
// key and name, to be changed in the app's shell settings.
func defaults(app dbgen.App) generator.ShellSpec {
	pkg := generator.PackageName(app.Key)
	return generator.ShellSpec{
		Name: app.Name, Package: pkg, ApplicationID: "com.example." + pkg, BundleID: "com.example." + pkg,
		AppVersion: "1.0.0+1", SplashColor: "#FFFFFF", Plux: generator.PluxSource{EntryRoute: "home"},
	}
}

// Check validates settings the way the generator would, so that a bad value is
// refused when it is stored rather than when a job runs.
func Check(spec generator.ShellSpec) (generator.ShellSpec, error) {
	probe := spec
	probe.Plux.AppID, probe.Plux.Endpoint, probe.Plux.Environment = "-", "-", "-"
	if _, err := generator.Derive(probe); err != nil {
		return spec, plxerr.Wrap(plxerr.InvalidFormat, err, "the shell settings are not valid: %v", err)
	}
	return Normalise(spec), nil
}

// settings returns the stored shell settings, or the defaults.
func settings(ctx context.Context, q *dbgen.Queries, app dbgen.App) (cur generator.ShellSpec, gen *generator.ShellSpec, at time.Time, err error) {
	row, err := q.GetShellSettings(ctx, app.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaults(app), nil, time.Time{}, nil
	}
	if err != nil {
		return cur, nil, at, failure(err, "shell settings")
	}
	if err := json.Unmarshal(row.Settings, &cur); err != nil {
		return cur, nil, at, fmt.Errorf("shellupdate: read the settings: %w", err)
	}
	if len(row.Generated) > 0 {
		var g generator.ShellSpec
		if err := json.Unmarshal(row.Generated, &g); err != nil {
			return cur, nil, at, fmt.Errorf("shellupdate: read the generated settings: %w", err)
		}
		gen, at = &g, storage.Time(row.GeneratedAt)
	}
	return cur, gen, at, nil
}

// Status returns the current settings and whether a shell update is
// required (GEN-004). It needs app.read.
func (s *Service) Status(ctx context.Context, p auth.Principal, appID string) (Status, error) {
	if err := authorize(p, auth.AppRead, appID); err != nil {
		return Status{}, err
	}
	var out Status
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		app, err := q.GetApp(ctx, storage.MustUUID(appID))
		if err != nil {
			return failure(err, "app")
		}
		cur, gen, at, err := settings(ctx, q, app)
		if err != nil {
			return err
		}
		out = Status{Current: Normalise(cur), Generated: gen, GeneratedAt: at}
		if cur.IconAssetID != "" {
			if row, err := currentIcon(ctx, q, app, cur.IconAssetID); err == nil {
				out.Current.IconSHA256 = hex.EncodeToString(row.Sha256)
			}
		}
		if gen != nil {
			out.Changes = Compare(*gen, out.Current)
		}
		return nil
	})
	return out, err
}

// SetSettings stores the shell settings. It needs app.manage and is audited.
func (s *Service) SetSettings(ctx context.Context, p auth.Principal, appID string, spec generator.ShellSpec) (generator.ShellSpec, error) {
	if err := authorize(p, auth.AppManage, appID); err != nil {
		return spec, err
	}
	spec, err := Check(spec)
	if err != nil {
		return spec, err
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return spec, fmt.Errorf("shellupdate: encode the settings: %w", err)
	}
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		app, err := q.GetApp(ctx, storage.MustUUID(appID))
		if err != nil {
			return failure(err, "app")
		}
		if spec.IconAssetID != "" {
			if err := s.checkIcon(ctx, q, app, spec.IconAssetID); err != nil {
				return err
			}
		}
		if _, err := q.UpsertShellSettings(ctx, dbgen.UpsertShellSettingsParams{AppID: app.ID, OrganizationID: app.OrganizationID, Settings: data}); err != nil {
			return failure(err, "shell settings")
		}
		return s.record(ctx, tx, p.OrganizationID, p.Actor(), audit.ShellSettingsSet, appID, "")
	})
	return spec, err
}

// checkFormat returns the format asked for, zip by default.
func checkFormat(format string) (string, error) {
	switch format {
	case "", FormatZip:
		return FormatZip, nil
	case FormatPatch:
		return FormatPatch, nil
	}
	return "", plxerr.New(plxerr.InvalidEnumValue, "a project is generated as zip or patch")
}

// Generate queues a project generation. At most one job per app is active:
// asking again while one is queued or running returns that job. The number
// of jobs per hour is limited by project.jobsPerHour. It needs app.manage.
func (s *Service) Generate(ctx context.Context, p auth.Principal, appID, environment, channel, format string) (ProjectJob, error) {
	if err := authorize(p, auth.AppManage, appID); err != nil {
		return ProjectJob{}, err
	}
	if s.o.Queue == nil {
		return ProjectJob{}, plxerr.New(plxerr.PreconditionFailed, "project generation is not available on this server")
	}
	format, err := checkFormat(format)
	if err != nil {
		return ProjectJob{}, err
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return ProjectJob{}, fmt.Errorf("shellupdate: id: %w", err)
	}
	var out ProjectJob
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		app, err := q.GetApp(ctx, storage.MustUUID(appID))
		if err != nil {
			return failure(err, "app")
		}
		if _, err := target(ctx, q, app, environment, channel); err != nil {
			return err
		}
		if err := patchPrecondition(ctx, q, app, format); err != nil {
			return err
		}
		if active, found, err := activeJob(ctx, q, app.ID, format); err != nil || found {
			out = active
			return err
		}
		if err := s.checkRate(ctx, q, app.ID); err != nil {
			return err
		}
		row, err := q.CreateProjectJob(ctx, dbgen.CreateProjectJobParams{
			ID: storage.MustUUID(id), OrganizationID: app.OrganizationID, AppID: app.ID, EnvironmentKey: environment, ChannelKey: channel,
			Format: format, CreatedBy: p.Actor().ID,
		})
		if err != nil {
			return failure(err, "project job")
		}
		out = jobOf(row)
		if err := s.o.Queue.Enqueue(ctx, tx, Job{OrganizationID: p.OrganizationID, AppID: appID, JobID: id}); err != nil {
			return fmt.Errorf("shellupdate: enqueue: %w", err)
		}
		return s.record(ctx, tx, p.OrganizationID, p.Actor(), audit.ProjectGenerated, appID, id)
	})
	return out, err
}

// Job returns a job's status. It needs app.read on the app, which is checked
// before the job is looked up; a job of another app is not found.
func (s *Service) Job(ctx context.Context, p auth.Principal, appID, jobID string) (ProjectJob, error) {
	if err := authorize(p, auth.AppRead, appID); err != nil {
		return ProjectJob{}, err
	}
	var out ProjectJob
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := loadJob(ctx, dbgen.New(tx), appID, jobID)
		out = jobOf(row)
		return err
	})
	return out, err
}

// loadJob loads a job that belongs to an app.
func loadJob(ctx context.Context, q *dbgen.Queries, appID, jobID string) (dbgen.ProjectJob, error) {
	id, err := storage.UUID(jobID)
	if err != nil {
		return dbgen.ProjectJob{}, plxerr.New(plxerr.InvalidFormat, "the job identifier is not valid")
	}
	row, err := q.GetProjectJob(ctx, id)
	if err != nil {
		return row, failure(err, "project job")
	}
	if storage.ID(row.AppID) != appID {
		return dbgen.ProjectJob{}, plxerr.New(plxerr.ResourceNotFound, "no such project job")
	}
	return row, nil
}

// Download returns the zip of a succeeded job. It needs app.manage.
func (s *Service) Download(ctx context.Context, p auth.Principal, appID, jobID string) (ProjectJob, []byte, error) {
	if err := authorize(p, auth.AppManage, appID); err != nil {
		return ProjectJob{}, nil, err
	}
	var (
		out  ProjectJob
		data []byte
	)
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := loadJob(ctx, q, appID, jobID)
		if err != nil {
			return err
		}
		if row.Status != StatusSucceeded {
			return plxerr.New(plxerr.ResourceNotFound, "the project is not generated (state %s)", row.Status)
		}
		if data, err = q.GetProjectJobResult(ctx, row.ID); err != nil {
			return failure(err, "project")
		}
		out = jobOf(row)
		return nil
	})
	return out, data, err
}

// target finds the environment and checks that it has the channel.
func target(ctx context.Context, q *dbgen.Queries, app dbgen.App, environment, channel string) (dbgen.Environment, error) {
	env, err := q.GetEnvironmentByKey(ctx, dbgen.GetEnvironmentByKeyParams{AppID: app.ID, Key: environment})
	if err != nil {
		return env, failure(err, "environment")
	}
	if _, err := q.GetChannel(ctx, dbgen.GetChannelParams{EnvironmentID: env.ID, Key: channel}); err != nil {
		return env, failure(err, "channel")
	}
	return env, nil
}

// patchPrecondition refuses a patch before any project was generated: it is
// the difference to that project.
func patchPrecondition(ctx context.Context, q *dbgen.Queries, app dbgen.App, format string) error {
	if format != FormatPatch {
		return nil
	}
	_, gen, _, err := settings(ctx, q, app)
	if err != nil {
		return err
	}
	if gen == nil {
		return plxerr.New(plxerr.PreconditionFailed, "a patch needs a project generated before; generate the project first")
	}
	return nil
}

// activeJob returns the job of an app that is queued or running, which a
// new request of the same format is answered with; a request of the other
// format waits for it.
func activeJob(ctx context.Context, q *dbgen.Queries, app pgtype.UUID, format string) (ProjectJob, bool, error) {
	row, err := q.GetActiveProjectJob(ctx, app)
	switch {
	case err == nil && row.Format != format:
		return ProjectJob{}, false, plxerr.New(plxerr.PreconditionFailed, "a %s is being generated; ask for the %s when it is done", row.Format, format)
	case err == nil:
		return jobOf(row), true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return ProjectJob{}, false, nil
	}
	return ProjectJob{}, false, failure(err, "project job")
}

// checkRate refuses a job beyond project.jobsPerHour.
func (s *Service) checkRate(ctx context.Context, q *dbgen.Queries, app pgtype.UUID) error {
	n, err := q.CountProjectJobsSince(ctx, dbgen.CountProjectJobsSinceParams{AppID: app, CreatedAt: storage.Timestamp(s.now().Add(-time.Hour))})
	if err != nil {
		return failure(err, "project job")
	}
	if max := s.o.Limits.Get(limits.ProjectJobsPerHour); n >= max {
		return plxerr.New(plxerr.RateLimited, "at most %d projects per hour may be generated for an app", max)
	}
	return nil
}

func jobOf(r dbgen.ProjectJob) ProjectJob {
	return ProjectJob{
		ID: storage.ID(r.ID), AppID: storage.ID(r.AppID), Environment: r.EnvironmentKey, Channel: r.ChannelKey, Format: r.Format,
		Status: r.Status, ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage, SHA256: r.Sha256, Size: r.Size,
		CreatedAt: storage.Time(r.CreatedAt), FinishedAt: storage.Time(r.FinishedAt),
	}
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, org string, actor audit.Actor, action audit.Action, appID, detail string) error {
	if _, err := s.o.Audit.Append(ctx, tx, audit.Entry{OrganizationID: org, Actor: actor, Action: action, TargetKind: "app", TargetID: appID, Detail: detail}); err != nil {
		return fmt.Errorf("shellupdate: audit: %w", err)
	}
	return nil
}

// claim is what a job needs, read in one transaction.
type claim struct {
	row   dbgen.ProjectJob
	app   dbgen.App
	spec  generator.ShellSpec
	gen   *generator.ShellSpec
	envID string
	// iconSHA is the content hash of the current icon asset; iconGone says
	// the setting names an asset that no longer exists.
	iconSHA  string
	iconGone bool
}

// Work generates the project of one job. A generation that fails is recorded
// on the job with its registered code and is not retried; only an
// infrastructure error is returned, for the queue to retry.
func (s *Service) Work(ctx context.Context, job Job) error {
	tenant := storage.Tenant{OrganizationID: job.OrganizationID}
	var (
		c       claim
		started bool
	)
	err := s.o.DB.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		var err error
		if c.row, err = q.GetProjectJobForUpdate(ctx, storage.MustUUID(job.JobID)); err != nil {
			return failure(err, "project job")
		}
		if n, err := q.StartProjectJob(ctx, c.row.ID); err != nil || n == 0 {
			return err //nolint:wrapcheck // wrapped below
		}
		started = true
		if c.app, err = q.GetApp(ctx, c.row.AppID); err != nil {
			return failure(err, "app")
		}
		if c.spec, c.gen, _, err = settings(ctx, q, c.app); err != nil {
			return err
		}
		if c.spec.IconAssetID != "" {
			if icon, err := currentIcon(ctx, q, c.app, c.spec.IconAssetID); err == nil {
				c.iconSHA = hex.EncodeToString(icon.Sha256)
			} else {
				c.iconGone = true
			}
		}
		env, err := q.GetEnvironmentByKey(ctx, dbgen.GetEnvironmentByKeyParams{AppID: c.app.ID, Key: c.row.EnvironmentKey})
		if err != nil {
			return failure(err, "environment")
		}
		c.envID = storage.ID(env.ID)
		return nil
	})
	if err != nil {
		return fmt.Errorf("shellupdate: claim the job: %w", err)
	}
	if !started {
		return nil
	}
	data, genSpec, genErr := s.build(ctx, job, c)
	return s.finish(ctx, tenant, c.row, c.spec, genSpec, data, genErr)
}

// render generates the files of one project from a spec and its icon.
func render(spec generator.ShellSpec, icon []byte, keys []codegen.RootKey) ([]generator.File, generator.ShellSpec, error) {
	derived, err := generator.Derive(spec)
	if err != nil {
		return nil, spec, plxerr.Wrap(plxerr.InvalidFormat, err, "the shell settings are not valid: %v", err)
	}
	files, err := generator.Generate(generator.Input{Spec: derived, Icon: icon, Keys: keys})
	if err != nil {
		return nil, spec, plxerr.Wrap(plxerr.InvalidFormat, err, "the project cannot be generated: %v", err)
	}
	if icon != nil {
		sum := sha256.Sum256(icon)
		derived.IconSHA256 = hex.EncodeToString(sum[:])
	} else {
		derived.IconSHA256 = ""
	}
	return files, derived, nil
}

// build generates the project's zip, or the patch from the last generation.
func (s *Service) build(ctx context.Context, job Job, c claim) ([]byte, generator.ShellSpec, error) {
	spec := c.spec
	if s.o.Keys == nil {
		return nil, spec, plxerr.New(plxerr.PreconditionFailed, "root keys are not available on this server")
	}
	keys, err := s.o.Keys(ctx, job.OrganizationID, c.envID)
	if err != nil {
		return nil, spec, fmt.Errorf("shellupdate: root keys: %w", err)
	}
	if len(keys) == 0 {
		return nil, spec, plxerr.New(plxerr.InvalidFormat, "the environment has no root keys yet; publish a release first")
	}
	spec.Plux = generator.PluxSource{
		AppKey: c.app.Key, AppID: storage.ID(c.app.ID), Endpoint: s.o.Endpoint, Environment: c.row.EnvironmentKey, Channel: c.row.ChannelKey,
		EntryRoute: spec.Plux.EntryRoute,
	}
	var icon []byte
	switch {
	case c.iconGone:
		return nil, spec, plxerr.New(plxerr.PreconditionFailed, "the icon asset no longer exists; choose another icon or the default")
	case spec.IconAssetID != "":
		if icon, err = s.loadIcon(ctx, c.iconSHA); err != nil {
			return nil, spec, err
		}
	}
	files, derived, err := render(spec, icon, keys)
	if err != nil {
		return nil, spec, err
	}
	root := derived.Package
	if c.row.Format == FormatPatch {
		if files, err = s.patch(ctx, c.gen, files, derived, keys); err != nil {
			return nil, spec, err
		}
	}
	zipped, err := generator.Zip(root, files)
	if err != nil {
		return nil, spec, fmt.Errorf("shellupdate: zip: %w", err)
	}
	if max := s.o.Limits.Get(limits.ProjectBytes); int64(len(zipped)) > max {
		return nil, spec, plxerr.New(plxerr.LimitExceeded, "the project has %d bytes; at most %d are kept", len(zipped), max)
	}
	return zipped, derived, nil
}

// patch diffs the project of the last generation against the new one. The
// old project is generated again from the stored spec and the icon it
// recorded, with the current root keys on both sides, so a key rotation is
// not a difference.
func (s *Service) patch(ctx context.Context, gen *generator.ShellSpec, updated []generator.File, to generator.ShellSpec, keys []codegen.RootKey) ([]generator.File, error) {
	if gen == nil {
		return nil, plxerr.New(plxerr.PreconditionFailed, "a patch needs a project generated before; generate the project first")
	}
	var icon []byte
	if gen.IconSHA256 != "" {
		var err error
		if icon, err = s.loadIcon(ctx, gen.IconSHA256); err != nil {
			return nil, err
		}
	}
	old, from, err := render(*gen, icon, keys)
	if err != nil {
		return nil, err
	}
	out, err := generator.Patch(old, updated, from, to)
	if err != nil {
		return nil, fmt.Errorf("shellupdate: patch: %w", err)
	}
	return out, nil
}

// currentIcon reads the icon asset of an app, under the app's row-level
// security: another app's or organisation's asset is not found.
func currentIcon(ctx context.Context, q *dbgen.Queries, app dbgen.App, assetID string) (dbgen.Asset, error) {
	id, err := storage.UUID(assetID)
	if err != nil {
		return dbgen.Asset{}, plxerr.New(plxerr.InvalidFormat, "the icon is not an asset of this app")
	}
	row, err := q.GetAppAssetByAssetID(ctx, dbgen.GetAppAssetByAssetIDParams{AppID: app.ID, AssetID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Asset{}, plxerr.New(plxerr.InvalidFormat, "the icon is not an asset of this app")
	}
	if err != nil {
		return dbgen.Asset{}, failure(err, "asset")
	}
	return row, nil
}

// checkIcon refuses an icon asset the generator cannot use (GEN-004): it
// must belong to the app, be a PNG, and be square from 1024 to 4096 pixels.
// Every failure to find it reads the same, so no other app's or
// organisation's asset is confirmed to exist.
func (s *Service) checkIcon(ctx context.Context, q *dbgen.Queries, app dbgen.App, assetID string) error {
	row, err := currentIcon(ctx, q, app, assetID)
	if err != nil {
		return err
	}
	if row.MediaType != "image/png" {
		return plxerr.New(plxerr.InvalidFormat, "the icon must be a PNG image")
	}
	data, err := s.loadIcon(ctx, hex.EncodeToString(row.Sha256))
	if err != nil {
		return err
	}
	if err := generator.CheckIcon(data); err != nil {
		return plxerr.Wrap(plxerr.InvalidFormat, err, "the icon cannot be used: %v", err)
	}
	return nil
}

// loadIcon reads an asset object by its content hash. An object that is gone
// fails the precondition; any other failure is the store's.
func (s *Service) loadIcon(ctx context.Context, sha string) ([]byte, error) {
	if s.o.Objects == nil {
		return nil, plxerr.New(plxerr.PreconditionFailed, "asset storage is not available on this server")
	}
	key, err := objects.Key(objects.KindAsset, sha)
	if err != nil {
		return nil, plxerr.New(plxerr.PreconditionFailed, "the icon is not available")
	}
	data, _, err := s.o.Objects.Get(ctx, key)
	if errors.Is(err, objects.ErrNotFound) {
		return nil, plxerr.New(plxerr.PreconditionFailed, "the icon is no longer available; choose another icon or the default")
	}
	if err != nil {
		return nil, fmt.Errorf("shellupdate: read the icon: %w", err)
	}
	return data, nil
}

// finish records the outcome. A refusal (a plxerr) fails the job for good.
func (s *Service) finish(ctx context.Context, tenant storage.Tenant, row dbgen.ProjectJob, settings, spec generator.ShellSpec, data []byte, buildErr error) error {
	var perr *plxerr.Error
	if buildErr != nil && !errors.As(buildErr, &perr) {
		return buildErr
	}
	return s.o.DB.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error { //nolint:wrapcheck // InTx wraps its own failures
		q := dbgen.New(tx)
		if perr != nil {
			return q.FinishProjectJob(ctx, dbgen.FinishProjectJobParams{ID: row.ID, Status: StatusFailed, ErrorCode: perr.Code.String(), ErrorMessage: perr.Message})
		}
		sum := sha256.Sum256(data)
		if err := q.SetProjectJobResult(ctx, dbgen.SetProjectJobResultParams{JobID: row.ID, OrganizationID: row.OrganizationID, AppID: row.AppID, Data: data}); err != nil {
			return failure(err, "project")
		}
		doc, err := json.Marshal(spec)
		if err != nil {
			return fmt.Errorf("shellupdate: encode the settings: %w", err)
		}
		stored, err := json.Marshal(settings)
		if err != nil {
			return fmt.Errorf("shellupdate: encode the settings: %w", err)
		}
		if err := q.SetShellGenerated(ctx, dbgen.SetShellGeneratedParams{
			AppID: row.AppID, OrganizationID: row.OrganizationID, Settings: stored, Generated: doc,
		}); err != nil {
			return failure(err, "shell settings")
		}
		return q.FinishProjectJob(ctx, dbgen.FinishProjectJobParams{
			ID: row.ID, Status: StatusSucceeded, Size: int64(len(data)), Sha256: hex.EncodeToString(sum[:]),
		})
	})
}
