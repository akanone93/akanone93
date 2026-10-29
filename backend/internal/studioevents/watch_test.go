// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package studioevents_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/studioevents"
)

const org = "org1"

func reader() auth.Principal {
	p := auth.Principal{OrganizationID: org, Permissions: []auth.Permission{auth.AppRead}}
	p.UserID = "u1"
	return p
}

type rig struct {
	pub    studioevents.Publisher
	svc    *studioevents.Service
	events chan *pluxv1.StudioEventsServiceWatchResponse
	cancel context.CancelFunc
	done   chan error
}

// watch starts a Watch of principal p from cursor on a fresh memory cache.
func newRig(t *testing.T, mem *cache.Memory, p auth.Principal, cursor string, _ time.Duration, set limits.Set) *rig {
	t.Helper()
	svc, err := studioevents.NewService(studioevents.Options{Logs: mem, Notifier: mem, Poll: 10 * time.Millisecond, Heartbeat: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &rig{pub: studioevents.NewPublisher(mem, set), svc: svc, events: make(chan *pluxv1.StudioEventsServiceWatchResponse, 64), cancel: cancel, done: make(chan error, 1)}
	go func() {
		r.done <- svc.Watch(ctx, p, cursor, func(context.Context) (auth.Principal, error) { return p, nil }, func(m *pluxv1.StudioEventsServiceWatchResponse) error {
			r.events <- m
			return nil
		})
	}()
	// The first heartbeat shows the stream has fixed where "now" is.
	if cursor == "" && r.any(t).GetHeartbeat() == nil {
		t.Fatal("want a first heartbeat")
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-r.done:
		case <-time.After(5 * time.Second):
			t.Error("Watch did not return after its context ended")
		}
	})
	return r
}

// any returns the next message, heartbeats included.
func (r *rig) any(t *testing.T) *pluxv1.StudioEventsServiceWatchResponse {
	t.Helper()
	select {
	case m := <-r.events:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("no event arrived")
		return nil
	}
}

// next returns the next message that is not a heartbeat.
func (r *rig) next(t *testing.T) *pluxv1.StudioEventsServiceWatchResponse {
	t.Helper()
	for {
		if m := r.any(t); m.GetHeartbeat() == nil {
			return m
		}
	}
}

func TestWatchLockAndDocumentInOrder_STU_075(t *testing.T) {
	mem := cache.NewMemory(nil)
	r := newRig(t, mem, reader(), "", time.Hour, limits.Defaults())
	ctx := context.Background()
	holder := audit.Actor{Kind: "user", ID: "u2", Display: "Ada"}
	r.pub.LockChanged(ctx, org, document.Lock{AppID: "a1", Holder: holder, Session: "s"}, false)
	r.pub.DocumentChanged(ctx, org, "a1", "", "pages/home.json", 7, holder)
	r.pub.LockChanged(ctx, org, document.Lock{AppID: "a1"}, true)

	first := r.next(t)
	if first.GetLock().GetLock().GetHolder().GetId() != "u2" || first.GetCursor() == "" {
		t.Fatalf("first event = %v", first)
	}
	if d := r.next(t).GetDocument(); d.GetPath() != "pages/home.json" || d.GetRevision() != 7 {
		t.Fatalf("second event is not the document change: %v", d)
	}
	if last := r.next(t); last.GetLock() == nil || last.GetLock().GetLock() != nil {
		t.Fatalf("third event is not a release: %v", last)
	}
}

func TestWatchFiltersUnreadableApps_STU_075(t *testing.T) {
	mem := cache.NewMemory(nil)
	only := reader()
	only.Permissions = nil
	only.AppPermissions = map[string][]auth.Permission{"visible": {auth.AppRead}}
	r := newRig(t, mem, only, "", time.Hour, limits.Defaults())
	ctx := context.Background()
	a := audit.Actor{Kind: "user", ID: "u2"}
	r.pub.DocumentChanged(ctx, org, "hidden", "", "x.json", 1, a)
	r.pub.DocumentChanged(ctx, org, "visible", "", "y.json", 2, a)
	if d := r.next(t).GetDocument(); d.GetAppId() != "visible" {
		t.Fatalf("got %v, want the visible app's event first", d)
	}
}

func TestWatchResumesAfterCursor_STU_075(t *testing.T) {
	mem := cache.NewMemory(nil)
	first := newRig(t, mem, reader(), "", time.Hour, limits.Defaults())
	ctx := context.Background()
	a := audit.Actor{Kind: "user", ID: "u2"}
	first.pub.DocumentChanged(ctx, org, "a1", "", "one.json", 1, a)
	cursor := first.next(t).GetCursor()
	first.cancel()
	first.pub.DocumentChanged(ctx, org, "a1", "", "two.json", 2, a)

	second := newRig(t, mem, reader(), cursor, time.Hour, limits.Defaults())
	if d := second.next(t).GetDocument(); d.GetPath() != "two.json" {
		t.Fatalf("resumed at %v, want two.json", d)
	}
}

func TestWatchResetsOnGap_STU_075(t *testing.T) {
	mem := cache.NewMemory(nil)
	logs := cache.LogWriter{Logs: mem, Notifier: mem, MaxLen: 2}
	ctx := context.Background()
	for range 5 {
		if _, err := logs.Append(ctx, "studio:org:"+org, []byte{}); err != nil {
			t.Fatal(err)
		}
	}
	// A cursor older than the trimmed entries.
	r := newRig(t, mem, reader(), "1-0|", time.Hour, limits.Defaults())
	if m := r.next(t); m.GetReset_() == nil || m.GetCursor() == "" {
		t.Fatalf("got %v, want a reset with a cursor", m)
	}
	r.pub.DocumentChanged(ctx, org, "a1", "", "after.json", 3, audit.Actor{})
	if d := r.next(t).GetDocument(); d.GetPath() != "after.json" {
		t.Fatalf("after the reset got %v", d)
	}
}

func TestWatchHeartbeat_STU_075(t *testing.T) {
	r := newRig(t, cache.NewMemory(nil), reader(), "", 0, limits.Defaults())
	if m := r.any(t); m.GetHeartbeat() == nil || m.GetCursor() != "|" {
		t.Fatalf("got %v, want a heartbeat carrying the cursor", m)
	}
}

func TestWatchRefusesForeignCursor_STU_075(t *testing.T) {
	mem := cache.NewMemory(nil)
	svc, err := studioevents.NewService(studioevents.Options{Logs: mem, Notifier: mem})
	if err != nil {
		t.Fatal(err)
	}
	err = svc.Watch(context.Background(), reader(), "garbage", func(context.Context) (auth.Principal, error) { return reader(), nil }, func(*pluxv1.StudioEventsServiceWatchResponse) error { return nil })
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

func TestWatchInboxCount_SRV_062(t *testing.T) {
	mem := cache.NewMemory(nil)
	r := newRig(t, mem, reader(), "", time.Hour, limits.Defaults())
	r.pub.InboxChanged(context.Background(), org, "u1", 3)
	if got := r.next(t).GetInbox().GetUnread(); got != 3 {
		t.Fatalf("unread = %d, want 3", got)
	}
	r.pub.InboxChanged(context.Background(), org, "someone-else", 9)
	r.pub.InboxChanged(context.Background(), org, "u1", 0)
	if m := r.next(t); m.GetInbox() == nil || m.GetInbox().GetUnread() != 0 {
		t.Fatalf("got %v, want only this person's own count", m)
	}
}

// Verifies: GOV-005, STU-075.
func TestWatchEndsWhenRefreshFails_GOV_005(t *testing.T) {
	mem := cache.NewMemory(nil)
	svc, err := studioevents.NewService(studioevents.Options{Logs: mem, Notifier: mem, Poll: 10 * time.Millisecond, Heartbeat: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	gone := errors.New("session revoked")
	err = svc.Watch(context.Background(), reader(), "", func(context.Context) (auth.Principal, error) { return auth.Principal{}, gone }, func(*pluxv1.StudioEventsServiceWatchResponse) error { return nil })
	if !errors.Is(err, gone) {
		t.Fatalf("Watch = %v, want the refresh error", err)
	}
}

// Verifies: GOV-005, STU-075.
func TestWatchFollowsRefreshedPermissions_GOV_005(t *testing.T) {
	mem := cache.NewMemory(nil)
	svc, err := studioevents.NewService(studioevents.Options{Logs: mem, Notifier: mem, Poll: 10 * time.Millisecond, Heartbeat: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	lost := reader()
	lost.Permissions = nil
	var refreshed atomic.Bool
	events := make(chan *pluxv1.StudioEventsServiceWatchResponse, 64)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- svc.Watch(ctx, reader(), "", func(context.Context) (auth.Principal, error) {
			refreshed.Store(true)
			return lost, nil
		}, func(m *pluxv1.StudioEventsServiceWatchResponse) error { events <- m; return nil })
	}()
	defer func() { cancel(); <-done }()
	deadline := time.After(5 * time.Second)
	for !refreshed.Load() {
		select {
		case <-events:
		case <-deadline:
			t.Fatal("no refresh")
		}
	}
	pub := studioevents.NewPublisher(mem, limits.Defaults())
	pub.DocumentChanged(context.Background(), org, "app", "", "x.json", 1, audit.Actor{Kind: "user", ID: "u2"})
	quiet := time.After(300 * time.Millisecond)
	for {
		select {
		case m := <-events:
			if m.GetDocument() != nil {
				t.Fatalf("event for an app the person lost: %v", m)
			}
		case <-quiet:
			return
		}
	}
}
