// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package receipttest simulates Apple's App Attest data service for tests,
// so that every path of the fraud-metric receipt client runs in CI without
// Apple credentials (ADR-0065, SEC-007).
//
// An Authority is a test certificate chain that issues receipts in the
// shape Apple's receipts have: a CMS SignedData with signed attributes
// and a payload of typed fields, in DER or in BER with indefinite
// lengths. A Server serves POST /v1/attestationData over httptest: it
// verifies the ES256 request token (key, kid, iss, iat), then answers
// according to its Mode. Nothing here belongs in production code.
//
// A Server and an Authority are safe for concurrent use.
package receipttest
