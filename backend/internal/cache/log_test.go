// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// logBackend is one implementation of Logs under test.
type logBackend struct {
	name string
	logs Logs
}

// logBackends returns the in-memory backend and Valkey on the fake server.
func logBackends(t *testing.T) []logBackend {
	t.Helper()
	srv := newRESPServer(t)
	v, err := NewValkey("redis://" + srv.addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v.Close() })
	return []logBackend{{"memory", NewMemory(nil)}, {"valkey", v}}
}

// appendAll appends payloads and returns their IDs.
func appendAll(t *testing.T, logs Logs, maxLen int64, payloads ...string) []string {
	t.Helper()
	ids := make([]string, 0, len(payloads))
	for _, p := range payloads {
		id, err := logs.LogAppend(t.Context(), "l", maxLen, time.Hour, []byte(p))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

// payloads returns the payloads of entries.
func payloads(entries []StreamEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, string(e.Payload))
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestLogsReadInOrderAndResume_DEV_021_STU_075 covers appending, reading
// oldest first, resuming after a cursor and the count limit.
//
// Verifies: DEV-021, STU-075.
func TestLogsReadInOrderAndResume_DEV_021_STU_075(t *testing.T) {
	t.Parallel()
	for _, b := range logBackends(t) {
		ctx := t.Context()
		ids := appendAll(t, b.logs, 100, "a", "b with spaces", "c", "d")
		for i := 1; i < len(ids); i++ {
			if !mustLogID(ids[i-1]).less(mustLogID(ids[i])) {
				t.Fatalf("%s: IDs do not rise: %v", b.name, ids)
			}
		}
		for _, start := range []string{"", "0"} {
			got, err := b.logs.LogRead(ctx, "l", start, 10)
			if err != nil || !equalStrings(payloads(got), []string{"a", "b with spaces", "c", "d"}) {
				t.Errorf("%s: read from %q = %v, %v", b.name, start, payloads(got), err)
			}
		}
		got, err := b.logs.LogRead(ctx, "l", ids[0], 2)
		if err != nil || !equalStrings(payloads(got), []string{"b with spaces", "c"}) || got[1].ID != ids[2] {
			t.Errorf("%s: resume after the first, two = %v, %v", b.name, got, err)
		}
		if got, err := b.logs.LogRead(ctx, "l", ids[3], 10); err != nil || len(got) != 0 {
			t.Errorf("%s: read after the newest = %v, %v", b.name, got, err)
		}
		if last, err := b.logs.LogLast(ctx, "l"); err != nil || last != ids[3] {
			t.Errorf("%s: LogLast = %q, %v; want %q", b.name, last, err, ids[3])
		}
		if _, err := b.logs.LogRead(ctx, "l", "not-an-id", 10); err == nil {
			t.Errorf("%s: a malformed cursor was accepted", b.name)
		}
		if _, err := b.logs.LogRead(ctx, "l", "", 0); err == nil {
			t.Errorf("%s: a read of zero entries was accepted", b.name)
		}
		if _, err := b.logs.LogAppend(ctx, "l", 0, time.Hour, nil); err == nil {
			t.Errorf("%s: an unbounded log was accepted", b.name)
		}
		if _, err := b.logs.LogAppend(ctx, "l", 10, -time.Second, nil); err == nil {
			t.Errorf("%s: a negative lifetime was accepted", b.name)
		}
	}
}

// TestLogsReportAGapOnlyPastTheWatermark_DEV_021_STU_075 shows that a
// reader learns it fell behind exactly when entries it has not read were
// trimmed, or the log it read is gone.
//
// Verifies: DEV-021, STU-075.
func TestLogsReportAGapOnlyPastTheWatermark_DEV_021_STU_075(t *testing.T) {
	t.Parallel()
	for _, b := range logBackends(t) {
		ctx := t.Context()
		ids := appendAll(t, b.logs, 3, "a", "b", "c", "d", "e")
		// a and b were trimmed: b is the watermark.
		if _, err := b.logs.LogRead(ctx, "l", ids[0], 10); !errors.Is(err, ErrLogGap) {
			t.Errorf("%s: a cursor before the watermark read %v; want ErrLogGap", b.name, err)
		}
		got, err := b.logs.LogRead(ctx, "l", ids[1], 10)
		if err != nil || !equalStrings(payloads(got), []string{"c", "d", "e"}) {
			t.Errorf("%s: a cursor at the watermark read %v, %v; nothing it missed was trimmed", b.name, payloads(got), err)
		}
		if got, err := b.logs.LogRead(ctx, "l", "", 10); err != nil || !equalStrings(payloads(got), []string{"c", "d", "e"}) {
			t.Errorf("%s: a read from the start = %v, %v", b.name, payloads(got), err)
		}
		if err := b.logs.LogDelete(ctx, "l"); err != nil {
			t.Fatal(err)
		}
		if last, err := b.logs.LogLast(ctx, "l"); err != nil || last != "" {
			t.Errorf("%s: LogLast of a deleted log = %q, %v", b.name, last, err)
		}
		if _, err := b.logs.LogRead(ctx, "l", ids[4], 10); !errors.Is(err, ErrLogGap) {
			t.Errorf("%s: a cursor into a deleted log read %v; want ErrLogGap", b.name, err)
		}
		if got, err := b.logs.LogRead(ctx, "l", "", 10); err != nil || len(got) != 0 {
			t.Errorf("%s: a missing log read from the start = %v, %v", b.name, got, err)
		}
		// A new incarnation of the log does not satisfy an old cursor.
		time.Sleep(5 * time.Millisecond)
		appendAll(t, b.logs, 3, "new")
		if _, err := b.logs.LogRead(ctx, "l", ids[0], 10); !errors.Is(err, ErrLogGap) {
			t.Errorf("%s: a cursor from a deleted incarnation read %v; want ErrLogGap", b.name, err)
		}
		if err := b.logs.LogDelete(ctx, "absent"); err != nil {
			t.Errorf("%s: deleting an absent log = %v", b.name, err)
		}
	}
}

// TestValkeyLogsSetTheLifetime_DEV_021 shows the lifetime is set on every
// append that has one, and kept when the append has none.
//
// Verifies: DEV-021.
func TestValkeyLogsSetTheLifetime_DEV_021(t *testing.T) {
	t.Parallel()
	srv := newRESPServer(t)
	v, err := NewValkey("redis://" + srv.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = v.Close() }()
	for _, ttl := range []time.Duration{time.Minute, 0, time.Microsecond} {
		if _, err := v.LogAppend(t.Context(), "l", 10, ttl, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if got := srv.count("PEXPIRE"); got != 2 {
		t.Errorf("PEXPIRE sent %d times; want 2", got)
	}
	if millis(time.Microsecond) != "1" || millis(1500*time.Microsecond) != "2" || millis(time.Second) != "1000" {
		t.Error("millis does not round a lifetime up to whole milliseconds")
	}
}

// TestMemoryLogsExpire_DEV_021 shows an in-memory log expiring ttl after
// its last append, and a log without a lifetime keeping it.
//
// Verifies: DEV-021.
func TestMemoryLogsExpire_DEV_021(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	m := NewMemory(func() time.Time { return now })
	ctx := t.Context()
	first, _ := m.LogAppend(ctx, "l", 10, time.Minute, []byte("a"))
	now = now.Add(50 * time.Second)
	second, _ := m.LogAppend(ctx, "l", 10, 0, []byte("b"))
	if !mustLogID(first).less(mustLogID(second)) {
		t.Errorf("IDs do not rise: %s, %s", first, second)
	}
	now = now.Add(9 * time.Second)
	if last, _ := m.LogLast(ctx, "l"); last != second {
		t.Errorf("the log expired early: LogLast = %q", last)
	}
	now = now.Add(time.Second)
	if last, _ := m.LogLast(ctx, "l"); last != "" {
		t.Errorf("the log outlived its lifetime: LogLast = %q", last)
	}
	// The same millisecond still gives rising IDs.
	a, _ := m.LogAppend(ctx, "same", 10, 0, nil)
	b, _ := m.LogAppend(ctx, "same", 10, 0, nil)
	if !mustLogID(a).less(mustLogID(b)) {
		t.Errorf("IDs in one millisecond do not rise: %s, %s", a, b)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.LogAppend(ctx, "l", 10, 0, nil); !errors.Is(err, ErrClosed) {
		t.Errorf("a closed cache appended: %v", err)
	}
	if _, err := m.LogRead(ctx, "l", "", 1); !errors.Is(err, ErrClosed) {
		t.Errorf("a closed cache read: %v", err)
	}
}

// quietNotifier is a Notifier that never delivers a notice, as when every
// notice is lost; a test can still raise a subscription's generation.
type quietNotifier struct {
	mu   sync.Mutex
	subs []*inbox
}

func (*quietNotifier) Publish(context.Context, string, []byte) error {
	return errors.New("notices are lost")
}

func (q *quietNotifier) Subscribe(context.Context, string) (Subscription, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	b := newInbox()
	q.subs = append(q.subs, b)
	return quietSubscription{b}, nil
}

// raise raises the generation of every subscription, as a reconnection
// does.
func (q *quietNotifier) raise() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, b := range q.subs {
		b.setLive(false)
		b.setLive(true)
	}
}

func (q *quietNotifier) subscribed() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.subs) > 0
}

type quietSubscription struct{ *inbox }

func (quietSubscription) Close() error { return nil }

// follow runs Follow in the background, sending each payload to the
// returned channel; the error channel receives what Follow returned.
func follow(ctx context.Context, logs Logs, n Notifier, afterID string, poll time.Duration, fn func(StreamEntry) error) (<-chan string, <-chan error) {
	got := make(chan string, 100)
	done := make(chan error, 1)
	go func() {
		done <- Follow(ctx, logs, n, "l", afterID, poll, 2, func(e StreamEntry) error {
			if fn != nil {
				if err := fn(e); err != nil {
					return err
				}
			}
			got <- string(e.Payload)
			return nil
		})
	}()
	return got, done
}

// expect receives the payloads in order or fails.
func expect(t *testing.T, got <-chan string, want ...string) {
	t.Helper()
	for _, w := range want {
		select {
		case g := <-got:
			if g != w {
				t.Fatalf("followed %q; want %q", g, w)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%q did not arrive", w)
		}
	}
}

// TestFollowDeliversOnNotice_DEV_021_STU_075 shows a follower woken by the
// notice of each append, delivering in order, resuming after its cursor,
// and stopping when its context ends.
//
// Verifies: DEV-021, STU-075.
func TestFollowDeliversOnNotice_DEV_021_STU_075(t *testing.T) {
	t.Parallel()
	m := NewMemory(nil)
	w := LogWriter{Logs: m, Notifier: m, MaxLen: 100, TTL: time.Hour}
	seen, err := w.Append(t.Context(), "l", []byte("seen"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	// The poll never fires: only the notices wake the follower.
	got, done := follow(ctx, m, m, seen, time.Hour, nil)
	for _, p := range []string{"a", "b", "c", "d", "e"} {
		if _, err := w.Append(t.Context(), "l", []byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	expect(t, got, "a", "b", "c", "d", "e")
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("Follow returned %v after cancel; want context.Canceled", err)
	}
}

// TestFollowRecoversLostNotices_DEV_021_STU_075 shows a follower that
// receives no notice at all still delivering, by its poll and when its
// subscription's generation rises.
//
// Verifies: DEV-021, STU-075.
func TestFollowRecoversLostNotices_DEV_021_STU_075(t *testing.T) {
	t.Parallel()
	m := NewMemory(nil)
	q := &quietNotifier{}
	w := LogWriter{Logs: m, Notifier: q, MaxLen: 100}
	got, _ := follow(t.Context(), m, q, "", 10*time.Millisecond, nil)
	if _, err := w.Append(t.Context(), "l", []byte("polled")); err != nil {
		t.Fatalf("a lost notice failed the append: %v", err)
	}
	expect(t, got, "polled")

	q2 := &quietNotifier{}
	got2, _ := follow(t.Context(), m, q2, "", time.Hour, nil)
	expect(t, got2, "polled")
	for !q2.subscribed() {
		time.Sleep(time.Millisecond)
	}
	if _, err := (LogWriter{Logs: m, MaxLen: 100}).Append(t.Context(), "l", []byte("raised")); err != nil {
		t.Fatal(err)
	}
	q2.raise()
	expect(t, got2, "raised")
}

// TestFollowReturnsErrors_DEV_021_STU_075 shows Follow ending with fn's
// error, and with ErrLogGap when the reader fell behind.
//
// Verifies: DEV-021, STU-075.
func TestFollowReturnsErrors_DEV_021_STU_075(t *testing.T) {
	t.Parallel()
	m := NewMemory(nil)
	ids := appendAll(t, m, 2, "a", "b", "c", "d")
	stop := errors.New("stop")
	_, done := follow(t.Context(), m, m, "", time.Hour, func(e StreamEntry) error {
		if string(e.Payload) == "d" {
			return stop
		}
		return nil
	})
	if err := <-done; !errors.Is(err, stop) {
		t.Errorf("Follow returned %v; want fn's error", err)
	}
	_, done = follow(t.Context(), m, m, ids[0], time.Hour, nil)
	if err := <-done; !errors.Is(err, ErrLogGap) {
		t.Errorf("Follow returned %v; want ErrLogGap", err)
	}
	if err := Follow(t.Context(), m, m, "l", "", 0, 1, nil); err == nil {
		t.Error("a zero poll interval was accepted")
	}
}
