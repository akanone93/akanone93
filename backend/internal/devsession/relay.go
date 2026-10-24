// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// endedRetained is how many ended sessions the hub keeps, so that a trace
// of a session that just ended can still be exported; the oldest goes
// first.
const endedRetained = 32

// endGraceIntervals is how many follower intervals the frame log of an
// ended session outlives its end record, so that every replica reads it.
const endGraceIntervals = 4

// SharedLayer is the shared cache a dev session's state lives in, so that
// every api replica serves every session (ADR-0087 revision 2026-10-24,
// ADR-0096): the event and frame logs, and the counters that number them.
// cache.Memory serves a single-node installation, cache.Valkey a cluster.
type SharedLayer interface {
	cache.Logs
	cache.Notifier
	cache.Counter
}

// relayLimits are the bounds of one session's relay, read from the limits
// registry when the session's relay opens (LIM-001).
type relayLimits struct {
	// events and bytes bound every buffer of the relay: the length of the
	// session's logs, the events kept per device, the frames waiting for
	// a device and the queue of each watcher (devtools.streamBufferEvents,
	// devtools.streamBufferBytes).
	events int64
	bytes  int64
	// eventBytes bounds one device event (devtools.eventBytes).
	eventBytes int64
	// commands bounds the commands per minute (devSession.commandsPerMinute).
	commands int64
	// lifetime is a session's longest life (devSession.lifetimeMinutes),
	// the lifetime of its logs when its expiry is not known.
	lifetime time.Duration
	// expires is the session's expiry when the caller knows it.
	expires time.Time
}

// relayLimitsOf reads the relay's bounds from a limit set.
func relayLimitsOf(set limits.Set) relayLimits {
	return relayLimits{
		events: set.Get(limits.DevtoolsStreamBufferEvents), bytes: set.Get(limits.DevtoolsStreamBufferBytes),
		eventBytes: set.Get(limits.DevtoolsEventBytes), commands: set.Get(limits.DevSessionCommandsPerMinute),
		lifetime: time.Duration(set.Get(limits.DevSessionLifetimeMinutes)) * time.Minute,
	}
}

// over reports whether a buffer of n entries and size bytes exceeds the
// bounds; a buffer always keeps its newest entry.
func (l relayLimits) over(n int, size int64) bool {
	return n > 1 && (int64(n) > l.events || size > l.bytes)
}

// Hub relays the events of dev sessions from their devices to their
// watchers, and the server's frames to the devices (DEV-020, DEV-021,
// ADR-0090). The state of a session lives in two logs on the shared layer:
// its events, numbered with the session sequence, and its frames, each
// naming its device and that device's frame number. Every replica follows
// the logs of the sessions it holds sockets or watchers for and keeps a
// bounded projection of them; it delivers only to the sockets it holds.
// Every buffer drops its oldest entry when full, so a slow reader never
// blocks a device.
type Hub struct {
	// ctx is the replica's lifetime: every log read and every follower
	// ends with it.
	ctx      context.Context //nolint:containedctx // the replica's lifetime, which ends every follower
	shared   SharedLayer
	follow   bool
	poll     time.Duration
	mu       sync.Mutex
	rooms    map[string]*room
	ended    []string
	defaults relayLimits
}

// NewHub returns a hub whose state lives in a private memory layer: the
// relay of a single process, which no other replica shares.
func NewHub() *Hub {
	return newHub(context.Background(), cache.NewMemory(nil), false, defaultPingInterval)
}

// NewSharedHub returns a hub whose state lives on a shared layer, one per
// api replica. Its followers read the logs when the layer announces an
// append and at least every poll; all of them end with ctx.
func NewSharedHub(ctx context.Context, shared SharedLayer, poll time.Duration) *Hub {
	if poll <= 0 {
		poll = defaultPingInterval
	}
	return newHub(ctx, shared, true, poll)
}

func newHub(ctx context.Context, shared SharedLayer, follow bool, poll time.Duration) *Hub {
	return &Hub{ctx: ctx, shared: shared, follow: follow, poll: poll, rooms: map[string]*room{}, defaults: relayLimitsOf(limits.Defaults())}
}

// Log names and counter keys of a session on the shared layer.
func eventsLog(sessionID string) string { return "devsession:" + sessionID + ":events" }
func framesLog(sessionID string) string { return "devsession:" + sessionID + ":frames" }
func seqKey(sessionID string) string    { return "devsession:" + sessionID + ":seq" }
func frameSeqKey(sessionID, deviceID string) string {
	return "devsession:" + sessionID + ":frameseq:" + deviceID
}

func commandsKey(sessionID string, window int64) string {
	return "devsession:" + sessionID + ":commands:" + strconv.FormatInt(window, 10)
}

// newRoom returns an empty projection of a session's logs.
func (h *Hub) newRoom(sessionID string, lim relayLimits) *room {
	return &room{h: h, id: sessionID, lim: lim, fresh: true, devices: map[string]*relayDevice{}, watchers: map[*watcher]struct{}{}}
}

// room returns a session's relay, opening it with lim, or with the
// defaults when lim is nil; an open relay takes lim as its new bounds.
func (h *Hub) room(sessionID string, lim *relayLimits) *room {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[sessionID]
	if !ok {
		r = h.newRoom(sessionID, h.defaults)
		h.rooms[sessionID] = r
	}
	if lim != nil {
		r.mu.Lock()
		next := *lim
		if next.expires.IsZero() {
			next.expires = r.lim.expires
		}
		r.lim = next
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

// End ends a session on every replica: an end record in its frame log
// closes the sockets each replica holds, and the session's logs go (the
// frame log once every follower had time to read the record). Here each
// connected device is told with code and its socket closed, and each
// watcher's stream ends after what it holds. The projection stays for an
// export until endedRetained later sessions have ended.
func (h *Hub) End(sessionID string, code plxerr.Code) {
	lim := h.defaults
	if r, ok := h.lookup(sessionID); ok {
		lim = r.limits()
	}
	// A failure leaves the logs to their lifetime; the replicas that hold
	// sockets find the end through the database.
	w := cache.LogWriter{Logs: h.shared, Notifier: h.shared, MaxLen: lim.events, TTL: endGraceIntervals * h.poll}
	_, _ = w.Append(h.ctx, framesLog(sessionID), endRecord(code))
	_ = h.shared.LogDelete(h.ctx, eventsLog(sessionID))
	h.endLocal(sessionID, code)
}

// endLocal ends this replica's relay of a session.
func (h *Hub) endLocal(sessionID string, code plxerr.Code) {
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
// frames from 1 on the shared layer and its frames stay in the session's
// frame log until the device's hello shows them carried out, so a device
// that reconnects, to any replica, resumes where it stopped (DEV-021). It
// fails with PLX-9200 once the session has ended.
func (h *Hub) Deliver(sessionID, deviceID string, frame *pluxv1.DevServerFrame) error {
	return h.room(sessionID, nil).deliver(deviceID, frame)
}

// allowCommand counts a command against devSession.commandsPerMinute,
// shared by every replica, and reports whether it is within it.
func (h *Hub) allowCommand(sessionID string, lim relayLimits, now time.Time) (bool, error) {
	n, err := h.shared.IncrementBy(h.ctx, commandsKey(sessionID, now.Unix()/60), 1, time.Minute)
	if err != nil {
		return false, fmt.Errorf("devsession: count the command: %w", err)
	}
	return n <= lim.commands, nil
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

// room is one replica's relay of one session: a projection of the
// session's logs and the sockets and watchers the replica holds. Its
// events are shared, read-only, between the retained buffers and the
// watchers' queues.
type room struct {
	h  *Hub
	id string
	// ingest serialises reading the logs; the cursors and fresh are
	// guarded by it.
	ingest   sync.Mutex
	evCursor string
	frCursor string
	// fresh is set until the first event read after the start or a gap,
	// which tells how many events went before it.
	fresh bool

	mu       sync.Mutex
	lim      relayLimits
	seq      uint64
	devices  map[string]*relayDevice
	watchers map[*watcher]struct{}
	ended    bool
	endCode  plxerr.Code
	// trimmedUpTo is the newest sequence no longer retained; replayUpTo
	// the newest relayed before the logs were read again after a gap.
	trimmedUpTo uint64
	replayUpTo  uint64
	// users counts the sockets and watchers that keep the follower
	// running; stop ends it.
	users int
	stop  func()
}

// relayDevice is the relay's state of one device of the session.
type relayDevice struct {
	// events are the device's retained events, oldest first.
	events     []retained
	eventBytes int64
	// lastDeviceSeq is the newest device sequence relayed: a resent event
	// at or below it is acknowledged again and not relayed twice.
	lastDeviceSeq uint64
	// outSeq is the newest frame number, done the newest the device showed
	// carried out; outbox holds the frames after done, in number order.
	outSeq   uint64
	done     uint64
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

// limits returns the room's bounds.
func (r *room) limits() relayLimits {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lim
}

// ttl is the lifetime the session's logs and counters get with each write:
// what is left of the session, or its longest life when that is unknown.
func (r *room) ttl() time.Duration {
	lim := r.limits()
	if left := time.Until(lim.expires); !lim.expires.IsZero() && left > 0 {
		return left
	}
	if lim.lifetime > 0 {
		return lim.lifetime
	}
	return r.h.defaults.lifetime
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

// use counts a socket or watcher of the room and starts following the
// session's logs on a shared hub; release undoes it, and the last release
// stops the follower.
func (r *room) use() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users++
	if r.users > 1 || !r.h.follow || r.ended {
		return
	}
	ctx, cancel := context.WithCancel(r.h.ctx)
	done := make(chan struct{})
	r.stop = func() { cancel(); <-done }
	go r.follow(ctx, done)
}

// release ends one use of the room.
func (r *room) release() {
	r.mu.Lock()
	r.users--
	var stop func()
	if r.users == 0 {
		stop, r.stop = r.stop, nil
	}
	r.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// follow reads the session's logs whenever the shared layer announces an
// append, and at least every poll, until ctx ends or the session does.
func (r *room) follow(ctx context.Context, done chan struct{}) {
	defer close(done)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var subs []cache.Subscription
	for _, log := range []string{eventsLog(r.id), framesLog(r.id)} {
		if sub, err := r.h.shared.Subscribe(ctx, cache.LogChannel(log)); err == nil {
			subs = append(subs, sub)
			defer func() { _ = sub.Close() }()
		}
	}
	wake := make(chan struct{}, 1)
	for _, sub := range subs {
		go relayNotices(ctx, sub, wake)
	}
	tick := time.NewTicker(r.h.poll)
	defer tick.Stop()
	for {
		// A failed read is retried with the next notice or tick.
		_ = r.catchUp()
		if r.isEnded() {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-tick.C:
		}
	}
}

// relayNotices turns a subscription's messages and changes into wakes,
// until ctx ends or the subscription closes.
func relayNotices(ctx context.Context, sub cache.Subscription, wake chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-sub.Messages():
			if !ok {
				return
			}
		case <-sub.Changes():
		}
		signal(wake)
	}
}

// catchUp reads what the session's logs hold beyond the room's cursors
// into the room.
func (r *room) catchUp() error {
	r.ingest.Lock()
	defer r.ingest.Unlock()
	if err := r.read(eventsLog(r.id), &r.evCursor, r.applyEvent); err != nil {
		return err
	}
	return r.read(framesLog(r.id), &r.frCursor, r.applyFrame)
}

// read applies the entries of a log after its cursor. A log that is gone
// ended the session elsewhere; one trimmed past the cursor is read again
// from its oldest entry, which the room applies once.
func (r *room) read(log string, cursor *string, apply func([]byte)) error {
	batch := int(max(r.limits().events, 1))
	for {
		entries, err := r.h.shared.LogRead(r.h.ctx, log, *cursor, batch)
		if errors.Is(err, cache.ErrLogGap) {
			last, lastErr := r.h.shared.LogLast(r.h.ctx, log)
			if lastErr != nil {
				return fmt.Errorf("devsession: read the session's log: %w", lastErr)
			}
			*cursor, r.fresh = "", true
			r.mu.Lock()
			r.replayUpTo = r.seq
			r.mu.Unlock()
			if last == "" {
				r.end(plxerr.DevSessionExpired)
				r.h.endLocal(r.id, plxerr.DevSessionExpired)
				return nil
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("devsession: read the session's log: %w", err)
		}
		for _, e := range entries {
			apply(e.Payload)
			*cursor = e.ID
		}
		if len(entries) < batch {
			return nil
		}
	}
}

// applyEvent takes an event record into the room; an event it applied
// before, read again after a gap, is not relayed twice.
func (r *room) applyEvent(payload []byte) {
	deviceID, deviceSeq, ev, err := decodeEvent(payload)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.device(deviceID)
	d.lastDeviceSeq = max(d.lastDeviceSeq, deviceSeq)
	seq := ev.GetSequence()
	if r.fresh {
		r.fresh = false
		if seq > r.seq+1 {
			r.trimmedUpTo = max(r.trimmedUpTo, seq-1)
		}
	}
	if seq <= r.replayUpTo {
		return
	}
	r.seq = max(r.seq, seq)
	r.relay(ev)
}

// applyFrame takes a frame record into the room: a frame for a device, a
// device's hello or the session's end.
func (r *room) applyFrame(payload []byte) {
	rec, err := decodeFrame(payload)
	if err != nil {
		return
	}
	if rec.kind == recordEnd {
		r.end(rec.code)
		r.h.endLocal(r.id, rec.code)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.device(rec.device)
	switch rec.kind {
	case recordHello:
		d.done = max(d.done, rec.done)
		d.outSeq = max(d.outSeq, rec.done)
		kept := d.outbox[:0]
		d.outBytes = 0
		for _, f := range d.outbox {
			if f.frame.GetSequence() > d.done {
				kept = append(kept, f)
				d.outBytes += f.size
			}
		}
		d.outbox = kept
	case recordFrame:
		n := rec.frame.GetSequence()
		if n <= d.done || slices.ContainsFunc(d.outbox, func(f outFrame) bool { return f.frame.GetSequence() == n }) {
			return
		}
		d.outSeq = max(d.outSeq, n)
		d.outbox = append(d.outbox, outFrame{frame: rec.frame, size: int64(proto.Size(rec.frame))})
		slices.SortStableFunc(d.outbox, func(a, b outFrame) int {
			return compareSeq(a.frame.GetSequence(), b.frame.GetSequence())
		})
		d.outBytes = 0
		for _, f := range d.outbox {
			d.outBytes += f.size
		}
		for r.lim.over(len(d.outbox), d.outBytes) {
			d.outBytes -= d.outbox[0].size
			d.outbox = d.outbox[1:]
		}
		if d.link != nil {
			signal(d.link.wake)
		}
	}
}

// append writes a record to one of the session's logs.
func (r *room) append(log string, payload []byte) error {
	w := cache.LogWriter{Logs: r.h.shared, Notifier: r.h.shared, MaxLen: max(r.limits().events, 1), TTL: r.ttl()}
	if _, err := w.Append(r.h.ctx, log, payload); err != nil {
		return fmt.Errorf("devsession: %w", err)
	}
	return nil
}

// appendEvent stamps an event with the session's next sequence and writes
// it to the event log with the device and the device's own sequence.
func (r *room) appendEvent(deviceID string, deviceSeq uint64, ev *pluxv1.DevEvent) error {
	seq, err := r.h.shared.IncrementBy(r.h.ctx, seqKey(r.id), 1, r.ttl())
	if err != nil {
		return fmt.Errorf("devsession: number the event: %w", err)
	}
	ev.Sequence = uint64(seq) //nolint:gosec // a counter that only grows from 1
	payload, err := encodeEvent(deviceID, deviceSeq, ev)
	if err != nil {
		return err
	}
	if err := r.append(eventsLog(r.id), payload); err != nil {
		return err
	}
	return r.catchUp()
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

// hello resumes a device after its hello, on whichever replica it
// reconnected to: a hello record in the frame log tells every replica the
// frames it carried out, the rest go out again, and events it resends are
// acknowledged without being relayed twice.
func (r *room) hello(deviceID string, l *link, h *pluxv1.DevDeviceHello) error {
	done := h.GetLastServerSequence()
	// Frames numbered after a lost counter still follow what the device
	// carried out.
	key := frameSeqKey(r.id, deviceID)
	if cur, err := r.h.shared.IncrementBy(r.h.ctx, key, 0, r.ttl()); err == nil && cur >= 0 && uint64(cur) < done {
		if _, err := r.h.shared.IncrementBy(r.h.ctx, key, int64(done-uint64(cur)), r.ttl()); err != nil { //nolint:gosec // frame numbers stay far below 2^63
			return fmt.Errorf("devsession: resume the device: %w", err)
		}
	}
	if err := r.append(framesLog(r.id), helloRecord(deviceID, done)); err != nil {
		return err
	}
	if err := r.catchUp(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.device(deviceID)
	l.sent = done
	d.lastDeviceSeq = max(d.lastDeviceSeq, h.GetLastAcknowledgedEvent())
	if d.lastDeviceSeq > h.GetLastAcknowledgedEvent() {
		l.ack, l.ackDue = d.lastDeviceSeq, true
	}
	signal(l.wake)
	return nil
}

// duplicate reports whether a device event was relayed before, by any
// replica.
func (r *room) duplicate(deviceID string, deviceSeq uint64) bool {
	_ = r.catchUp() // publish reads again and reports a failure
	r.mu.Lock()
	defer r.mu.Unlock()
	return deviceSeq <= r.device(deviceID).lastDeviceSeq
}

// accept reads the logs and reports whether a device event is new; it
// fails with PLX-9200 once the session has ended.
func (r *room) accept(deviceID string, deviceSeq uint64) (bool, error) {
	if err := r.catchUp(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ended {
		return false, expired()
	}
	return deviceSeq > r.device(deviceID).lastDeviceSeq, nil
}

// acknowledge queues the acknowledgement of a device's events on its link.
func (r *room) acknowledge(deviceID string, l *link) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l.ack, l.ackDue = r.device(deviceID).lastDeviceSeq, true
	signal(l.wake)
}

// publish relays a device event: it is stamped with the session's next
// sequence and the device, written to the event log, which every
// replica's watchers read, and acknowledged to the device. A resent event
// is only acknowledged. It fails with PLX-9200 once the session has ended.
func (r *room) publish(deviceID string, l *link, ev *pluxv1.DevEvent) error {
	deviceSeq := ev.GetSequence()
	fresh, err := r.accept(deviceID, deviceSeq)
	if err != nil {
		return err
	}
	if fresh {
		ev.DeviceId = deviceID
		if err := r.appendEvent(deviceID, deviceSeq, ev); err != nil {
			return err
		}
	}
	r.acknowledge(deviceID, l)
	return nil
}

// refuse acknowledges a device event the relay will not carry, such as one
// over devtools.eventBytes, and relays the drop in its place (PLX-9209).
func (r *room) refuse(deviceID string, l *link, deviceSeq uint64, reason pluxv1.DevDropReason) error {
	fresh, err := r.accept(deviceID, deviceSeq)
	if err != nil {
		return err
	}
	if fresh {
		drop := &pluxv1.DevEvent{Payload: &pluxv1.DevEvent_Drop{Drop: &pluxv1.DropEvent{Dropped: 1, Reason: reason}}}
		if err := r.appendEvent(deviceID, deviceSeq, drop); err != nil {
			return err
		}
	}
	r.acknowledge(deviceID, l)
	return nil
}

// relay retains a stamped event with its device (the empty device holds
// the server's own events) and queues it for the watchers; the caller
// holds the lock.
func (r *room) relay(ev *pluxv1.DevEvent) {
	e := retained{ev: ev, size: int64(proto.Size(ev))}
	d := r.device(ev.GetDeviceId())
	d.events = append(d.events, e)
	d.eventBytes += e.size
	for r.lim.over(len(d.events), d.eventBytes) {
		r.trimmedUpTo = max(r.trimmedUpTo, d.events[0].ev.GetSequence())
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
// that nothing is missed or repeated. A cursor older than what the session
// still holds, on any replica, is answered with a drop event first.
func (r *room) subscribe(after uint64, devices map[string]bool) (*watcher, []*pluxv1.DevEvent) {
	_ = r.catchUp() // a failed read is retried by the follower
	r.mu.Lock()
	defer r.mu.Unlock()
	w := &watcher{devices: devices, wake: make(chan struct{}, 1)}
	r.watchers[w] = struct{}{}
	if r.ended {
		signal(w.wake)
	}
	backlog := r.snapshot(after, devices)
	if after > 0 && after < r.trimmedUpTo {
		drop := &pluxv1.DevEvent{Sequence: r.trimmedUpTo, Payload: &pluxv1.DevEvent_Drop{Drop: &pluxv1.DropEvent{
			Dropped: r.trimmedUpTo - after, Reason: pluxv1.DevDropReason_DEV_DROP_REASON_BUFFER_FULL,
		}}}
		backlog = append([]*pluxv1.DevEvent{drop}, backlog...)
	}
	return w, backlog
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

// deliver writes a frame for one device, or for every device the session's
// logs know when deviceID is empty, to the frame log, numbered by the
// device's shared counter.
func (r *room) deliver(deviceID string, frame *pluxv1.DevServerFrame) error {
	if err := r.catchUp(); err != nil {
		return err
	}
	r.mu.Lock()
	if r.ended {
		r.mu.Unlock()
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
		slices.Sort(targets)
	}
	r.mu.Unlock()
	for _, id := range targets {
		n, err := r.h.shared.IncrementBy(r.h.ctx, frameSeqKey(r.id, id), 1, r.ttl())
		if err != nil {
			return fmt.Errorf("devsession: number the frame: %w", err)
		}
		f := proto.CloneOf(frame)
		f.Sequence = uint64(n) //nolint:gosec // a counter that only grows from 1
		payload, err := frameRecord(id, f)
		if err != nil {
			return err
		}
		if err := r.append(framesLog(r.id), payload); err != nil {
			return err
		}
	}
	return r.catchUp()
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
