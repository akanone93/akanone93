// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package storage

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AdvisoryLock is a session-level PostgreSQL advisory lock held on a
// connection of its own. It is how replicas of the worker role elect the one
// that runs singleton work, such as the scheduler (FN-025): the lock is
// released when Release is called, and by the database when the connection
// or the process dies, so a crashed leader never blocks its successor.
type AdvisoryLock struct {
	mu   sync.Mutex
	key  int64
	conn *pgxpool.Conn
}

// TryAdvisoryLock takes the lock named by key if no other session holds it.
// It returns nil, nil when another session does. The caller releases a lock
// it was given.
func (db *DB) TryAdvisoryLock(ctx context.Context, key int64) (*AdvisoryLock, error) {
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage: advisory lock: %w", err)
	}
	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&got); err != nil {
		conn.Release()
		return nil, fmt.Errorf("storage: advisory lock: %w", err)
	}
	if !got {
		conn.Release()
		return nil, nil //nolint:nilnil // another session holds the lock
	}
	return &AdvisoryLock{key: key, conn: conn}, nil
}

// Held reports whether the lock's connection still answers. A connection
// that is gone has lost the lock with it, and the holder must stop acting
// as the leader.
func (l *AdvisoryLock) Held(ctx context.Context) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return false
	}
	var one int
	return l.conn.QueryRow(ctx, `SELECT 1`).Scan(&one) == nil
}

// Release gives the lock up and returns its connection to the pool. It is
// safe to call more than once. The connection is closed rather than pooled
// if the unlock fails, so that the lock cannot outlive the call.
func (l *AdvisoryLock) Release(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return
	}
	var unlocked bool
	if err := l.conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, l.key).Scan(&unlocked); err != nil || !unlocked {
		_ = l.conn.Conn().Close(ctx)
	}
	l.conn.Release()
	l.conn = nil
}
