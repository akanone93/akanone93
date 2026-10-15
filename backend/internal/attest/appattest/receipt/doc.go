// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package receipt fetches and verifies Apple's App Attest fraud-metric
// receipt, the evidence that lifts an iOS device from AL2 to AL3 (SEC-007,
// ADR-0065).
//
// A Client asks Apple's data service to refresh the receipt that came with
// a device's attestation. The request is authenticated with an ES256 JWT
// signed by the operator's imported App Store Connect key; the key never
// enters this package, which only holds an ES256Signer (SEC-123, L-3). A
// Verifier opens what Apple returns: a CMS SignedData whose signer
// certificate must chain to a configured root, whose payload names the
// app, the attested key, the receipt type, a validity window and the risk
// metric. Apple's response is untrusted until all of that checks out.
//
// Every failure, whether Apple is unreachable, answers with an error status
// or returns a receipt that does not verify, is PLX-6080
// (FRAUD_RECEIPT_UNAVAILABLE). The caller treats it as "no receipt": the
// device keeps AL2, and the error never raises a level. Messages name the
// failed check and never contain keys, tokens or receipt bytes (SEC-092).
//
// Invariants: input is size-bounded before it is parsed; BER, which Apple
// may use, is normalised to DER by a bounded, depth-limited decoder before
// any structure is read; a Client and a Verifier hold no mutable state
// and are safe for concurrent use. Package receipttest is a simulator of
// Apple's service for tests.
package receipt
