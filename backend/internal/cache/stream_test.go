// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package cache

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeStream is one stream with one level of consumer groups: the part of
// Valkey's stream commands the backend uses.
type fakeStream struct {
	entries []fakeEntry
	groups  map[string]*fakeGroup
	// maxDeleted is the newest trimmed ID, first the first ID ever added.
	maxDeleted, first string
	// expires is when PEXPIRE makes the stream disappear; zero is never.
	expires time.Time
}

type fakeEntry struct {
	id      string
	payload string
}

type fakeGroup struct {
	// delivered is how many entries were handed out as new.
	delivered int
	// pending maps an entry ID to when it was delivered.
	pending map[string]time.Time
}

// streamCommand answers INCRBY and the stream commands; ok is false for
// any other command. The caller holds the lock.
func (s *respServer) streamCommand(name string, args []string) ([]byte, bool) {
	if s.streams == nil {
		s.streams = map[string]*fakeStream{}
	}
	switch name {
	case "INCRBY":
		v, _ := s.live(args[1])
		n, _ := strconv.ParseInt(string(v.data), 10, 64)
		add, _ := strconv.ParseInt(args[2], 10, 64)
		n += add
		expires := v.expires
		if expires.IsZero() {
			expires = time.Now().Add(time.Hour)
		}
		s.values[args[1]] = respValue{data: []byte(strconv.FormatInt(n, 10)), expires: expires}
		return []byte(fmt.Sprintf(":%d\r\n", n)), true
	case "XADD":
		st := s.stream(args[1])
		// XADD key [MAXLEN ~|= n] * field value
		i, maxLen := 2, 0
		if strings.EqualFold(args[i], "MAXLEN") {
			maxLen, _ = strconv.Atoi(args[4])
			i += 3
		}
		s.clock++
		id := fmt.Sprintf("%d-0", s.clock)
		if st.first == "" {
			st.first = id
		}
		st.entries = append(st.entries, fakeEntry{id: id, payload: args[i+2]})
		if drop := len(st.entries) - maxLen; maxLen > 0 && drop > 0 {
			st.maxDeleted = st.entries[drop-1].id
			st.entries = st.entries[drop:]
		}
		return bulk([]byte(id)), true
	case "XRANGE":
		// XRANGE key -|(id + COUNT n
		count, _ := strconv.Atoi(args[5])
		var out []fakeEntry
		if st := s.existingStream(args[1]); st != nil {
			after, exclusive := strings.CutPrefix(args[2], "(")
			for _, e := range st.entries {
				if exclusive && !mustLogID(after).less(mustLogID(e.id)) {
					continue
				}
				if len(out) < count {
					out = append(out, e)
				}
			}
		}
		return []byte(encodeEntries(out)), true
	case "XREVRANGE":
		// XREVRANGE key + - COUNT n
		count, _ := strconv.Atoi(args[5])
		var out []fakeEntry
		if st := s.existingStream(args[1]); st != nil {
			for i := len(st.entries) - 1; i >= 0 && len(out) < count; i-- {
				out = append(out, st.entries[i])
			}
		}
		return []byte(encodeEntries(out)), true
	case "XLEN":
		n := 0
		if st := s.existingStream(args[1]); st != nil {
			n = len(st.entries)
		}
		return []byte(fmt.Sprintf(":%d\r\n", n)), true
	case "XINFO":
		// XINFO STREAM key
		st := s.existingStream(args[2])
		if st == nil {
			return []byte("-ERR no such key\r\n"), true
		}
		maxDeleted := st.maxDeleted
		if maxDeleted == "" {
			maxDeleted = "0-0"
		}
		reply := fmt.Sprintf("*6\r\n$6\r\nlength\r\n:%d\r\n", len(st.entries))
		reply += string(bulk([]byte("max-deleted-entry-id"))) + string(bulk([]byte(maxDeleted)))
		reply += string(bulk([]byte("recorded-first-entry-id"))) + string(bulk([]byte(st.first)))
		return []byte(reply), true
	case "PEXPIRE":
		st := s.existingStream(args[1])
		if st == nil {
			return []byte(":0\r\n"), true
		}
		ms, _ := strconv.Atoi(args[2])
		st.expires = time.Now().Add(time.Duration(ms) * time.Millisecond)
		return []byte(":1\r\n"), true
	case "XGROUP":
		st := s.stream(args[2])
		if _, ok := st.groups[args[3]]; ok {
			return []byte("-BUSYGROUP Consumer Group name already exists\r\n"), true
		}
		st.groups[args[3]] = &fakeGroup{pending: map[string]time.Time{}}
		return []byte("+OK\r\n"), true
	case "XREADGROUP":
		// XREADGROUP GROUP g c COUNT n STREAMS key >
		st, g := s.stream(args[7]), s.stream(args[7]).groups[args[2]]
		count, _ := strconv.Atoi(args[5])
		var out []fakeEntry
		for g.delivered < len(st.entries) && len(out) < count {
			e := st.entries[g.delivered]
			g.delivered++
			g.pending[e.id] = time.Now()
			out = append(out, e)
		}
		if len(out) == 0 {
			return []byte("*-1\r\n"), true
		}
		return []byte(fmt.Sprintf("*1\r\n*2\r\n$%d\r\n%s\r\n%s", len(args[7]), args[7], encodeEntries(out))), true
	case "XAUTOCLAIM":
		// XAUTOCLAIM key g c min-idle start COUNT n
		st, g := s.stream(args[1]), s.stream(args[1]).groups[args[2]]
		idle, _ := strconv.Atoi(args[4])
		count, _ := strconv.Atoi(args[7])
		var out []fakeEntry
		for _, e := range st.entries {
			at, ok := g.pending[e.id]
			if ok && time.Since(at) >= time.Duration(idle)*time.Millisecond && len(out) < count {
				g.pending[e.id] = time.Now()
				out = append(out, e)
			}
		}
		return []byte("*3\r\n$3\r\n0-0\r\n" + encodeEntries(out) + "*0\r\n"), true
	case "XACK":
		g := s.stream(args[1]).groups[args[2]]
		n := 0
		for _, id := range args[3:] {
			if _, ok := g.pending[id]; ok {
				delete(g.pending, id)
				n++
			}
		}
		return []byte(fmt.Sprintf(":%d\r\n", n)), true
	}
	return nil, false
}

// existingStream returns the stream if it exists and has not expired.
func (s *respServer) existingStream(key string) *fakeStream {
	st, ok := s.streams[key]
	if !ok {
		return nil
	}
	if !st.expires.IsZero() && !st.expires.After(time.Now()) {
		delete(s.streams, key)
		return nil
	}
	return st
}

// mustLogID parses an ID the fake itself produced.
func mustLogID(id string) logID {
	parsed, _ := parseLogID(id)
	return parsed
}

func (s *respServer) stream(key string) *fakeStream {
	st := s.existingStream(key)
	if st == nil {
		st = &fakeStream{groups: map[string]*fakeGroup{}}
		s.streams[key] = st
	}
	return st
}

func encodeEntries(es []fakeEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(es))
	for _, e := range es {
		fmt.Fprintf(&b, "*2\r\n$%d\r\n%s\r\n*2\r\n$1\r\np\r\n$%d\r\n%s\r\n", len(e.id), e.id, len(e.payload), e.payload)
	}
	return b.String()
}

func TestValkeyStreamsDeliverOnceAndRedeliverUnacknowledged(t *testing.T) {
	t.Parallel()
	srv := newRESPServer(t)
	v, err := NewValkey("redis://" + srv.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = v.Close() }()
	ctx := t.Context()
	if err := v.StreamGroup(ctx, "events", "workers"); err != nil {
		t.Fatal(err)
	}
	if err := v.StreamGroup(ctx, "events", "workers"); err != nil {
		t.Fatalf("a group that exists is an error: %v", err)
	}
	for _, p := range []string{"a", "b with spaces", "c"} {
		if _, err := v.StreamAdd(ctx, "events", 1000, []byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := v.StreamRead(ctx, "events", "workers", "w1", 2)
	if err != nil || len(got) != 2 || string(got[1].Payload) != "b with spaces" {
		t.Fatalf("StreamRead = %v, %v", got, err)
	}
	rest, err := v.StreamRead(ctx, "events", "workers", "w2", 10)
	if err != nil || len(rest) != 1 || string(rest[0].Payload) != "c" {
		t.Fatalf("second consumer read %v, %v; want only the undelivered entry", rest, err)
	}
	if more, err := v.StreamRead(ctx, "events", "workers", "w2", 10); err != nil || len(more) != 0 {
		t.Fatalf("an empty stream returned %v, %v", more, err)
	}
	if err := v.StreamAck(ctx, "events", "workers", got[0].ID, rest[0].ID); err != nil {
		t.Fatal(err)
	}
	// The entry nobody acknowledged is claimed by another consumer.
	claimed, err := v.StreamClaim(ctx, "events", "workers", "w2", 0, 10)
	if err != nil || len(claimed) != 1 || string(claimed[0].Payload) != "b with spaces" {
		t.Fatalf("StreamClaim = %v, %v", claimed, err)
	}
	if err := v.StreamAck(ctx, "events", "workers", claimed[0].ID); err != nil {
		t.Fatal(err)
	}
	if again, _ := v.StreamClaim(ctx, "events", "workers", "w1", 0, 10); len(again) != 0 {
		t.Errorf("an acknowledged entry was claimed again: %v", again)
	}
}

func TestIncrementByCountsAndExpires(t *testing.T) {
	t.Parallel()
	srv := newRESPServer(t)
	v, err := NewValkey("redis://" + srv.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = v.Close() }()
	for _, tc := range []struct {
		c    Counter
		name string
	}{{v, "valkey"}, {NewMemory(nil), "memory"}} {
		n, err := tc.c.IncrementBy(t.Context(), "q", 5, time.Hour)
		if err != nil || n != 5 {
			t.Fatalf("%s: first = %d, %v", tc.name, n, err)
		}
		if n, _ = tc.c.IncrementBy(t.Context(), "q", 3, time.Hour); n != 8 {
			t.Errorf("%s: second = %d; want 8", tc.name, n)
		}
		if _, err := tc.c.IncrementBy(t.Context(), "q", 1, 0); err == nil {
			t.Errorf("%s: a counter without a lifetime was accepted", tc.name)
		}
		if _, err := tc.c.IncrementBy(t.Context(), "q", -1, time.Hour); err == nil {
			t.Errorf("%s: a negative increment was accepted", tc.name)
		}
	}
	if srv.count("EXPIRE") != 1 {
		t.Errorf("EXPIRE sent %d times; want once, when the counter is created", srv.count("EXPIRE"))
	}
}
