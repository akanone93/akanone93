// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

// Push configures the silent push that tells devices to check their control
// document (SYN-060, Q22).
type Push struct {
	// Enabled turns silent push on. It is off by default: a server that has
	// not enabled it stores no device push token and sends no message. The
	// operator's FCM and APNs credentials are environment secrets.
	Enabled bool `json:"enabled"`
}
