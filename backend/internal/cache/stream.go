// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package cache

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

// streamField is the one field of every stream entry: the payload.
const streamField = "p"

// StreamEntry is one entry of a stream.
type StreamEntry struct {
	// ID is the entry's identifier, which acknowledges it.
	ID      string
	Payload []byte
}

// Streams is the queue the Valkey backend offers beside the cache
// (ADR-0080): producers append payloads, a consumer group delivers each
// entry to one consumer, and an entry stays pending until it is
// acknowledged, so a consumer that dies loses nothing. Delivery is
// at-least-once; consumers must be idempotent.
//
// The in-memory backend does not implement it: a single-node
// installation writes directly instead.
type Streams interface {
	// StreamAdd appends a payload, keeping the stream to about maxLen
	// entries, and returns the entry's ID.
	StreamAdd(ctx context.Context, stream string, maxLen int64, payload []byte) (string, error)
	// StreamGroup creates the consumer group, and the stream when it
	// does not exist, reading from the start. A group that exists is
	// not an error.
	StreamGroup(ctx context.Context, stream, group string) error
	// StreamRead returns up to count entries never delivered to the
	// group. It does not block.
	StreamRead(ctx context.Context, stream, group, consumer string, count int) ([]StreamEntry, error)
	// StreamClaim takes over up to count entries that were delivered
	// to any consumer of the group and not acknowledged for minIdle,
	// including the caller's own after a restart.
	StreamClaim(ctx context.Context, stream, group, consumer string, minIdle time.Duration, count int) ([]StreamEntry, error)
	// StreamAck acknowledges entries.
	StreamAck(ctx context.Context, stream, group string, ids ...string) error
}

// Counter is a cache that can add more than one to a counter, which is
// how a quota of events rather than of calls is counted.
type Counter interface {
	// IncrementBy adds n to a counter, setting the lifetime when it is
	// created, and returns the new value.
	IncrementBy(ctx context.Context, key string, n int64, ttl time.Duration) (int64, error)
}

var (
	_ Streams = (*Valkey)(nil)
	_ Counter = (*Valkey)(nil)
	_ Counter = (*Memory)(nil)
)

// IncrementBy adds n to a counter and sets its lifetime when it is
// created.
func (v *Valkey) IncrementBy(ctx context.Context, key string, n int64, ttl time.Duration) (int64, error) {
	if ttl <= 0 {
		return 0, errors.New("cache: a counter needs a lifetime")
	}
	if n < 0 {
		return 0, errors.New("cache: a counter only grows")
	}
	reply, err := v.call(ctx, "INCRBY", key, strconv.FormatInt(n, 10))
	if err != nil {
		return 0, err
	}
	total, ok := reply.(int64)
	if !ok {
		return 0, errors.New("cache: INCRBY did not return a number")
	}
	if total == n {
		if _, err := v.call(ctx, "EXPIRE", key, seconds(ttl)); err != nil {
			return total, err
		}
	}
	return total, nil
}

// IncrementBy adds n to a counter.
func (m *Memory) IncrementBy(_ context.Context, key string, n int64, ttl time.Duration) (int64, error) {
	if ttl <= 0 {
		return 0, errors.New("cache: a counter needs a lifetime")
	}
	if n < 0 {
		return 0, errors.New("cache: a counter only grows")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, ErrClosed
	}
	var total int64
	expires := m.now().Add(ttl)
	if e, ok := m.live(key); ok {
		parsed, err := strconv.ParseInt(string(e.value), 10, 64)
		if err != nil {
			return 0, errors.New("cache: " + key + " does not hold a counter")
		}
		total, expires = parsed, e.expires
	}
	total += n
	m.sweep()
	m.entries[key] = entry{value: []byte(strconv.FormatInt(total, 10)), expires: expires}
	return total, nil
}

// StreamAdd appends a payload to a stream.
func (v *Valkey) StreamAdd(ctx context.Context, stream string, maxLen int64, payload []byte) (string, error) {
	args := []string{"XADD", stream}
	if maxLen > 0 {
		args = append(args, "MAXLEN", "~", strconv.FormatInt(maxLen, 10))
	}
	reply, err := v.call(ctx, append(args, "*", streamField, string(payload))...)
	if err != nil {
		return "", err
	}
	id, ok := reply.([]byte)
	if !ok {
		return "", errors.New("cache: XADD did not return an ID")
	}
	return string(id), nil
}

// StreamGroup creates a consumer group.
func (v *Valkey) StreamGroup(ctx context.Context, stream, group string) error {
	_, err := v.call(ctx, "XGROUP", "CREATE", stream, group, "0", "MKSTREAM")
	var re replyError
	if errors.As(err, &re) && strings.HasPrefix(re.msg, "BUSYGROUP") {
		return nil
	}
	return err
}

// StreamRead returns entries never delivered to the group.
func (v *Valkey) StreamRead(ctx context.Context, stream, group, consumer string, count int) ([]StreamEntry, error) {
	reply, err := v.call(ctx, "XREADGROUP", "GROUP", group, consumer, "COUNT", strconv.Itoa(count), "STREAMS", stream, ">")
	if err != nil {
		return nil, err
	}
	if reply == nil {
		return nil, nil
	}
	streams, ok := reply.([]any)
	if !ok || len(streams) != 1 {
		return nil, errors.New("cache: XREADGROUP returned an unexpected reply")
	}
	pair, ok := streams[0].([]any)
	if !ok || len(pair) != 2 {
		return nil, errors.New("cache: XREADGROUP returned an unexpected reply")
	}
	return parseEntries(pair[1])
}

// StreamClaim takes over idle pending entries.
func (v *Valkey) StreamClaim(ctx context.Context, stream, group, consumer string, minIdle time.Duration, count int) ([]StreamEntry, error) {
	reply, err := v.call(ctx, "XAUTOCLAIM", stream, group, consumer, strconv.FormatInt(minIdle.Milliseconds(), 10), "0-0", "COUNT", strconv.Itoa(count))
	if err != nil {
		return nil, err
	}
	parts, ok := reply.([]any)
	if !ok || len(parts) < 2 {
		return nil, errors.New("cache: XAUTOCLAIM returned an unexpected reply")
	}
	return parseEntries(parts[1])
}

// StreamAck acknowledges entries.
func (v *Valkey) StreamAck(ctx context.Context, stream, group string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := v.call(ctx, append([]string{"XACK", stream, group}, ids...)...)
	return err
}

// parseEntries reads a list of [id, [field, value, ...]] entries. An
// entry deleted from the stream while pending has a null body and is
// skipped.
func parseEntries(v any) ([]StreamEntry, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, nil
	}
	out := make([]StreamEntry, 0, len(list))
	for _, item := range list {
		pair, ok := item.([]any)
		if !ok || len(pair) != 2 {
			return nil, errors.New("cache: a stream entry has an unexpected shape")
		}
		id, ok := pair[0].([]byte)
		if !ok {
			return nil, errors.New("cache: a stream entry has no ID")
		}
		fields, ok := pair[1].([]any)
		if !ok {
			continue
		}
		for i := 0; i+1 < len(fields); i += 2 {
			name, _ := fields[i].([]byte)
			val, _ := fields[i+1].([]byte)
			if string(name) == streamField {
				out = append(out, StreamEntry{ID: string(id), Payload: val})
			}
		}
	}
	return out, nil
}
