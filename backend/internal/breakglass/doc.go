// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package breakglass lets designated roles take an emergency action without
// prior approval and without being held back by a change freeze, and makes
// sure it is always looked at afterwards (GOV-024, SEC-103, ADR-0076).
//
// The emergency actions are the ones that stop or undo harm: a kill switch,
// including the kill switch of a function, a forced rollback and a rollback.
// A caller that wants one of them as an emergency asks for break-glass
// explicitly, with a written justification; an ordinary call still goes
// through the approval engine and the freeze check. Invoke checks that the
// principal holds breakglass.invoke on the environment, which carries a
// fresh step-up, refuses an empty justification with PLX-8005, records the
// invocation with its review due date, audits it and, for a production
// environment, enters it in the ledger of production changes that
// compliance reports are read from. Once the caller's transaction has
// committed, Alert tells everyone subscribed to the event, the approvers
// among them, at once.
//
// # Post-hoc review
//
// Every invocation opens a review, due within
// governance.breakGlassReviewHours of the action, 24 hours by default. Someone
// who holds approvals.decide and is not the person who acted completes it
// with Review. A review still open when it falls due is escalated once by
// the worker's Escalate job: an audit entry, and a critical notification
// that carries PLX-8006. An emergency stop is never blocked by an overdue
// review, because a delayed review must not delay the next emergency; the
// overdue state is shown wherever reviews are listed.
//
// Invariants:
//   - A restriction still wins: Invoke checks the permission with the
//     ordinary authorisation, which applies restrictions (GOV-006).
//   - An invocation is recorded in the transaction of the action itself,
//     so an action that did not happen leaves no invocation, and one that
//     did cannot be missing.
//   - Time is injected.
//
// Specification: GOV-024, GOV-006, SEC-103, LIM-001; ADR-0076.
package breakglass
