// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package cache

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Logs are append-only, bounded, resumable streams shared by every replica
// (ADR-0096): readers resume after the last entry ID they saw.
//
// Unlike Streams, a log has no consumer groups: every reader sees every
// entry it has not seen yet, and keeps its own cursor.
type Logs interface {
	// LogAppend appends payload, trims the log to about maxLen entries, sets the
	// log to expire ttl after this append (0 keeps it), and returns the entry ID.
	LogAppend(ctx context.Context, log string, maxLen int64, ttl time.Duration, payload []byte) (string, error)
	// LogRead returns up to count entries after afterID ("" or "0" = from the start),
	// oldest first. It fails with ErrLogGap when entries after afterID were trimmed
	// (afterID is older than the newest trimmed entry), or when the log was
	// deleted or expired since afterID was read, so the reader reloads.
	LogRead(ctx context.Context, log, afterID string, count int) ([]StreamEntry, error)
	// LogLast returns the newest entry ID, "" for an empty or missing log.
	LogLast(ctx context.Context, log string) (string, error)
	// LogDelete removes the log. Deleting what is absent succeeds.
	LogDelete(ctx context.Context, log string) error
}

// ErrLogGap reports that a reader fell behind a log: entries it has not
// read are gone, so it must reload its state and resume from LogLast.
var ErrLogGap = errors.New("cache: entries after the cursor were trimmed")

var (
	_ Logs = (*Valkey)(nil)
	_ Logs = (*Memory)(nil)
)

// logID is a parsed entry ID: Valkey's "<unix-ms>-<seq>".
type logID struct{ ms, seq uint64 }

// parseLogID reads an entry ID; a bare number has sequence zero.
func parseLogID(s string) (logID, error) {
	msPart, seqPart, hasSeq := strings.Cut(s, "-")
	ms, err := strconv.ParseUint(msPart, 10, 64)
	if err != nil {
		return logID{}, fmt.Errorf("cache: %q is not a log entry ID", s)
	}
	var seq uint64
	if hasSeq {
		if seq, err = strconv.ParseUint(seqPart, 10, 64); err != nil {
			return logID{}, fmt.Errorf("cache: %q is not a log entry ID", s)
		}
	}
	return logID{ms: ms, seq: seq}, nil
}

// less reports whether a is older than b.
func (a logID) less(b logID) bool {
	return a.ms < b.ms || (a.ms == b.ms && a.seq < b.seq)
}

// String formats the ID the way Valkey does.
func (a logID) String() string {
	return strconv.FormatUint(a.ms, 10) + "-" + strconv.FormatUint(a.seq, 10)
}

// cursor parses a reader's afterID; start is true when the reader reads
// from the start of the log and no gap can be detected.
func cursor(afterID string) (after logID, start bool, err error) {
	if afterID == "" || afterID == "0" {
		return logID{}, true, nil
	}
	after, err = parseLogID(afterID)
	return after, false, err
}

// checkLogArgs refuses an unbounded log or a negative lifetime.
func checkLogArgs(maxLen int64, ttl time.Duration) error {
	if maxLen <= 0 {
		return errors.New("cache: a log needs a length bound")
	}
	if ttl < 0 {
		return errors.New("cache: a log lifetime cannot be negative")
	}
	return nil
}

// LogAppend appends a payload to a log, trimming it to exactly maxLen
// entries; Valkey records the newest trimmed ID, which LogRead checks.
func (v *Valkey) LogAppend(ctx context.Context, log string, maxLen int64, ttl time.Duration, payload []byte) (string, error) {
	if err := checkLogArgs(maxLen, ttl); err != nil {
		return "", err
	}
	reply, err := v.call(ctx, "XADD", log, "MAXLEN", "=", strconv.FormatInt(maxLen, 10), "*", streamField, string(payload))
	if err != nil {
		return "", err
	}
	id, ok := reply.([]byte)
	if !ok {
		return "", errors.New("cache: XADD did not return an ID")
	}
	if ttl > 0 {
		if _, err := v.call(ctx, "PEXPIRE", log, millis(ttl)); err != nil {
			return string(id), err
		}
	}
	return string(id), nil
}

// LogRead returns entries after a cursor. It reads the entries first and
// the trim watermark after, so a trim racing the read reports a gap rather
// than hiding one.
func (v *Valkey) LogRead(ctx context.Context, log, afterID string, count int) ([]StreamEntry, error) {
	if count <= 0 {
		return nil, errors.New("cache: a log read needs a positive count")
	}
	after, start, err := cursor(afterID)
	if err != nil {
		return nil, err
	}
	from := "-"
	if !start {
		from = "(" + after.String()
	}
	reply, err := v.call(ctx, "XRANGE", log, from, "+", "COUNT", strconv.Itoa(count))
	if err != nil {
		return nil, err
	}
	entries, err := parseEntries(reply)
	if err != nil || start {
		return entries, err
	}
	trimmed, first, exists, err := v.logBounds(ctx, log)
	if err != nil {
		return nil, err
	}
	if !exists || after.less(trimmed) || after.less(first) {
		return nil, ErrLogGap
	}
	return entries, nil
}

// logBounds returns the newest ID ever trimmed from a log and the first ID
// ever added to it, which identifies this incarnation of the log: a cursor
// older than it was read from a log since deleted or expired. Both need
// Valkey 7 or later.
func (v *Valkey) logBounds(ctx context.Context, log string) (trimmed, first logID, exists bool, err error) {
	reply, err := v.call(ctx, "XINFO", "STREAM", log)
	var re replyError
	if errors.As(err, &re) && strings.Contains(re.msg, "no such key") {
		return logID{}, logID{}, false, nil
	}
	if err != nil {
		return logID{}, logID{}, false, err
	}
	fields, ok := reply.([]any)
	if !ok {
		return logID{}, logID{}, false, errors.New("cache: XINFO STREAM returned an unexpected reply")
	}
	var haveTrimmed, haveFirst bool
	for i := 0; i+1 < len(fields); i += 2 {
		name, _ := fields[i].([]byte)
		value, _ := fields[i+1].([]byte)
		switch string(name) {
		case "max-deleted-entry-id":
			if trimmed, err = parseLogID(string(value)); err != nil {
				return logID{}, logID{}, false, err
			}
			haveTrimmed = true
		case "recorded-first-entry-id":
			if first, err = parseLogID(string(value)); err != nil {
				return logID{}, logID{}, false, err
			}
			haveFirst = true
		}
	}
	if !haveTrimmed || !haveFirst {
		return logID{}, logID{}, false, errors.New("cache: the server does not report trimmed log entries; logs need Valkey 7 or later")
	}
	return trimmed, first, true, nil
}

// LogLast returns the newest entry ID of a log.
func (v *Valkey) LogLast(ctx context.Context, log string) (string, error) {
	reply, err := v.call(ctx, "XREVRANGE", log, "+", "-", "COUNT", "1")
	if err != nil {
		return "", err
	}
	entries, err := parseEntries(reply)
	if err != nil || len(entries) == 0 {
		return "", err
	}
	return entries[0].ID, nil
}

// LogDelete removes a log.
func (v *Valkey) LogDelete(ctx context.Context, log string) error {
	_, err := v.call(ctx, "DEL", log)
	return err
}

// millis renders a lifetime in milliseconds, rounding up so that a short
// lifetime does not become none.
func millis(ttl time.Duration) string {
	ms := int64(ttl / time.Millisecond)
	if ttl%time.Millisecond != 0 || ms < 1 {
		ms++
	}
	return strconv.FormatInt(ms, 10)
}

// memoryLog is one log of the in-memory backend.
type memoryLog struct {
	entries []memoryLogEntry
	// first is the first ID ever added to this incarnation of the log,
	// last the newest, and trimmed the newest ID trimmed (zero when none).
	first, last, trimmed logID
	// expires is when the log expires; zero means never.
	expires time.Time
}

// memoryLogEntry is one entry of an in-memory log.
type memoryLogEntry struct {
	id      logID
	payload []byte
}

// liveLog returns the log if it is present and unexpired; the caller holds
// m.mu.
func (m *Memory) liveLog(name string) (*memoryLog, bool) {
	l, ok := m.logs[name]
	if !ok {
		return nil, false
	}
	if !l.expires.IsZero() && !l.expires.After(m.now()) {
		delete(m.logs, name)
		return nil, false
	}
	return l, true
}

// sweepLogs removes expired logs when there are many; the caller holds
// m.mu.
func (m *Memory) sweepLogs() {
	if len(m.logs) < sweepAt {
		return
	}
	now := m.now()
	for name, l := range m.logs {
		if !l.expires.IsZero() && !l.expires.After(now) {
			delete(m.logs, name)
		}
	}
}

// LogAppend appends a payload to a log, trimming it to exactly maxLen
// entries. IDs follow Valkey's: the clock's milliseconds, and a sequence
// number that keeps them rising when the clock does not.
func (m *Memory) LogAppend(_ context.Context, log string, maxLen int64, ttl time.Duration, payload []byte) (string, error) {
	if err := checkLogArgs(maxLen, ttl); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", ErrClosed
	}
	now := m.now()
	ms := uint64(max(now.UnixMilli(), 0))
	id := logID{ms: ms}
	l, ok := m.liveLog(log)
	switch {
	case !ok:
		m.sweepLogs()
		if m.logs == nil {
			m.logs = map[string]*memoryLog{}
		}
		l = &memoryLog{first: id}
		m.logs[log] = l
	case ms <= l.last.ms:
		id = logID{ms: l.last.ms, seq: l.last.seq + 1}
	}
	l.last = id
	l.entries = append(l.entries, memoryLogEntry{id: id, payload: append([]byte(nil), payload...)})
	if drop := int64(len(l.entries)) - maxLen; drop > 0 {
		l.trimmed = l.entries[drop-1].id
		clear(l.entries[:drop])
		l.entries = l.entries[drop:]
	}
	if ttl > 0 {
		l.expires = now.Add(ttl)
	}
	return id.String(), nil
}

// LogRead returns entries after a cursor.
func (m *Memory) LogRead(_ context.Context, log, afterID string, count int) ([]StreamEntry, error) {
	if count <= 0 {
		return nil, errors.New("cache: a log read needs a positive count")
	}
	after, start, err := cursor(afterID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	l, ok := m.liveLog(log)
	if !ok {
		if start {
			return nil, nil
		}
		return nil, ErrLogGap
	}
	i := 0
	if !start {
		if after.less(l.trimmed) || after.less(l.first) {
			return nil, ErrLogGap
		}
		i = sort.Search(len(l.entries), func(k int) bool { return after.less(l.entries[k].id) })
	}
	rest := l.entries[i:]
	out := make([]StreamEntry, 0, min(count, len(rest)))
	for _, e := range rest[:min(count, len(rest))] {
		out = append(out, StreamEntry{ID: e.id.String(), Payload: append([]byte(nil), e.payload...)})
	}
	return out, nil
}

// LogLast returns the newest entry ID of a log.
func (m *Memory) LogLast(_ context.Context, log string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", ErrClosed
	}
	l, ok := m.liveLog(log)
	if !ok || len(l.entries) == 0 {
		return "", nil
	}
	return l.last.String(), nil
}

// LogDelete removes a log.
func (m *Memory) LogDelete(_ context.Context, log string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	delete(m.logs, log)
	return nil
}
