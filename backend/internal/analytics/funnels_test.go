// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"slices"
	"testing"
)

func TestCountSessionsOrdered_ANL_020(t *testing.T) {
	a, b := [16]byte{1}, [16]byte{2}
	ev := func(d [16]byte, name string) funnelEvent { return funnelEvent{device: d, name: name} }
	steps := []string{"cart", "pay", "done"}
	for _, c := range []struct {
		name   string
		events []funnelEvent
		want   []int64
	}{
		{"in order", []funnelEvent{ev(a, "cart"), ev(a, "pay"), ev(a, "done")}, []int64{1, 1, 1}},
		{"out of order", []funnelEvent{ev(a, "pay"), ev(a, "cart"), ev(a, "done")}, []int64{1, 0, 0}},
		{"repeat does not skip", []funnelEvent{ev(a, "cart"), ev(a, "cart"), ev(a, "done")}, []int64{1, 0, 0}},
		{"other steps between", []funnelEvent{ev(a, "cart"), ev(a, "pay"), ev(a, "pay"), ev(a, "done")}, []int64{1, 1, 1}},
		{"across sessions", []funnelEvent{ev(a, sessionStartEvent), ev(a, "cart"), ev(a, sessionStartEvent), ev(a, "pay"), ev(a, "done")}, []int64{1, 0, 0}},
		{"two sessions of a device", []funnelEvent{ev(a, "cart"), ev(a, "pay"), ev(a, sessionStartEvent), ev(a, "cart")}, []int64{2, 1, 0}},
		{"devices do not mix", []funnelEvent{ev(a, "cart"), ev(b, "pay"), ev(b, "done")}, []int64{1, 0, 0}},
	} {
		if got := countSessions(steps, c.events); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if got := countSessions([]string{sessionStartEvent, "pay"}, []funnelEvent{ev(a, sessionStartEvent), ev(a, "pay"), ev(a, sessionStartEvent)}); !slices.Equal(got, []int64{2, 1}) {
		t.Errorf("a funnel that starts at the session start: %v", got)
	}
}
