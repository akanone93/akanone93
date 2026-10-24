// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package cache

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// LogChannel is the Notifier channel on which appends to a log are
// announced.
func LogChannel(log string) string { return "log:" + log }

// LogWriter appends to logs and announces every append on the log's
// channel, so that followers on every replica read at once.
type LogWriter struct {
	// Logs holds the logs.
	Logs Logs
	// Notifier announces appends; nil announces nothing, and followers
	// fall back to polling.
	Notifier Notifier
	// MaxLen is the number of entries each log keeps.
	MaxLen int64
	// TTL is how long a log lives after its last append; 0 keeps the
	// lifetime it has.
	TTL time.Duration
}

// Append appends a payload to a log and announces it. A failed
// announcement is not an error: followers also poll (ADR-0086).
func (w LogWriter) Append(ctx context.Context, log string, payload []byte) (string, error) {
	id, err := w.Logs.LogAppend(ctx, log, w.MaxLen, w.TTL, payload)
	if err != nil {
		return "", fmt.Errorf("cache: append to log: %w", err)
	}
	if w.Notifier != nil {
		// The poll of every follower covers a lost announcement.
		_ = w.Notifier.Publish(ctx, LogChannel(log), []byte(id))
	}
	return id, nil
}

// Follow delivers the log's entries after afterID to fn in order until ctx ends or fn
// returns an error. It reads when the Notifier announces an append on channel
// LogChannel(log), when the subscription's generation rises, and every poll
// interval as the fallback for a lost notice (ADR-0086).
//
// It returns ctx's error when ctx ends, fn's error unchanged, ErrLogGap
// (matched with errors.Is) when the reader fell behind the log, and ErrClosed when the
// subscription ends with its backend.
func Follow(ctx context.Context, logs Logs, n Notifier, log, afterID string, poll time.Duration, batch int, fn func(StreamEntry) error) error {
	if poll <= 0 || batch <= 0 {
		return errors.New("cache: following a log needs a positive poll interval and batch")
	}
	// Subscribing before the first read means no append between the two
	// goes unnoticed.
	sub, err := n.Subscribe(ctx, LogChannel(log))
	if err != nil {
		return fmt.Errorf("cache: follow log: %w", err)
	}
	defer func() { _ = sub.Close() }()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	cursor := afterID
	for {
		if cursor, err = drainLog(ctx, logs, log, cursor, batch, fn); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("cache: follow log: %w", ctx.Err())
		case _, ok := <-sub.Messages():
			if !ok {
				return ErrClosed
			}
		case <-sub.Changes():
		case <-ticker.C:
		}
	}
}

// drainLog delivers every entry after cursor to fn and returns the new
// cursor.
func drainLog(ctx context.Context, logs Logs, log, cursor string, batch int, fn func(StreamEntry) error) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return cursor, fmt.Errorf("cache: follow log: %w", err)
		}
		entries, err := logs.LogRead(ctx, log, cursor, batch)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return cursor, fmt.Errorf("cache: follow log: %w", ctxErr)
			}
			return cursor, fmt.Errorf("cache: follow log: %w", err)
		}
		for _, e := range entries {
			if err := fn(e); err != nil {
				return cursor, err
			}
			cursor = e.ID
		}
		if len(entries) < batch {
			return cursor, nil
		}
	}
}
