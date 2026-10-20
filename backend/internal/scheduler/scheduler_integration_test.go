// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package scheduler_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/scheduler"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
)

// testKey keeps these tests off the advisory lock of a running server.
const testKey int64 = 0x7465737431

// newScheduler returns a scheduler over db whose job counts its runs.
func newScheduler(t *testing.T, db *storage.DB, runs *atomic.Int64, extra ...scheduler.Job) *scheduler.Scheduler {
	t.Helper()
	jobs := append([]scheduler.Job{scheduler.JobFunc{Label: "count", F: func(context.Context, string) error {
		runs.Add(1)
		return nil
	}}}, extra...)
	s, err := scheduler.New(scheduler.Options{
		Locker: db, LockKey: testKey, Interval: 10 * time.Millisecond, Jobs: jobs,
		Organizations: func(_ context.Context, f func(string) error) error { return f("org") },
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Verifies: FN-025, SRV-063.
// Two schedulers on one database elect one leader: a pass runs on exactly
// one of them, however often and however concurrently both step, and the
// other takes over when the leader stops.
func TestSchedulerLeaderElection_FN_025(t *testing.T) { //nolint:paralleltest // the advisory lock is database-wide
	db := storagetest.Open(t)
	ctx := context.Background()
	var runs atomic.Int64
	a, b := newScheduler(t, db, &runs), newScheduler(t, db, &runs)
	const rounds = 5
	var ran atomic.Int64
	for range rounds {
		var wg sync.WaitGroup
		for _, s := range []*scheduler.Scheduler{a, b} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				did, err := s.Step(ctx)
				if err != nil {
					t.Error(err)
				}
				if did {
					ran.Add(1)
				}
			}()
		}
		wg.Wait()
	}
	if ran.Load() != rounds || runs.Load() != rounds {
		t.Errorf("passes = %d, job runs = %d, want %d each", ran.Load(), runs.Load(), rounds)
	}
	if a.Leader() == b.Leader() {
		t.Fatalf("leaders: a=%v b=%v, want exactly one", a.Leader(), b.Leader())
	}
	leader, follower := a, b
	if b.Leader() {
		leader, follower = b, a
	}
	if err := leader.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if did, err := follower.Step(ctx); err != nil || !did || !follower.Leader() {
		t.Errorf("the follower after the leader stopped: ran %v, leader %v, %v", did, follower.Leader(), err)
	}
	if err := follower.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

// Verifies: FN-025.
// A failing job does not stop the jobs after it or the other organisations,
// and the pass reports the failure.
func TestSchedulerJobFailureIsolated_FN_025(t *testing.T) { //nolint:paralleltest // the advisory lock is database-wide
	db := storagetest.Open(t)
	var runs, after atomic.Int64
	boom := scheduler.JobFunc{Label: "boom", F: func(context.Context, string) error { return errors.New("boom") }}
	s := newScheduler(t, db, &runs, boom, scheduler.JobFunc{Label: "after", F: func(context.Context, string) error {
		after.Add(1)
		return nil
	}})
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	did, err := s.Step(context.Background())
	if !did || err == nil || runs.Load() != 1 || after.Load() != 1 {
		t.Errorf("ran %v, err %v, count %d, after %d", did, err, runs.Load(), after.Load())
	}
}

// Verifies: FN-025.
// Start runs passes on the interval, and Stop ends the loop and releases the
// lock so that a second scheduler can lead at once.
func TestSchedulerStartStop_FN_025(t *testing.T) { //nolint:paralleltest // the advisory lock is database-wide
	db := storagetest.Open(t)
	ctx := context.Background()
	var runs atomic.Int64
	s := newScheduler(t, db, &runs)
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx); err == nil {
		t.Error("a second Start must fail")
	}
	deadline := time.Now().Add(10 * time.Second)
	for runs.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if runs.Load() < 3 {
		t.Fatalf("the loop ran %d passes", runs.Load())
	}
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	stopped := runs.Load()
	time.Sleep(50 * time.Millisecond)
	if runs.Load() != stopped {
		t.Error("a pass ran after Stop")
	}
	other := newScheduler(t, db, &runs)
	t.Cleanup(func() { _ = other.Stop(ctx) })
	if did, _ := other.Step(ctx); !did {
		t.Error("the lock was not released by Stop")
	}
}

// Verifies: FN-025.
// A scheduler needs a locker and a way to list organisations.
func TestSchedulerNewValidates_FN_025(t *testing.T) {
	t.Parallel()
	if _, err := scheduler.New(scheduler.Options{}); err == nil {
		t.Error("an empty configuration was accepted")
	}
}
