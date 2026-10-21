// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package analytics

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	ext "github.com/nightCode42/plux3/backend/ext/analytics"
)

// conn is the part of a ClickHouse connection the store uses. A
// clickhouse-go driver.Conn satisfies it; tests supply a recording fake.
type conn interface {
	Exec(ctx context.Context, query string, args ...any) error
	Query(ctx context.Context, query string, args ...any) (driver.Rows, error)
	PrepareBatch(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error)
	Close() error
}

// rollupWatermarkLag is how long after receipt an event is folded into the
// rollups, so that an insert that became visible late is not missed. It is
// the PostgreSQL store's lag.
const rollupWatermarkLag = time.Minute

// Store is the ClickHouse implementation of the analytics store (ANL-010).
type Store struct {
	db  conn
	now func() time.Time
	// aggregateRaw is the query that folds raw events into five-minute
	// rollups; it is built once from the histogram bounds.
	aggregateRaw string

	mu          sync.Mutex
	lastVersion uint64
}

var (
	_ ext.Store                  = (*Store)(nil)
	_ interface{ Close() error } = (*Store)(nil)
)

// newStore returns a store on a connection. A nil clock uses time.Now.
func newStore(db conn, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{db: db, now: now, aggregateRaw: aggregateRawQuery()}
}

// Close closes the connection; the server calls it when the process ends.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("analytics: close the connection: %w", err)
	}
	return nil
}

// nextVersion returns a row version that is higher than any this store
// handed out, so that of two writes of one row the later wins even when the
// clock does not move.
func (s *Store) nextVersion() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := uint64(s.now().UnixMicro()) //nolint:gosec // the clock is after 1970
	if v <= s.lastVersion {
		v = s.lastVersion + 1
	}
	s.lastVersion = v
	return v
}

// param binds a value to the placeholder {name:Type} of a query. The value
// travels to the server apart from the query text.
func param(name string, value any) any { return clickhouse.Named(name, value) }

// timeParam binds a time at the microsecond precision of the tables.
func timeParam(name string, t time.Time) any {
	return clickhouse.DateNamed(name, t.UTC(), clickhouse.MicroSeconds)
}

// rowScanner is the current row of a result.
type rowScanner interface {
	Scan(dest ...any) error
}

// collect runs a query and calls scan for each row.
func (s *Store) collect(ctx context.Context, query string, args []any, scan func(rowScanner) error) error {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("analytics: query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("analytics: read a row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("analytics: read rows: %w", err)
	}
	return nil
}

// insert sends rows to a table in one batch, in the order of columns.
func (s *Store) insert(ctx context.Context, table string, columns []string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.db.PrepareBatch(ctx, "INSERT INTO "+table+" ("+strings.Join(columns, ", ")+")")
	if err != nil {
		return fmt.Errorf("analytics: prepare the insert into %s: %w", table, err)
	}
	for _, row := range rows {
		if err := batch.Append(row...); err != nil {
			_ = batch.Abort()
			return fmt.Errorf("analytics: append to %s: %w", table, err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("analytics: insert into %s: %w", table, err)
	}
	return nil
}

// Prepare makes sure the tables exist. Partitions are created by ClickHouse
// when the first event of a month arrives, so monthsAhead needs no work.
func (s *Store) Prepare(ctx context.Context, _ time.Time, _ int) error {
	return s.migrate(ctx)
}
