// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics_test

import (
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/analytics"
)

func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func window(t *testing.T, start, end, zone string) analytics.Window {
	t.Helper()
	w, err := analytics.ParseWindow(start, end, zone)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// Verifies: ANL-010.
// The window is read on the wall clock of the installation's zone, and one
// that crosses midnight is open on both sides of it.
func TestWindowOpen_ANL_010(t *testing.T) {
	t.Parallel()
	berlin := window(t, "02:00", "04:00", "Europe/Berlin")
	for s, want := range map[string]bool{
		"2026-10-19T23:59:00Z": false, // 01:59 in Berlin (CEST)
		"2026-10-20T00:00:00Z": true,  // 02:00
		"2026-10-20T01:59:00Z": true,  // 03:59
		"2026-10-20T02:00:00Z": false, // 04:00 is outside
		"2026-12-01T01:00:00Z": true,  // 02:00 in winter (CET)
	} {
		if got := berlin.Open(at(t, s)); got != want {
			t.Errorf("Berlin window open at %s = %v, want %v", s, got, want)
		}
	}
	overnight := window(t, "23:00", "01:00", "UTC")
	for s, want := range map[string]bool{
		"2026-10-19T22:59:00Z": false, "2026-10-19T23:00:00Z": true, "2026-10-20T00:30:00Z": true, "2026-10-20T01:00:00Z": false,
	} {
		if got := overnight.Open(at(t, s)); got != want {
			t.Errorf("overnight window open at %s = %v, want %v", s, got, want)
		}
	}
}

// Verifies: ANL-010.
// A change requested outside the window waits for its next opening, and one
// requested inside it applies at once.
func TestWindowNextOpen_ANL_010(t *testing.T) {
	t.Parallel()
	berlin := window(t, "02:00", "04:00", "Europe/Berlin")
	for _, c := range []struct{ from, want string }{
		{"2026-10-19T10:00:00Z", "2026-10-20T00:00:00Z"},
		{"2026-10-20T00:00:00Z", "2026-10-20T00:00:00Z"},
		{"2026-10-20T00:00:30Z", "2026-10-20T00:00:00Z"},
		{"2026-10-20T01:30:00Z", "2026-10-20T01:30:00Z"},
		{"2026-10-20T02:00:00Z", "2026-10-21T00:00:00Z"},
	} {
		if got := berlin.NextOpen(at(t, c.from)); !got.Equal(at(t, c.want)) {
			t.Errorf("NextOpen(%s) = %s, want %s", c.from, got.Format(time.RFC3339), c.want)
		}
	}
}

// Verifies: ANL-010.
// On the night a clock moves forward the 02:00 hour does not exist, so the
// window opens at the first minute that does (03:00 summer time) and stays
// open for the 03:00 hour only.
func TestWindowSpringForward_ANL_010(t *testing.T) {
	t.Parallel()
	berlin := window(t, "02:00", "04:00", "Europe/Berlin")
	// 2027-03-28: 02:00 CET jumps to 03:00 CEST at 01:00 UTC.
	got := berlin.NextOpen(at(t, "2027-03-27T12:00:00Z"))
	if want := at(t, "2027-03-28T01:00:00Z"); !got.Equal(want) {
		t.Errorf("NextOpen across the spring change = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if berlin.Open(at(t, "2027-03-28T00:59:00Z")) {
		t.Error("the window was open at 01:59 CET, a minute before the clock moves")
	}
	if !berlin.Open(at(t, "2027-03-28T01:59:00Z")) || berlin.Open(at(t, "2027-03-28T02:00:00Z")) {
		t.Error("the window is open from 03:00 to 04:00 CEST on the day of the change")
	}
}

// Verifies: ANL-010.
// On the night a clock moves back the 02:00 hour occurs twice; the window is
// open both times, so a change is applied at the first opening and never
// waits a day for nothing.
func TestWindowFallBack_ANL_010(t *testing.T) {
	t.Parallel()
	berlin := window(t, "02:00", "04:00", "Europe/Berlin")
	// 2026-10-25: 03:00 CEST becomes 02:00 CET at 01:00 UTC.
	got := berlin.NextOpen(at(t, "2026-10-24T20:00:00Z"))
	if want := at(t, "2026-10-25T00:00:00Z"); !got.Equal(want) {
		t.Errorf("NextOpen before the autumn change = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	for s, want := range map[string]bool{
		"2026-10-24T23:59:00Z": false, "2026-10-25T00:00:00Z": true, "2026-10-25T00:59:00Z": true,
		"2026-10-25T01:00:00Z": true, "2026-10-25T02:59:00Z": true, "2026-10-25T03:00:00Z": false,
	} {
		if got := berlin.Open(at(t, s)); got != want {
			t.Errorf("open at %s = %v, want %v", s, got, want)
		}
	}
}

// Verifies: ANL-010.
// A window is refused when its zone is not an IANA name, a time is not
// HH:MM, or it is empty.
func TestParseWindowRefusals_ANL_010(t *testing.T) {
	t.Parallel()
	for name, c := range map[string][3]string{
		"unknown zone": {"02:00", "04:00", "Mars/Olympus"},
		"empty zone":   {"02:00", "04:00", ""},
		"local zone":   {"02:00", "04:00", "Local"},
		"bad start":    {"25:00", "04:00", "UTC"},
		"bad end":      {"02:00", "4am", "UTC"},
		"empty window": {"03:00", "03:00", "UTC"},
	} {
		if _, err := analytics.ParseWindow(c[0], c[1], c[2]); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
