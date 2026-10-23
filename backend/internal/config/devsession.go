// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package config

// DevSession configures the dev sessions of the api role (DEV-002, DEV-003,
// ADR-0087). The lifetime of a pairing token, the devices per session and
// the session's lifetime and idle time are the limits devSession.*.
type DevSession struct {
	// EnabledEnvironments are the environment keys that may host a dev
	// session. A production environment is refused whatever is listed
	// (DEV-003); an empty list disables dev sessions.
	EnabledEnvironments []string `json:"enabledEnvironments"`
	// IdleTimeout closes a device socket that carries no frame, pings
	// included, for this long.
	IdleTimeout Duration `json:"idleTimeout"`
	// PingInterval is how often the relay pings a device socket.
	PingInterval Duration `json:"pingInterval"`
}
