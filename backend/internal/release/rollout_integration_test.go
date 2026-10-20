// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/rollout"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// fakeSignals returns the event counts a test sets for each release.
type fakeSignals struct {
	mu     sync.Mutex
	counts map[int64]rollout.Counts
}

func (s *fakeSignals) set(release int64, c rollout.Counts) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[release] = c
}

func (s *fakeSignals) Counts(_ context.Context, _ string, q rollout.SignalQuery) (rollout.Counts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[q.Release], nil
}

// recorder keeps the notifications a rollout publishes.
type recorder struct {
	mu     sync.Mutex
	events []notify.Event
}

func (r *recorder) Publish(_ context.Context, e notify.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}

func (r *recorder) outcomes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		out = append(out, e.Attributes["outcome"])
	}
	return out
}

// rolloutWorld is a production channel serving a first release, with a newer
// second release ready to roll out.
type rolloutWorld struct {
	*fixture
	svc           *rollout.Service
	signals       *fakeSignals
	notes         *recorder
	first, second release.Release
	prod          tenancy.Environment
	gen           ids
}

func newRolloutWorld(t *testing.T) *rolloutWorld {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	w := &rolloutWorld{fixture: f, prod: f.envs["production"], signals: &fakeSignals{counts: map[int64]rollout.Counts{}}, notes: &recorder{}}
	w.gen = ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	w.svc = w.newService(t, limits.Defaults(), time.Second)
	f.rel.UseRollouts(w.svc)
	f.publish(t, "", false)
	f.publish(t, f.loans, false)
	var err error
	if w.first, _, err = f.rel.CreateRelease(ctx, f.owner, f.app, f.envs["development"].ID, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = f.rel.PromoteRelease(ctx, f.owner, f.app, w.first.Sequence, w.prod.ID, ""); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.app, "", "edit", false); err != nil {
		t.Fatal(err)
	}
	en, err := f.docs.GetDocument(ctx, f.owner, f.app, "", "translations/en.json")
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(en.Content, []byte("Your schedule"), []byte("Your repayment plan"), 1)
	if _, err := f.docs.PutDocument(ctx, f.owner, f.app, "", "edit", en.Path, changed, en.Revision); err != nil {
		t.Fatal(err)
	}
	f.publish(t, "", false)
	f.publish(t, f.loans, false)
	if w.second, _, err = f.rel.CreateRelease(ctx, f.owner, f.app, f.envs["development"].ID, map[string]int64{"": 2, "loans": 2}, ""); err != nil {
		t.Fatal(err)
	}
	return w
}

// newService builds a rollout service on the fixture, with the given limits
// and gate-check interval.
func (w *rolloutWorld) newService(t *testing.T, set limits.Set, interval time.Duration) *rollout.Service {
	t.Helper()
	svc, err := rollout.NewService(rollout.Options{
		DB: w.db, Audit: audit.NewLog(w.gen, nil), IDs: w.gen, Releases: w.rel, Signals: w.signals, Notify: w.notes,
		Now: func() time.Time { return w.now }, CheckInterval: interval,
		Limits: func(ctx context.Context, tx pgx.Tx, org, app string) (limits.Set, error) { return set, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// input is a rollout of the second release on production.
func (w *rolloutWorld) input(stages ...rollout.Stage) rollout.CreateInput {
	return rollout.CreateInput{AppID: w.app, EnvironmentID: w.prod.ID, Candidate: w.second.Sequence, Stages: stages}
}

func (w *rolloutWorld) create(t *testing.T, in rollout.CreateInput) rollout.Rollout {
	t.Helper()
	r, err := w.svc.Create(context.Background(), w.owner, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	w.settle(t)
	return r
}

// settle has the worker sign what the last step queued and lets the
// replicas' manifest caches expire.
func (w *rolloutWorld) settle(t *testing.T) {
	t.Helper()
	w.run(t)
	w.now = w.now.Add(2 * time.Second)
}

// tick runs the scheduler job once, then the worker.
func (w *rolloutWorld) tick(t *testing.T, svc *rollout.Service) {
	t.Helper()
	if err := svc.Tick(context.Background(), w.org); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	w.settle(t)
}

func (w *rolloutWorld) advance(d time.Duration) { w.now = w.now.Add(d) }

func (w *rolloutWorld) get(t *testing.T, id string) rollout.Rollout {
	t.Helper()
	r, err := w.svc.Get(context.Background(), w.owner, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func deviceID(i int) string { return fmt.Sprintf("00000000-0000-7000-8000-%012d", i) }

// served is the release sequence of the manifest a device receives.
func (w *rolloutWorld) served(t *testing.T, id, os string) int64 {
	t.Helper()
	m, err := w.rel.GetManifest(context.Background(), release.ManifestRequest{
		OrganizationID: w.org, AppID: w.app, EnvironmentID: w.prod.ID, DeviceID: id, Targeting: release.Targeting{OS: os},
	})
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	return m.Document.ReleaseSequence
}

// inSet returns which of n Android devices are offered the candidate.
func (w *rolloutWorld) inSet(t *testing.T, n int) map[int]bool {
	t.Helper()
	in := map[int]bool{}
	for i := range n {
		switch got := w.served(t, deviceID(i), "android"); got {
		case w.second.Sequence:
			in[i] = true
		case w.first.Sequence:
		default:
			t.Fatalf("device %d was served release %d", i, got)
		}
	}
	return in
}

func (w *rolloutWorld) channelRelease(t *testing.T) int64 {
	t.Helper()
	chs, err := w.tenancy.ListChannels(context.Background(), w.owner, w.prod.ID, tenancy.Page{Size: 10})
	if err != nil || len(chs) != 1 {
		t.Fatalf("ListChannels: %v %v", chs, err)
	}
	return chs[0].ReleaseSequence
}

func (w *rolloutWorld) auditCount(t *testing.T, action audit.Action) int {
	t.Helper()
	var n int
	err := w.db.InTx(context.Background(), storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = $1`, string(action)).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (w *rolloutWorld) releaseCount(t *testing.T) int {
	t.Helper()
	n := 0
	for _, env := range []tenancy.Environment{w.envs["development"], w.prod} {
		rs, err := w.rel.ListReleases(context.Background(), w.owner, w.app, env.ID, 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		n += len(rs)
	}
	return n
}

func wantCode(t *testing.T, err error, want plxerr.Code) {
	t.Helper()
	if got := code(err); got != want {
		t.Fatalf("error = %v (PLX-%d), want PLX-%d", err, got, want)
	}
}

// A device inside a rollout receives the candidate's manifest, one outside it
// the previous release's: by bucket, by targeting, and by the testers' lists
// (Q10).
//
// Verifies: REL-050, REL-051.
func TestRolloutServesInAndOutDevices_REL_051(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	in := w.input(rollout.Stage{BasisPoints: 5000})
	in.Rule = []byte(`{"attribute":"os","operator":"eq","value":"android"}`)
	in.AllowDevices, in.DenyDevices = []string{deviceID(9001)}, []string{deviceID(9002)}
	r := w.create(t, in)
	if r.State != rollout.StateActive || r.PreviousSequence != w.first.Sequence || r.BasisPoints != 5000 {
		t.Fatalf("rollout = %+v", r)
	}
	if got := w.channelRelease(t); got != w.first.Sequence {
		t.Fatalf("the channel serves %d, want %d until the rollout completes", got, w.first.Sequence)
	}
	got := w.inSet(t, 80)
	if len(got) < 20 || len(got) > 60 {
		t.Errorf("%d of 80 devices are in at 50%%", len(got))
	}
	for i := range 40 {
		if got := w.served(t, deviceID(i), "ios"); got != w.first.Sequence {
			t.Fatalf("an iOS device outside the targeting was served release %d", got)
		}
	}
	if got := w.served(t, deviceID(9001), "ios"); got != w.second.Sequence {
		t.Errorf("an allowed tester was served release %d", got)
	}
	if got := w.served(t, deviceID(9002), "android"); got != w.first.Sequence {
		t.Errorf("a denied device was served release %d", got)
	}
}

// The share only grows: a device in at one stage is in at the next, and a held
// last stage stays where it is.
//
// Verifies: REL-050, REL-052.
func TestRolloutStagesGrowMonotonically_REL_052(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	r := w.create(t, w.input(rollout.Stage{BasisPoints: 1000, DurationSeconds: 3600}, rollout.Stage{BasisPoints: 5000, DurationSeconds: 3600}, rollout.Stage{BasisPoints: 9000}))
	stage1 := w.inSet(t, 100)
	w.tick(t, w.svc)
	if got := w.get(t, r.ID); got.StageIndex != 0 {
		t.Fatalf("a tick inside the first stage moved it to %d", got.StageIndex)
	}
	w.advance(time.Hour)
	w.tick(t, w.svc)
	if got := w.get(t, r.ID); got.StageIndex != 1 || got.BasisPoints != 5000 {
		t.Fatalf("after an hour: %+v", got)
	}
	stage2 := w.inSet(t, 100)
	for i := range stage1 {
		if !stage2[i] {
			t.Fatalf("device %d was in at 10%% and out at 50%%", i)
		}
	}
	if len(stage2) <= len(stage1) {
		t.Errorf("the share did not grow: %d then %d", len(stage1), len(stage2))
	}
	w.advance(10 * time.Hour)
	w.tick(t, w.svc)
	if got := w.get(t, r.ID); got.State != rollout.StateActive || got.BasisPoints != 9000 {
		t.Errorf("the held last stage = %+v", got)
	}
}

// A rollout that reaches every device and soaks promotes the candidate: the
// channel then serves it to everyone.
//
// Verifies: REL-052.
func TestRolloutCompletes_REL_052(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	r := w.create(t, w.input(rollout.Stage{BasisPoints: 3000, DurationSeconds: 3600}, rollout.Stage{BasisPoints: 10000}))
	w.advance(time.Hour)
	w.tick(t, w.svc)
	if got := w.get(t, r.ID); got.State != rollout.StateCompleted {
		t.Fatalf("rollout = %+v", got)
	}
	if got := w.channelRelease(t); got != w.second.Sequence {
		t.Fatalf("the channel serves %d, want %d", got, w.second.Sequence)
	}
	if n := len(w.inSet(t, 30)); n != 30 {
		t.Errorf("%d of 30 devices have the new release", n)
	}
	if n := w.auditCount(t, audit.RolloutCompleted); n != 1 {
		t.Errorf("%d completions audited", n)
	}
}

// The scheduled end reverts to the previous release as a new release, so no
// device's sequence ever goes down; two schedulers that run at once revert
// once.
//
// Verifies: REL-052.
func TestRolloutEndRevertsOnce_REL_052(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	in := w.input(rollout.Stage{BasisPoints: 5000})
	in.EndAt = w.now.Add(time.Hour)
	r := w.create(t, in)
	if n := len(w.inSet(t, 40)); n == 0 {
		t.Fatal("no device is in the rollout")
	}
	other := w.newService(t, limits.Defaults(), time.Second)
	w.advance(2 * time.Hour)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, svc := range []*rollout.Service{w.svc, other} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = svc.Tick(context.Background(), w.org)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("Tick: %v", err)
		}
	}
	w.settle(t)
	got := w.get(t, r.ID)
	if got.State != rollout.StateEnded || got.RevertedSequence == 0 {
		t.Fatalf("rollout = %+v", got)
	}
	if n := w.releaseCount(t); n != 3 {
		t.Fatalf("%d releases, want 3: the two and one revert", n)
	}
	if cur := w.channelRelease(t); cur != got.RevertedSequence || cur <= w.second.Sequence {
		t.Fatalf("the channel serves %d, want the revert %d above %d", cur, got.RevertedSequence, w.second.Sequence)
	}
	for i := range 20 {
		if s := w.served(t, deviceID(i), "android"); s != got.RevertedSequence {
			t.Fatalf("device %d is served release %d after the end", i, s)
		}
	}
	if n := w.auditCount(t, audit.RolloutReverted); n != 1 {
		t.Errorf("%d reverts audited", n)
	}
	w.tick(t, w.svc)
	if n := w.releaseCount(t); n != 3 {
		t.Errorf("a further tick made a release: %d", n)
	}
}

// Two schedulers that tick at the same moment advance a stage once.
//
// Verifies: REL-052.
func TestRolloutAdvanceIsIdempotentUnderTwoSchedulers_REL_052(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	r := w.create(t, w.input(rollout.Stage{BasisPoints: 1000, DurationSeconds: 60}, rollout.Stage{BasisPoints: 2000, DurationSeconds: 3600}, rollout.Stage{BasisPoints: 3000}))
	other := w.newService(t, limits.Defaults(), time.Second)
	w.advance(5 * time.Minute)
	var wg sync.WaitGroup
	for _, svc := range []*rollout.Service{w.svc, other, w.svc, other} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.Tick(context.Background(), w.org); err != nil {
				t.Errorf("Tick: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := w.get(t, r.ID); got.StageIndex != 1 || got.BasisPoints != 2000 {
		t.Errorf("rollout = %+v", got)
	}
	if n := w.auditCount(t, audit.RolloutAdvanced); n != 1 {
		t.Errorf("%d advances audited, want 1", n)
	}
	if n := w.auditCount(t, audit.RolloutStarted); n != 1 {
		t.Errorf("%d starts audited, want 1", n)
	}
}

// A rollout that starts later is scheduled until then, and the channel serves
// its release to nobody extra in the meantime.
//
// Verifies: REL-052.
func TestRolloutScheduledStart_REL_052(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	in := w.input(rollout.Stage{BasisPoints: 10000, DurationSeconds: 60})
	in.StartAt = w.now.Add(time.Hour)
	r := w.create(t, in)
	if r.State != rollout.StateScheduled {
		t.Fatalf("state = %s", r.State)
	}
	w.tick(t, w.svc)
	if n := len(w.inSet(t, 10)); n != 0 {
		t.Errorf("%d devices are in a rollout that has not started", n)
	}
	w.advance(time.Hour)
	w.tick(t, w.svc)
	if got := w.get(t, r.ID); got.State != rollout.StateActive || got.PreviousSequence != w.first.Sequence {
		t.Fatalf("rollout = %+v", got)
	}
	if n := len(w.inSet(t, 10)); n != 10 {
		t.Errorf("%d of 10 devices are in at 100%%", n)
	}
}

// Windows that meet are refused; a later one is accepted; the app's limit
// bounds how many are in progress.
//
// Verifies: REL-052.
func TestRolloutOverlapAndLimit_REL_052(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	first := w.input(rollout.Stage{BasisPoints: 1000})
	first.EndAt = w.now.Add(2 * time.Hour)
	w.create(t, first)
	overlapping := w.input(rollout.Stage{BasisPoints: 1000})
	overlapping.StartAt = w.now.Add(time.Hour)
	_, err := w.svc.Create(context.Background(), w.owner, overlapping)
	wantCode(t, err, plxerr.ScheduleOverlapsRollout)
	_, err = w.svc.Create(context.Background(), w.owner, w.input(rollout.Stage{BasisPoints: 1000}))
	wantCode(t, err, plxerr.ScheduleOverlapsRollout)
	later := w.input(rollout.Stage{BasisPoints: 1000})
	later.StartAt, later.EndAt = w.now.Add(3*time.Hour), w.now.Add(4*time.Hour)
	if _, err := w.svc.Create(context.Background(), w.owner, later); err != nil {
		t.Fatalf("a later window: %v", err)
	}
	tight, err := limits.Defaults().Tighten(limits.RolloutsActivePerApp, limits.ScopeApp, 2)
	if err != nil {
		t.Fatal(err)
	}
	capped := w.newService(t, tight, time.Second)
	after := w.input(rollout.Stage{BasisPoints: 1000})
	after.StartAt = w.now.Add(10 * time.Hour)
	_, err = capped.Create(context.Background(), w.owner, after)
	wantCode(t, err, plxerr.LimitExceeded)
}

// Invalid input is refused before anything is stored.
//
// Verifies: REL-051, REL-052.
func TestRolloutCreateRefusals_REL_051(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	ctx := context.Background()
	bad := w.input(rollout.Stage{BasisPoints: 1000})
	bad.Rule = []byte(`{"attribute":"colour","operator":"eq","value":"red"}`)
	_, err := w.svc.Create(ctx, w.owner, bad)
	wantCode(t, err, plxerr.TargetingRuleInvalid)
	_, err = w.svc.Create(ctx, w.owner, w.input())
	wantCode(t, err, plxerr.MissingProperty)
	old := w.input(rollout.Stage{BasisPoints: 1000})
	old.Candidate = w.first.Sequence
	_, err = w.svc.Create(ctx, w.owner, old)
	wantCode(t, err, plxerr.OutOfRange)
	_, err = w.svc.Create(ctx, w.viewer, w.input(rollout.Stage{BasisPoints: 1000}))
	wantCode(t, err, plxerr.PermissionDenied)
	if rs, err := w.svc.List(ctx, w.owner, w.app, false, "", 10); err != nil || len(rs) != 0 {
		t.Errorf("List = %v, %v; want nothing stored", rs, err)
	}
}

// A person may pause, resume and abort; a transition from the wrong state is
// refused; abort reverts.
//
// Verifies: REL-052, REL-053.
func TestRolloutManualControl_REL_053(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	ctx := context.Background()
	r := w.create(t, w.input(rollout.Stage{BasisPoints: 5000}))
	_, err := w.svc.Resume(ctx, w.owner, r.ID)
	wantCode(t, err, plxerr.RevisionConflict)
	_, err = w.svc.Pause(ctx, w.viewer, r.ID, "")
	wantCode(t, err, plxerr.PermissionDenied)
	paused, err := w.svc.Pause(ctx, w.owner, r.ID, "investigating")
	if err != nil || paused.State != rollout.StatePaused || !strings.Contains(paused.Reason, "investigating") {
		t.Fatalf("Pause = %+v, %v", paused, err)
	}
	// A paused rollout keeps its share and does not advance.
	w.settle(t)
	if len(w.inSet(t, 40)) == 0 {
		t.Error("pausing took the candidate from its devices")
	}
	_, err = w.svc.Pause(ctx, w.owner, r.ID, "")
	wantCode(t, err, plxerr.RevisionConflict)
	if resumed, err := w.svc.Resume(ctx, w.owner, r.ID); err != nil || resumed.State != rollout.StateActive {
		t.Fatalf("Resume = %+v, %v", resumed, err)
	}
	aborted, err := w.svc.Abort(ctx, w.owner, r.ID, "bad build")
	if err != nil || aborted.State != rollout.StateAborted || aborted.RevertedSequence == 0 {
		t.Fatalf("Abort = %+v, %v", aborted, err)
	}
	w.settle(t)
	if got := w.channelRelease(t); got != aborted.RevertedSequence {
		t.Errorf("the channel serves %d, want the revert %d", got, aborted.RevertedSequence)
	}
	_, err = w.svc.Abort(ctx, w.owner, r.ID, "")
	wantCode(t, err, plxerr.RevisionConflict)
	if n := w.auditCount(t, audit.RolloutPaused); n != 1 {
		t.Errorf("%d pauses audited", n)
	}
}

// A candidate that regresses beyond the gate pauses the rollout (PLX-3150) and
// notifies; the pause keeps the share.
//
// Verifies: REL-053.
func TestRolloutGatePauses_REL_053(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	in := w.input(rollout.Stage{BasisPoints: 1000, DurationSeconds: 3600}, rollout.Stage{BasisPoints: 10000})
	in.Health = rollout.Gate{MinSample: 100, MaxCrashFreeDrop: 50}
	r := w.create(t, in)
	w.signals.set(w.first.Sequence, rollout.Counts{Sessions: 1000, Crashed: 5})
	w.signals.set(w.second.Sequence, rollout.Counts{Sessions: 1000, Crashed: 100})
	w.advance(time.Minute)
	w.tick(t, w.svc)
	got := w.get(t, r.ID)
	if got.State != rollout.StatePaused || !strings.Contains(got.Reason, "PLX-3150") || !strings.Contains(got.Reason, "crashFreeSessions") {
		t.Fatalf("rollout = %+v", got)
	}
	if got.BasisPoints != 1000 || len(got.LastCheck) == 0 {
		t.Errorf("the pause lost the share or the figures: %+v", got)
	}
	if o := w.notes.outcomes(); len(o) != 1 || o[0] != "paused" {
		t.Errorf("notifications = %v", o)
	}
	w.advance(2 * time.Hour)
	w.tick(t, w.svc)
	if again := w.get(t, r.ID); again.State != rollout.StatePaused || again.StageIndex != 0 {
		t.Errorf("a paused rollout moved: %+v", again)
	}
	if o := w.notes.outcomes(); len(o) != 1 {
		t.Errorf("a second notification: %v", o)
	}
}

// With the rollback action a regression reverts the rollout (PLX-3151).
//
// Verifies: REL-053.
func TestRolloutGateRollsBack_REL_053(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	in := w.input(rollout.Stage{BasisPoints: 5000})
	in.Health = rollout.Gate{MinSample: 100, MaxErrorRateIncrease: 100, Action: rollout.ActionRollback}
	r := w.create(t, in)
	w.signals.set(w.first.Sequence, rollout.Counts{Sessions: 1000, Errors: 10})
	w.signals.set(w.second.Sequence, rollout.Counts{Sessions: 1000, Errors: 400})
	w.advance(time.Minute)
	w.tick(t, w.svc)
	got := w.get(t, r.ID)
	if got.State != rollout.StateRolledBack || !strings.Contains(got.Reason, "PLX-3151") || got.RevertedSequence == 0 {
		t.Fatalf("rollout = %+v", got)
	}
	if o := w.notes.outcomes(); len(o) != 1 || o[0] != "rolled_back" {
		t.Errorf("notifications = %v", o)
	}
	if cur := w.channelRelease(t); cur != got.RevertedSequence {
		t.Errorf("the channel serves %d, want %d", cur, got.RevertedSequence)
	}
}

// A sample that is too small decides nothing, and a healthy candidate goes on;
// a due stage is gated before it is entered, whatever the check interval.
//
// Verifies: REL-053.
func TestRolloutGateNeedsASample_REL_053(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	in := w.input(rollout.Stage{BasisPoints: 1000, DurationSeconds: 3600}, rollout.Stage{BasisPoints: 5000})
	in.Health = rollout.Gate{MinSample: 500, MaxCrashFreeDrop: 50}
	r := w.create(t, in)
	w.signals.set(w.first.Sequence, rollout.Counts{Sessions: 1000, Crashed: 5})
	w.signals.set(w.second.Sequence, rollout.Counts{Sessions: 20, Crashed: 20})
	w.advance(time.Minute)
	w.tick(t, w.svc)
	got := w.get(t, r.ID)
	if got.State != rollout.StateActive || !strings.Contains(string(got.LastCheck), `"decided": false`) {
		t.Fatalf("a candidate of 20 sessions: %+v (%s)", got, got.LastCheck)
	}
	w.signals.set(w.second.Sequence, rollout.Counts{Sessions: 1000, Crashed: 6})
	w.advance(time.Hour)
	w.tick(t, w.svc)
	if got := w.get(t, r.ID); got.State != rollout.StateActive || got.StageIndex != 1 {
		t.Errorf("a healthy candidate did not advance: %+v", got)
	}
}

// The gate stands between the schedule and the next stage: a rollout whose
// check interval has not passed is still checked when a stage is due.
//
// Verifies: REL-053.
func TestRolloutGateChecksBeforeAdvancing_REL_053(t *testing.T) {
	t.Parallel()
	w := newRolloutWorld(t)
	w.svc = w.newService(t, limits.Defaults(), 24*time.Hour)
	w.rel.UseRollouts(w.svc)
	in := w.input(rollout.Stage{BasisPoints: 1000, DurationSeconds: 60}, rollout.Stage{BasisPoints: 5000})
	in.Health = rollout.Gate{MinSample: 100, MaxCrashFreeDrop: 50}
	r := w.create(t, in)
	w.signals.set(w.first.Sequence, rollout.Counts{Sessions: 1000, Crashed: 5})
	w.signals.set(w.second.Sequence, rollout.Counts{Sessions: 1000, Crashed: 5})
	w.tick(t, w.svc) // a first check, healthy; the next is a day away
	w.signals.set(w.second.Sequence, rollout.Counts{Sessions: 1000, Crashed: 200})
	w.advance(time.Minute)
	w.tick(t, w.svc)
	if got := w.get(t, r.ID); got.State != rollout.StatePaused || got.StageIndex != 0 {
		t.Fatalf("the due stage was entered without a check: %+v", got)
	}
}
