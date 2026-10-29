// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package releasereq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Job states of a publish job, as Builds reports them.
const (
	jobSucceeded = "succeeded"
	jobFailed    = "failed"
	jobCancelled = "cancelled"
)

// CreateInput is what opens a request.
type CreateInput struct {
	AppID         string
	EnvironmentID string
	// Channel defaults to DefaultChannel.
	Channel string
	Plugins []Selected
}

// createKeys are the parsed identifiers of a request to open.
type createKeys struct {
	key     openKey
	org, id pgtype.UUID
}

// parseCreate validates the identifiers of an input and generates the new
// request's.
func (s *Service) parseCreate(p auth.Principal, in CreateInput) (createKeys, error) {
	channel := in.Channel
	if channel == "" {
		channel = DefaultChannel
	}
	if !channelForm.MatchString(channel) {
		return createKeys{}, plxerr.New(plxerr.InvalidFormat, "the channel key is not valid")
	}
	app, err := storage.UUID(in.AppID)
	if err != nil {
		return createKeys{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	env, err := storage.UUID(in.EnvironmentID)
	if err != nil {
		return createKeys{}, plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
	}
	org, err := storage.UUID(p.OrganizationID)
	if err != nil {
		return createKeys{}, fmt.Errorf("releasereq: %w", err)
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return createKeys{}, fmt.Errorf("releasereq: %w", err)
	}
	rid, err := storage.UUID(id)
	if err != nil {
		return createKeys{}, fmt.Errorf("releasereq: %w", err)
	}
	return createKeys{key: openKey{app: app, env: env, channel: channel}, org: org, id: rid}, nil
}

// Create opens a request at the select step. When the app, environment and
// channel already have an open request, that one is returned and existing is
// true (F11). It needs release.publish.
func (s *Service) Create(ctx context.Context, p auth.Principal, in CreateInput) (r Request, existing bool, err error) {
	if err := authorize(p, auth.ReleasePublish, in.AppID); err != nil {
		return Request{}, false, err
	}
	k, err := s.parseCreate(p, in)
	if err != nil {
		return Request{}, false, err
	}
	app, env, channel := k.key.app, k.key.env, k.key.channel
	rid, org := k.id, k.org
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := checkEnvironment(ctx, q, app, env); err != nil {
			return err
		}
		plugins, err := checkSelection(ctx, q, app, in.Plugins)
		if err != nil {
			return err
		}
		if r, existing, err = findOpen(ctx, q, k.key); existing || err != nil {
			return err
		}
		raw, err := json.Marshal(plugins)
		if err != nil {
			return fmt.Errorf("releasereq: %w", err)
		}
		row, err := q.InsertReleaseRequest(ctx, dbgen.InsertReleaseRequestParams{
			ID: rid, OrganizationID: org, AppID: app, EnvironmentID: env, Channel: channel, Plugins: raw,
			CreatedBy: p.Actor().ID, CreatedByName: p.Actor().Display,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// Another call opened it between the read and the insert.
			r, existing, err = findOpen(ctx, q, k.key)
			if err == nil && !existing {
				err = failure(pgx.ErrNoRows, "release request")
			}
			return err
		}
		if err != nil {
			return failure(err, "release request")
		}
		r, err = requestOf(row)
		return err
	})
	return r, existing, err
}

// openKey names the slot a request holds: one open request per app,
// environment and channel.
type openKey struct {
	app, env pgtype.UUID
	channel  string
}

// findOpen returns the open request of a slot, if there is one.
func findOpen(ctx context.Context, q *dbgen.Queries, k openKey) (Request, bool, error) {
	row, err := q.GetOpenReleaseRequest(ctx, dbgen.GetOpenReleaseRequestParams{AppID: k.app, EnvironmentID: k.env, Channel: k.channel})
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, false, nil
	}
	if err != nil {
		return Request{}, false, failure(err, "release request")
	}
	r, err := requestOf(row)
	return r, err == nil, err
}

// checkEnvironment verifies that the environment belongs to the app. Row-level
// security hides an app or environment of another organisation, so a foreign
// identifier is not found.
func checkEnvironment(ctx context.Context, q *dbgen.Queries, app, env pgtype.UUID) error {
	if _, err := q.GetApp(ctx, app); err != nil {
		return failure(err, "app")
	}
	e, err := q.GetEnvironment(ctx, env)
	if err != nil {
		return failure(err, "environment")
	}
	if e.AppID != app {
		return plxerr.New(plxerr.ResourceNotFound, "no such environment")
	}
	return nil
}

// checkSelection validates a selection against the app's plugins and returns
// it sorted by key.
func checkSelection(ctx context.Context, q *dbgen.Queries, app pgtype.UUID, in []Selected) ([]Selected, error) {
	if len(in) == 0 {
		return nil, plxerr.New(plxerr.InvalidFormat, "a release request needs at least one plugin")
	}
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b Selected) int { return strings.Compare(a.PluginKey, b.PluginKey) })
	for i, sel := range out {
		if i > 0 && out[i-1].PluginKey == sel.PluginKey {
			return nil, plxerr.New(plxerr.InvalidFormat, "the plugin %q is selected twice", sel.PluginKey)
		}
		if sel.Reason != "changed" && sel.Reason != "dependency" {
			return nil, plxerr.New(plxerr.InvalidFormat, "the reason of plugin %q must be changed or dependency", sel.PluginKey)
		}
		if _, err := q.GetPluginByKey(ctx, dbgen.GetPluginByKeyParams{AppID: app, Key: sel.PluginKey}); err != nil {
			return nil, failure(err, "plugin")
		}
	}
	return out, nil
}

// Get returns a request. It needs plugin.read.
func (s *Service) Get(ctx context.Context, p auth.Principal, appID, id string) (Request, error) {
	if err := authorize(p, auth.PluginRead, appID); err != nil {
		return Request{}, err
	}
	app, req, err := ids(appID, id)
	if err != nil {
		return Request{}, err
	}
	var out Request
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetReleaseRequest(ctx, dbgen.GetReleaseRequestParams{ID: req, AppID: app})
		if err != nil {
			return failure(err, "release request")
		}
		out, err = requestOf(row)
		return err
	})
	return out, err
}

// List lists an app's requests, newest first; before is the request to
// continue before. It needs plugin.read.
func (s *Service) List(ctx context.Context, p auth.Principal, appID string, openOnly bool, before storage.Cursor, size int32) ([]Request, error) {
	if err := authorize(p, auth.PluginRead, appID); err != nil {
		return nil, err
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	params := dbgen.ListReleaseRequestsParams{AppID: app, OpenOnly: openOnly, PageSize: size}
	if before.ID != "" {
		if params.BeforeID, err = storage.UUID(before.ID); err != nil {
			return nil, plxerr.New(plxerr.InvalidPageToken, "the page token is not valid")
		}
	}
	var out []Request
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListReleaseRequests(ctx, params)
		if err != nil {
			return failure(err, "release requests")
		}
		for _, row := range rows {
			r, err := requestOf(row)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return nil
	})
	return out, err
}

// Step is the payload of a call that completes a step. Kind names the step;
// the field of that step is the one read.
type Step struct {
	Kind      State
	Selection []Selected
	Builds    []BuildRef
	// AcknowledgedWarnings are the warnings the user acknowledged.
	AcknowledgedWarnings []string
	Notes, Label         string
	EnvironmentID        string
	Channel              string
	Plan                 []byte
}

// BuildRef is a publish job a request is to freeze.
type BuildRef struct{ PluginKey, JobID string }

// Advance completes the step the request is at with that step's payload and
// moves it on. Completing a step the request has already passed returns the
// request unchanged, so a repeated call is safe (SRV-005). It needs
// release.publish.
func (s *Service) Advance(ctx context.Context, p auth.Principal, appID, id string, step Step) (Request, error) {
	if err := authorize(p, auth.ReleasePublish, appID); err != nil {
		return Request{}, err
	}
	return s.update(ctx, p, appID, id, func(ctx context.Context, tx pgx.Tx, r *Request) error {
		switch {
		case !r.State.Open():
			return ended(*r)
		case r.State.Passed(step.Kind):
			return nil
		case r.State != step.Kind:
			return plxerr.New(plxerr.ReleaseRequestState, "the request is at the %s step, not the %s step", r.State, step.Kind)
		}
		q := dbgen.New(tx)
		app, _ := storage.UUID(r.AppID)
		var err error
		switch step.Kind {
		case StateSelect:
			err = r.selectPlugins(ctx, q, app, step)
		case StateBuild:
			err = s.freeze(ctx, p, r, step)
		case StateReports:
			err = r.acknowledge(step)
		case StateDiffs:
			err = diffDone(ctx, q, r)
		case StateChangelog:
			r.ChangelogNotes, r.Label = step.Notes, step.Label
			r.moveOn()
		case StateTarget:
			err = r.retarget(ctx, q, app, step)
		case StateRollout:
			err = r.plan(step)
		default:
			return plxerr.New(plxerr.ReleaseRequestState, "the %s step is not completed with this call", step.Kind)
		}
		return err
	})
}

// moveOn goes to the next step and clears the hold.
func (r *Request) moveOn() {
	if next, ok := r.State.Next(); ok {
		r.State = next
	}
	r.Hold, r.HoldReason = HoldNone, ""
}

func (r *Request) selectPlugins(ctx context.Context, q *dbgen.Queries, app pgtype.UUID, step Step) error {
	plugins, err := checkSelection(ctx, q, app, step.Selection)
	if err != nil {
		return err
	}
	r.Plugins = plugins
	r.moveOn()
	return nil
}

// freeze reads the publish jobs the request froze. All succeeded: the content
// hash is set and the request moves to the reports. One failed or was
// cancelled: the request is held as failed, and a retry with new jobs clears
// the hold. Some still running: the request stays, with what was seen. A
// changed content hash voids an approval (GOV-022).
func (s *Service) freeze(ctx context.Context, p auth.Principal, r *Request, step Step) error {
	if s.o.Builds == nil {
		return plxerr.New(plxerr.PreconditionFailed, "this installation cannot read publish jobs")
	}
	want := make([]string, 0, len(r.Plugins))
	for _, sel := range r.Plugins {
		want = append(want, sel.PluginKey)
	}
	got := make([]string, 0, len(step.Builds))
	for _, b := range step.Builds {
		got = append(got, b.PluginKey)
	}
	slices.Sort(got)
	if !slices.Equal(want, got) {
		return plxerr.New(plxerr.InvalidFormat, "the builds must be one publish job for each selected plugin")
	}
	builds := make([]Build, 0, len(step.Builds))
	for _, ref := range step.Builds {
		b, err := s.o.Builds.Build(ctx, p, r.AppID, ref.JobID)
		if err != nil {
			return err //nolint:wrapcheck // a domain error
		}
		if b.State == jobSucceeded && b.PluginKey != ref.PluginKey {
			return plxerr.New(plxerr.InvalidFormat, "the publish job %s did not build plugin %q", ref.JobID, ref.PluginKey)
		}
		b.PluginKey, b.JobID = ref.PluginKey, ref.JobID
		builds = append(builds, b)
	}
	slices.SortFunc(builds, func(a, b Build) int { return strings.Compare(a.PluginKey, b.PluginKey) })
	r.Builds = builds
	failed, running := false, false
	for _, b := range builds {
		switch b.State {
		case jobSucceeded:
		case jobFailed, jobCancelled:
			failed = true
		default:
			running = true
		}
	}
	switch {
	case failed:
		r.Hold, r.HoldReason = HoldFailed, "a publish job failed or was cancelled"
	case running:
		r.Hold, r.HoldReason = HoldNone, ""
	default:
		hash := contentHash(builds)
		if r.ApprovedHash != "" && r.ApprovedHash != hash {
			r.voidApproval()
		}
		r.ContentHash = hash
		r.Acknowledged = nil
		r.moveOn()
	}
	return nil
}

// contentHash is the SHA-256 of the versions the builds produced, in key
// order; an approval is bound to it (GOV-022).
func contentHash(builds []Build) string {
	sorted := slices.Clone(builds)
	slices.SortFunc(sorted, func(a, b Build) int { return strings.Compare(a.PluginKey, b.PluginKey) })
	parts := make([][]byte, 0, 2*len(sorted))
	for _, b := range sorted {
		parts = append(parts, []byte(b.PluginKey), []byte(b.BundleSHA256))
	}
	return approval.HashOf(parts...)
}

// voidApproval forgets an approval, which named other content.
func (r *Request) voidApproval() {
	r.ApprovedHash, r.ApprovalRequestID = "", ""
}

// acknowledge completes the reports step: a build with errors holds the
// request as blocked; every warning needs an acknowledgement (SRV-051).
func (r *Request) acknowledge(step Step) error {
	for _, b := range r.Builds {
		if b.Errors > 0 {
			r.Hold, r.HoldReason = HoldBlocked, "the builds have errors; fix them and freeze again"
			return nil
		}
	}
	var missing []string
	for _, b := range r.Builds {
		for _, w := range b.Warnings {
			if !slices.Contains(step.AcknowledgedWarnings, w) && !slices.Contains(missing, w) {
				missing = append(missing, w)
			}
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return detailed(plxerr.WarningsNotAcknowledged, "warnings", strings.Join(missing, ","), "%d warning(s) are not acknowledged", len(missing))
	}
	r.Acknowledged = slices.Clone(step.AcknowledgedWarnings)
	slices.Sort(r.Acknowledged)
	r.moveOn()
	return nil
}

// retarget completes the target step. A change of environment or channel must
// not collide with another open request (F11).
func (r *Request) retarget(ctx context.Context, q *dbgen.Queries, app pgtype.UUID, step Step) error {
	env, channel := r.EnvironmentID, r.Channel
	if step.EnvironmentID != "" {
		env = step.EnvironmentID
	}
	if step.Channel != "" {
		channel = step.Channel
	}
	if !channelForm.MatchString(channel) {
		return plxerr.New(plxerr.InvalidFormat, "the channel key is not valid")
	}
	envID, err := storage.UUID(env)
	if err != nil {
		return plxerr.New(plxerr.InvalidFormat, "the environment identifier is not valid")
	}
	if err := checkEnvironment(ctx, q, app, envID); err != nil {
		return err
	}
	if env != r.EnvironmentID || channel != r.Channel {
		open, err := q.GetOpenReleaseRequest(ctx, dbgen.GetOpenReleaseRequestParams{AppID: app, EnvironmentID: envID, Channel: channel})
		if err == nil && storage.ID(open.ID) != r.ID {
			return detailed(plxerr.ResourceExists, "request", storage.ID(open.ID), "the app, environment and channel already have an open release request")
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return failure(err, "release request")
		}
	}
	r.EnvironmentID, r.Channel = env, channel
	r.moveOn()
	return nil
}

// plan completes the rollout step; the plan is a JSON object.
func (r *Request) plan(step Step) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(step.Plan, &obj); err != nil || obj == nil {
		return plxerr.New(plxerr.InvalidJSON, "the rollout plan must be a JSON object")
	}
	r.RolloutPlan = slices.Clone(step.Plan)
	r.moveOn()
	return nil
}

// Refreeze returns a request to the build step after the draft changed:
// builds, reports, diffs and any approval are void. It needs release.publish.
func (s *Service) Refreeze(ctx context.Context, p auth.Principal, appID, id string) (Request, error) {
	if err := authorize(p, auth.ReleasePublish, appID); err != nil {
		return Request{}, err
	}
	return s.update(ctx, p, appID, id, func(_ context.Context, _ pgx.Tx, r *Request) error {
		if !r.State.Open() {
			return ended(*r)
		}
		if r.State.position() < StateBuild.position() {
			return plxerr.New(plxerr.ReleaseRequestState, "nothing is frozen at the %s step", r.State)
		}
		r.State, r.Hold, r.HoldReason = StateBuild, HoldNone, ""
		r.Builds, r.Acknowledged, r.ContentHash = nil, nil, ""
		r.voidApproval()
		return nil
	})
}

// Cancel ends an open request; versions already built stay, being immutable.
// Cancelling a cancelled request is a replay. It needs release.publish.
func (s *Service) Cancel(ctx context.Context, p auth.Principal, appID, id string) (Request, error) {
	if err := authorize(p, auth.ReleasePublish, appID); err != nil {
		return Request{}, err
	}
	return s.update(ctx, p, appID, id, func(_ context.Context, _ pgx.Tx, r *Request) error {
		switch {
		case r.State == StateCancelled:
			return nil
		case !r.State.Open():
			return ended(*r)
		}
		r.State, r.Hold, r.HoldReason = StateCancelled, HoldNone, ""
		r.EndedAt = time.Now().UTC()
		return nil
	})
}

// detailed is a registered error that names one structured detail.
func detailed(code plxerr.Code, key, value, format string, args ...any) error {
	return (&plxerr.Error{Code: code, Message: fmt.Sprintf(format, args...)}).WithDetail(key, value)
}
