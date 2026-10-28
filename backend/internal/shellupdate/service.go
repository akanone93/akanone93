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
)

// The states of a project job.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// FormatZip is the only project format (a Git push is not offered; see the
// S8.1 hand-off note).
const FormatZip = "zip"

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
		if gen != nil {
			out.Changes = Compare(*gen, cur)
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
		if _, err := q.UpsertShellSettings(ctx, dbgen.UpsertShellSettingsParams{AppID: app.ID, OrganizationID: app.OrganizationID, Settings: data}); err != nil {
			return failure(err, "shell settings")
		}
		return s.record(ctx, tx, p.OrganizationID, p.Actor(), audit.ShellSettingsSet, appID, "")
	})
	return spec, err
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
	if format == "" {
		format = FormatZip
	}
	if format != FormatZip {
		return ProjectJob{}, plxerr.New(plxerr.InvalidEnumValue, "a project is generated as zip")
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
		if active, found, err := activeJob(ctx, q, app.ID); err != nil || found {
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

// activeJob returns the job of an app that is queued or running.
func activeJob(ctx context.Context, q *dbgen.Queries, app pgtype.UUID) (ProjectJob, bool, error) {
	row, err := q.GetActiveProjectJob(ctx, app)
	switch {
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

// Work generates the project of one job. A generation that fails is recorded
// on the job with its registered code and is not retried; only an
// infrastructure error is returned, for the queue to retry.
func (s *Service) Work(ctx context.Context, job Job) error {
	tenant := storage.Tenant{OrganizationID: job.OrganizationID}
	var (
		row     dbgen.ProjectJob
		app     dbgen.App
		spec    generator.ShellSpec
		envID   string
		started bool
	)
	err := s.o.DB.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		var err error
		if row, err = q.GetProjectJobForUpdate(ctx, storage.MustUUID(job.JobID)); err != nil {
			return failure(err, "project job")
		}
		if n, err := q.StartProjectJob(ctx, row.ID); err != nil || n == 0 {
			return err //nolint:wrapcheck // wrapped below
		}
		started = true
		if app, err = q.GetApp(ctx, row.AppID); err != nil {
			return failure(err, "app")
		}
		if spec, _, _, err = settings(ctx, q, app); err != nil {
			return err
		}
		env, err := q.GetEnvironmentByKey(ctx, dbgen.GetEnvironmentByKeyParams{AppID: app.ID, Key: row.EnvironmentKey})
		if err != nil {
			return failure(err, "environment")
		}
		envID = storage.ID(env.ID)
		return nil
	})
	if err != nil {
		return fmt.Errorf("shellupdate: claim the job: %w", err)
	}
	if !started {
		return nil
	}
	data, genSpec, genErr := s.build(ctx, job, app, spec, envID, row)
	return s.finish(ctx, tenant, row, spec, genSpec, data, genErr)
}

// build generates the project's zip.
func (s *Service) build(ctx context.Context, job Job, app dbgen.App, spec generator.ShellSpec, envID string, row dbgen.ProjectJob) ([]byte, generator.ShellSpec, error) {
	if s.o.Keys == nil {
		return nil, spec, plxerr.New(plxerr.PreconditionFailed, "root keys are not available on this server")
	}
	keys, err := s.o.Keys(ctx, job.OrganizationID, envID)
	if err != nil {
		return nil, spec, fmt.Errorf("shellupdate: root keys: %w", err)
	}
	if len(keys) == 0 {
		return nil, spec, plxerr.New(plxerr.InvalidFormat, "the environment has no root keys yet; publish a release first")
	}
	spec.Plux = generator.PluxSource{
		AppKey: app.Key, AppID: storage.ID(app.ID), Endpoint: s.o.Endpoint, Environment: row.EnvironmentKey, Channel: row.ChannelKey,
		EntryRoute: spec.Plux.EntryRoute,
	}
	derived, err := generator.Derive(spec)
	if err != nil {
		return nil, spec, plxerr.Wrap(plxerr.InvalidFormat, err, "the shell settings are not valid: %v", err)
	}
	files, err := generator.Generate(generator.Input{Spec: derived, Keys: keys})
	if err != nil {
		return nil, spec, plxerr.Wrap(plxerr.InvalidFormat, err, "the project cannot be generated: %v", err)
	}
	zipped, err := generator.Zip(derived.Package, files)
	if err != nil {
		return nil, spec, fmt.Errorf("shellupdate: zip: %w", err)
	}
	if max := s.o.Limits.Get(limits.ProjectBytes); int64(len(zipped)) > max {
		return nil, spec, plxerr.New(plxerr.LimitExceeded, "the project has %d bytes; at most %d are kept", len(zipped), max)
	}
	return zipped, derived, nil
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
