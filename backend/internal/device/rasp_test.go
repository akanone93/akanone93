// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"slices"
	"testing"
)

// Verifies: SEC-071.
// Only a detection of a modified environment lowers a device; screen
// capture, overlays and accessibility services, other events, and fields
// that are not valid do not.
func TestStickyKinds(t *testing.T) {
	t.Parallel()
	ev := func(name, fields string) RiskEvent { return RiskEvent{Name: name, Fields: []byte(fields)} }
	cases := []struct {
		name   string
		events []RiskEvent
		want   []string
	}{
		{"root", []RiskEvent{ev("rasp_detection", `{"kind":"root","severity":"high"}`)}, []string{"root"}},
		{"every sticky kind", []RiskEvent{
			ev("rasp_detection", `{"kind":"debugger"}`), ev("rasp_detection", `{"kind":"emulator"}`),
			ev("rasp_detection", `{"kind":"hooking"}`), ev("rasp_detection", `{"kind":"repackaging"}`),
			ev("rasp_detection", `{"kind":"untrusted_installer"}`),
		}, []string{"debugger", "emulator", "hooking", "repackaging", "untrusted_installer"}},
		{"live kinds", []RiskEvent{
			ev("rasp_detection", `{"kind":"screen_capture"}`), ev("rasp_detection", `{"kind":"overlay"}`),
			ev("rasp_detection", `{"kind":"accessibility"}`),
		}, nil},
		{"another event", []RiskEvent{ev("error", `{"kind":"root"}`)}, nil},
		{"unknown kind", []RiskEvent{ev("rasp_detection", `{"kind":"time_travel"}`)}, nil},
		{"fields that are not JSON", []RiskEvent{ev("rasp_detection", `nope`), ev("rasp_detection", ``), ev("rasp_detection", `{"kind":7}`)}, nil},
	}
	for _, c := range cases {
		if got := stickyKinds(c.events); !slices.Equal(got, c.want) {
			t.Errorf("%s: kinds = %v, want %v", c.name, got, c.want)
		}
	}
}

// Verifies: SEC-071.
func TestLevelsAbove(t *testing.T) {
	t.Parallel()
	if got := levelsAbove(AL1); !slices.Equal(got, []string{"AL2", "AL3"}) {
		t.Errorf("levelsAbove(AL1) = %v", got)
	}
}
