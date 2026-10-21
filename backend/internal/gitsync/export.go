// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gitsync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Exporter renders an app in the SCH-006 layout; *document.Service is one.
type Exporter interface {
	Export(ctx context.Context, p auth.Principal, appID, pluginID string) ([]document.File, error)
}

// SecretOpener opens an environment secret for the server's own use;
// *tenancy.Service is one.
type SecretOpener interface {
	OpenEnvironmentSecret(ctx context.Context, organizationID, envID, key string) ([]byte, error)
}

// Enqueuer adds an export job in the caller's transaction.
type Enqueuer interface {
	Enqueue(ctx context.Context, tx pgx.Tx, job ExportJob) error
}

// IDs generates identifiers for the rows this package writes.
type IDs interface{ New() (string, error) }

// ExportJob asks the worker to export one published version (GOV-011).
type ExportJob struct {
	OrganizationID string `json:"organizationId"`
	AppID          string `json:"appId"`
	// VersionID is the published version, the release identifier of the
	// commit message.
	VersionID string `json:"versionId"`
	// Summary says what was published, for the commit message.
	Summary string `json:"summary"`
	// ActorID and Actor are the publisher, who authors the commit.
	ActorID string `json:"actorId"`
	Actor   string `json:"actor"`
}

// Kind names the job.
func (ExportJob) Kind() string { return "git.export" }

// Options configure a Service.
type Options struct {
	DB        *storage.DB
	Audit     *audit.Log
	Documents Exporter
	Secrets   SecretOpener
	// Jobs enqueues exports; the process that records publishes sets it.
	Jobs Enqueuer
	IDs  IDs
	// Limits are the installation's limits; zero means the defaults.
	Limits limits.Set
	// Policy decides which addresses a remote may have; the zero value
	// forbids the reserved ranges.
	Policy httpx.Policy
	// Resolver resolves remote names; nil uses the system resolver.
	Resolver Resolver
	// Random is the source of the proxy secret; nil uses crypto/rand.
	Random io.Reader
	// CABundle is a PEM bundle added to the roots a remote's certificate
	// is checked against; it exists for tests and private CAs.
	CABundle []byte
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service configures and runs the Git export.
type Service struct {
	o   Options
	now func() time.Time
}

// New returns the service.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil {
		return nil, errors.New("gitsync: a database, audit log and identifiers are required")
	}
	if o.Limits.IsZero() {
		o.Limits = limits.Defaults()
	}
	if o.Resolver == nil {
		o.Resolver = systemResolver
	}
	if o.Random == nil {
		o.Random = rand.Reader
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{o: o, now: now}, nil
}

// Export is a row of the export log.
type Export struct {
	ID        string
	AppID     string
	VersionID string
	// State is "pending", "pushed", or "failed" for an attempt that failed
	// and is retried.
	State     string
	Commit    string
	Attempts  int32
	ErrorCode int32
	CreatedAt time.Time
	UpdatedAt time.Time
}

func exportOf(r dbgen.GitExport) Export {
	return Export{
		ID: storage.ID(r.ID), AppID: storage.ID(r.AppID), VersionID: storage.ID(r.VersionID), State: r.State,
		Commit: r.CommitSha, Attempts: r.Attempts, ErrorCode: r.ErrorCode,
		CreatedAt: storage.Time(r.CreatedAt), UpdatedAt: storage.Time(r.UpdatedAt),
	}
}

// Published is called by the publish, in the transaction that records a
// version, and queues an export when the app has one enabled. It is the
// hook release.Options.Published expects.
func (s *Service) Published(ctx context.Context, tx pgx.Tx, organizationID, appID, versionID, summary, actorID, actor string) error {
	q := dbgen.New(tx)
	settings, err := q.GetGitSettings(ctx, storage.MustUUID(appID))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !settings.Enabled) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("gitsync: read the setting: %w", err)
	}
	job := ExportJob{OrganizationID: organizationID, AppID: appID, VersionID: versionID, Summary: summary, ActorID: actorID, Actor: actor}
	_, err = s.queue(ctx, tx, job)
	return err
}

// queue records an export of a version and enqueues it, once: a version
// already recorded is left to the job that has it.
func (s *Service) queue(ctx context.Context, tx pgx.Tx, job ExportJob) (bool, error) {
	id, err := s.o.IDs.New()
	if err != nil {
		return false, fmt.Errorf("gitsync: %w", err)
	}
	_, err = dbgen.New(tx).InsertGitExport(ctx, dbgen.InsertGitExportParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(job.OrganizationID),
		AppID: storage.MustUUID(job.AppID), VersionID: storage.MustUUID(job.VersionID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("gitsync: record the export: %w", err)
	}
	if s.o.Jobs == nil {
		return false, errors.New("gitsync: the job queue is not ready")
	}
	if err := s.o.Jobs.Enqueue(ctx, tx, job); err != nil {
		return false, fmt.Errorf("gitsync: %w", err)
	}
	return true, nil
}

// List returns the newest exports of an app.
func (s *Service) List(ctx context.Context, p auth.Principal, appID string) ([]Export, error) {
	if err := p.AuthorizeApp(auth.GitExport, appID); err != nil {
		return nil, fmt.Errorf("gitsync: %w", err)
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	var out []Export
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListGitExports(ctx, dbgen.ListGitExportsParams{AppID: app, PageSize: int32(s.o.Limits.Get(limits.APIPageSize))}) //nolint:gosec // bounded by the registry
		if err != nil {
			return fmt.Errorf("gitsync: list: %w", err)
		}
		for _, r := range rows {
			out = append(out, exportOf(r))
		}
		return nil
	})
	return out, err
}

// Run queues the export of the newest published version of the app bundle,
// or returns the one already made; a version is exported once, unless its
// last attempt failed, which Run tries again.
func (s *Service) Run(ctx context.Context, p auth.Principal, appID string) (Export, error) {
	if err := p.AuthorizeApp(auth.GitExport, appID); err != nil {
		return Export{}, fmt.Errorf("gitsync: %w", err)
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return Export{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	var out Export
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		settings, err := q.GetGitSettings(ctx, app)
		if err != nil {
			return missing(err, "Git export setting")
		}
		if !settings.Enabled {
			return plxerr.New(plxerr.GitRemoteRefused, "the Git export of this app is not enabled")
		}
		v, err := q.LatestVersion(ctx, dbgen.LatestVersionParams{AppID: app})
		if err != nil {
			return missing(err, "published version of the app")
		}
		job := ExportJob{
			OrganizationID: p.OrganizationID, AppID: appID, VersionID: storage.ID(v.ID),
			Summary: fmt.Sprintf("app bundle version %d", v.Version), ActorID: p.ID, Actor: p.Display,
		}
		queued, err := s.queue(ctx, tx, job)
		if err != nil {
			return err
		}
		row, err := q.GetGitExportForUpdate(ctx, dbgen.GetGitExportForUpdateParams{AppID: app, VersionID: v.ID})
		if err != nil {
			return missing(err, "export")
		}
		if !queued && row.State == "failed" {
			if err := s.o.Jobs.Enqueue(ctx, tx, job); err != nil {
				return fmt.Errorf("gitsync: %w", err)
			}
		}
		out = exportOf(row)
		return nil
	})
	return out, err
}

// Work performs an export job. A failure is returned for the job system to
// retry with backoff; it is also recorded on the export as a code.
func (s *Service) Work(ctx context.Context, job ExportJob) error {
	settings, export, run, err := s.begin(ctx, job)
	if err != nil || !run {
		return err
	}
	commit, err := s.pushVersion(ctx, job, settings)
	if err != nil {
		return s.fail(ctx, job, export, err)
	}
	return s.succeed(ctx, job, export, commit)
}

// begin reads the setting and the export row, and counts the attempt. It
// reports run == false when there is nothing to do: the export is disabled
// or already pushed.
func (s *Service) begin(ctx context.Context, job ExportJob) (dbgen.GitSetting, dbgen.GitExport, bool, error) {
	var (
		settings dbgen.GitSetting
		export   dbgen.GitExport
		run      bool
	)
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: job.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		app := storage.MustUUID(job.AppID)
		var err error
		if settings, err = q.GetGitSettings(ctx, app); errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("gitsync: read the setting: %w", err)
		}
		if export, err = q.GetGitExportForUpdate(ctx, dbgen.GetGitExportForUpdateParams{AppID: app, VersionID: storage.MustUUID(job.VersionID)}); err != nil {
			return missing(err, "export")
		}
		if !settings.Enabled || export.State == "pushed" {
			return nil
		}
		run = true
		return q.MarkGitExportAttempt(ctx, export.ID)
	})
	if err != nil {
		return settings, export, run, fmt.Errorf("gitsync: claim the export: %w", err)
	}
	return settings, export, run, nil
}

// pushVersion renders the app and pushes it as one commit.
func (s *Service) pushVersion(ctx context.Context, job ExportJob, st dbgen.GitSetting) (string, error) {
	if s.o.Documents == nil || s.o.Secrets == nil {
		return "", errors.New("gitsync: the export is not wired to documents and secrets")
	}
	host, err := remoteHost(st.RemoteUrl)
	if err != nil {
		return "", err
	}
	if _, err := allowedAddresses(ctx, s.o.Policy, s.o.Resolver, host); err != nil && isRefusal(err) {
		return "", plxerr.Wrap(plxerr.GitRemoteRefused, err, "the remote's address is not one the server may connect to")
	}
	files, err := s.o.Documents.Export(ctx, auth.System(job.OrganizationID), job.AppID, "")
	if err != nil {
		return "", fmt.Errorf("gitsync: render the app: %w", err)
	}
	token, err := s.o.Secrets.OpenEnvironmentSecret(ctx, job.OrganizationID, storage.ID(st.CredentialEnvironmentID), st.CredentialKey)
	if err != nil {
		return "", fmt.Errorf("gitsync: open the credential: %w", err)
	}
	secret, err := s.proxySecret()
	if err != nil {
		return "", err
	}
	px, err := startProxy(ctx, s.o.Policy, s.o.Resolver, secret)
	if err != nil {
		return "", err
	}
	defer px.Close()
	name, email := author(job)
	return commitAndPush(ctx, push{
		remote: st.RemoteUrl, branch: st.Branch, username: firstNonEmpty(st.Username, DefaultUsername), token: string(token),
		files: files, name: name, email: email, when: s.now().UTC(), message: message(job),
		proxy:    transport.ProxyOptions{URL: "http://" + px.address(), Username: "plux", Password: secret},
		caBundle: s.o.CABundle,
	})
}

func (s *Service) proxySecret() (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(s.o.Random, b); err != nil {
		return "", fmt.Errorf("gitsync: random: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// author is the publisher's display name and a no-reply address.
func author(job ExportJob) (string, string) {
	name := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '<' || r == '>' {
			return -1
		}
		return r
	}, job.Actor))
	if name == "" {
		name = "Plux"
	}
	id := strings.Map(func(r rune) rune {
		if unicode.IsLower(r) && r < unicode.MaxASCII || unicode.IsDigit(r) && r < unicode.MaxASCII || r == '-' {
			return r
		}
		return -1
	}, strings.ToLower(job.ActorID))
	if id == "" {
		id = "plux"
	}
	return name, id + "@users.noreply.plux.invalid"
}

// message is the commit message: what was published, and its release.
func message(job ExportJob) string {
	summary := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, job.Summary)
	return fmt.Sprintf("Publish %s\n\nRelease-ID: %s\n", strings.TrimSpace(summary), job.VersionID)
}

// succeed records a pushed export and audits it.
func (s *Service) succeed(ctx context.Context, job ExportJob, export dbgen.GitExport, commit string) error {
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: job.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		if err := dbgen.New(tx).MarkGitExportPushed(ctx, dbgen.MarkGitExportPushedParams{ID: export.ID, CommitSha: commit}); err != nil {
			return fmt.Errorf("gitsync: record the push: %w", err)
		}
		actor := auth.System(job.OrganizationID).Actor()
		return s.record(ctx, tx, actor, job.OrganizationID, audit.GitExported, "app", job.AppID, "release "+job.VersionID+" commit "+commit)
	})
	if err != nil {
		return fmt.Errorf("gitsync: record the export: %w", err)
	}
	return nil
}

// fail records the code of a failed attempt, in a transaction of its own,
// and returns the failure for the job system to retry.
func (s *Service) fail(ctx context.Context, job ExportJob, export dbgen.GitExport, cause error) error {
	var perr *plxerr.Error
	code := int32(plxerr.GitRemoteRefused)
	if errors.As(cause, &perr) {
		code = int32(perr.Code) //nolint:gosec // registry codes are four digits
	}
	err := s.o.DB.InTx(context.WithoutCancel(ctx), storage.Tenant{OrganizationID: job.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		return dbgen.New(tx).MarkGitExportFailed(ctx, dbgen.MarkGitExportFailedParams{ID: export.ID, ErrorCode: code})
	})
	if err != nil {
		return errors.Join(cause, fmt.Errorf("gitsync: record the failure: %w", err))
	}
	return cause
}

// record appends an audit entry.
func (s *Service) record(ctx context.Context, tx pgx.Tx, actor audit.Actor, organizationID string, action audit.Action, kind, id, detail string) error {
	if _, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: organizationID, Actor: actor, Action: action, TargetKind: kind, TargetID: id, Detail: detail,
	}); err != nil {
		return fmt.Errorf("gitsync: audit: %w", err)
	}
	return nil
}

// userID returns the signed-in user, or NULL for a token or the server.
func userID(p auth.Principal) pgtype.UUID {
	id, err := storage.NullUUID(p.UserID)
	if err != nil {
		return pgtype.UUID{}
	}
	return id
}
