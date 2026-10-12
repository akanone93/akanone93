// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package attest holds what the verifiers of platform attestation under it
// share (SEC-002, SEC-007, SEC-009).
package attest

import "errors"

// ErrProviderUnavailable marks a failure that is the attestation provider's
// and not the evidence's: a status list that cannot be fetched or read, a
// receipt endpoint that does not answer. A verifier wraps it in the error it
// returns, so that the device service can tell an outage, which the
// attestationOutageGrace setting governs (SEC-009), from evidence that
// fails verification, which is always refused.
var ErrProviderUnavailable = errors.New("attest: the attestation provider is unavailable")
