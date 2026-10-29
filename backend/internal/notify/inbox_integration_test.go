// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package notify_test

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// subscribed is a world whose developer follows release.published in the
// inbox.
func subscribed(t *testing.T) *world { return subscribedWith(t, nil) }

// subscribedWith is subscribed with the service's options changed first.
func subscribedWith(t *testing.T, mutate func(*notify.Options)) *world {
	t.Helper()
	w := newWorld(t, mutate)
	if _, err := w.svc.Subscribe(context.Background(), w.developer, notify.Subscription{EventType: notify.ReleasePublished}); err != nil {
		t.Fatal(err)
	}
	return w
}

// SRV-062, STU-012: the display parameters of an event reach the inbox
// unchanged, beside the server-rendered title and body.
func TestInboxParamsRoundTrip_SRV_062_STU_012(t *testing.T) {
	w := subscribed(t)
	ctx := context.Background()
	e := w.event(notify.ReleasePublished, "Release 7 published")
	e.Params = map[string]string{"release": "7", "channel": "production", "note": "ünïcode ✓"}
	e.Subject = "release-7"
	if err := w.svc.Publish(ctx, e); err != nil {
		t.Fatal(err)
	}
	items, _, err := w.svc.Inbox(ctx, w.developer, false, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("inbox = %+v, %v", items, err)
	}
	it := items[0]
	if !maps.Equal(it.Params, e.Params) || it.Title != e.Title || it.Body != e.Body {
		t.Fatalf("item = %+v", it)
	}
	if len(it.GroupKey) != 32 || it.GroupCount != 1 {
		t.Fatalf("group = %q %d", it.GroupKey, it.GroupCount)
	}

	// An event without parameters lists an empty map and groups alone,
	// keyed by its own ID.
	if err := w.svc.Publish(ctx, w.event(notify.ReleasePublished, "Plain")); err != nil {
		t.Fatal(err)
	}
	items, _, err = w.svc.Inbox(ctx, w.developer, false, 10)
	if err != nil || len(items) != 2 {
		t.Fatalf("inbox = %+v, %v", items, err)
	}
	for _, it := range items {
		if it.Title == "Plain" && (len(it.Params) != 0 || it.GroupKey != it.ID || it.GroupCount != 1) {
			t.Fatalf("plain item = %+v", it)
		}
	}
}

// SRV-062: notifications.paramsPerEvent and notifications.paramBytes bound
// the display parameters; an event over them is refused.
func TestInboxParamsBounds_SRV_062(t *testing.T) {
	w := subscribed(t)
	ctx := context.Background()
	set := limits.Defaults()
	count, size := int(set.Get(limits.NotificationsParamsPerEvent)), int(set.Get(limits.NotificationsParamBytes))
	if count != 16 || size != 1024 {
		t.Fatalf("defaults = %d, %d", count, size)
	}
	params := func(n, valueBytes int) map[string]string {
		m := map[string]string{}
		for i := range n {
			m["p"+strconv.Itoa(i)] = strings.Repeat("v", valueBytes)
		}
		return m
	}
	at := w.event(notify.ReleasePublished, "At the bounds")
	at.Params = params(count, size)
	at.Params = maps.Clone(at.Params)
	if err := w.svc.Publish(ctx, at); err != nil {
		t.Fatalf("an event at the bounds: %v", err)
	}
	over := []map[string]string{
		params(count+1, 1),
		params(1, size+1),
		{strings.Repeat("k", 65): "v"},
		{"": "v"},
	}
	for i, p := range over {
		e := w.event(notify.ReleasePublished, "Over")
		e.Params = p
		if err := w.svc.Publish(ctx, e); code(err) != plxerr.LimitExceeded {
			t.Errorf("event %d: %v", i, err)
		}
	}
	if items, _, err := w.svc.Inbox(ctx, w.developer, false, 10); err != nil || len(items) != 1 {
		t.Fatalf("inbox = %+v, %v", items, err)
	}
}

// SRV-062, STU-012: repeats of one event (same type, app and subject) list
// as one item, the newest, with the group's count; another subject is
// another group.
func TestInboxGroupsRepeats_SRV_062_STU_012(t *testing.T) {
	w := subscribed(t)
	ctx := context.Background()
	publish := func(title, subject string) {
		t.Helper()
		e := w.event(notify.ReleasePublished, title)
		e.Subject = subject
		e.Params = map[string]string{"title": title}
		if err := w.svc.Publish(ctx, e); err != nil {
			t.Fatal(err)
		}
		w.advance(time.Second)
	}
	publish("first", "plugin-a")
	publish("other", "plugin-b")
	publish("second", "plugin-a")
	publish("third", "plugin-a")

	items, unread, err := w.svc.Inbox(ctx, w.developer, false, 10)
	if err != nil || len(items) != 2 || unread != 4 {
		t.Fatalf("inbox = %+v unread %d, %v", items, unread, err)
	}
	if items[0].Title != "third" || items[0].GroupCount != 3 || items[0].Params["title"] != "third" {
		t.Fatalf("newest group = %+v", items[0])
	}
	if items[1].Title != "other" || items[1].GroupCount != 1 || items[1].GroupKey == items[0].GroupKey {
		t.Fatalf("other group = %+v", items[1])
	}

	// Reading the newest leaves two unread repeats, still one group.
	if _, err := w.svc.MarkRead(ctx, w.developer, []string{items[0].ID}); err != nil {
		t.Fatal(err)
	}
	items, unread, err = w.svc.Inbox(ctx, w.developer, true, 10)
	if err != nil || len(items) != 2 || unread != 3 {
		t.Fatalf("unread inbox = %+v unread %d, %v", items, unread, err)
	}
	if items[0].Title != "second" || items[0].GroupCount != 2 {
		t.Fatalf("unread group = %+v", items[1])
	}
}

// SRV-062: the sweep deletes the notifications older than the
// organisation's notifications.retentionDays, and only that organisation's.
func TestInboxRetentionSweep_SRV_062(t *testing.T) {
	w := subscribed(t)
	ctx := context.Background()
	if err := w.svc.Publish(ctx, w.event(notify.ReleasePublished, "old")); err != nil {
		t.Fatal(err)
	}
	w.advance(60 * 24 * time.Hour)
	if err := w.svc.Publish(ctx, w.event(notify.ReleasePublished, "recent")); err != nil {
		t.Fatal(err)
	}
	w.advance(31 * 24 * time.Hour)
	titles := func() []string {
		t.Helper()
		items, _, err := w.svc.Inbox(ctx, w.developer, false, 10)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, it := range items {
			out = append(out, it.Title)
		}
		return out
	}

	// Another organisation's sweep does not reach these rows.
	if _, err := w.svc.Purge(ctx, "00000000-0000-7000-8000-000000000001"); err != nil {
		t.Fatalf("purge of another organisation: %v", err)
	}
	if got := titles(); len(got) != 2 {
		t.Fatalf("after another organisation's sweep: %v", got)
	}

	// At the default of 90 days, only the 91-day-old row goes.
	if _, err := w.svc.Purge(ctx, w.org); err != nil {
		t.Fatal(err)
	}
	if got := titles(); len(got) != 1 || got[0] != "recent" {
		t.Fatalf("after the sweep: %v", got)
	}

	// A shorter retention for the organisation takes the rest.
	set, err := limits.Defaults().Tighten(limits.NotificationsRetentionDays, limits.ScopeOrganization, 30)
	if err != nil {
		t.Fatal(err)
	}
	w.limits.Store(&set)
	if _, err := w.svc.Purge(ctx, w.org); err != nil {
		t.Fatal(err)
	}
	if got := titles(); len(got) != 0 {
		t.Fatalf("after a 30-day sweep: %v", got)
	}
}

// unreadLog records the unread counts announced to the Studio stream.
type unreadLog struct {
	mu     sync.Mutex
	counts map[string][]int64
}

func (l *unreadLog) InboxChanged(_ context.Context, _, user string, unread int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[user] = append(l.counts[user], unread)
}

func (l *unreadLog) of(user string) []int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.counts[user])
}

// STU-075, SRV-062: an inbox entry and a read each announce the person's
// new unread count to the Studio stream, after the change is committed.
func TestInboxAnnouncesUnreadCount_STU_075_SRV_062(t *testing.T) {
	log := &unreadLog{counts: map[string][]int64{}}
	w := subscribedWith(t, func(o *notify.Options) { o.Events = log })
	ctx := context.Background()
	for _, title := range []string{"one", "two"} {
		if err := w.svc.Publish(ctx, w.event(notify.ReleasePublished, title)); err != nil {
			t.Fatal(err)
		}
	}
	if got := log.of(w.developer.UserID); !slices.Equal(got, []int64{1, 2}) {
		t.Fatalf("after two events announced %v, want [1 2]", got)
	}
	if _, err := w.svc.MarkRead(ctx, w.developer, nil); err != nil {
		t.Fatal(err)
	}
	if got := log.of(w.developer.UserID); !slices.Equal(got, []int64{1, 2, 0}) {
		t.Fatalf("after reading announced %v, want [1 2 0]", got)
	}
}
