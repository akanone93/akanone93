// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/auth"
)

type runLog struct {
	mu    sync.Mutex
	calls [][]string
}

func (r *runLog) add(p []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, p)
}

func (r *runLog) get() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal("condition not reached before the deadline")
}

// Verifies: STU-130, NFR-030.
func TestDispatcher_SubmitReturnsBeforeCompile_Coalesces_STU_130_NFR_030(t *testing.T) {
	var log runLog
	release := make(chan struct{})
	started := make(chan struct{}, 4)
	d := newDispatcher(func(_ context.Context, _ auth.Principal, _ string, ids []string) {
		log.add(ids)
		started <- struct{}{}
		<-release
	}, func(context.Context, auth.Principal, string) int { return 4 })
	t.Cleanup(d.stop)

	returned := make(chan struct{})
	go func() {
		d.submit(auth.Principal{}, "app", "plug", []string{"a"})
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("submit blocked on the compile")
	}
	<-started
	d.submit(auth.Principal{}, "app", "plug", []string{"b"})
	d.submit(auth.Principal{}, "app", "plug", []string{"c", "b"})
	d.submit(auth.Principal{}, "app", "plug", []string{"d"})
	release <- struct{}{}
	<-started
	close(release)
	waitFor(t, func() bool { d.mu.Lock(); defer d.mu.Unlock(); return len(d.pending) == 0 })
	got := log.get()
	if len(got) != 2 || !slices.Equal(got[0], []string{"a"}) || !slices.Equal(got[1], []string{"b", "c", "d"}) {
		t.Errorf("compiles = %v, want [a] then [b c d]", got)
	}
}

// Verifies: STU-130.
func TestDispatcher_WorkerLimitAndClose_STU_130(t *testing.T) {
	var mu sync.Mutex
	running, peak := 0, 0
	release := make(chan struct{})
	d := newDispatcher(func(ctx context.Context, _ auth.Principal, _ string, _ []string) {
		mu.Lock()
		running++
		peak = max(peak, running)
		mu.Unlock()
		select {
		case <-release:
		case <-ctx.Done():
		}
		mu.Lock()
		running--
		mu.Unlock()
	}, func(context.Context, auth.Principal, string) int { return 2 })
	for _, plug := range []string{"p1", "p2", "p3", "p4"} {
		d.submit(auth.Principal{}, "app", plug, []string{"x"})
	}
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return running == 2 })
	d.stop() // cancels the running compiles and waits for them
	mu.Lock()
	defer mu.Unlock()
	if peak != 2 || running != 0 {
		t.Errorf("peak = %d, running after stop = %d, want 2 and 0", peak, running)
	}
}
