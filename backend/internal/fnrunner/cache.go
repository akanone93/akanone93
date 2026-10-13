// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"container/list"
	"context"
	"sync"

	"github.com/nightCode42/plux3/backend/internal/fnhost"
)

// entry is a compiled module in the cache. A module that was evicted while
// an invocation still runs on it is closed by the last release.
type entry struct {
	key     string
	module  *fnhost.Module
	size    int64
	refs    int
	evicted bool
	// shut is set once the module was closed, so that it is closed once.
	shut bool
}

// flight is a module load in progress, shared by concurrent callers.
type flight struct {
	done chan struct{}
	err  error
}

// moduleCache is a least-recently-used cache of compiled modules bounded in
// count and in the total size of the module bytes (FN-010). It is safe for
// concurrent use.
type moduleCache struct {
	maxCount int
	maxBytes int64

	mu       sync.Mutex
	bytes    int64
	order    *list.List // of *entry, most recently used first
	entries  map[string]*list.Element
	inflight map[string]*flight
	closed   bool
}

func newModuleCache(maxCount int, maxBytes int64) *moduleCache {
	return &moduleCache{
		maxCount: maxCount, maxBytes: maxBytes,
		order: list.New(), entries: map[string]*list.Element{}, inflight: map[string]*flight{},
	}
}

// acquire returns the cached module for key with a reference the caller must
// release, or nil.
func (c *moduleCache) acquire(key string) *entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		return nil
	}
	c.order.MoveToFront(el)
	e := el.Value.(*entry) //nolint:forcetypeassert // only *entry is stored
	e.refs++
	return e
}

// begin joins the load of key. The first caller is the leader and must call
// finish; the others wait on the returned flight.
func (c *moduleCache) begin(key string) (f *flight, leader bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f, ok := c.inflight[key]; ok {
		return f, false
	}
	f = &flight{done: make(chan struct{})}
	c.inflight[key] = f
	return f, true
}

// finish ends a load, waking the waiters with err.
func (c *moduleCache) finish(key string, f *flight, err error) {
	c.mu.Lock()
	delete(c.inflight, key)
	c.mu.Unlock()
	f.err = err
	close(f.done)
}

// add stores a compiled module and returns it with one reference. The
// caller has checked that size fits the byte budget. A module added after
// Close is not cached: it is closed when its invocation releases it.
// Entries are evicted, least recently used first, until the new one fits.
func (c *moduleCache) add(ctx context.Context, key string, m *fnhost.Module, size int64) *entry {
	e := &entry{key: key, module: m, size: size, refs: 1}
	c.mu.Lock()
	if c.closed {
		e.evicted = true
		c.mu.Unlock()
		return e
	}
	var drop []*entry
	for c.order.Len() > 0 && (c.order.Len() >= c.maxCount || c.bytes+size > c.maxBytes) {
		drop = append(drop, c.evictOldest())
	}
	c.entries[key] = c.order.PushFront(e)
	c.bytes += size
	c.mu.Unlock()
	for _, d := range drop {
		c.closeIfIdle(ctx, d)
	}
	return e
}

// evictOldest unlinks the least recently used entry. The caller holds mu.
func (c *moduleCache) evictOldest() *entry {
	el := c.order.Back()
	e := el.Value.(*entry) //nolint:forcetypeassert // only *entry is stored
	c.order.Remove(el)
	delete(c.entries, e.key)
	c.bytes -= e.size
	e.evicted = true
	return e
}

// closeIfIdle closes an evicted module nobody uses.
func (c *moduleCache) closeIfIdle(ctx context.Context, e *entry) {
	c.mu.Lock()
	idle := e.evicted && e.refs == 0 && !e.shut
	if idle {
		e.shut = true
	}
	c.mu.Unlock()
	if idle {
		_ = e.module.Close(context.WithoutCancel(ctx))
	}
}

// release gives back a reference from acquire or add.
func (c *moduleCache) release(ctx context.Context, e *entry) {
	c.mu.Lock()
	e.refs--
	c.mu.Unlock()
	c.closeIfIdle(ctx, e)
}

// len reports the cached modules and their total size.
func (c *moduleCache) len() (count int, bytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len(), c.bytes
}

// close empties the cache; modules in use close when released.
func (c *moduleCache) close(ctx context.Context) {
	c.mu.Lock()
	c.closed = true
	var drop []*entry
	for c.order.Len() > 0 {
		drop = append(drop, c.evictOldest())
	}
	c.mu.Unlock()
	for _, d := range drop {
		c.closeIfIdle(ctx, d)
	}
}
