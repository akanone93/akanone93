// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/nightCode42/plux3/backend/internal/cache"
)

// Stream and Group name the Valkey stream accepted batches go through and
// the consumer group of the worker role.
const (
	Stream = "plux:telemetry:events"
	Group  = "telemetry-writers"
)

// StreamQueue is a Queue on a Valkey stream (ADR-0080).
type StreamQueue struct {
	Streams cache.Streams
	// MaxLen bounds the stream to about this many batches; zero leaves it
	// unbounded. Trimming drops batches no consumer has written yet, so it
	// is a last resort against a Valkey that fills up.
	MaxLen int64
}

// Enqueue appends a batch.
func (q StreamQueue) Enqueue(ctx context.Context, payload []byte) error {
	if _, err := q.Streams.StreamAdd(ctx, Stream, q.MaxLen, payload); err != nil {
		return fmt.Errorf("telemetry: queue a batch: %w", err)
	}
	return nil
}

// Consumer is a member of the writers' consumer group: it reads batches the
// api role queued and writes them through the Service, in bulk. Several
// consumers, in several worker processes, share the stream. A batch that
// failed to write stays pending and is claimed again after ClaimAfter, so
// delivery is at least once; events carry their identifiers, so a repeated
// write stores them once.
type Consumer struct {
	Streams cache.Streams
	Service *Service
	// Name identifies the consumer within the group; it must be unique per
	// process.
	Name string
	// Count is how many batches one read takes; the default is 100.
	Count int
	// Idle is the wait when the stream is empty; the default is 500 ms.
	Idle time.Duration
	// ClaimAfter is how long a batch may stay unacknowledged before
	// another consumer takes it over; the default is a minute.
	ClaimAfter time.Duration
	Log        *slog.Logger
}

func (c *Consumer) withDefaults() {
	if c.Count <= 0 {
		c.Count = 100
	}
	if c.Idle <= 0 {
		c.Idle = 500 * time.Millisecond
	}
	if c.ClaimAfter <= 0 {
		c.ClaimAfter = time.Minute
	}
}

// Run consumes until the context ends.
func (c *Consumer) Run(ctx context.Context) {
	c.withDefaults()
	for ctx.Err() == nil {
		if err := c.Streams.StreamGroup(ctx, Stream, Group); err != nil {
			c.fail(ctx, "create the consumer group", err)
			c.wait(ctx)
			continue
		}
		break
	}
	for ctx.Err() == nil {
		n, err := c.Once(ctx)
		if err != nil {
			c.fail(ctx, "consume", err)
		}
		if n == 0 || err != nil {
			c.wait(ctx)
		}
	}
}

func (c *Consumer) wait(ctx context.Context) {
	t := time.NewTimer(c.Idle)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func (c *Consumer) fail(ctx context.Context, what string, err error) {
	if c.Log != nil && ctx.Err() == nil {
		c.Log.ErrorContext(ctx, "telemetry consumer", slog.String("step", what), slog.Any("error", err))
	}
}

// Once takes over batches left pending by a consumer that stopped, reads
// new ones, writes them and acknowledges what was written. It returns how
// many batches it handled.
func (c *Consumer) Once(ctx context.Context) (int, error) {
	c.withDefaults()
	stale, err := c.Streams.StreamClaim(ctx, Stream, Group, c.Name, c.ClaimAfter, c.Count)
	if err != nil {
		return 0, fmt.Errorf("telemetry: claim: %w", err)
	}
	fresh, err := c.Streams.StreamRead(ctx, Stream, Group, c.Name, c.Count)
	if err != nil {
		return 0, fmt.Errorf("telemetry: read: %w", err)
	}
	entries := slices.Concat(stale, fresh)
	var done []string
	var errs []error
	for _, e := range entries {
		if err := c.handle(ctx, e.Payload); err != nil {
			errs = append(errs, err)
			continue
		}
		done = append(done, e.ID)
	}
	if err := c.Streams.StreamAck(ctx, Stream, Group, done...); err != nil {
		errs = append(errs, fmt.Errorf("telemetry: acknowledge: %w", err))
	}
	return len(entries), errors.Join(errs...)
}

// handle writes one batch. A batch that can never be written, because it is
// malformed or its device is gone, is dropped, not retried for ever.
func (c *Consumer) handle(ctx context.Context, payload []byte) error {
	var m message
	if err := json.Unmarshal(payload, &m); err != nil {
		c.fail(ctx, "drop a malformed batch", err)
		return nil
	}
	err := c.Service.write(ctx, m)
	if permanent(err) {
		c.fail(ctx, "drop a batch that cannot be written", err)
		return nil
	}
	return err
}

// permanent reports whether a write failed in a way a retry cannot cure:
// the device or app was deleted since the batch was accepted (the device is
// not found, or a foreign key is violated), or an identifier is not valid.
func permanent(err error) bool {
	if errors.Is(err, pgx.ErrNoRows) {
		return true
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code == "23503" || pg.Code == "22P02"
	}
	return false
}

// Component runs a Consumer with the process.
type Component struct {
	Consumer *Consumer

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// Name implements the server's Component.
func (*Component) Name() string { return "telemetry-consumer" }

// Start begins consuming.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	run, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c.cancel, c.done = cancel, make(chan struct{})
	go func() {
		defer close(c.done)
		c.Consumer.Run(run)
	}()
	return nil
}

// Stop ends consuming and waits for the batch in flight, within the
// context's deadline.
func (c *Component) Stop(ctx context.Context) error {
	c.mu.Lock()
	cancel, done := c.cancel, c.done
	c.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("telemetry: stop the consumer: %w", ctx.Err())
	}
}
