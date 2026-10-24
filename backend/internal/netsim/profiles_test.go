// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package netsim_test

import (
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/netsim"
)

// The registry defines the six profiles the specification names, in order.
func TestDevProfilesAreTheRegistrys(t *testing.T) {
	t.Parallel()
	want := []string{"good_wifi", "offline", "2g", "3g", "high_latency", "packet_loss"}
	if len(netsim.DevProfiles) != len(want) {
		t.Fatalf("%d profiles, want %d", len(netsim.DevProfiles), len(want))
	}
	for i, id := range want {
		if netsim.DevProfiles[i].ID != id {
			t.Errorf("profile %d is %q, want %q", i, netsim.DevProfiles[i].ID, id)
		}
	}
}

// The registry's units become the units of Profile: kbit/s to bit/s,
// milliseconds to durations, percent to a fraction.
func TestDevProfileUnits(t *testing.T) {
	t.Parallel()
	p, ok := netsim.LookupDevProfile("3g")
	if !ok {
		t.Fatal("no 3g profile")
	}
	if p.Profile.Down != 750_000 || p.Profile.Up != 250_000 {
		t.Errorf("bandwidth = %d/%d bit/s", p.Profile.Down, p.Profile.Up)
	}
	if p.Profile.RTT != 300*time.Millisecond || p.Jitter != 50*time.Millisecond {
		t.Errorf("rtt = %s, jitter = %s", p.Profile.RTT, p.Jitter)
	}
	if p.Profile.Loss != 0.01 || p.Profile.MSS != 1460 {
		t.Errorf("loss = %v, mss = %d", p.Profile.Loss, p.Profile.MSS)
	}
	if p.Offline() {
		t.Error("3g is offline")
	}
}

// Only the offline profile carries nothing, and an unknown identifier is
// not found.
func TestDevProfileOfflineAndLookup(t *testing.T) {
	t.Parallel()
	for _, p := range netsim.DevProfiles {
		if got, want := p.Offline(), p.ID == "offline"; got != want {
			t.Errorf("%s: Offline() = %v, want %v", p.ID, got, want)
		}
	}
	if _, ok := netsim.LookupDevProfile("5g"); ok {
		t.Error("found a 5g profile")
	}
}
