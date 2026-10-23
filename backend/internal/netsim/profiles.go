// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package netsim

import (
	"time"
)

//go:generate go run ./internal/gen ../../../schema/devtools/network-profiles.json profiles_gen.go ../../../packages/plux_flutter/lib/src/devtools_api/network_profiles.g.dart

// defaultMSS is the TCP maximum segment size of the generated profiles.
const defaultMSS = 1460

// DevProfile is a named network profile of dev sessions (DEV-031). The
// table [DevProfiles] is generated from schema/devtools/network-profiles.json,
// which also generates the device's table, so both sides name the same
// conditions.
type DevProfile struct {
	// ID is the stable identifier, the value of a network profile command.
	ID string
	// Description says what the profile stands for.
	Description string
	// Jitter is the random variation of the round-trip time, up to this much
	// either way.
	Jitter time.Duration
	// Profile is the path: bandwidth, round-trip time and loss.
	Profile Profile
}

// Offline reports whether the profile carries nothing at all.
func (p DevProfile) Offline() bool {
	return p.Profile.Down == 0 && p.Profile.Up == 0
}

// LookupDevProfile returns the dev-session profile with the identifier.
func LookupDevProfile(id string) (DevProfile, bool) {
	for _, p := range DevProfiles {
		if p.ID == id {
			return p, true
		}
	}
	return DevProfile{}, false
}
