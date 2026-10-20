// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package scheduler runs the worker role's periodic, per-organisation work
// on exactly one replica at a time: draining the notification and webhook
// queues and firing the cron schedules of functions (SRV-062, SRV-063,
// FN-025, ADR-0081).
//
// Replicas elect a leader with a session-level PostgreSQL advisory lock. The
// leader holds it on a connection of its own for as long as it runs; the
// database releases it when the connection or the process dies, and another
// replica takes over on its next pass. The jobs are idempotent as well —
// a schedule's fire time is claimed by a row, a delivery is claimed with a
// row lock — so if a lock is lost between two checks and two replicas run a
// pass at once, nothing happens twice.
//
// Invariants:
//
//   - At most one replica runs a pass at a time (the advisory lock).
//   - A failing job or organisation never stops the others; the pass
//     reports the failures it met.
//   - Time is injected through the jobs; the scheduler only ticks.
//   - Stop releases the lock, so a clean shutdown hands over at once.
package scheduler
