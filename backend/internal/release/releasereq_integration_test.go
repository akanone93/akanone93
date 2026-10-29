// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/releasereq"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// fakeBuilds answers publish jobs from a table.
type fakeBuilds map[string]releasereq.Build

func (b fakeBuilds) Build(_ context.Context, _ auth.Principal, _, jobID string) (releasereq.Build, error) {
	if out, ok := b[jobID]; ok {
		return out, nil
	}
	return releasereq.Build{}, plxerr.New(plxerr.ResourceNotFound, "no such publish job")
}

// fakeQueue records the render jobs.
type fakeQueue struct {
	mu   sync.Mutex
	jobs []releasereq.RenderJob
}

func (q *fakeQueue) Enqueue(_ context.Context, _ pgx.Tx, job releasereq.RenderJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, job)
	return nil
}

func (q *fakeQueue) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.jobs)
}

// fakeRenderer returns one changed screen; it waits on hold when it is set.
type fakeRenderer struct {
	hold    chan struct{}
	started chan struct{}
	inputs  []releasereq.RenderInput
	mu      sync.Mutex
}

func (r *fakeRenderer) Render(_ context.Context, in releasereq.RenderInput) ([]releasereq.Screen, error) {
	r.mu.Lock()
	r.inputs = append(r.inputs, in)
	r.mu.Unlock()
	if r.started != nil {
		r.started <- struct{}{}
	}
	if r.hold != nil {
		<-r.hold
	}
	return []releasereq.Screen{{PluginKey: "loans", Screen: "home", Device: in.Devices[0], Before: []byte("old"), After: []byte("new")}}, nil
}

// fakeSources hands out two projects, or fails.
type fakeSources struct {
	mu    sync.Mutex
	err   error
	calls []map[string]int64
}

func (s *fakeSources) Projects(_ context.Context, _ pgx.Tx, _, _, _ string, built map[string]int64) (before, after fs.FS, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, built)
	if s.err != nil {
		return nil, nil, s.err
	}
	return fstest.MapFS{"app.json": {Data: []byte(`{"v":1}`)}}, fstest.MapFS{"app.json": {Data: []byte(`{"v":2}`)}}, nil
}

// fakeFunctions hands out a function catalogue, or fails, and records what it
// was asked for.
type fakeFunctions struct {
	mu    sync.Mutex
	err   error
	asked [][3]string
}

func (f *fakeFunctions) Catalogue(_ context.Context, organizationID, appID, environmentID string) (compiler.FunctionCatalogue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, [3]string{organizationID, appID, environmentID})
	if f.err != nil {
		return nil, f.err
	}
	return compiler.NewFunctionSet([]compiler.FunctionEntry{{Name: "loans.quote", Info: compiler.FunctionInfo{Placement: schema.PlacementServer, Version: 3}}}, true), nil
}

// fakeReleaser records the releases it makes.
type fakeReleaser struct {
	created, promoted int
}

func (r *fakeReleaser) Create(context.Context, auth.Principal, string, string, map[string]int64, string) (int64, error) {
	r.created++
	return 7, nil
}

func (r *fakeReleaser) Promote(context.Context, auth.Principal, string, int64, string, string) error {
	r.promoted++
	return nil
}

// stubGate refuses until it is told the release is approved, like the
// approval engine does for a policy that matches.
type stubGate struct {
	approved bool
	request  string
	calls    int
	hashes   []string
}

func (g *stubGate) Require(_ context.Context, _ pgx.Tx, arts ...approval.Artifact) error {
	g.calls++
	g.hashes = append(g.hashes, arts[0].Hash)
	if g.approved {
		return nil
	}
	return (&plxerr.Error{Code: plxerr.ApprovalRequired, Message: "needs approval"}).WithDetail("request", g.request)
}

type reqKit struct {
	svc      *releasereq.Service
	builds   fakeBuilds
	queue    *fakeQueue
	renderer *fakeRenderer
	releaser *fakeReleaser
	gate     *stubGate
	sources  *fakeSources
	// functions is the catalogue source of the render input.
	functions *fakeFunctions
}

func (f *fixture) requests(t *testing.T, limit int64) *reqKit {
	t.Helper()
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	approvalID, err := gen.New()
	if err != nil {
		t.Fatal(err)
	}
	k := &reqKit{builds: fakeBuilds{}, queue: &fakeQueue{}, renderer: &fakeRenderer{}, releaser: &fakeReleaser{}, gate: &stubGate{request: approvalID}, sources: &fakeSources{}, functions: &fakeFunctions{}}
	set, err := limits.Defaults().Tighten(limits.RenderVisualDiffJobsPerOrganization, limits.ScopeInstallation, limit)
	if err != nil {
		t.Fatal(err)
	}
	k.svc = releasereq.New(releasereq.Options{
		DB: f.db, IDs: gen, Builds: k.builds, Jobs: k.queue, Objects: f.store, Releaser: k.releaser, Approvals: k.gate, Renderer: k.renderer, Sources: k.sources, Functions: k.functions,
		Limits: func(context.Context, pgx.Tx, string) (limits.Set, error) { return set, nil },
	})
	return k
}

func codeIs(err error, code plxerr.Code) bool {
	return err != nil && errors.Is(err, plxerr.New(code, ""))
}

// Verifies: STU-100, GOV-022, GOV-023, SRV-005, SRV-051.
// A release request walks the nine steps of the publish flow with every
// result kept on the server: one open request per app and channel, repeated
// calls are replays, the visual diff gates the approval, the approval is bound
// to the content hash and a changed hash voids it, and the request ends
// Released with the release the release service made.
func TestReleaseRequestFlow_STU_100(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	k := f.requests(t, 2)
	ctx := context.Background()
	dev := f.envs["development"].ID
	app := f.app

	r, existing, err := k.svc.Create(ctx, f.owner, releasereq.CreateInput{AppID: app, EnvironmentID: dev, Plugins: []releasereq.Selected{{PluginKey: "loans", Reason: "changed"}}})
	if err != nil || existing || r.State != releasereq.StateSelect || r.Channel != "production" {
		t.Fatalf("Create = %+v, %v, %v", r, existing, err)
	}
	again, existing, err := k.svc.Create(ctx, f.owner, releasereq.CreateInput{AppID: app, EnvironmentID: dev, Plugins: []releasereq.Selected{{PluginKey: "loans", Reason: "changed"}}})
	if err != nil || !existing || again.ID != r.ID {
		t.Fatalf("a second Create = %+v, %v, %v; want the open request", again, existing, err)
	}
	if _, _, err := k.svc.Create(ctx, f.viewer, releasereq.CreateInput{AppID: app, EnvironmentID: dev, Channel: "beta", Plugins: []releasereq.Selected{{PluginKey: "loans", Reason: "changed"}}}); err == nil {
		t.Error("a viewer opened a release request")
	}
	if _, _, err := k.svc.Create(ctx, f.owner, releasereq.CreateInput{AppID: app, EnvironmentID: dev, Channel: "beta", Plugins: []releasereq.Selected{{PluginKey: "nope", Reason: "changed"}}}); err == nil {
		t.Error("a plugin the app does not have was selected")
	}

	// select -> build; repeating the call is a replay.
	sel := releasereq.Step{Kind: releasereq.StateSelect, Selection: []releasereq.Selected{{PluginKey: "loans", Reason: "changed"}}}
	if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, sel); err != nil || r.State != releasereq.StateBuild {
		t.Fatalf("select = %+v, %v", r, err)
	}
	revision := r.Revision
	if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, sel); err != nil || r.State != releasereq.StateBuild || r.Revision != revision {
		t.Fatalf("a replayed select = %+v, %v", r, err)
	}
	if _, err := k.svc.Advance(ctx, f.owner, app, r.ID, releasereq.Step{Kind: releasereq.StateChangelog}); !codeIs(err, plxerr.ReleaseRequestState) {
		t.Errorf("a step the request has not reached: %v", err)
	}

	// build: running stays, failed holds, succeeded moves on with the hash.
	build := func(job string) releasereq.Step {
		return releasereq.Step{Kind: releasereq.StateBuild, Builds: []releasereq.BuildRef{{PluginKey: "loans", JobID: job}}}
	}
	k.builds["j1"] = releasereq.Build{State: "running"}
	if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, build("j1")); err != nil || r.State != releasereq.StateBuild || r.Hold != releasereq.HoldNone {
		t.Fatalf("running build = %+v, %v", r, err)
	}
	k.builds["j1"] = releasereq.Build{State: "failed"}
	if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, build("j1")); err != nil || r.Hold != releasereq.HoldFailed {
		t.Fatalf("failed build = %+v, %v", r, err)
	}
	k.builds["j2"] = releasereq.Build{State: "succeeded", PluginKey: "loans", Version: 3, BundleSHA256: "aa", Warnings: []string{"w1"}}
	if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, build("j2")); err != nil || r.State != releasereq.StateReports || r.Hold != releasereq.HoldNone || r.ContentHash == "" {
		t.Fatalf("built = %+v, %v", r, err)
	}
	firstHash := r.ContentHash

	// reports: the warning needs acknowledging.
	if _, err := k.svc.Advance(ctx, f.owner, app, r.ID, releasereq.Step{Kind: releasereq.StateReports}); !codeIs(err, plxerr.WarningsNotAcknowledged) {
		t.Errorf("an unacknowledged warning: %v", err)
	}
	if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, releasereq.Step{Kind: releasereq.StateReports, AcknowledgedWarnings: []string{"w1"}}); err != nil || r.State != releasereq.StateDiffs {
		t.Fatalf("reports = %+v, %v", r, err)
	}

	// diffs: the step needs a finished render; asking twice queues one job.
	if _, err := k.svc.Advance(ctx, f.owner, app, r.ID, releasereq.Step{Kind: releasereq.StateDiffs}); !codeIs(err, plxerr.VisualDiffRequired) {
		t.Errorf("diffs without a render: %v", err)
	}
	job, err := k.svc.RequestDiff(ctx, f.owner, app, r.ID)
	if err != nil || job.State != releasereq.DiffQueued || k.queue.count() != 1 {
		t.Fatalf("RequestDiff = %+v, %v (%d queued)", job, err, k.queue.count())
	}
	if again, err := k.svc.RequestDiff(ctx, f.owner, app, r.ID); err != nil || again.ID != job.ID || k.queue.count() != 1 {
		t.Fatalf("a second RequestDiff = %+v, %v (%d queued)", again, err, k.queue.count())
	}
	if err := k.svc.RunRender(ctx, k.queue.jobs[0]); err != nil {
		t.Fatalf("RunRender: %v", err)
	}
	if err := k.svc.RunRender(ctx, k.queue.jobs[0]); err != nil {
		t.Fatalf("a repeated RunRender: %v", err)
	}
	if len(k.renderer.inputs) != 1 || len(k.renderer.inputs[0].Plugins) != 1 || k.renderer.inputs[0].Plugins[0].BundleSHA256 != "aa" {
		t.Fatalf("renderer inputs = %+v", k.renderer.inputs)
	}
	diff, found, err := k.svc.Diff(ctx, f.owner, app, r.ID)
	if err != nil || !found || diff.State != releasereq.DiffSucceeded || len(diff.Screens) != 1 || !diff.Screens[0].Changed {
		t.Fatalf("Diff = %+v, %v, %v", diff, found, err)
	}
	if r, err = k.svc.Get(ctx, f.owner, app, r.ID); err != nil || r.State != releasereq.StateChangelog {
		t.Fatalf("the finished diff did not move the request on: %+v, %v", r, err)
	}

	// changelog, target, then approval is held until the policy is satisfied.
	if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, releasereq.Step{Kind: releasereq.StateChangelog, Notes: "n", Label: "l"}); err != nil || r.State != releasereq.StateTarget {
		t.Fatalf("changelog = %+v, %v", r, err)
	}
	if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, releasereq.Step{Kind: releasereq.StateTarget}); err != nil || r.State != releasereq.StateApproval {
		t.Fatalf("target = %+v, %v", r, err)
	}
	if r, err = k.svc.RequestApproval(ctx, f.owner, app, r.ID); err != nil || r.State != releasereq.StateApproval || r.Hold != releasereq.HoldAwaiting || r.ApprovalRequestID != k.gate.request {
		t.Fatalf("approval needed = %+v, %v", r, err)
	}
	k.gate.approved = true
	if r, err = k.svc.RequestApproval(ctx, f.owner, app, r.ID); err != nil || r.State != releasereq.StateRollout || r.ApprovedHash != r.ContentHash {
		t.Fatalf("approved = %+v, %v", r, err)
	}
	calls := k.gate.calls
	if r, err = k.svc.RequestApproval(ctx, f.owner, app, r.ID); err != nil || r.State != releasereq.StateRollout || k.gate.calls != calls {
		t.Fatalf("a replayed approval = %+v, %v (%d calls)", r, err, k.gate.calls)
	}

	// New content voids the approval: back to build, and the engine is asked
	// about the new hash.
	if r, err = k.svc.Refreeze(ctx, f.owner, app, r.ID); err != nil || r.State != releasereq.StateBuild || r.ApprovedHash != "" || r.ContentHash != "" {
		t.Fatalf("Refreeze = %+v, %v", r, err)
	}
	k.builds["j3"] = releasereq.Build{State: "succeeded", PluginKey: "loans", Version: 4, BundleSHA256: "bb"}
	if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, build("j3")); err != nil || r.ContentHash == firstHash || r.State != releasereq.StateReports {
		t.Fatalf("rebuilt = %+v, %v", r, err)
	}
	if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, releasereq.Step{Kind: releasereq.StateReports}); err != nil || r.State != releasereq.StateDiffs {
		t.Fatalf("reports again = %+v, %v", r, err)
	}
	if _, err := k.svc.RequestApproval(ctx, f.owner, app, r.ID); err == nil {
		t.Error("approval was asked for before the request reached it")
	}
	if job, err = k.svc.RequestDiff(ctx, f.owner, app, r.ID); err != nil || k.queue.count() != 2 {
		t.Fatalf("a diff of the new content = %+v, %v (%d queued)", job, err, k.queue.count())
	}
	if err := k.svc.RunRender(ctx, k.queue.jobs[1]); err != nil {
		t.Fatal(err)
	}
	for _, step := range []releasereq.Step{{Kind: releasereq.StateChangelog}, {Kind: releasereq.StateTarget}} {
		if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, step); err != nil {
			t.Fatalf("%s: %v", step.Kind, err)
		}
	}
	if r, err = k.svc.RequestApproval(ctx, f.owner, app, r.ID); err != nil || r.State != releasereq.StateRollout {
		t.Fatalf("approval of the new content = %+v, %v", r, err)
	}
	if k.gate.hashes[len(k.gate.hashes)-1] == k.gate.hashes[0] {
		t.Error("the approval was asked for the same hash after the content changed")
	}

	// rollout, then release: the release is made once.
	if _, err := k.svc.Advance(ctx, f.owner, app, r.ID, releasereq.Step{Kind: releasereq.StateRollout, Plan: []byte("[]")}); err == nil {
		t.Error("a rollout plan that is not an object was accepted")
	}
	if r, err = k.svc.Advance(ctx, f.owner, app, r.ID, releasereq.Step{Kind: releasereq.StateRollout, Plan: []byte(`{"stages":[]}`)}); err != nil || r.State != releasereq.StateRelease {
		t.Fatalf("rollout = %+v, %v", r, err)
	}
	if _, err := k.svc.Complete(ctx, f.viewer, app, r.ID); err == nil {
		t.Error("a viewer completed a release request")
	}
	if r, err = k.svc.Complete(ctx, f.owner, app, r.ID); err != nil || r.State != releasereq.StateReleased || r.ReleaseSequence != 7 || r.EndedAt.IsZero() {
		t.Fatalf("Complete = %+v, %v", r, err)
	}
	if r, err = k.svc.Complete(ctx, f.owner, app, r.ID); err != nil || r.State != releasereq.StateReleased || k.releaser.created != 1 || k.releaser.promoted != 1 {
		t.Fatalf("a repeated Complete = %+v, %v (%d created, %d promoted)", r, err, k.releaser.created, k.releaser.promoted)
	}
	if _, err := k.svc.Advance(ctx, f.owner, app, r.ID, releasereq.Step{Kind: releasereq.StateChangelog}); !codeIs(err, plxerr.ReleaseRequestEnded) {
		t.Errorf("a step of an ended request: %v", err)
	}
	// The channel is free again once the request ended.
	if next, existing, err := k.svc.Create(ctx, f.owner, releasereq.CreateInput{AppID: app, EnvironmentID: dev, Plugins: []releasereq.Selected{{PluginKey: "loans", Reason: "changed"}}}); err != nil || existing || next.ID == r.ID {
		t.Errorf("Create after release = %+v, %v, %v", next, existing, err)
	}
}

// Verifies: STU-100, SRV-022.
// An organisation sees nothing of another's release requests: a foreign app
// and a foreign request are refused, and its list is empty.
func TestReleaseRequestsAreTenantScoped_STU_100(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	k := f.requests(t, 2)
	ctx := context.Background()
	dev := f.envs["development"].ID
	r, _, err := k.svc.Create(ctx, f.owner, releasereq.CreateInput{AppID: f.app, EnvironmentID: dev, Plugins: []releasereq.Selected{{PluginKey: "loans", Reason: "changed"}}})
	if err != nil {
		t.Fatal(err)
	}
	admin := auth.Identity{Kind: auth.KindUser, ID: f.owner.UserID, UserID: f.owner.UserID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	other, err := f.tenancy.CreateOrganization(ctx, admin, "other", "Other")
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := f.auth.Resolve(ctx, admin, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.Get(ctx, stranger, f.app, r.ID); err == nil {
		t.Error("another organisation read a release request")
	}
	if _, _, err := k.svc.Create(ctx, stranger, releasereq.CreateInput{AppID: f.app, EnvironmentID: dev, Channel: "beta", Plugins: []releasereq.Selected{{PluginKey: "loans", Reason: "changed"}}}); err == nil {
		t.Error("another organisation opened a request on this app")
	}
	for name, call := range map[string]func() error{
		"advance": func() error {
			_, err := k.svc.Advance(ctx, stranger, f.app, r.ID, releasereq.Step{Kind: releasereq.StateSelect})
			return err
		},
		"refreeze": func() error { _, err := k.svc.Refreeze(ctx, stranger, f.app, r.ID); return err },
		"cancel":   func() error { _, err := k.svc.Cancel(ctx, stranger, f.app, r.ID); return err },
		"diff":     func() error { _, err := k.svc.RequestDiff(ctx, stranger, f.app, r.ID); return err },
		"approval": func() error { _, err := k.svc.RequestApproval(ctx, stranger, f.app, r.ID); return err },
		"complete": func() error { _, err := k.svc.Complete(ctx, stranger, f.app, r.ID); return err },
	} {
		if call() == nil {
			t.Errorf("%s: another organisation was served", name)
		}
	}
	if seen, err := k.svc.List(ctx, stranger, f.app, false, storage.Cursor{}, 10); err != nil || len(seen) != 0 {
		t.Errorf("another organisation listed %d request(s): %v", len(seen), err)
	}
	if got, err := k.svc.Get(ctx, f.owner, f.app, r.ID); err != nil || got.State != releasereq.StateSelect || got.Revision != r.Revision {
		t.Errorf("the owner's request changed: %+v, %v", got, err)
	}
}

// Verifies: STU-100.
// A request cannot be moved onto a channel that has another open request, a
// cancelled request ends, and cancelling again is a replay.
func TestReleaseRequestTargetAndCancel_STU_100(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	k := f.requests(t, 2)
	ctx := context.Background()
	dev := f.envs["development"].ID
	sel := []releasereq.Selected{{PluginKey: "loans", Reason: "changed"}}
	prod, _, err := k.svc.Create(ctx, f.owner, releasereq.CreateInput{AppID: f.app, EnvironmentID: dev, Plugins: sel})
	if err != nil {
		t.Fatal(err)
	}
	beta, _, err := k.svc.Create(ctx, f.owner, releasereq.CreateInput{AppID: f.app, EnvironmentID: dev, Channel: "beta", Plugins: sel})
	if err != nil || beta.ID == prod.ID {
		t.Fatalf("a request on another channel = %+v, %v", beta, err)
	}
	// Bring beta to the target step by hand-made builds and skipped reports.
	k.builds["j"] = releasereq.Build{State: "succeeded", PluginKey: "loans", Version: 1, BundleSHA256: "aa"}
	steps := []releasereq.Step{
		{Kind: releasereq.StateSelect, Selection: sel},
		{Kind: releasereq.StateBuild, Builds: []releasereq.BuildRef{{PluginKey: "loans", JobID: "j"}}},
		{Kind: releasereq.StateReports},
	}
	for _, step := range steps {
		if _, err = k.svc.Advance(ctx, f.owner, f.app, beta.ID, step); err != nil {
			t.Fatalf("%s: %v", step.Kind, err)
		}
	}
	if _, err := k.svc.RequestDiff(ctx, f.owner, f.app, beta.ID); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.RunRender(ctx, k.queue.jobs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = k.svc.Advance(ctx, f.owner, f.app, beta.ID, releasereq.Step{Kind: releasereq.StateChangelog}); err != nil {
		t.Fatal(err)
	}
	_, err = k.svc.Advance(ctx, f.owner, f.app, beta.ID, releasereq.Step{Kind: releasereq.StateTarget, Channel: "production"})
	if !codeIs(err, plxerr.ResourceExists) {
		t.Fatalf("a channel with an open request: %v", err)
	}
	if c, err := k.svc.Cancel(ctx, f.owner, f.app, prod.ID); err != nil || c.State != releasereq.StateCancelled || c.EndedAt.IsZero() {
		t.Fatalf("Cancel = %+v, %v", c, err)
	}
	if c, err := k.svc.Cancel(ctx, f.owner, f.app, prod.ID); err != nil || c.State != releasereq.StateCancelled {
		t.Fatalf("a repeated Cancel = %+v, %v", c, err)
	}
	if _, err := k.svc.Refreeze(ctx, f.owner, f.app, prod.ID); !codeIs(err, plxerr.ReleaseRequestEnded) {
		t.Errorf("Refreeze of a cancelled request: %v", err)
	}
	if r, err := k.svc.Advance(ctx, f.owner, f.app, beta.ID, releasereq.Step{Kind: releasereq.StateTarget, Channel: "production"}); err != nil || r.Channel != "production" || r.State != releasereq.StateApproval {
		t.Fatalf("the freed channel: %+v, %v", r, err)
	}
	open, err := k.svc.List(ctx, f.owner, f.app, true, storage.Cursor{}, 10)
	if err != nil || len(open) != 1 || open[0].ID != beta.ID {
		t.Fatalf("open requests = %+v, %v", open, err)
	}
}

// Verifies: STU-100, LIM-001.
// At most render.visualDiffJobsPerOrganization of an organisation's jobs run
// at once; a job over the limit stays queued and runs when a slot is free. The
// renderer is given the versions and devices and nothing else.
func TestVisualDiffRateLimit_STU_100(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	k := f.requests(t, 1)
	k.renderer.hold = make(chan struct{})
	k.renderer.started = make(chan struct{}, 2)
	ctx := context.Background()
	dev := f.envs["development"].ID
	sel := []releasereq.Selected{{PluginKey: "loans", Reason: "changed"}}
	var jobs []releasereq.RenderJob
	for i, channel := range []string{"production", "beta"} {
		r, _, err := k.svc.Create(ctx, f.owner, releasereq.CreateInput{AppID: f.app, EnvironmentID: dev, Channel: channel, Plugins: sel})
		if err != nil {
			t.Fatal(err)
		}
		job := "job" + channel
		k.builds[job] = releasereq.Build{State: "succeeded", PluginKey: "loans", Version: int64(i + 1), BundleSHA256: "aa"}
		for _, step := range []releasereq.Step{
			{Kind: releasereq.StateSelect, Selection: sel},
			{Kind: releasereq.StateBuild, Builds: []releasereq.BuildRef{{PluginKey: "loans", JobID: job}}},
			{Kind: releasereq.StateReports},
		} {
			if _, err := k.svc.Advance(ctx, f.owner, f.app, r.ID, step); err != nil {
				t.Fatalf("%s: %v", step.Kind, err)
			}
		}
		if _, err := k.svc.RequestDiff(ctx, f.owner, f.app, r.ID); err != nil {
			t.Fatal(err)
		}
	}
	jobs = append(jobs, k.queue.jobs...)
	first := make(chan error, 1)
	go func() { first <- k.svc.RunRender(ctx, jobs[0]) }()
	<-k.renderer.started
	if err := k.svc.RunRender(ctx, jobs[1]); !errors.Is(err, releasereq.ErrDeferred) {
		t.Fatalf("a job over the limit = %v, want ErrDeferred", err)
	}
	close(k.renderer.hold)
	if err := <-first; err != nil {
		t.Fatalf("the first job: %v", err)
	}
	if err := k.svc.RunRender(ctx, jobs[1]); err != nil {
		t.Fatalf("the deferred job once a slot was free: %v", err)
	}
	if len(k.renderer.inputs) != 2 || len(k.renderer.inputs[0].Devices) == 0 {
		t.Errorf("renderer inputs = %+v", k.renderer.inputs)
	}
}

// toDiffsStep opens a request on a channel and walks it to the diffs step with
// one built version of the loans plugin, and returns it.
func toDiffsStep(t *testing.T, f *fixture, k *reqKit, channel string, version int64) releasereq.Request {
	t.Helper()
	ctx := context.Background()
	sel := []releasereq.Selected{{PluginKey: "loans", Reason: "changed"}}
	r, _, err := k.svc.Create(ctx, f.owner, releasereq.CreateInput{AppID: f.app, EnvironmentID: f.envs["development"].ID, Channel: channel, Plugins: sel})
	if err != nil {
		t.Fatal(err)
	}
	job := "job-" + channel
	k.builds[job] = releasereq.Build{State: "succeeded", PluginKey: "loans", Version: version, BundleSHA256: "aa"}
	for _, step := range []releasereq.Step{
		{Kind: releasereq.StateSelect, Selection: sel},
		{Kind: releasereq.StateBuild, Builds: []releasereq.BuildRef{{PluginKey: "loans", JobID: job}}},
		{Kind: releasereq.StateReports},
	} {
		if r, err = k.svc.Advance(ctx, f.owner, f.app, r.ID, step); err != nil {
			t.Fatalf("%s: %v", step.Kind, err)
		}
	}
	return r
}

// Verifies: STU-100, SRV-052.
// The render job is given the projects before and after the request, read in
// a transaction of their own, and the limits of its organisation; a project
// that cannot be read puts the job back in the queue instead of failing it,
// and the next attempt renders it.
func TestRenderJobGetsTheProjects_STU_100(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	k := f.requests(t, 2)
	ctx := context.Background()
	r := toDiffsStep(t, f, k, "production", 3)
	if _, err := k.svc.RequestDiff(ctx, f.owner, f.app, r.ID); err != nil {
		t.Fatal(err)
	}
	job := k.queue.jobs[0]

	k.sources.err = errors.New("blob store unavailable")
	if err := k.svc.RunRender(ctx, job); err == nil {
		t.Fatal("RunRender succeeded although the sources could not be read")
	}
	if len(k.renderer.inputs) != 0 {
		t.Fatalf("the renderer ran without sources: %+v", k.renderer.inputs)
	}
	if d, found, err := k.svc.Diff(ctx, f.owner, f.app, r.ID); err != nil || !found || d.State != releasereq.DiffQueued {
		t.Fatalf("after the failed read the job is %+v (found %v, %v); want queued", d, found, err)
	}

	// The catalogue of the environment is part of the input, as the release
	// build sees it; one that cannot be read is retried like the sources.
	k.sources.err = nil
	k.functions.err = errors.New("database unavailable")
	if err := k.svc.RunRender(ctx, job); err == nil || len(k.renderer.inputs) != 0 {
		t.Fatalf("RunRender with no catalogue = %v (renderer inputs %d)", err, len(k.renderer.inputs))
	}
	k.functions.err = nil
	if err := k.svc.RunRender(ctx, job); err != nil {
		t.Fatalf("the second attempt: %v", err)
	}
	if len(k.renderer.inputs) != 1 {
		t.Fatalf("renderer inputs = %d", len(k.renderer.inputs))
	}
	in := k.renderer.inputs[0]
	if in.Before == nil || in.After == nil || in.AppID != f.app || in.Channel != "production" || in.Limits.IsZero() {
		t.Errorf("render input = %+v", in)
	}
	if info, ok := in.Functions.Function("loans.quote", ""); !ok || info.Version != 3 || !in.Functions.DeviceFunctionsEnabled() {
		t.Errorf("the render input has no function catalogue: %+v", in.Functions)
	}
	if got := k.functions.asked[len(k.functions.asked)-1]; got[1] != f.app || got[0] == "" || got[2] == "" {
		t.Errorf("the catalogue was asked for %v; want the app and environment of the request", got)
	}
	if got := k.sources.calls[len(k.sources.calls)-1]; len(got) != 1 || got["loans"] != 3 {
		t.Errorf("the sources were asked for %v; want the built version of loans", got)
	}
	if d, _, err := k.svc.Diff(ctx, f.owner, f.app, r.ID); err != nil || d.State != releasereq.DiffSucceeded {
		t.Errorf("the job did not finish: %+v, %v", d, err)
	}
}

// Verifies: STU-100, SRV-022.
// A rendered image is read by its hash with plugin.read on the app: the hash
// must belong to the request's current diff, and another organisation, another
// request's image or an unknown or malformed hash is not found.
func TestVisualDiffImageRead_STU_100(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	k := f.requests(t, 2)
	ctx := context.Background()
	sha := func(b string) string { sum := sha256.Sum256([]byte(b)); return hex.EncodeToString(sum[:]) }

	r := toDiffsStep(t, f, k, "production", 1)
	if _, err := k.svc.Image(ctx, f.owner, f.app, r.ID, sha("new")); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("an image before any diff: %v", err)
	}
	if _, err := k.svc.RequestDiff(ctx, f.owner, f.app, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.RunRender(ctx, k.queue.jobs[0]); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"old", "new"} {
		got, err := k.svc.Image(ctx, f.owner, f.app, r.ID, sha(want))
		if err != nil || string(got) != want {
			t.Errorf("Image(%s) = %q, %v", want, got, err)
		}
	}
	if got, err := k.svc.Image(ctx, f.viewer, f.app, r.ID, sha("new")); err != nil || string(got) != "new" {
		t.Errorf("a viewer reads the image: %q, %v; plugin.read is enough", got, err)
	}
	for name, hash := range map[string]string{"an image the diff does not name": sha("other"), "upper case": strings.ToUpper(sha("new"))} {
		if _, err := k.svc.Image(ctx, f.owner, f.app, r.ID, hash); err == nil {
			t.Errorf("%s was served", name)
		}
	}
	if _, err := k.svc.Image(ctx, f.owner, f.app, r.ID, "not-a-hash"); !codeIs(err, plxerr.InvalidFormat) {
		t.Errorf("a malformed hash: %v", err)
	}

	// Another request of the same app cannot read this request's images: its
	// diff names no such hash.
	beta := toDiffsStep(t, f, k, "beta", 2)
	if _, err := k.svc.RequestDiff(ctx, f.owner, f.app, beta.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.Image(ctx, f.owner, f.app, beta.ID, sha("new")); !codeIs(err, plxerr.ResourceNotFound) {
		t.Errorf("an image through a request whose diff has not run: %v", err)
	}

	// Another organisation finds neither the request nor the image.
	admin := auth.Identity{Kind: auth.KindUser, ID: f.owner.UserID, UserID: f.owner.UserID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}.WithStepUp(0)
	other, err := f.tenancy.CreateOrganization(ctx, admin, "other-img", "Other")
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := f.auth.Resolve(ctx, admin, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.Image(ctx, stranger, f.app, r.ID, sha("new")); err == nil {
		t.Error("another organisation read an image")
	}
}
