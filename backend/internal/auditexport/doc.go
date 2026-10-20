// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package auditexport delivers an organisation's audit chain to a SIEM
// (SEC-143, ADR-0082, spec section 15).
//
// An administrator registers destinations (Service); the worker role's
// Exporter reads each destination's organisation chain after the
// destination's durable cursor, in bounded batches, hands every batch to
// the destination's sink and moves the cursor only after the sink has
// acknowledged it. Delivery is therefore at least once and in order: a
// failed or interrupted batch is sent again whole, and no entry is skipped.
// A failure is recorded on the destination (PLX-8500), the next attempt
// waits for an exponential backoff with jitter, and a destination that stays
// unreachable is reported through the Notifier seam.
//
// Every record carries its entry hash and the previous entry's hash, so the
// SIEM can verify the chain (SEC-140). The core ships the syslog sink
// (RFC 5424 over TLS with RFC 5425 framing); CEF, OTLP logs and signed
// webhooks are enterprise sinks plugged in through ext/export, and the core
// refuses their kinds with PLX-8600 without a licence (GOV-030).
//
// Invariants:
//
//   - The cursor moves only after an acknowledged batch, in the transaction
//     that holds the destination's row, so two workers never deliver to one
//     destination at once.
//   - Every connection goes through the egress guard: private, loopback and
//     metadata addresses are refused unless the destination was explicitly
//     allowed for development, which is reported with a warning (SEC-104).
//   - Credentials (client certificate, key, signing secret) are sealed at
//     rest and appear in no log, error, audit entry or API response
//     (SEC-092, SEC-106).
//   - Batch sizes and the number of destinations come from the limits
//     registry (LIM-001).
package auditexport
