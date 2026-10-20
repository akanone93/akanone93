// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package push sends the silent push that makes a device look at its
// control document at once (SYN-060, ADR-0077, maintainer decision Q22).
//
// A push carries nothing a person could read and nothing about the
// change: a data-only message with the marker {"plux":"control"} and the
// app's identifier, which the host passes to Plux.handleSilentPush. The
// device then fetches its signed control document as it would on resume;
// a forged push can at most cause one request, and everything that
// matters is verified there (SEC-052). The messages go through the
// operator's own FCM HTTP v1 and APNs credentials, held as environment
// secrets in the secret store (SEC-106), so no Plux-owned credential
// sees an operator's devices.
//
// Sending is off unless the installation turns it on, and an environment
// that has no credentials is skipped. Push tokens are personal data: they
// are stored encrypted with envelope encryption, bound to their device,
// and deleted with it. All outbound requests go through the SSRF-safe
// client of package httpx and only to the hosts egressDestinations lists.
//
// Invariants: a message never carries content beyond the marker and the
// app identifier; a credential is never logged or returned; a token the
// provider reports as dead is deleted; one job sends to at most
// push.batchSize devices and hands the rest to a follow-up job.
package push
