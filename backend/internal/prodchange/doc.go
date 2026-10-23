// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package prodchange is the ledger of production changes that a compliance
// report is read from (GOV-026, ADR-0076).
//
// Every change to a production environment that goes through the approval
// engine, and every break-glass action, writes one row in the transaction of
// the change itself: a change that is refused or rolled back leaves no row,
// and a change that happens cannot fail to leave one. A row names who made
// the change, when, what it changed and to which content hash, and the
// approval request or break-glass invocation that authorised it; the
// approvers, their signed decision records and the audit entries are
// reached through those references.
//
// Invariants:
//   - Rows are only ever appended; nothing here updates or deletes one.
//   - The time of a row comes from the caller's clock, never the database's,
//     so a test can place a change in a period.
//
// Specification: GOV-026, QA-070; ADR-0076.
package prodchange
