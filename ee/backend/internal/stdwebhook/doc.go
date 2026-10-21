// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

// Package stdwebhook signs outgoing messages as the Standard Webhooks
// specification defines (SEC-143, ANL-011). The audit export and the
// analytics event export share it, so both produce one signature that a
// receiver using any Standard Webhooks library verifies.
//
// A secret is "whsec_" followed by the base64 of the key. A message is signed
// over "id.timestamp.body" with HMAC-SHA256 and carried in the
// webhook-id, webhook-timestamp and webhook-signature headers. The package
// holds no state and never logs a secret or a key.
package stdwebhook
