// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package loadgen simulates a fleet of Plux devices against a running
// server, to size an installation and to measure how fast a control change
// reaches the fleet (DEP-051, NFR-023, NFR-024, NFR-040).
//
// A synthetic device does what the runtime does, at the pace the runtime
// does it: it registers with development evidence and a fresh DPoP key,
// checks its manifest, fetches the signed control document (SYN-060),
// reports control_applied when the document's version is new to it, sends
// telemetry batches and occasionally calls a server function. Its identity,
// its address and every event it sends are synthetic: nothing is derived
// from a person, a real device or a real app's data.
//
// Invariants:
//   - Every request carries a DPoP proof bound to the device's access token
//     and the server's latest nonce, as the runtime's does (SEC-020).
//   - Pacing is the only source of load: a device never retries faster than
//     its schedule, so the request rates of a run are those of the fleet it
//     models and extrapolate linearly (docs/operations/sizing.md).
//   - Errors are counted by code, never hidden; a run with errors is a
//     result, not a crash.
//   - The report carries counts, rates and latencies only.
//
// Specification: DEP-051, NFR-023, NFR-024, SYN-060, OBS-005.
package loadgen
