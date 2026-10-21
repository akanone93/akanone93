// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nightCode42/plux3/backend/internal/cache"
)

// permissionsChannel is the channel on which a replica announces an
// organisation's new permissions version, in the form "<organisation>:<version>"
// (GOV-007, ADR-0086).
const permissionsChannel = "plux:permissions"

// resync is what a watcher receives instead of a version when change
// notices may have been lost: it reads the current version itself.
const resync int64 = -1

// permissionKey names what a replica caches: the resolver's input for one
// person in one organisation. Every app's and environment's permission set
// derives from it, so the version, not the scope, tells whether it is fresh.
type permissionKey struct {
	user, organization string
}

// cachedGrants is one cache entry.
type cachedGrants struct {
	loaded  loadedGrants
	gen     uint64
	expires time.Time
}

// ticket is a reader's view of the subscription taken before it reads the
// database: what it loads under the ticket is cached only while the
// subscription has been live, without a break, since.
type ticket struct {
	gen  uint64
	live bool
}

// permissionBus is one replica's side of the change notices (GOV-007,
// ADR-0086). It publishes the organisation's new permissions version when
// a change commits, learns of changes made on every replica, and so
//
//   - keeps the permission cache: an entry is dropped when its organisation's
//     version has moved on, when its lifetime ends (which covers a missed
//     notice), and whenever the subscription has been down, because then
//     notices may have been missed; while it is down the cache is bypassed;
//   - tells the watchers of an organisation, WatchPermissions, that it
//     changed.
//
// Its goroutine is owned by the bus and stops in close.
type permissionBus struct {
	notifier cache.Notifier
	sub      cache.Subscription
	lifetime time.Duration
	now      func() time.Time

	stop chan struct{}
	done chan struct{}
	once sync.Once

	mu       sync.Mutex
	latest   map[string]int64
	entries  map[permissionKey]cachedGrants
	swept    time.Time
	watchers map[*watcher]struct{}
}

// watcher is one WatchPermissions call: the versions of one organisation.
type watcher struct {
	organization string
	// versions holds the newest version, or resync, not yet read.
	versions chan int64
}

// newPermissionBus subscribes to the channel and starts the bus.
func newPermissionBus(ctx context.Context, n cache.Notifier, lifetime time.Duration, now func() time.Time) (*permissionBus, error) {
	sub, err := n.Subscribe(ctx, permissionsChannel)
	if err != nil {
		return nil, fmt.Errorf("auth: subscribe to permission changes: %w", err)
	}
	b := &permissionBus{
		notifier: n, sub: sub, lifetime: lifetime, now: now,
		stop: make(chan struct{}), done: make(chan struct{}),
		latest: map[string]int64{}, entries: map[permissionKey]cachedGrants{}, watchers: map[*watcher]struct{}{},
	}
	go b.run()
	return b, nil
}

// close stops the bus and its subscription, and waits for its goroutine.
func (b *permissionBus) close() {
	b.once.Do(func() {
		close(b.stop)
		_ = b.sub.Close()
		<-b.done
	})
}

// run applies the notices the subscription delivers, and tells the
// watchers to resynchronise when it may have missed some.
func (b *permissionBus) run() {
	defer close(b.done)
	_, seen := b.sub.State()
	for {
		select {
		case <-b.stop:
			return
		case message, ok := <-b.sub.Messages():
			if !ok {
				return
			}
			if org, version, ok := parseNotice(message); ok {
				b.observe(org, version)
			}
		case <-b.sub.Changes():
			if _, gen := b.sub.State(); gen != seen {
				seen = gen
				b.resynchronise()
			}
		}
	}
}

// parseNotice reads "<organisation>:<version>".
func parseNotice(message []byte) (organization string, version int64, ok bool) {
	org, v, found := strings.Cut(string(message), ":")
	if !found || org == "" {
		return "", 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	return org, n, err == nil && n >= 0
}

// publish announces an organisation's new version to every replica, and
// applies it here at once, so that this replica never serves what it has
// just changed from its cache.
func (b *permissionBus) publish(ctx context.Context, organization string, version int64) error {
	b.observe(organization, version)
	if err := b.notifier.Publish(ctx, permissionsChannel, []byte(organization+":"+strconv.FormatInt(version, 10))); err != nil {
		return fmt.Errorf("auth: publish a permissions version: %w", err)
	}
	return nil
}

// observe records an organisation's version, and tells its watchers when it
// is newer than the latest this replica knows.
func (b *permissionBus) observe(organization string, version int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if version <= b.latest[organization] {
		return
	}
	b.latest[organization] = version
	for w := range b.watchers {
		if w.organization == organization {
			w.offer(version)
		}
	}
}

// resynchronise tells every watcher that notices may have been missed.
func (b *permissionBus) resynchronise() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for w := range b.watchers {
		w.offer(resync)
	}
}

// offer makes v the watcher's pending value, replacing an unread one: a
// version supersedes the older ones, and the reader asks for the latest.
func (w *watcher) offer(v int64) {
	for {
		select {
		case w.versions <- v:
			return
		default:
		}
		select {
		case <-w.versions:
		default:
		}
	}
}

// watch registers a watcher of an organisation; the returned function
// unregisters it.
func (b *permissionBus) watch(organization string) (*watcher, func()) {
	w := &watcher{organization: organization, versions: make(chan int64, 1)}
	b.mu.Lock()
	b.watchers[w] = struct{}{}
	b.mu.Unlock()
	return w, func() {
		b.mu.Lock()
		delete(b.watchers, w)
		b.mu.Unlock()
	}
}

// begin takes the ticket a read of the database is made under.
func (b *permissionBus) begin() ticket {
	live, gen := b.sub.State()
	return ticket{gen: gen, live: live}
}

// lookup returns a cached entry, when the subscription is live and the entry
// is of its generation, within its lifetime, and not older than the
// organisation's latest version. Otherwise the caller resolves afresh.
func (b *permissionBus) lookup(key permissionKey) (loadedGrants, bool) {
	live, gen := b.sub.State()
	if !live {
		return loadedGrants{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.entries[key]
	if !ok {
		return loadedGrants{}, false
	}
	if e.gen != gen || !b.now().Before(e.expires) || e.loaded.version < b.latest[key.organization] {
		delete(b.entries, key)
		return loadedGrants{}, false
	}
	return e.loaded, true
}

// store caches what was loaded under a ticket, unless the subscription has
// been down since or the organisation has moved on.
func (b *permissionBus) store(t ticket, key permissionKey, loaded loadedGrants) {
	live, gen := b.sub.State()
	if !t.live || !live || gen != t.gen {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if loaded.version < b.latest[key.organization] {
		return
	}
	now := b.now()
	b.sweep(now)
	b.entries[key] = cachedGrants{loaded: loaded, gen: gen, expires: now.Add(b.lifetime)}
}

// sweep drops the expired entries, at most once in a lifetime; the caller
// holds b.mu.
func (b *permissionBus) sweep(now time.Time) {
	if now.Sub(b.swept) < b.lifetime {
		return
	}
	b.swept = now
	for key, e := range b.entries {
		if !now.Before(e.expires) {
			delete(b.entries, key)
		}
	}
}
