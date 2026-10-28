// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package releasereq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/plxgolden"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// The states of a visual-diff job.
const (
	DiffQueued    = "queued"
	DiffRunning   = "running"
	DiffSucceeded = "succeeded"
	DiffFailed    = "failed"
)

// ErrDeferred is returned by RunRender when the organisation already has
// render.visualDiffJobsPerOrganization jobs running: the job stays queued and
// the worker runs it again later.
var ErrDeferred = errors.New("releasereq: the organisation's render jobs are at their limit")

// RenderJob is the queued work of one visual diff (O-9). It names the job row
// and nothing else: the render worker is given its input, not the database.
type RenderJob struct {
	JobID          string `json:"jobId"`
	OrganizationID string `json:"organizationId"`
}

// Kind names the job type for the queue.
func (RenderJob) Kind() string { return "releasereq.render" }

// RenderInput is everything a renderer gets: the versions to render and the
// standard devices. It carries no credential (L-4).
type RenderInput struct {
	JobID       string
	ContentHash string
	Plugins     []RenderPlugin
	// Devices are the standard device frames of ADR-0070.
	Devices []string
	// AppID, EnvironmentID and Channel name where the request releases to.
	AppID, EnvironmentID, Channel string
	// After is the project a release of the built versions would hold, and
	// Before the project the channel serves now, nil when it serves none, so
	// that every screen is new. Both are nil where the service has no Sources.
	Before, After fs.FS
	// Limits are the limits of the request's organisation.
	Limits limits.Set
}

// RenderPlugin is a built plugin version to render.
type RenderPlugin struct {
	PluginKey    string
	Version      int64
	BundleSHA256 string
}

// Screen is one changed screen rendered on one device. Before is nil for a new
// screen; both are PNG images.
type Screen struct {
	PluginKey, Screen, Device string
	Before, After             []byte
}

// Renderer renders the visual diff of the exact versions of a request on the
// standard devices, for each changed screen. The implementation runs in the
// render worker image, which holds no database credentials and no signing
// capability (L-4); it is an interface so that tests use a fake.
type Renderer interface {
	Render(ctx context.Context, in RenderInput) ([]Screen, error)
}

// ScreenResult is a stored screen: the hashes of the images in object storage.
type ScreenResult struct {
	PluginKey    string `json:"pluginKey"`
	Screen       string `json:"screen"`
	Device       string `json:"device"`
	BeforeSHA256 string `json:"beforeSha256"`
	AfterSHA256  string `json:"afterSha256"`
	Changed      bool   `json:"changed"`
}

// VisualDiff is a visual-diff job and, once it has finished, its result.
type VisualDiff struct {
	ID, RequestID, ContentHash string
	State, Error               string
	Screens                    []ScreenResult
	CreatedAt, FinishedAt      time.Time
}

func diffOf(j dbgen.VisualDiffJob) (VisualDiff, error) {
	out := VisualDiff{
		ID: storage.ID(j.ID), RequestID: storage.ID(j.ReleaseRequestID), ContentHash: j.ContentHash, State: j.State, Error: j.Error,
		CreatedAt: j.CreatedAt.Time, FinishedAt: j.FinishedAt.Time,
	}
	if len(j.Result) > 0 {
		if err := json.Unmarshal(j.Result, &out.Screens); err != nil {
			return VisualDiff{}, fmt.Errorf("releasereq: stored diff result: %w", err)
		}
	}
	return out, nil
}

// RequestDiff queues the visual-diff job of the request's content, or returns
// the job already queued, running or finished for it: asking again is safe.
// It needs release.publish.
func (s *Service) RequestDiff(ctx context.Context, p auth.Principal, appID, id string) (VisualDiff, error) {
	if err := authorize(p, auth.ReleasePublish, appID); err != nil {
		return VisualDiff{}, err
	}
	if s.o.Jobs == nil {
		return VisualDiff{}, plxerr.New(plxerr.PreconditionFailed, "this installation does not render visual diffs")
	}
	app, req, err := ids(appID, id)
	if err != nil {
		return VisualDiff{}, err
	}
	org, err := storage.UUID(p.OrganizationID)
	if err != nil {
		return VisualDiff{}, fmt.Errorf("releasereq: %w", err)
	}
	var out VisualDiff
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
		if !r.State.Open() {
			return ended(r)
		}
		if r.State != StateDiffs || r.Hold != HoldNone {
			return plxerr.New(plxerr.ReleaseRequestState, "a visual diff is requested at the diffs step; the request is at the %s step", r.State)
		}
		if job, ok, err := reusableDiff(ctx, q, req, r.ContentHash); ok || err != nil {
			out = job
			return err
		}
		out, err = s.queueDiff(ctx, tx, p, org, req, r.ContentHash)
		return err
	})
	return out, err
}

// reusableDiff finds the job already queued, running or finished for the
// content, which asking again returns instead of queueing another.
func reusableDiff(ctx context.Context, q *dbgen.Queries, req pgtype.UUID, content string) (VisualDiff, bool, error) {
	job, err := q.LatestVisualDiffJob(ctx, dbgen.LatestVisualDiffJobParams{ReleaseRequestID: req, ContentHash: content})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return VisualDiff{}, false, nil
	case err != nil:
		return VisualDiff{}, false, failure(err, "visual diff")
	case job.State == DiffFailed:
		return VisualDiff{}, false, nil
	}
	out, err := diffOf(job)
	return out, true, err
}

// queueDiff records a job and queues it in the caller's transaction.
func (s *Service) queueDiff(ctx context.Context, tx pgx.Tx, p auth.Principal, org, req pgtype.UUID, content string) (VisualDiff, error) {
	jobID, err := s.o.IDs.New()
	if err != nil {
		return VisualDiff{}, fmt.Errorf("releasereq: %w", err)
	}
	jid, err := storage.UUID(jobID)
	if err != nil {
		return VisualDiff{}, fmt.Errorf("releasereq: %w", err)
	}
	job, err := dbgen.New(tx).InsertVisualDiffJob(ctx, dbgen.InsertVisualDiffJobParams{
		ID: jid, OrganizationID: org, ReleaseRequestID: req, ContentHash: content, CreatedBy: p.Actor().ID,
	})
	if err != nil {
		return VisualDiff{}, failure(err, "visual diff")
	}
	if err := s.o.Jobs.Enqueue(ctx, tx, RenderJob{JobID: jobID, OrganizationID: p.OrganizationID}); err != nil {
		return VisualDiff{}, fmt.Errorf("releasereq: queue the render job: %w", err)
	}
	return diffOf(job)
}

// Diff returns the newest visual-diff job for the content the request has now;
// found is false when none was requested. It needs plugin.read.
func (s *Service) Diff(ctx context.Context, p auth.Principal, appID, id string) (out VisualDiff, found bool, err error) {
	if err := authorize(p, auth.PluginRead, appID); err != nil {
		return VisualDiff{}, false, err
	}
	app, req, err := ids(appID, id)
	if err != nil {
		return VisualDiff{}, false, err
	}
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetReleaseRequest(ctx, dbgen.GetReleaseRequestParams{ID: req, AppID: app})
		if err != nil {
			return failure(err, "release request")
		}
		job, err := q.LatestVisualDiffJob(ctx, dbgen.LatestVisualDiffJobParams{ReleaseRequestID: req, ContentHash: row.ContentHash})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return failure(err, "visual diff")
		}
		found = true
		out, err = diffOf(job)
		return err
	})
	return out, found, err
}

// diffDone completes the diffs step: the latest job for the content must have
// succeeded (GOV-023).
func diffDone(ctx context.Context, q *dbgen.Queries, r *Request) error {
	if err := finishedDiff(ctx, q, r); err != nil {
		return err
	}
	r.moveOn()
	return nil
}

func finishedDiff(ctx context.Context, q *dbgen.Queries, r *Request) error {
	req, err := storage.UUID(r.ID)
	if err != nil {
		return fmt.Errorf("releasereq: %w", err)
	}
	job, err := q.LatestVisualDiffJob(ctx, dbgen.LatestVisualDiffJobParams{ReleaseRequestID: req, ContentHash: r.ContentHash})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && job.State != DiffSucceeded) {
		return plxerr.New(plxerr.VisualDiffRequired, "the visual diff of this content is not finished")
	}
	if err != nil {
		return failure(err, "visual diff")
	}
	return nil
}

// system runs f in the tenant of an organisation, as the worker.
func (s *Service) system(ctx context.Context, org string, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: org}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// limitSet is the effective limits of an organisation.
func (s *Service) limitSet(ctx context.Context, tx pgx.Tx, org string) (limits.Set, error) {
	set := limits.Defaults()
	if s.o.Limits != nil {
		var err error
		if set, err = s.o.Limits(ctx, tx, org); err != nil {
			return limits.Set{}, fmt.Errorf("releasereq: %w", err)
		}
	}
	return set, nil
}

func (s *Service) concurrency(ctx context.Context, tx pgx.Tx, org string) (int64, error) {
	set, err := s.limitSet(ctx, tx, org)
	if err != nil {
		return 0, err
	}
	return set.Get(limits.RenderVisualDiffJobsPerOrganization), nil
}

// RunRender runs one queued job. At most render.visualDiffJobsPerOrganization
// of an organisation's jobs run at once; past that the job stays queued and
// ErrDeferred is returned. The renderer is called outside any transaction and
// is given no more than RenderInput.
func (s *Service) RunRender(ctx context.Context, job RenderJob) error {
	if s.o.Renderer == nil {
		return errors.New("releasereq: this process has no renderer")
	}
	in, started, err := s.startRender(ctx, job)
	if err != nil || !started {
		return err
	}
	if err := s.loadSources(ctx, job, &in); err != nil {
		return s.requeueAfter(ctx, job, "the sources could not be read", err)
	}
	screens, err := s.o.Renderer.Render(ctx, in)
	if err != nil {
		return s.requeueAfter(ctx, job, "the renderer failed", fmt.Errorf("releasereq: render: %w", err))
	}
	results, err := s.storeScreens(ctx, job.OrganizationID, screens)
	if err != nil {
		return err
	}
	return s.system(ctx, job.OrganizationID, func(ctx context.Context, tx pgx.Tx) error {
		return finishRender(ctx, tx, job.JobID, DiffSucceeded, results, "")
	})
}

// requeueAfter puts a started job back in the queue after it failed with
// cause, and returns the cause, so that the queue retries it later.
func (s *Service) requeueAfter(ctx context.Context, job RenderJob, reason string, cause error) error {
	if err := s.system(ctx, job.OrganizationID, func(ctx context.Context, tx pgx.Tx) error {
		return requeue(ctx, tx, job.JobID, reason)
	}); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

// loadSources reads the two projects of the job from the service's Sources,
// in a transaction of their own so that the claim of the job stays short.
func (s *Service) loadSources(ctx context.Context, job RenderJob, in *RenderInput) error {
	if s.o.Sources == nil {
		return nil
	}
	built := make(map[string]int64, len(in.Plugins))
	for _, p := range in.Plugins {
		built[p.PluginKey] = p.Version
	}
	return s.system(ctx, job.OrganizationID, func(ctx context.Context, tx pgx.Tx) (err error) {
		in.Before, in.After, err = s.o.Sources.Projects(ctx, tx, in.AppID, in.EnvironmentID, in.Channel, built)
		return err //nolint:wrapcheck // a domain error
	})
}

func requeue(ctx context.Context, tx pgx.Tx, jobID, reason string) error {
	id, err := storage.UUID(jobID)
	if err != nil {
		return fmt.Errorf("releasereq: %w", err)
	}
	if err := dbgen.New(tx).QueueVisualDiffJob(ctx, dbgen.QueueVisualDiffJobParams{ID: id, Error: reason}); err != nil {
		return failure(err, "visual diff")
	}
	return nil
}

// startRender claims a queued job within the organisation's limit and builds
// the renderer's input. started is false when there is nothing to do: the job
// ran already.
func (s *Service) startRender(ctx context.Context, job RenderJob) (in RenderInput, started bool, err error) {
	id, err := storage.UUID(job.JobID)
	if err != nil {
		return in, false, fmt.Errorf("releasereq: %w", err)
	}
	err = s.system(ctx, job.OrganizationID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := q.LockVisualDiffOrganization(ctx, job.OrganizationID); err != nil {
			return failure(err, "visual diff")
		}
		row, err := q.GetVisualDiffJob(ctx, id)
		if err != nil {
			return failure(err, "visual diff")
		}
		if row.State != DiffQueued {
			return nil
		}
		if err := s.withinLimit(ctx, tx, row); err != nil {
			return err
		}
		set, err := s.limitSet(ctx, tx, job.OrganizationID)
		if err != nil {
			return err
		}
		if in, err = renderInput(ctx, q, row, set); err != nil {
			return err
		}
		if err := q.StartVisualDiffJob(ctx, id); err != nil {
			return failure(err, "visual diff")
		}
		started = true
		return nil
	})
	return in, started, err
}

// withinLimit answers ErrDeferred when the organisation already runs as many
// render jobs as render.visualDiffJobsPerOrganization allows.
func (s *Service) withinLimit(ctx context.Context, tx pgx.Tx, row dbgen.VisualDiffJob) error {
	limit, err := s.concurrency(ctx, tx, storage.ID(row.OrganizationID))
	if err != nil {
		return err
	}
	running, err := dbgen.New(tx).CountRunningVisualDiffJobs(ctx, row.OrganizationID)
	if err != nil {
		return failure(err, "visual diff")
	}
	if running >= limit {
		return ErrDeferred
	}
	return nil
}

// renderInput is what the renderer gets for a job: the versions the request
// built and the standard devices.
func renderInput(ctx context.Context, q *dbgen.Queries, row dbgen.VisualDiffJob, set limits.Set) (RenderInput, error) {
	req, err := q.GetReleaseRequestByID(ctx, row.ReleaseRequestID)
	if err != nil {
		return RenderInput{}, failure(err, "release request")
	}
	r, err := requestOf(req)
	if err != nil {
		return RenderInput{}, err
	}
	frames, err := plxgolden.DefaultFrames()
	if err != nil {
		return RenderInput{}, fmt.Errorf("releasereq: %w", err)
	}
	in := RenderInput{
		JobID: storage.ID(row.ID), ContentHash: row.ContentHash, Limits: set,
		AppID: r.AppID, EnvironmentID: r.EnvironmentID, Channel: r.Channel,
	}
	for _, b := range r.Builds {
		in.Plugins = append(in.Plugins, RenderPlugin{PluginKey: b.PluginKey, Version: b.Version, BundleSHA256: b.BundleSHA256})
	}
	for _, f := range frames.Frames {
		in.Devices = append(in.Devices, f.ID)
	}
	return in, nil
}

// imageKey is where a rendered image lives: by the digest of the organisation
// and the image's hash, so storing it twice is a no-op.
func imageKey(org string, sum []byte) (string, error) {
	h := sha256.New()
	h.Write([]byte("visual-diff"))
	h.Write([]byte{0})
	h.Write([]byte(org))
	h.Write([]byte{0})
	h.Write(sum)
	key, err := objects.Key(objects.KindGolden, hex.EncodeToString(h.Sum(nil)))
	if err != nil {
		return "", fmt.Errorf("releasereq: %w", err)
	}
	return key, nil
}

// storeScreens puts the rendered images in object storage and returns the
// hashes the result keeps.
func (s *Service) storeScreens(ctx context.Context, org string, screens []Screen) ([]ScreenResult, error) {
	out := make([]ScreenResult, 0, len(screens))
	put := func(img []byte) (string, error) {
		if len(img) == 0 {
			return "", nil
		}
		sum := sha256.Sum256(img)
		if s.o.Objects != nil {
			key, err := imageKey(org, sum[:])
			if err != nil {
				return "", err
			}
			if _, err := s.o.Objects.Put(ctx, key, img, "image/png"); err != nil {
				return "", fmt.Errorf("releasereq: store image: %w", err)
			}
		}
		return hex.EncodeToString(sum[:]), nil
	}
	for _, sc := range screens {
		before, err := put(sc.Before)
		if err != nil {
			return nil, err
		}
		after, err := put(sc.After)
		if err != nil {
			return nil, err
		}
		out = append(out, ScreenResult{PluginKey: sc.PluginKey, Screen: sc.Screen, Device: sc.Device, BeforeSHA256: before, AfterSHA256: after, Changed: before != after})
	}
	return out, nil
}

// finishRender stores the outcome of a job. A job that succeeded moves its
// request on from the diffs step when the request still has the content the
// job rendered (F11).
func finishRender(ctx context.Context, tx pgx.Tx, jobID, state string, results []ScreenResult, reason string) error {
	id, err := storage.UUID(jobID)
	if err != nil {
		return fmt.Errorf("releasereq: %w", err)
	}
	q := dbgen.New(tx)
	job, err := q.GetVisualDiffJobForUpdate(ctx, id)
	if err != nil {
		return failure(err, "visual diff")
	}
	raw, err := json.Marshal(nonNil(results))
	if err != nil {
		return fmt.Errorf("releasereq: %w", err)
	}
	if err := q.FinishVisualDiffJob(ctx, dbgen.FinishVisualDiffJobParams{ID: id, State: state, Result: raw, Error: reason}); err != nil {
		return failure(err, "visual diff")
	}
	if state != DiffSucceeded {
		return nil
	}
	return advanceAfterDiff(ctx, q, job)
}

func advanceAfterDiff(ctx context.Context, q *dbgen.Queries, job dbgen.VisualDiffJob) error {
	row, err := q.GetReleaseRequestByIDForUpdate(ctx, job.ReleaseRequestID)
	if err != nil {
		return failure(err, "release request")
	}
	r, err := requestOf(row)
	if err != nil {
		return err
	}
	if r.State != StateDiffs || r.Hold != HoldNone || r.ContentHash != job.ContentHash {
		return nil
	}
	r.moveOn()
	params, err := updateParams(r)
	if err != nil {
		return err
	}
	if _, err := q.UpdateReleaseRequest(ctx, params); err != nil {
		return failure(err, "release request")
	}
	return nil
}

// FailRender ends a job that gave up after its last attempt, so that none
// stays queued or running for ever.
func (s *Service) FailRender(ctx context.Context, job RenderJob, attempts int) error {
	return s.system(ctx, job.OrganizationID, func(ctx context.Context, tx pgx.Tx) error {
		return finishRender(ctx, tx, job.JobID, DiffFailed, nil, fmt.Sprintf("the renderer failed after %d attempts", attempts))
	})
}

// Image returns one PNG of the request's current visual diff by its SHA-256,
// for the thumbnails of the diffs step. The hash must be one the newest diff
// of the request's content names, so that a hash of another request, app or
// organisation is not found: the request is read under row-level security,
// and the image's key is derived from the caller's organisation. It needs
// plugin.read, like the diff itself.
func (s *Service) Image(ctx context.Context, p auth.Principal, appID, id, sha256Hex string) ([]byte, error) {
	if err := authorize(p, auth.PluginRead, appID); err != nil {
		return nil, err
	}
	sum, err := hex.DecodeString(sha256Hex)
	if err != nil || len(sum) != sha256.Size || sha256Hex != hex.EncodeToString(sum) {
		return nil, plxerr.New(plxerr.InvalidFormat, "the image hash is not a lower-case SHA-256 in hex")
	}
	diff, found, err := s.Diff(ctx, p, appID, id)
	if err != nil {
		return nil, err
	}
	if !found || !diff.names(sha256Hex) {
		return nil, plxerr.New(plxerr.ResourceNotFound, "no such image in this visual diff")
	}
	if s.o.Objects == nil {
		return nil, plxerr.New(plxerr.ResourceNotFound, "no such image in this visual diff")
	}
	key, err := imageKey(p.OrganizationID, sum)
	if err != nil {
		return nil, err
	}
	png, _, err := s.o.Objects.Get(ctx, key)
	if errors.Is(err, objects.ErrNotFound) {
		return nil, plxerr.New(plxerr.ResourceNotFound, "no such image in this visual diff")
	}
	if err != nil {
		return nil, fmt.Errorf("releasereq: read image: %w", err)
	}
	if def, ok := limits.Lookup(limits.GoldenImageBytes); ok && int64(len(png)) > def.Max {
		return nil, fmt.Errorf("releasereq: the stored image has %d bytes; golden.imageBytes allows at most %d", len(png), def.Max)
	}
	return png, nil
}

// names reports whether the diff has a screen with the image of that hash.
func (d VisualDiff) names(sha256Hex string) bool {
	return slices.ContainsFunc(d.Screens, func(s ScreenResult) bool { return s.BeforeSHA256 == sha256Hex || s.AfterSHA256 == sha256Hex })
}
