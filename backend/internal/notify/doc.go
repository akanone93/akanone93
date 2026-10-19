// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package notify tells people and systems what happened (SRV-062,
// SRV-063, ADR-0081).
//
// Other modules publish typed events to a Publisher. The service turns an
// event into rows, in the publisher's transaction-sized unit of work: an
// inbox entry for each subscribed user, a delivery for each subscribed
// channel (in-app inbox, SMTP, Slack, Microsoft Teams, Telegram) and a
// signed delivery for each outbound webhook endpoint. A worker then sends
// the deliveries, retrying with exponential backoff and jitter for 24 hours
// before it gives up with PLX-8401.
//
// Invariants:
//
//   - Every outbound call goes through the SSRF-safe client or dialer
//     (SEC-105); a destination in a private range is refused when the
//     endpoint is registered and again when it is dialled.
//   - No secret reaches a log, an error, an API response or the delivery
//     log (SEC-092). Channel credentials and webhook secrets are stored
//     under envelope encryption, bound to the row that owns them.
//   - SMTP requires TLS (STARTTLS or implicit) unless the installation
//     allows insecure development settings, which logs a distinct warning
//     on every use (SEC-092, security-practices.md).
//   - Publishing is bounded by notifications.perHour per organisation;
//     notifications over it are dropped, counted and audited once an hour
//     (LIM-001).
//   - The number of outbound webhooks is bounded by webhooks.perOrg
//     (LIM-001).
//   - The queue is durable: a delivery is a PostgreSQL row, claimed with
//     row locks, so several workers never send the same one twice.
//
// The sub-package webhook implements the Standard Webhooks signature scheme,
// which inbound function hooks (FN-025) use as well.
package notify
