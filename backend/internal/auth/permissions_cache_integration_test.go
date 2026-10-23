// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/cache/cachetest"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// replicas is a governance fixture served by two auth services that share a
// database and a Valkey server, as two API replicas do. The first is the
// fixture's own service.
type replicas struct {
	*governance
	server *cachetest.Server
	other  *auth.Service
}

// newReplicas starts a Valkey server and two services on it. A subscription
// is awaited to be live before the test starts, so that its cache is on.
func newReplicas(t *testing.T) *replicas {
	t.Helper()
	server := cachetest.Start(t)
	valkey := func() *cache.Valkey {
		v, err := cache.NewValkey(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = v.Close() })
		return v
	}
	var options auth.Options
	g := newGovernance(t, func(o *auth.Options) {
		o.Notifier = valkey()
		options = *o
	})
	t.Cleanup(g.svc.Close)
	options.Notifier = valkey()
	other, err := auth.NewService(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(other.Close)
	awaitPermission(t, "both subscriptions to go live", func() bool { return g.svc.CacheLive() && other.CacheLive() })
	return &replicas{governance: g, server: server, other: other}
}

// bobAtOther resolves Bob on the second replica.
func (r *replicas) bobAtOther(t *testing.T) auth.Principal {
	t.Helper()
	p, err := r.other.Resolve(context.Background(), r.bob.Identity, r.org)
	if err != nil {
		t.Fatalf("Resolve on the other replica: %v", err)
	}
	return p
}

// grantSilently gives Bob the developer role by writing the row directly, as
// a change that announces nothing: whatever a replica serves afterwards, it
// serves from what it cached or from the database.
func (r *replicas) grantSilently(t *testing.T) {
	t.Helper()
	id, _ := r.ids.New()
	err := r.db.InTx(context.Background(), storage.Tenant{OrganizationID: r.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO role_grants (id, organization_id, user_id, role, scope) VALUES ($1, $2, $3, 'developer', 'organization')`, id, r.org, r.bobID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// awaitPermission waits until check holds: a change announced on another
// replica takes a moment to reach it.
func awaitPermission(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Verifies: GOV-007.
// A replica serves a person's permissions from its cache, and drops them
// when another replica announces a change to the organisation: the change
// reaches it through Valkey, not by waiting for the entry to expire.
func TestPermissionCacheIsInvalidatedByAnotherReplica_GOV_007(t *testing.T) {
	r := newReplicas(t)
	ctx := context.Background()
	if r.bobAtOther(t).Holds(auth.PluginEdit) {
		t.Fatal("a viewer holds plugin.edit")
	}
	// A change nobody announced is not seen: the entry is served from the cache.
	r.grantSilently(t)
	if r.bobAtOther(t).Holds(auth.PluginEdit) {
		t.Fatal("the second replica read the database again instead of serving its cache")
	}
	// A change the first replica announces is seen by the second, through the channel.
	if _, err := r.svc.CreateGrant(ctx, r.owner, auth.GrantSpec{UserID: r.bobID, Role: string(auth.RoleTranslator), Scope: auth.ScopeOrganization}); err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	awaitPermission(t, "the announced change to reach the other replica", func() bool {
		return r.bobAtOther(t).Holds(auth.PluginEdit)
	})
	if got, want := r.bobAtOther(t).Version, r.version(t); got != want {
		t.Errorf("the other replica resolved at version %d, the organisation is at %d", got, want)
	}
}

// Verifies: GOV-007.
// While the subscription is down a replica cannot learn of changes, so it
// resolves afresh rather than serve what it cached, and caches again once the
// subscription is back.
func TestPermissionCacheIsBypassedWhileValkeyIsDown_GOV_007(t *testing.T) {
	r := newReplicas(t)
	if r.bobAtOther(t).Holds(auth.PluginEdit) {
		t.Fatal("a viewer holds plugin.edit")
	}
	r.server.Stop()
	awaitPermission(t, "the subscription to notice the outage", func() bool { return !r.other.CacheLive() })
	r.grantSilently(t)
	awaitPermission(t, "the cache to be bypassed", func() bool {
		return r.bobAtOther(t).Holds(auth.PluginEdit)
	})
}

// Verifies: GOV-007.
// A service without a change channel resolves every call afresh and cannot
// watch: it says so.
func TestWatchPermissionsNeedsAChangeChannel_GOV_007(t *testing.T) {
	t.Parallel()
	g := newGovernance(t)
	err := g.svc.WatchPermissions(context.Background(), g.owner, func(int64) error { return nil })
	if err == nil {
		t.Fatal("WatchPermissions without a notifier succeeded")
	}
	g.grantDeveloper(t, g.bobID)
	if !g.principalOf(t, "bob@example.com", g.org).Holds(auth.PluginEdit) {
		t.Error("a service with no cache missed a change")
	}
}

// Verifies: GOV-007.
// A watcher is sent the organisation's version when it starts and each newer
// one after a change, whichever replica made it, and ends with its caller.
func TestWatchPermissionsDeliversTheNewVersion_GOV_007(t *testing.T) {
	r := newReplicas(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	versions := make(chan int64, 8)
	done := make(chan error, 1)
	go func() {
		done <- r.other.WatchPermissions(ctx, r.owner, func(v int64) error {
			versions <- v
			return nil
		})
	}()
	next := func() int64 {
		t.Helper()
		select {
		case v := <-versions:
			return v
		case <-time.After(10 * time.Second):
			t.Fatal("no version arrived")
			return 0
		}
	}
	first := next()
	if want := r.version(t); first != want {
		t.Fatalf("the first version is %d, the organisation is at %d", first, want)
	}
	r.grantDeveloper(t, r.bobID)
	if got := next(); got <= first {
		t.Errorf("after a change the watcher was sent %d, not newer than %d", got, first)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("WatchPermissions ended with %v, want the cancellation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("WatchPermissions did not end with its caller")
	}
}
