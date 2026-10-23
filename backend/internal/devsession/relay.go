// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"slices"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// endedRetained is how many ended sessions the hub keeps, so that a trace
// of a session that just ended can still be exported; the oldest goes
// first.
const endedRetained = 32

// relayLimits are the bounds of one session's relay, read from the limits
// registry when the session's relay opens (LIM-001).
type relayLimits struct {
	// events and bytes bound every buffer of the relay: the events kept
	// per device, the frames waiting for a device and the queue of each
	// watcher (devtools.streamBufferEvents, devtools.streamBufferBytes).
	events int64
	bytes  int64
	// eventBytes bounds one device event (devtools.eventBytes).
	eventBytes int64
	// commands bounds the commands per minute (devSession.commandsPerMinute).
	commands int64
}

// relayLimitsOf reads the relay's bounds from a limit set.
func relayLimitsOf(set limits.Set) relayLimits {
	return relayLimits{
		events: set.Get(limits.DevtoolsStreamBufferEvents), bytes: set.Get(limits.DevtoolsStreamBufferBytes),
		eventBytes: set.Get(limits.DevtoolsEventBytes), commands: set.Get(limits.DevSessionCommandsPerMinute),
	}
}

// over reports whether a buffer of n entries and size bytes exceeds the
// bounds; a buffer always keeps its newest entry.
func (l relayLimits) over(n int, size int64) bool {
	return n > 1 && (int64(n) > l.events || size > l.bytes)
}

// Hub relays the events of the dev sessions this process serves, from
// their devices to their watchers, and the server's frames to the devices
// (DEV-020, DEV-021, ADR-0090). Every buffer is bounded and drops its
// oldest entry when full, so a slow reader never blocks a device.
type Hub struct {
	mu       sync.Mutex
	rooms    map[string]*room
	ended    []string
	defaults relayLimits
}

// NewHub returns an empty hub whose sessions take the registry's default
// bounds until their own limits are read.
func NewHub() *Hub {
	return &Hub{rooms: map[string]*room{}, defaults: relayLimitsOf(limits.Defaults())}
}

// room returns a session's relay, opening it with lim, or with the
// defaults when lim is nil; an open relay takes lim as its new bounds.
func (h *Hub) room(sessionID string, lim *relayLimits) *room {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[sessionID]
	if !ok {
		r = &room{lim: h.defaults, devices: map[string]*relayDevice{}, watchers: map[*watcher]struct{}{}}
		h.rooms[sessionID] = r
	}
	if lim != nil {
		r.mu.Lock()
		r.lim = *lim
		r.mu.Unlock()
	}
	return r
}

// lookup returns a session's relay when this process holds one.
func (h *Hub) lookup(sessionID string) (*room, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[sessionID]
	return r, ok
}

// End ends a session's relay: each connected device is told with code and
// its socket closed, each watcher's stream ends after what it holds. The
// retained events stay for an export until endedRetained later sessions
// have ended.
func (h *Hub) End(sessionID string, code plxerr.Code) {
	h.mu.Lock()
	r, ok := h.rooms[sessionID]
	if ok && !slices.Contains(h.ended, sessionID) {
		h.ended = append(h.ended, sessionID)
		if len(h.ended) > endedRetained {
			delete(h.rooms, h.ended[0])
			h.ended = h.ended[1:]
		}
	}
	h.mu.Unlock()
	if ok {
		r.end(code)
	}
}

// Deliver queues a server frame for a device of a session, or for every
// device the relay knows when deviceID is empty. Each device numbers its
// frames from 1 and keeps them until the device's hello shows them carried
// out, so a device that reconnects resumes where it stopped (DEV-021). It
// fails with PLX-9200 once the session has ended.
func (h *Hub) Deliver(sessionID, deviceID string, frame *pluxv1.DevServerFrame) error {
	return h.room(sessionID, nil).deliver(deviceID, frame)
}

// retained is an event with its encoded size.
type retained struct {
	ev   *pluxv1.DevEvent
	size int64
}

// outFrame is a numbered server frame waiting for its device.
type outFrame struct {
	frame *pluxv1.DevServerFrame
	size  int64
}

// room is the relay of one session. Its events are shared, read-only,
// between the retained buffers and the watchers' queues.
type room struct {
	mu       sync.Mutex
	lim      relayLimits
	seq      uint64
	devices  map[string]*relayDevice
	watchers map[*watcher]struct{}
	ended    bool
	endCode  plxerr.Code
	// window and sent count the commands of the current minute.
	window time.Time
	sent   int64
}

// relayDevice is the relay's state of one device of the session.
type relayDevice struct {
	// events are the device's retained events, oldest first.
	events     []retained
	eventBytes int64
	// lastDeviceSeq is the newest device sequence relayed: a resent event
	// at or below it is acknowledged again and not relayed twice.
	lastDeviceSeq uint64
	// outSeq numbers the server frames; outbox holds those the device has
	// not yet shown carried out.
	outSeq   uint64
	outbox   []outFrame
	outBytes int64
	link     *link
}

// link is one open socket of a device, served by one writer.
type link struct {
	wake chan struct{}
	// sent is the last numbered frame written on the link.
	sent uint64
	// ack, when ackDue, is the device sequence to acknowledge next.
	ack    uint64
	ackDue bool
	// replaced is set when a newer socket of the device took over.
	replaced bool
}

// signal wakes a writer or a watcher without blocking.
func signal(c chan struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
}

// device returns a device's state, creating it; the caller holds the lock.
func (r *room) device(id string) *relayDevice {
	d, ok := r.devices[id]
	if !ok {
		d = &relayDevice{}
		r.devices[id] = d
	}
	return d
}

// attach makes a new socket the device's link; an earlier one is told to
// close. It fails with PLX-9200 once the session has ended.
func (r *room) attach(deviceID string) (*link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ended {
		return nil, expired()
	}
	d := r.device(deviceID)
	if d.link != nil {
		d.link.replaced = true
		signal(d.link.wake)
	}
	l := &link{wake: make(chan struct{}, 1)}
	d.link = l
	return l, nil
}

// detach forgets a link that closed, unless a newer one replaced it.
func (r *room) detach(deviceID string, l *link) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d, ok := r.devices[deviceID]; ok && d.link == l {
		d.link = nil
	}
}

// hello resumes a device after its hello: the frames it carried out leave
// the outbox, the rest go out again, and events it resends are
// acknowledged without being relayed twice.
func (r *room) hello(deviceID string, l *link, h *pluxv1.DevDeviceHello) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.device(deviceID)
	done := h.GetLastServerSequence()
	d.outSeq = max(d.outSeq, done)
	kept := d.outbox[:0]
	d.outBytes = 0
	for _, f := range d.outbox {
		if f.frame.GetSequence() > done {
			kept = append(kept, f)
			d.outBytes += f.size
		}
	}
	d.outbox = kept
	l.sent = done
	d.lastDeviceSeq = max(d.lastDeviceSeq, h.GetLastAcknowledgedEvent())
	if d.lastDeviceSeq > h.GetLastAcknowledgedEvent() {
		l.ack, l.ackDue = d.lastDeviceSeq, true
	}
	signal(l.wake)
}

// duplicate reports whether a device event was relayed before.
func (r *room) duplicate(deviceID string, deviceSeq uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return deviceSeq <= r.device(deviceID).lastDeviceSeq
}

// publish relays a device event: it is stamped with the session's next
// sequence and the device, retained, queued for every watcher and
// acknowledged to the device. A resent event is only acknowledged. It
// fails with PLX-9200 once the session has ended.
func (r *room) publish(deviceID string, l *link, ev *pluxv1.DevEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ended {
		return expired()
	}
	d := r.device(deviceID)
	deviceSeq := ev.GetSequence()
	if deviceSeq > d.lastDeviceSeq {
		d.lastDeviceSeq = deviceSeq
		ev.DeviceId = deviceID
		r.relay(d, ev)
	}
	l.ack, l.ackDue = d.lastDeviceSeq, true
	signal(l.wake)
	return nil
}

// refuse acknowledges a device event the relay will not carry, such as one
// over devtools.eventBytes, and relays the drop in its place (PLX-9209).
func (r *room) refuse(deviceID string, l *link, deviceSeq uint64, reason pluxv1.DevDropReason) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ended {
		return expired()
	}
	d := r.device(deviceID)
	if deviceSeq > d.lastDeviceSeq {
		d.lastDeviceSeq = deviceSeq
		r.relay(nil, &pluxv1.DevEvent{Payload: &pluxv1.DevEvent_Drop{Drop: &pluxv1.DropEvent{Dropped: 1, Reason: reason}}})
	}
	l.ack, l.ackDue = d.lastDeviceSeq, true
	signal(l.wake)
	return nil
}

// relay stamps an event with the next sequence, retains it with its
// device (d is nil for an event of the server's) and queues it for the
// watchers; the caller holds the lock.
func (r *room) relay(d *relayDevice, ev *pluxv1.DevEvent) {
	r.seq++
	ev.Sequence = r.seq
	e := retained{ev: ev, size: int64(proto.Size(ev))}
	if d == nil {
		d = r.device("")
	}
	d.events = append(d.events, e)
	d.eventBytes += e.size
	for r.lim.over(len(d.events), d.eventBytes) {
		d.eventBytes -= d.events[0].size
		d.events = d.events[1:]
	}
	for w := range r.watchers {
		w.push(e, r.lim)
	}
}

// snapshot returns the retained events after a sequence, of the devices
// the filter allows, in sequence order; the caller holds the lock.
func (r *room) snapshot(after uint64, devices map[string]bool) []*pluxv1.DevEvent {
	var out []*pluxv1.DevEvent
	for id, d := range r.devices {
		if !allows(devices, id) {
			continue
		}
		for _, e := range d.events {
			if e.ev.GetSequence() > after {
				out = append(out, e.ev)
			}
		}
	}
	slices.SortFunc(out, func(a, b *pluxv1.DevEvent) int {
		return compareSeq(a.GetSequence(), b.GetSequence())
	})
	return out
}

// compareSeq orders two sequences.
func compareSeq(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// allows says whether a filter of devices lets a device's events through;
// an empty filter allows all, and the server's own events always pass.
func allows(devices map[string]bool, deviceID string) bool {
	return len(devices) == 0 || deviceID == "" || devices[deviceID]
}

// retainedEvents returns the retained events of the devices a filter
// allows, in sequence order.
func (r *room) retainedEvents(devices map[string]bool) []*pluxv1.DevEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshot(0, devices)
}

// subscribe adds a watcher of the devices a filter allows and returns it
// with the retained events after a sequence, taken in the same instant so
// that nothing is missed or repeated.
func (r *room) subscribe(after uint64, devices map[string]bool) (*watcher, []*pluxv1.DevEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w := &watcher{devices: devices, wake: make(chan struct{}, 1)}
	r.watchers[w] = struct{}{}
	if r.ended {
		signal(w.wake)
	}
	return w, r.snapshot(after, devices)
}

// unsubscribe removes a watcher.
func (r *room) unsubscribe(w *watcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.watchers, w)
}

// take empties a watcher's queue: a drop event first when events were
// dropped since the last take, then the queued events in order. ended is
// true once the session ended and nothing is left.
func (r *room) take(w *watcher) (events []*pluxv1.DevEvent, ended bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w.dropped > 0 {
		events = append(events, &pluxv1.DevEvent{Sequence: w.lastDropped, Payload: &pluxv1.DevEvent_Drop{Drop: &pluxv1.DropEvent{
			Dropped: w.dropped, Reason: pluxv1.DevDropReason_DEV_DROP_REASON_BUFFER_FULL,
		}}})
		w.dropped, w.lastDropped = 0, 0
	}
	for _, e := range w.queue {
		events = append(events, e.ev)
	}
	w.queue, w.bytes = nil, 0
	return events, r.ended && len(events) == 0
}

// deliver queues a frame for one device, or for every device the relay
// knows when deviceID is empty.
func (r *room) deliver(deviceID string, frame *pluxv1.DevServerFrame) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ended {
		return expired()
	}
	targets := []string{deviceID}
	if deviceID == "" {
		targets = targets[:0]
		for id := range r.devices {
			if id != "" {
				targets = append(targets, id)
			}
		}
	}
	for _, id := range targets {
		d := r.device(id)
		d.outSeq++
		f := proto.CloneOf(frame)
		f.Sequence = d.outSeq
		d.outbox = append(d.outbox, outFrame{frame: f, size: int64(proto.Size(f))})
		d.outBytes += d.outbox[len(d.outbox)-1].size
		for r.lim.over(len(d.outbox), d.outBytes) {
			d.outBytes -= d.outbox[0].size
			d.outbox = d.outbox[1:]
		}
		if d.link != nil {
			signal(d.link.wake)
		}
	}
	return nil
}

// pending returns what a link's writer sends next: an acknowledgement when
// one is due, then the numbered frames after the last it wrote. ended is
// the session's end code once it ended; replaced is true when a newer
// socket took over.
func (r *room) pending(deviceID string, l *link) (frames []*pluxv1.DevServerFrame, ended plxerr.Code, replaced bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l.replaced {
		return nil, 0, true
	}
	if l.ackDue {
		frames = append(frames, &pluxv1.DevServerFrame{Payload: &pluxv1.DevServerFrame_Ack{Ack: &pluxv1.DevEventAck{UpTo: l.ack}}})
		l.ackDue = false
	}
	for _, f := range r.device(deviceID).outbox {
		if f.frame.GetSequence() > l.sent {
			frames = append(frames, f.frame)
			l.sent = f.frame.GetSequence()
		}
	}
	if r.ended {
		ended = r.endCode
	}
	return frames, ended, false
}

// allowCommand counts a command against devSession.commandsPerMinute and
// reports whether it is within it.
func (r *room) allowCommand(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now.Sub(r.window) >= time.Minute || now.Before(r.window) {
		r.window, r.sent = now, 0
	}
	if r.sent >= r.lim.commands {
		return false
	}
	r.sent++
	return true
}

// end marks the relay ended and wakes every link and watcher.
func (r *room) end(code plxerr.Code) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ended {
		return
	}
	r.ended, r.endCode = true, code
	for _, d := range r.devices {
		if d.link != nil {
			signal(d.link.wake)
		}
	}
	for w := range r.watchers {
		signal(w.wake)
	}
}

// isEnded reports whether the relay ended.
func (r *room) isEnded() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ended
}

// watcher is one stream of a session's events, with its own bounded queue.
type watcher struct {
	devices map[string]bool
	queue   []retained
	bytes   int64
	// dropped counts the events dropped since the last take, and
	// lastDropped is the sequence of the newest of them.
	dropped     uint64
	lastDropped uint64
	wake        chan struct{}
}

// push queues an event the filter allows, dropping the oldest while the
// queue is over its bounds; the room's lock is held.
func (w *watcher) push(e retained, lim relayLimits) {
	if !allows(w.devices, e.ev.GetDeviceId()) {
		return
	}
	w.queue = append(w.queue, e)
	w.bytes += e.size
	for lim.over(len(w.queue), w.bytes) {
		w.dropped++
		w.lastDropped = w.queue[0].ev.GetSequence()
		w.bytes -= w.queue[0].size
		w.queue = w.queue[1:]
	}
	signal(w.wake)
}
