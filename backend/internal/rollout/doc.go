// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package rollout offers a release to a share of a channel's devices, in
// stages and under a health gate, and decides at manifest time which release
// a device receives (REL-050 to REL-053, ADR-0078, maintainer decisions Q10
// and Q13).
//
// # Model
//
// A rollout names a channel and a candidate release. While it is scheduled,
// active or paused the channel keeps pointing at the release it had, the
// previous release; devices inside the rollout are served the candidate's
// manifest, devices outside it the channel's. The release package signs the
// candidate's manifest beside the channel's own while a rollout is in
// progress and asks Service.Select, at manifest time, which one a device gets.
//
// # Deciding
//
// Select is pure over the rollout and the device (Decide): an explicit deny
// list excludes a device, an allow list includes it, otherwise the device
// must satisfy the targeting rule and the named segments (Evaluate) and its
// bucket must lie below the rollout's current share. The bucket is
// sha256(salt || subject) mod 10000, with the hashed user identifier as the
// subject when the host app set one and the device identifier otherwise, so
// a device that is in at 10% is in at 50%. User attributes reach the server
// only as hashes; a condition on one compares hashes and supports equality
// and membership only.
//
// # Schedule
//
// A scheduler job (Service.Tick) runs for every organisation on the elected
// leader of internal/scheduler. It starts rollouts that are due, moves the
// active ones through their stages, ends the ones past their end time and
// evaluates health gates. Every step is a conditional update, so two
// schedulers that overlap for a moment do each step once. Ending a rollout
// is a two-step transition through the closing state so that a crash between
// the steps is finished by the next pass: promoting the candidate points the
// channel at it; reverting creates a new release that repeats the previous
// one and points the channel there, because a device's anti-rollback rule is
// never relaxed (REL-006).
//
// # Health gates
//
// Between two runs of the gate the candidate and the previous release are
// compared over a window and a minimum sample of sessions (Judge): crash-free
// sessions, the rate of Plux errors, of action errors, of function errors and
// of custom events per session. A regression beyond a threshold pauses the
// rollout (PLX-3150) or, when configured, rolls it back (PLX-3151), and
// notifies the organisation. A sample that is too small decides nothing.
//
// Invariants:
//
//   - A device's bucket depends only on the rollout's salt and its subject.
//   - The share of a rollout never decreases while it is in progress.
//   - At most one rollout of a channel is in progress at any moment (PLX-3153).
//   - Every state transition is audited; health decisions name the figures.
//   - No clock or randomness is read except through the injected Options.
package rollout
