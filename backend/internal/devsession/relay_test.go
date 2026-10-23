// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"slices"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// event is a device event with a device sequence.
func event(seq uint64) *pluxv1.DevEvent { return &pluxv1.DevEvent{Sequence: seq} }

// wantExpired fails unless err is the refusal of an ended session.
func wantExpired(t *testing.T, err error) {
	t.Helper()
	code, ok := plxerr.CodeOf(err)
	if !ok || code != plxerr.DevSessionExpired {
		t.Fatalf("error %v (code %v), want %v", err, code, plxerr.DevSessionExpired)
	}
}

// Verifies: DEV-020. An event the relay will not carry is acknowledged to
// the device and relayed as a drop in its place; the same event resent is
// acknowledged again and not relayed twice.
func TestRefuseAcknowledgesAndRelaysTheDrop_DEV_020(t *testing.T) {
	r := NewHub().room("s", nil)
	l, err := r.attach("d1")
	if err != nil {
		t.Fatal(err)
	}
	w, _ := r.subscribe(0, nil)

	if err := r.refuse("d1", l, 7, pluxv1.DevDropReason_DEV_DROP_REASON_EVENT_TOO_LARGE); err != nil {
		t.Fatal(err)
	}
	if !l.ackDue || l.ack != 7 {
		t.Fatalf("link acknowledges %d (due %v), want 7", l.ack, l.ackDue)
	}
	got, _ := r.take(w)
	if len(got) != 1 {
		t.Fatalf("watcher got %d events, want the drop", len(got))
	}
	drop := got[0].GetDrop()
	if drop.GetDropped() != 1 || drop.GetReason() != pluxv1.DevDropReason_DEV_DROP_REASON_EVENT_TOO_LARGE {
		t.Errorf("drop %v, want one event dropped as too large", drop)
	}

	l.ackDue = false
	if err := r.refuse("d1", l, 7, pluxv1.DevDropReason_DEV_DROP_REASON_EVENT_TOO_LARGE); err != nil {
		t.Fatal(err)
	}
	if !l.ackDue || l.ack != 7 {
		t.Errorf("a resent event is not acknowledged again: ack %d due %v", l.ack, l.ackDue)
	}
	if again, _ := r.take(w); len(again) != 0 {
		t.Errorf("a resent event relayed %d more events, want none", len(again))
	}
	if !r.duplicate("d1", 7) || r.duplicate("d1", 8) {
		t.Error("duplicate does not tell the refused sequence from a new one")
	}

	// The drop carries the reason it was given.
	if err := r.refuse("d1", l, 8, pluxv1.DevDropReason_DEV_DROP_REASON_BUFFER_FULL); err != nil {
		t.Fatal(err)
	}
	next, _ := r.take(w)
	if len(next) != 1 || next[0].GetDrop().GetReason() != pluxv1.DevDropReason_DEV_DROP_REASON_BUFFER_FULL {
		t.Errorf("second drop %v, want one with the buffer-full reason", next)
	}
}

// Verifies: DEV-020. Once the session has ended its relay refuses every
// device operation with PLX-9200.
func TestEndedRoomRefusesDevices_DEV_020(t *testing.T) {
	h := NewHub()
	r := h.room("s", nil)
	l, err := r.attach("d1")
	if err != nil {
		t.Fatal(err)
	}
	h.End("s", plxerr.DevSessionExpired)

	wantExpired(t, r.refuse("d1", l, 1, pluxv1.DevDropReason_DEV_DROP_REASON_EVENT_TOO_LARGE))
	wantExpired(t, r.publish("d1", l, event(1)))
	_, err = r.attach("d2")
	wantExpired(t, err)
	wantExpired(t, h.Deliver("s", "d1", &pluxv1.DevServerFrame{}))
}

// Verifies: DEV-021. Watchers see events in the order the session
// sequenced them, whichever device's list they come from.
func TestCompareSeqOrders_DEV_021(t *testing.T) {
	for _, c := range []struct {
		a, b uint64
		want int
	}{{1, 2, -1}, {2, 1, 1}, {3, 3, 0}, {0, 0, 0}} {
		if got := compareSeq(c.a, c.b); got != c.want {
			t.Errorf("compareSeq(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}

	r := NewHub().room("s", nil)
	l1, _ := r.attach("d1")
	l2, _ := r.attach("d2")
	for i, step := range []struct {
		device string
		l      *link
		seq    uint64
	}{{"d1", l1, 1}, {"d2", l2, 1}, {"d1", l1, 2}, {"d2", l2, 2}} {
		if err := r.publish(step.device, step.l, event(step.seq)); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	_, all := r.subscribe(0, nil)
	var seqs []uint64
	for _, e := range all {
		seqs = append(seqs, e.GetSequence())
	}
	if !slices.Equal(seqs, []uint64{1, 2, 3, 4}) {
		t.Errorf("snapshot sequences %v, want 1..4 in order", seqs)
	}
	_, only := r.subscribe(1, map[string]bool{"d2": true})
	if len(only) != 2 || only[0].GetDeviceId() != "d2" || only[0].GetSequence() != 2 || only[1].GetSequence() != 4 {
		t.Errorf("filtered snapshot after 1 = %v, want the events 2 and 4 of d2", only)
	}
}

// Verifies: DEV-021. An empty device list watches every device, a list
// watches only its devices.
func TestFilterOf_DEV_021(t *testing.T) {
	if f := filterOf(nil); f != nil {
		t.Errorf("filterOf(nil) = %v, want no filter", f)
	}
	if f := filterOf([]string{}); f != nil {
		t.Errorf("filterOf(empty) = %v, want no filter", f)
	}
	f := filterOf([]string{"a", "b", "a"})
	if len(f) != 2 || !f["a"] || !f["b"] {
		t.Errorf("filterOf = %v, want a and b", f)
	}
	if !allows(f, "a") || allows(f, "c") {
		t.Error("the filter lets through a device it does not list")
	}
	if !allows(f, "") {
		t.Error("the server's own events must pass every filter")
	}
}

// Verifies: FN-036. Function log levels map to the stream's; one the
// stream does not know is unspecified, never a guess.
func TestLogLevel_FN_036(t *testing.T) {
	for level, want := range map[string]pluxv1.DevLogLevel{
		"debug": pluxv1.DevLogLevel_DEV_LOG_LEVEL_DEBUG,
		"info":  pluxv1.DevLogLevel_DEV_LOG_LEVEL_INFO,
		"warn":  pluxv1.DevLogLevel_DEV_LOG_LEVEL_WARNING,
		"error": pluxv1.DevLogLevel_DEV_LOG_LEVEL_ERROR,
		"trace": pluxv1.DevLogLevel_DEV_LOG_LEVEL_UNSPECIFIED,
		"":      pluxv1.DevLogLevel_DEV_LOG_LEVEL_UNSPECIFIED,
	} {
		if got := logLevel(level); got != want {
			t.Errorf("logLevel(%q) = %v, want %v", level, got, want)
		}
	}
}

// Verifies: DEV-031. The command a request carries is named by its field;
// a request without one has no kind.
func TestCommandKind_DEV_031(t *testing.T) {
	if k := commandKind(&pluxv1.DevSessionServiceSendCommandRequest{}); k != "" {
		t.Errorf("commandKind of an empty request = %q, want none", k)
	}
	if k := commandKind(evaluateRequest("1", nil)); k != "evaluate_pxl" {
		t.Errorf("commandKind = %q, want evaluate_pxl", k)
	}
}

// Verifies: DEV-020. The service exposes the hub it relays through.
func TestServiceHub_DEV_020(t *testing.T) {
	h := NewHub()
	if (&Service{hub: h}).Hub() != h {
		t.Error("Hub does not return the service's hub")
	}
}

// Verifies: DEV-002, GOV-001. A call that names a session before its
// environment is known needs the permission somewhere: in the organisation,
// on an app or on an environment. Without an organisation or without the
// permission it is refused before anything is read.
func TestAuthorizeAnywhere_DEV_002(t *testing.T) {
	want := auth.DevSessionObserve
	cases := []struct {
		name string
		p    auth.Principal
		code plxerr.Code // zero when allowed
	}{
		{"organisation", auth.Principal{OrganizationID: "o", Permissions: []auth.Permission{want}}, 0},
		{"app", auth.Principal{OrganizationID: "o", AppPermissions: map[string][]auth.Permission{"a": {want}}}, 0},
		{"environment", auth.Principal{OrganizationID: "o", EnvPermissions: map[auth.EnvScope][]auth.Permission{{AppID: "a", Environment: "dev"}: {want}}}, 0},
		{"another permission", auth.Principal{OrganizationID: "o", Permissions: []auth.Permission{auth.DevSessionCreate}, AppPermissions: map[string][]auth.Permission{"a": {auth.DevSessionCreate}}}, plxerr.PermissionDenied},
		{"nothing", auth.Principal{OrganizationID: "o"}, plxerr.PermissionDenied},
		{"no organisation", auth.Principal{Permissions: []auth.Permission{want}}, plxerr.AuthenticationRequired},
	}
	for _, c := range cases {
		err := authorizeAnywhere(c.p, want)
		if c.code == 0 {
			if err != nil {
				t.Errorf("%s: %v, want allowed", c.name, err)
			}
			continue
		}
		if code, ok := plxerr.CodeOf(err); !ok || code != c.code {
			t.Errorf("%s: %v, want code %v", c.name, err, c.code)
		}
	}
}

// Verifies: DEV-002. A session records its creator as the user, or as the
// kind and ID of the access token that acted.
func TestCreatorOf_DEV_002(t *testing.T) {
	if got := creatorOf(auth.Principal{Identity: auth.Identity{Kind: auth.KindUser, ID: "u1", UserID: "u1"}}); got != "u1" {
		t.Errorf("creator of a user = %q, want u1", got)
	}
	if got := creatorOf(auth.Principal{Identity: auth.Identity{Kind: "token", ID: "t9"}}); got != "token:t9" {
		t.Errorf("creator of a token = %q, want token:t9", got)
	}
}

// Verifies: DEV-002. A code shows as two groups of four; anything of
// another length is shown as it is.
func TestFormatCode_DEV_002(t *testing.T) {
	if got := FormatCode("ABCD2345"); got != "ABCD-2345" {
		t.Errorf("FormatCode = %q", got)
	}
	if got := FormatCode("ABC"); got != "ABC" {
		t.Errorf("FormatCode of a short code = %q, want it unchanged", got)
	}
}

// Verifies: DEV-020. The ping interval and idle timeout come from the
// options, or default.
func TestSocketTimingDefaults_DEV_020(t *testing.T) {
	s := &Service{}
	if s.pingInterval() != defaultPingInterval || s.idleTimeout() != defaultIdleTimeout {
		t.Error("an unset service does not use the defaults")
	}
	s.o.PingInterval, s.o.IdleTimeout = time.Second, 3*time.Second
	if s.pingInterval() != time.Second || s.idleTimeout() != 3*time.Second {
		t.Error("the configured timing is not used")
	}
}

// Verifies: DEV-021. A frame for no device in particular goes to every
// device the relay knows, not to the server's own slot, and each device
// numbers its frames from 1.
func TestDeliverToAllDevices_DEV_021(t *testing.T) {
	r := NewHub().room("s", nil)
	for _, id := range []string{"d1", "d2"} {
		if _, err := r.attach(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.publish("d1", &link{wake: make(chan struct{}, 1)}, event(1)); err != nil {
		t.Fatal(err) // the server's own events live under the empty device
	}
	if err := r.deliver("", &pluxv1.DevServerFrame{}); err != nil {
		t.Fatal(err)
	}
	if err := r.deliver("d2", &pluxv1.DevServerFrame{}); err != nil {
		t.Fatal(err)
	}
	if got := len(r.devices["d1"].outbox); got != 1 {
		t.Errorf("d1 holds %d frames, want 1", got)
	}
	d2 := r.devices["d2"].outbox
	if len(d2) != 2 || d2[0].frame.GetSequence() != 1 || d2[1].frame.GetSequence() != 2 {
		t.Errorf("d2 frames %v, want numbered 1 and 2", d2)
	}
	if _, ok := r.devices[""]; ok && len(r.devices[""].outbox) != 0 {
		t.Error("a broadcast reached the server's own slot")
	}
}

// Verifies: DEV-021, LIM-001. The frames waiting for a device are bounded
// by the registry's limit: the oldest are dropped, the newest kept.
func TestDeliverDropsOldestFramesOverTheLimit_DEV_021(t *testing.T) {
	r := NewHub().room("s", &relayLimits{events: 2, bytes: 1 << 20})
	for range 5 {
		if err := r.deliver("d1", &pluxv1.DevServerFrame{}); err != nil {
			t.Fatal(err)
		}
	}
	box := r.devices["d1"].outbox
	if len(box) != 2 || box[0].frame.GetSequence() != 4 || box[1].frame.GetSequence() != 5 {
		t.Errorf("outbox holds %d frames from %d, want the newest two (4 and 5)", len(box), box[0].frame.GetSequence())
	}
}

// Verifies: DEV-020. A second socket of a device replaces the first, which
// is told to close; a stale link detaching later does not remove the new.
func TestAttachReplacesEarlierLink_DEV_020(t *testing.T) {
	r := NewHub().room("s", nil)
	first, _ := r.attach("d1")
	second, _ := r.attach("d1")
	if !first.replaced {
		t.Error("the first link was not told it was replaced")
	}
	select {
	case <-first.wake:
	default:
		t.Error("the first link's writer was not woken")
	}
	r.detach("d1", first)
	if r.devices["d1"].link != second {
		t.Error("a stale link's detach removed the current link")
	}
	r.detach("d1", second)
	if r.devices["d1"].link != nil {
		t.Error("the current link did not detach")
	}
}

// Verifies: DEV-021. A watcher that subscribes after the session ended is
// woken at once and finds the stream over once it has read what is kept.
func TestSubscribeAfterEnd_DEV_021(t *testing.T) {
	h := NewHub()
	r := h.room("s", nil)
	l, _ := r.attach("d1")
	if err := r.publish("d1", l, event(1)); err != nil {
		t.Fatal(err)
	}
	h.End("s", plxerr.DevSessionExpired)
	w, kept := r.subscribe(0, nil)
	if len(kept) != 1 {
		t.Fatalf("an ended session keeps %d events for the export, want 1", len(kept))
	}
	select {
	case <-w.wake:
	default:
		t.Error("a watcher of an ended session was not woken")
	}
	if _, ended := r.take(w); !ended {
		t.Error("the watcher does not see the end")
	}
}

// Verifies: DEV-021. The hub keeps the events of only endedRetained ended
// sessions for an export; the oldest ended relay is forgotten, a live one
// is never, and ending a session twice counts once.
func TestEndEvictsOldestEndedRooms_DEV_021(t *testing.T) {
	h := NewHub()
	h.room("live", nil)
	for i := range endedRetained + 1 {
		id := "ended-" + string(rune('A'+i))
		h.room(id, nil)
		h.End(id, plxerr.DevSessionExpired)
		h.End(id, plxerr.DevSessionExpired)
	}
	if _, ok := h.lookup("ended-A"); ok {
		t.Error("the oldest ended session is still retained")
	}
	if _, ok := h.lookup("ended-B"); !ok {
		t.Error("a recently ended session was dropped")
	}
	if _, ok := h.lookup("live"); !ok {
		t.Error("a live session was dropped")
	}
	h.End("unknown", plxerr.DevSessionExpired)
}
