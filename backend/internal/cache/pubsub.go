// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package cache

import (
	"context"
	"sync"
	"time"
)

// Notifier carries small change notices between replicas on named
// channels (ADR-0086). Delivery is at most once and best effort: a notice
// can be lost while a replica is cut off, so a subscriber never trusts the
// absence of a notice and reads Subscription.State to learn when it may
// have missed one.
type Notifier interface {
	// Publish sends a message to every subscriber of a channel, on every
	// replica.
	Publish(ctx context.Context, channel string, message []byte) error
	// Subscribe starts receiving the messages published on a channel. It
	// does not wait for the backend: a subscription whose backend is out
	// of reach is not live and keeps trying to become so.
	Subscribe(ctx context.Context, channel string) (Subscription, error)
}

// Subscription is the receiving end of a channel. Its owner closes it.
type Subscription interface {
	// Messages delivers the messages received, and is closed by Close.
	Messages() <-chan []byte
	// State reports whether the subscription is live, and its generation.
	// The generation rises whenever a message may have been lost: after
	// every reconnection and every message dropped because the reader was
	// slow. Whatever a reader learned under an earlier generation may be
	// out of date.
	State() (live bool, generation uint64)
	// Changes receives a signal whenever the state changes: the
	// subscription goes live or down, or its generation rises. Signals
	// coalesce; read State after one.
	Changes() <-chan struct{}
	// Close ends the subscription and releases its resources.
	Close() error
}

var (
	_ Notifier = (*Valkey)(nil)
	_ Notifier = (*Memory)(nil)
)

// inboxSize is how many messages a subscription holds for a slow reader
// before it drops them.
const inboxSize = 64

// inbox is the part of a subscription the backends share: the message
// channel, and the live flag and generation State reports.
type inbox struct {
	messages chan []byte
	changes  chan struct{}

	mu   sync.Mutex
	live bool
	gen  uint64
}

// newInbox returns an empty inbox that is not live.
func newInbox() *inbox {
	return &inbox{messages: make(chan []byte, inboxSize), changes: make(chan struct{}, 1)}
}

// Messages delivers the messages received.
func (b *inbox) Messages() <-chan []byte { return b.messages }

// Changes signals that the state changed.
func (b *inbox) Changes() <-chan struct{} { return b.changes }

// signal tells the reader the state changed, unless a signal is waiting.
func (b *inbox) signal() {
	select {
	case b.changes <- struct{}{}:
	default:
	}
}

// State reports whether the inbox is live, and its generation.
func (b *inbox) State() (live bool, generation uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.live, b.gen
}

// setLive records that the backend delivers (true) or no longer does
// (false). Delivery resuming is a new generation: what was published in
// between was not seen.
func (b *inbox) setLive(live bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if live == b.live {
		return
	}
	if live {
		b.gen++
	}
	b.live = live
	b.signal()
}

// deliver queues a message for the reader; a message that does not fit is
// dropped and the generation rises, so that the reader knows.
func (b *inbox) deliver(message []byte) {
	select {
	case b.messages <- message:
	default:
		b.mu.Lock()
		b.gen++
		b.signal()
		b.mu.Unlock()
	}
}

// memorySubscription is a subscription to the in-process channels.
type memorySubscription struct {
	*inbox
	m       *Memory
	channel string
}

// Close ends the subscription.
func (s *memorySubscription) Close() error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	if _, ok := s.m.subs[s.channel][s]; !ok {
		return nil
	}
	delete(s.m.subs[s.channel], s)
	s.setLive(false)
	close(s.messages)
	return nil
}

// Publish sends a message to the subscribers of a channel in this process.
func (m *Memory) Publish(_ context.Context, channel string, message []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	for s := range m.subs[channel] {
		s.deliver(append([]byte(nil), message...))
	}
	return nil
}

// Subscribe starts receiving the messages published on a channel in this
// process. It is live at once.
func (m *Memory) Subscribe(_ context.Context, channel string) (Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	if m.subs == nil {
		m.subs = map[string]map[*memorySubscription]struct{}{}
	}
	if m.subs[channel] == nil {
		m.subs[channel] = map[*memorySubscription]struct{}{}
	}
	s := &memorySubscription{inbox: newInbox(), m: m, channel: channel}
	s.setLive(true)
	m.subs[channel][s] = struct{}{}
	return s, nil
}

// closeSubscriptions ends every subscription; the caller holds m.mu.
func (m *Memory) closeSubscriptions() {
	for _, group := range m.subs {
		for s := range group {
			s.setLive(false)
			close(s.messages)
		}
	}
	m.subs = nil
}

// Delays between attempts to re-establish a Valkey subscription: the
// first, doubling up to the longest.
const (
	resubscribeFirst   = 100 * time.Millisecond
	resubscribeLongest = 5 * time.Second
)

// Publish sends a message to the subscribers of a channel on every
// replica.
func (v *Valkey) Publish(ctx context.Context, channel string, message []byte) error {
	_, err := v.call(ctx, "PUBLISH", channel, string(message))
	return err
}

// Subscribe starts receiving the messages published on a channel. A
// goroutine owned by the subscription keeps one connection subscribed and
// reconnects, with growing pauses, when it breaks; Close stops it.
func (v *Valkey) Subscribe(ctx context.Context, channel string) (Subscription, error) {
	v.mu.Lock()
	closed := v.closed
	v.mu.Unlock()
	if closed {
		return nil, ErrClosed
	}
	// The subscription outlives the call that made it; it keeps the values
	// of ctx and ends with Close.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := &valkeySubscription{inbox: newInbox(), v: v, channel: channel, stop: make(chan struct{}), done: make(chan struct{}), cancel: cancel}
	go s.run(runCtx)
	return s, nil
}

// valkeySubscription is a subscription held on a connection of its own.
type valkeySubscription struct {
	*inbox
	v       *Valkey
	channel string

	stop   chan struct{}
	done   chan struct{}
	cancel context.CancelFunc

	mu     sync.Mutex
	conn   *conn
	closed bool
}

// Close stops the subscription and waits for its goroutine.
func (s *valkeySubscription) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.stop)
	s.cancel()
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.mu.Unlock()
	<-s.done
	return nil
}

// run keeps the subscription connected until it is closed.
func (s *valkeySubscription) run(ctx context.Context) {
	defer close(s.done)
	defer close(s.messages)
	delay := resubscribeFirst
	for {
		if s.listen(ctx) {
			delay = resubscribeFirst
		}
		s.setLive(false)
		timer := time.NewTimer(delay)
		select {
		case <-s.stop:
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, resubscribeLongest)
	}
}

// listen subscribes on a new connection and delivers messages until the
// connection breaks or the subscription is closed. It reports whether it
// was live at all.
func (s *valkeySubscription) listen(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, s.v.timeout)
	defer cancel()
	c, err := s.v.dialNew(ctx)
	if err != nil {
		return false
	}
	if !s.adopt(c) {
		return false
	}
	defer func() { _ = c.Close() }()
	if _, err := s.v.do(ctx, c, "SUBSCRIBE", s.channel); err != nil {
		return false
	}
	if err := c.SetDeadline(time.Time{}); err != nil {
		return false
	}
	s.setLive(true)
	for {
		reply, err := readReply(c.r)
		if err != nil {
			return true
		}
		if message, ok := publishedMessage(reply); ok {
			s.deliver(message)
		}
	}
}

// adopt makes c the connection Close shuts to interrupt a read, unless the
// subscription is already closed.
func (s *valkeySubscription) adopt(c *conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		_ = c.Close()
		return false
	}
	s.conn = c
	return true
}

// publishedMessage returns the payload of a pushed message reply
// ["message", channel, payload]; the confirmations and anything else a
// subscribed connection may receive are not messages.
func publishedMessage(reply any) ([]byte, bool) {
	parts, ok := reply.([]any)
	if !ok || len(parts) != 3 {
		return nil, false
	}
	kind, _ := parts[0].([]byte)
	payload, isBytes := parts[2].([]byte)
	return payload, isBytes && string(kind) == "message"
}
