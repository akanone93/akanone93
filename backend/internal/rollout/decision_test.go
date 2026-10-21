// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"fmt"
	"testing"
)

// testRollout is an active rollout at a share, targeting Android only.
func testRollout(t *testing.T, bp int) Rollout {
	t.Helper()
	rule := ruleOf(t, `{"attribute":"os","operator":"eq","value":"android"}`)
	return Rollout{State: StateActive, BasisPoints: bp, salt: []byte("rollout-salt"), Targeting: Targeting{Rule: &rule}}
}

func device(id, os string) Device { return Device{ID: id, Attributes: Attributes{OS: os}} }

// Verifies: REL-050, REL-051.
func TestRolloutIncludes_REL_051_Order(t *testing.T) {
	r := testRollout(t, 10000)
	r.DenyDevices = []string{"denied"}
	r.AllowDevices = []string{"tester", "denied"}
	tests := []struct {
		name string
		d    Device
		want bool
	}{
		{"matching device at 100%", device("a", "android"), true},
		{"device outside the targeting", device("a", "ios"), false},
		{"deny beats allow and everything", device("denied", "android"), false},
		{"allow list skips the targeting", device("tester", "ios"), true},
	}
	for _, tc := range tests {
		if got := r.Includes(tc.d); got != tc.want {
			t.Errorf("%s: Includes = %v, want %v", tc.name, got, tc.want)
		}
	}
	r.BasisPoints = 0
	if !r.Includes(device("tester", "ios")) {
		t.Error("a tester is excluded by a share of zero")
	}
	if r.Includes(device("a", "android")) {
		t.Error("an ordinary device is included at a share of zero")
	}
}

// Verifies: REL-050.
func TestRolloutIncludes_REL_050_MonotonicAndUserBased(t *testing.T) {
	small, large := testRollout(t, 1000), testRollout(t, 5000)
	inSmall := 0
	for i := range 4000 {
		d := device(fmt.Sprint("dev-", i), "android")
		if small.Includes(d) {
			inSmall++
			if !large.Includes(d) {
				t.Fatalf("device %d is in at 10%% and out at 50%%", i)
			}
		}
	}
	if inSmall == 0 {
		t.Fatal("no device is in at 10%")
	}
	// Two devices of one user are placed together, whatever their identifiers.
	for i := range 200 {
		a, b := device(fmt.Sprint("phone-", i), "android"), device(fmt.Sprint("tablet-", i), "android")
		a.UserHash, b.UserHash = fmt.Sprint("user-", i), fmt.Sprint("user-", i)
		if small.Includes(a) != small.Includes(b) {
			t.Fatalf("the devices of user %d are placed apart", i)
		}
	}
}

// Verifies: REL-050.
func TestState_REL_052(t *testing.T) {
	for st, open := range map[State]bool{
		StateScheduled: true, StateActive: true, StatePaused: true, StateClosing: true,
		StateCompleted: false, StateEnded: false, StateAborted: false, StateRolledBack: false,
	} {
		if st.Open() != open {
			t.Errorf("%s.Open() = %v", st, st.Open())
		}
	}
	if !StateActive.Serving() || !StatePaused.Serving() || StateScheduled.Serving() || StateClosing.Serving() {
		t.Error("Serving is wrong")
	}
}
