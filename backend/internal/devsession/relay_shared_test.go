// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// The memory layer carries everything a shared hub needs.
var _ SharedLayer = (*cache.Memory)(nil)

// sharedWait bounds every wait for a follower in these tests.
const sharedWait = 5 * time.Second

// replicas returns two hubs on one shared layer, two api replicas, with
// the cancel that stops replica A.
func replicas(t *testing.T) (a, b *Hub, stopA context.CancelFunc) {
	t.Helper()
	shared := cache.NewMemory(nil)
	ctxA, stopA := context.WithCancel(context.Background())
	ctxB, stopB := context.WithCancel(context.Background())
	t.Cleanup(func() { stopA(); stopB() })
	return NewSharedHub(ctxA, shared, 10*time.Millisecond), NewSharedHub(ctxB, shared, 10*time.Millisecond), stopA
}

// socketOn opens a device's socket on a replica's room of session "s": it holds the
// room's follower and says hello with the last frame it carried out.
func socketOn(t *testing.T, h *Hub, deviceID string, done uint64) (*room, *link) {
	t.Helper()
	r := h.room("s", nil)
	r.use()
	t.Cleanup(r.release)
	l, err := r.attach(deviceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.hello(deviceID, l, &pluxv1.DevDeviceHello{LastServerSequence: done}); err != nil {
		t.Fatal(err)
	}
	return r, l
}

// framesUntil collects the frames a link's writer would send until it
// holds n numbered frames, or the session ended.
func framesUntil(t *testing.T, r *room, deviceID string, l *link, n int) (got []*pluxv1.DevServerFrame, ended plxerr.Code) {
	t.Helper()
	deadline := time.After(sharedWait)
	for {
		frames, code, _ := r.pending(deviceID, l)
		for _, f := range frames {
			if f.GetSequence() > 0 {
				got = append(got, f)
			}
		}
		if len(got) >= n || code != 0 {
			return got, code
		}
		select {
		case <-l.wake:
		case <-deadline:
			t.Fatalf("the link holds %d frames, want %d", len(got), n)
		}
	}
}

// Verifies: DEV-021, DEV-020. A frame queued on one replica reaches the
// device whose socket another replica holds.
func TestSharedHubDeliversAcrossReplicas_DEV_021(t *testing.T) {
	a, b, _ := replicas(t)
	rb, lb := socketOn(t, b, "d1", 0)
	for range 2 {
		if err := a.Deliver("s", "d1", &pluxv1.DevServerFrame{}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := framesUntil(t, rb, "d1", lb, 2)
	if len(got) != 2 || got[0].GetSequence() != 1 || got[1].GetSequence() != 2 {
		t.Errorf("replica B wrote %v, want frames 1 and 2", got)
	}
}

// Verifies: DEV-021, DEV-001. Events a device sends to one replica reach
// a watcher on the other, numbered with the session sequence, and a
// watcher resumes on either replica after its cursor.
func TestSharedHubWatchesAcrossReplicas_DEV_021_DEV_001(t *testing.T) {
	a, b, _ := replicas(t)
	ra, la := socketOn(t, a, "d1", 0)
	rb := b.room("s", nil)
	rb.use()
	t.Cleanup(rb.release)
	w, backlog := rb.subscribe(0, nil)
	t.Cleanup(func() { rb.unsubscribe(w) })
	if len(backlog) != 0 {
		t.Fatalf("backlog %v before any event", backlog)
	}
	for i := uint64(1); i <= 3; i++ {
		if err := ra.publish("d1", la, event(i)); err != nil {
			t.Fatal(err)
		}
	}
	var got []*pluxv1.DevEvent
	deadline := time.After(sharedWait)
	for len(got) < 3 {
		select {
		case <-w.wake:
		case <-deadline:
			t.Fatalf("the watcher on B got %d events, want 3", len(got))
		}
		evs, _ := rb.take(w)
		got = append(got, evs...)
	}
	for i, ev := range got {
		if ev.GetSequence() != uint64(i+1) || ev.GetDeviceId() != "d1" {
			t.Errorf("event %d = %v, want sequence %d from d1", i, ev, i+1)
		}
	}
	for _, h := range []*Hub{a, b} {
		w2, resumed := h.room("s", nil).subscribe(2, nil)
		h.room("s", nil).unsubscribe(w2)
		if len(resumed) != 1 || resumed[0].GetSequence() != 3 {
			t.Errorf("resuming after 2 gave %v, want event 3", resumed)
		}
	}
}

// Verifies: DEV-021. A device that reconnects to the other replica
// resumes after the last frame it carried out.
func TestSharedHubResumesFramesOnTheOtherReplica_DEV_021(t *testing.T) {
	a, b, _ := replicas(t)
	ra, la := socketOn(t, a, "d1", 0)
	for range 3 {
		if err := a.Deliver("s", "d1", &pluxv1.DevServerFrame{}); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := framesUntil(t, ra, "d1", la, 3); len(got) != 3 {
		t.Fatalf("replica A wrote %d frames, want 3", len(got))
	}
	ra.detach("d1", la)
	rb, lb := socketOn(t, b, "d1", 1)
	got, _ := framesUntil(t, rb, "d1", lb, 2)
	if len(got) != 2 || got[0].GetSequence() != 2 || got[1].GetSequence() != 3 {
		t.Errorf("resumed on B with %v, want frames 2 and 3", got)
	}
	if err := b.Deliver("s", "d1", &pluxv1.DevServerFrame{}); err != nil {
		t.Fatal(err)
	}
	if next, _ := framesUntil(t, rb, "d1", lb, 1); next[0].GetSequence() != 4 {
		t.Errorf("the next frame is numbered %d, want 4", next[0].GetSequence())
	}
}

// Verifies: DEV-021. A replica that stops mid-session ends its follower;
// the other replica keeps serving the session.
func TestSharedHubSurvivesAStoppedReplica_DEV_021(t *testing.T) {
	a, b, stopA := replicas(t)
	ra := a.room("s", nil)
	ra.use()
	stopA()
	ra.release() // waits for A's follower to end
	rb, lb := socketOn(t, b, "d1", 0)
	if err := b.Deliver("s", "d1", &pluxv1.DevServerFrame{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := framesUntil(t, rb, "d1", lb, 1); got[0].GetSequence() != 1 {
		t.Errorf("replica B wrote %v, want frame 1", got)
	}
	if err := rb.publish("d1", lb, event(1)); err != nil {
		t.Fatal(err)
	}
	if _, backlog := rb.subscribe(0, nil); len(backlog) != 1 {
		t.Errorf("replica B holds %d events, want 1", len(backlog))
	}
}

// Verifies: DEV-021, DEV-020. A session's end on one replica closes the
// sockets every replica holds.
func TestSharedHubEndClosesSocketsOnEveryReplica_DEV_021(t *testing.T) {
	a, b, _ := replicas(t)
	ra, la := socketOn(t, a, "d1", 0)
	rb, lb := socketOn(t, b, "d2", 0)
	a.End("s", plxerr.DevSessionExpired)
	for name, side := range map[string]struct {
		r *room
		d string
		l *link
	}{"A": {ra, "d1", la}, "B": {rb, "d2", lb}} {
		if _, code := framesUntil(t, side.r, side.d, side.l, 1); code != plxerr.DevSessionExpired {
			t.Errorf("replica %s ended its socket with %v, want %v", name, code, plxerr.DevSessionExpired)
		}
	}
	if err := b.Deliver("s", "d2", &pluxv1.DevServerFrame{}); err == nil {
		t.Error("replica B delivered to an ended session")
	}
}

// Verifies: DEV-021, LIM-001. A cursor older than the session's event
// log keeps, on the other replica, is answered with the drop signal.
func TestSharedHubTrimmedCursorDrops_DEV_021(t *testing.T) {
	a, b, _ := replicas(t)
	lim := relayLimits{events: 2, bytes: 1 << 20, eventBytes: 1 << 20, commands: 10, lifetime: time.Hour}
	ra := a.room("s", &lim)
	la, err := ra.attach("d1")
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(1); i <= 5; i++ {
		if err := ra.publish("d1", la, event(i)); err != nil {
			t.Fatal(err)
		}
	}
	rb := b.room("s", &lim)
	w, backlog := rb.subscribe(1, nil)
	rb.unsubscribe(w)
	if len(backlog) == 0 || backlog[0].GetDrop() == nil {
		t.Fatalf("resuming after 1 gave %v, want the drop first", backlog)
	}
	if last := backlog[len(backlog)-1]; last.GetSequence() != 5 {
		t.Errorf("the newest event is %d, want 5", last.GetSequence())
	}
}
