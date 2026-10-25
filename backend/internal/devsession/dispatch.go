// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/nightCode42/plux3/backend/internal/auth"
)

// dispatcher runs the Studio draft compiles off the write path (STU-130,
// NFR-030). It keeps, per (app, plugin), the latest pending set of written
// paths; writes that arrive while a compile for that key runs are unioned
// into one further compile. At most the configured number of compiles run
// at once. submit never blocks.
type dispatcher struct {
	run     func(ctx context.Context, p auth.Principal, appID string, paths []string)
	workers func(ctx context.Context, p auth.Principal, appID string) int

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	pending map[dispatchKey]*dispatchJob
	order   []dispatchKey
	active  int
	limit   int // 0 until read; one compile runs meanwhile
	closed  bool
	// panics counts compiles that ended abnormally; nothing reaches the writer.
	failed atomic.Int64
}

type dispatchKey struct{ app, plugin string }

type dispatchJob struct {
	p       auth.Principal
	paths   map[string]struct{}
	running bool
}

func newDispatcher(run func(context.Context, auth.Principal, string, []string), workers func(context.Context, auth.Principal, string) int) *dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &dispatcher{run: run, workers: workers, ctx: ctx, cancel: cancel, pending: map[dispatchKey]*dispatchJob{}}
}

// submit records the paths written and returns at once.
func (d *dispatcher) submit(p auth.Principal, appID, pluginID string, paths []string) {
	k := dispatchKey{appID, pluginID}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	j := d.pending[k]
	if j == nil {
		j = &dispatchJob{p: p, paths: map[string]struct{}{}}
		d.pending[k] = j
		d.order = append(d.order, k)
	}
	j.p = p
	for _, x := range paths {
		j.paths[x] = struct{}{}
	}
	d.startLocked()
}

// startLocked starts goroutines for queued keys while a worker is free.
func (d *dispatcher) startLocked() {
	for _, k := range d.order {
		j := d.pending[k]
		if j == nil || j.running {
			continue
		}
		if d.active >= max(d.limit, 1) {
			return
		}
		j.running = true
		d.active++
		d.wg.Add(1)
		go d.work(k, j)
	}
}

func (d *dispatcher) work(k dispatchKey, j *dispatchJob) {
	defer d.wg.Done()
	d.mu.Lock()
	if d.limit == 0 {
		p := j.p
		d.mu.Unlock()
		n := d.workers(d.ctx, p, k.app)
		d.mu.Lock()
		d.limit = n
		d.startLocked()
	}
	for !d.closed {
		p := j.p
		ids := make([]string, 0, len(j.paths))
		for x := range j.paths {
			ids = append(ids, x)
		}
		slices.Sort(ids)
		clear(j.paths)
		d.mu.Unlock()
		d.runOne(p, k.app, ids)
		d.mu.Lock()
		if len(j.paths) == 0 {
			break
		}
	}
	delete(d.pending, k)
	d.order = slices.DeleteFunc(d.order, func(x dispatchKey) bool { return x == k })
	d.active--
	d.startLocked()
	d.mu.Unlock()
}

// runOne runs one compile; a failure is counted and goes no further.
func (d *dispatcher) runOne(p auth.Principal, appID string, ids []string) {
	defer func() {
		if recover() != nil {
			d.failed.Add(1)
		}
	}()
	d.run(d.ctx, p, appID, ids)
}

// stop ends the dispatcher: no further compiles start, the running ones are
// cancelled and waited for.
func (d *dispatcher) stop() {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()
	d.cancel()
	d.wg.Wait()
}
