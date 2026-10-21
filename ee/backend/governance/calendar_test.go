// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package governance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/governance"
)

// fixedLicence is an edition provider with a fixed status.
type fixedLicence struct{ status edition.Status }

// Status returns the fixed status.
func (f fixedLicence) Status() edition.Status { return f.status }

// licensed returns a context whose licence is in state and lists both
// governance features.
func licensed(state edition.State) context.Context {
	return edition.WithProvider(context.Background(), fixedLicence{status: edition.Status{
		State:    state,
		Features: []edition.Feature{edition.FeatureChangeFreezes, edition.FeatureComplianceReports},
	}})
}

// utc builds an instant.
func utc(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}

// held reports whether the windows hold a change to the environment at the
// instant, failing the test on an error.
func held(t *testing.T, windows []governance.Window, env string, at time.Time) bool {
	t.Helper()
	_, ok, err := Calendar{}.InForce(licensed(edition.StateValid), windows, env, at)
	if err != nil {
		t.Fatalf("InForce(%s): %v", at.Format(time.RFC3339), err)
	}
	return ok
}

// Verifies: GOV-025.
// A one-off window holds from its start, inclusive, to its end, exclusive,
// on the wall clock of its time zone.
func TestCalendar_OneOff_GOV_025(t *testing.T) {
	t.Parallel()
	// Month-end close in Berlin: 2026-12-30 18:00 CET (17:00Z) for 36 hours.
	w := []governance.Window{{Name: "close", TimeZone: "Europe/Berlin", Start: "2026-12-30T18:00", Duration: 36 * time.Hour}}
	for _, c := range []struct {
		at   time.Time
		want bool
	}{
		{utc(2026, 12, 30, 16, 59), false},
		{utc(2026, 12, 30, 17, 0), true},
		{utc(2026, 12, 31, 12, 0), true},
		{utc(2027, 1, 1, 4, 59), true},
		{utc(2027, 1, 1, 5, 0), false},
	} {
		if got := held(t, w, "production", c.at); got != c.want {
			t.Errorf("held at %s = %v, want %v", c.at.Format(time.RFC3339), got, c.want)
		}
	}
}

// Verifies: GOV-025.
// The same wall-clock window means different instants in different time
// zones.
func TestCalendar_TimeZones_GOV_025(t *testing.T) {
	t.Parallel()
	tokyo := []governance.Window{{Name: "w", TimeZone: "Asia/Tokyo", Start: "2026-06-01T09:00", Duration: time.Hour}}
	la := []governance.Window{{Name: "w", TimeZone: "America/Los_Angeles", Start: "2026-06-01T09:00", Duration: time.Hour}}
	// 09:00 in Tokyo is 00:00Z; 09:00 in Los Angeles in June is 16:00Z.
	if !held(t, tokyo, "production", utc(2026, 6, 1, 0, 30)) || held(t, tokyo, "production", utc(2026, 6, 1, 16, 30)) {
		t.Error("the Tokyo window is not at 00:00Z")
	}
	if !held(t, la, "production", utc(2026, 6, 1, 16, 30)) || held(t, la, "production", utc(2026, 6, 1, 0, 30)) {
		t.Error("the Los Angeles window is not at 16:00Z")
	}
}

// Verifies: GOV-025.
// A weekly window follows the wall clock across the change to daylight-saving
// time: Friday 18:00 in New York is 23:00Z before the change and 22:00Z
// after it.
func TestCalendar_WeeklyAcrossDST_GOV_025(t *testing.T) {
	t.Parallel()
	w := []governance.Window{{
		Name: "weekend", TimeZone: "America/New_York", Start: "2026-02-27T18:00", Duration: 2 * time.Hour,
		Recurrence: "FREQ=WEEKLY;BYDAY=FR",
	}}
	for _, c := range []struct {
		at   time.Time
		want bool
	}{
		{utc(2026, 3, 6, 22, 30), false}, // Friday before the change: 18:00 EST is 23:00Z.
		{utc(2026, 3, 6, 23, 30), true},
		{utc(2026, 3, 13, 21, 59), false}, // Friday after the change: 18:00 EDT is 22:00Z.
		{utc(2026, 3, 13, 22, 0), true},
		{utc(2026, 3, 13, 23, 59), true},
		{utc(2026, 3, 14, 0, 0), false},
		{utc(2026, 3, 7, 23, 30), false}, // Saturday is not a Friday.
	} {
		if got := held(t, w, "production", c.at); got != c.want {
			t.Errorf("held at %s = %v, want %v", c.at.Format(time.RFC3339), got, c.want)
		}
	}
}

// Verifies: GOV-025.
// A daily window whose wall-clock start the clocks jump over begins when the
// gap ends and lasts its full duration; the day before and after are
// unaffected.
func TestCalendar_StartInSpringForwardGap_GOV_025(t *testing.T) {
	t.Parallel()
	// 02:30 does not exist in New York on 2026-03-08: the clocks go from 02:00
	// EST (07:00Z) to 03:00 EDT.
	w := []governance.Window{{
		Name: "nightly", TimeZone: "America/New_York", Start: "2026-03-01T02:30", Duration: time.Hour,
		Recurrence: "FREQ=DAILY",
	}}
	for _, c := range []struct {
		at   time.Time
		want bool
	}{
		{utc(2026, 3, 7, 7, 45), true}, // 02:45 EST the day before.
		{utc(2026, 3, 8, 6, 59), false},
		{utc(2026, 3, 8, 7, 0), true}, // The gap ends: 03:00 EDT.
		{utc(2026, 3, 8, 7, 59), true},
		{utc(2026, 3, 8, 8, 0), false},
		{utc(2026, 3, 9, 6, 45), true}, // 02:45 EDT the day after.
	} {
		if got := held(t, w, "production", c.at); got != c.want {
			t.Errorf("held at %s = %v, want %v", c.at.Format(time.RFC3339), got, c.want)
		}
	}
}

// Verifies: GOV-025.
// A wall-clock start the clocks repeat takes its first occurrence.
func TestCalendar_StartInFallBackOverlap_GOV_025(t *testing.T) {
	t.Parallel()
	// 01:30 happens twice in New York on 2026-11-01: 05:30Z (EDT) and 06:30Z
	// (EST).
	w := []governance.Window{{Name: "once", TimeZone: "America/New_York", Start: "2026-11-01T01:30", Duration: 30 * time.Minute}}
	for _, c := range []struct {
		at   time.Time
		want bool
	}{
		{utc(2026, 11, 1, 5, 29), false},
		{utc(2026, 11, 1, 5, 30), true},
		{utc(2026, 11, 1, 5, 59), true},
		{utc(2026, 11, 1, 6, 0), false},
		{utc(2026, 11, 1, 6, 30), false}, // The second 01:30 is not the start.
	} {
		if got := held(t, w, "production", c.at); got != c.want {
			t.Errorf("held at %s = %v, want %v", c.at.Format(time.RFC3339), got, c.want)
		}
	}
}

// Verifies: GOV-025.
// A window lasts its duration in elapsed time: across the spring-forward
// change a 60-hour window from Friday 18:00 ends an hour earlier on the wall.
func TestCalendar_DurationIsElapsedTime_GOV_025(t *testing.T) {
	t.Parallel()
	w := []governance.Window{{Name: "long", TimeZone: "America/New_York", Start: "2026-03-06T18:00", Duration: 60 * time.Hour}}
	// Starts 23:00Z Friday; 60 hours later is 11:00Z Monday, 07:00 EDT.
	if !held(t, w, "production", utc(2026, 3, 9, 10, 59)) || held(t, w, "production", utc(2026, 3, 9, 11, 0)) {
		t.Error("the window does not end 60 elapsed hours after it starts")
	}
}

// Verifies: GOV-025.
// A recurring window that outlasts a day is found from the dates after the one
// it started on, and UNTIL bounds the series.
func TestCalendar_LongRecurringAndUntil_GOV_025(t *testing.T) {
	t.Parallel()
	w := []governance.Window{{
		Name: "weekends", TimeZone: "UTC", Start: "2026-01-02T20:00", Duration: 56 * time.Hour,
		Recurrence: "FREQ=WEEKLY;BYDAY=FR;UNTIL=20260116",
	}}
	for _, c := range []struct {
		at   time.Time
		want bool
	}{
		{utc(2026, 1, 3, 12, 0), true}, // Saturday of the first weekend.
		{utc(2026, 1, 5, 3, 59), true}, // Monday 04:00 is the end, exclusive.
		{utc(2026, 1, 5, 4, 0), false},
		{utc(2026, 1, 17, 12, 0), true},   // The weekend of the last start, 16 January.
		{utc(2026, 1, 24, 12, 0), false},  // After UNTIL.
		{utc(2025, 12, 27, 12, 0), false}, // Before the series began.
	} {
		if got := held(t, w, "production", c.at); got != c.want {
			t.Errorf("held at %s = %v, want %v", c.at.Format(time.RFC3339), got, c.want)
		}
	}
}

// Verifies: GOV-025.
// A window with an environment key holds only that environment; one without
// holds every environment it is asked about; the first window in force is the
// one reported, with its end.
func TestCalendar_EnvironmentsAndOccurrence_GOV_025(t *testing.T) {
	t.Parallel()
	windows := []governance.Window{
		{Name: "staging-only", EnvironmentKey: "staging", TimeZone: "UTC", Start: "2026-05-01T00:00", Duration: 24 * time.Hour},
		{Name: "everywhere", TimeZone: "UTC", Start: "2026-05-01T10:00", Duration: time.Hour, Reason: "release train"},
	}
	at := utc(2026, 5, 1, 10, 30)
	occ, ok, err := Calendar{}.InForce(licensed(edition.StateValid), windows, "production", at)
	if err != nil || !ok || occ.Window.Name != "everywhere" || !occ.End.Equal(utc(2026, 5, 1, 11, 0)) {
		t.Errorf("production: %+v %v %v", occ, ok, err)
	}
	if occ, ok, _ = (Calendar{}).InForce(licensed(edition.StateValid), windows, "staging", at); !ok || occ.Window.Name != "staging-only" {
		t.Errorf("staging: the first window in force was %q", occ.Window.Name)
	}
	if held(t, windows[:1], "production", at) {
		t.Error("a window of staging held production")
	}
}

// Verifies: GOV-025.
// A window outside the supported subset, or one that cannot be evaluated, is
// refused when written, never approximated.
func TestCalendar_ValidateRefusals_GOV_025(t *testing.T) {
	t.Parallel()
	ok := governance.Window{Name: "w", TimeZone: "Europe/Berlin", Start: "2026-12-30T18:00", Duration: time.Hour}
	if err := (Calendar{}).Validate(licensed(edition.StateValid), ok); err != nil {
		t.Fatalf("a valid window: %v", err)
	}
	for name, mutate := range map[string]func(*governance.Window){
		"unknown zone":       func(w *governance.Window) { w.TimeZone = "Mars/Olympus" },
		"local zone":         func(w *governance.Window) { w.TimeZone = "Local" },
		"no zone":            func(w *governance.Window) { w.TimeZone = "" },
		"bad start":          func(w *governance.Window) { w.Start = "2026-12-30 18:00" },
		"zero duration":      func(w *governance.Window) { w.Duration = 0 },
		"monthly":            func(w *governance.Window) { w.Recurrence = "FREQ=MONTHLY" },
		"interval":           func(w *governance.Window) { w.Recurrence = "FREQ=DAILY;INTERVAL=2" },
		"count":              func(w *governance.Window) { w.Recurrence = "FREQ=DAILY;COUNT=3" },
		"byday on daily":     func(w *governance.Window) { w.Recurrence = "FREQ=DAILY;BYDAY=MO" },
		"unknown day":        func(w *governance.Window) { w.Recurrence = "FREQ=WEEKLY;BYDAY=XX" },
		"repeated key":       func(w *governance.Window) { w.Recurrence = "FREQ=DAILY;FREQ=WEEKLY" },
		"no frequency":       func(w *governance.Window) { w.Recurrence = "BYDAY=MO" },
		"empty value":        func(w *governance.Window) { w.Recurrence = "FREQ=" },
		"bad until":          func(w *governance.Window) { w.Recurrence = "FREQ=DAILY;UNTIL=2027-01-01" },
		"until before start": func(w *governance.Window) { w.Recurrence = "FREQ=DAILY;UNTIL=20261201" },
	} {
		w := ok
		mutate(&w)
		if err := (Calendar{}).Validate(licensed(edition.StateValid), w); !errors.Is(err, governance.ErrInvalidWindow) {
			t.Errorf("%s: %v, want ErrInvalidWindow", name, err)
		}
	}
}

// Verifies: GOV-025, GOV-030.
// Writing needs the licence for writing, reading the licence for reading;
// without either the calendar refuses, and an expired licence still reads.
func TestCalendar_Licence_GOV_030(t *testing.T) {
	t.Parallel()
	w := governance.Window{Name: "w", TimeZone: "UTC", Start: "2026-05-01T10:00", Duration: time.Hour}
	community := context.Background()
	if err := (Calendar{}).Validate(community, w); err == nil {
		t.Error("Validate without a licence succeeded")
	}
	if _, _, err := (Calendar{}).InForce(community, []governance.Window{w}, "production", utc(2026, 5, 1, 10, 30)); err == nil {
		t.Error("InForce without a licence succeeded")
	}
	expired := licensed(edition.StateExpired)
	if err := (Calendar{}).Validate(expired, w); err == nil {
		t.Error("Validate with an expired licence succeeded")
	}
	if _, ok, err := (Calendar{}).InForce(expired, []governance.Window{w}, "production", utc(2026, 5, 1, 10, 30)); err != nil || !ok {
		t.Errorf("an expired licence reads: %v %v", ok, err)
	}
	for _, state := range []edition.State{edition.StateAbsent, edition.StateInvalid} {
		if err := (Calendar{}).Validate(licensed(state), w); err == nil {
			t.Errorf("Validate with a %s licence succeeded", state)
		}
	}
}

// Verifies: GOV-025.
// A stored window that cannot be evaluated is an error, never a window that
// is out of force.
func TestCalendar_UnevaluableWindowFailsClosed_GOV_025(t *testing.T) {
	t.Parallel()
	bad := []governance.Window{{Name: "broken", TimeZone: "Mars/Olympus", Start: "2026-05-01T10:00", Duration: time.Hour}}
	if _, ok, err := (Calendar{}).InForce(licensed(edition.StateValid), bad, "production", utc(2026, 5, 1, 10, 30)); err == nil || ok {
		t.Errorf("InForce = %v, %v, want an error", ok, err)
	}
}

// Verifies: GOV-025.
// A weekly window without BYDAY recurs on the weekday of its first start.
func TestCalendar_WeeklyDefaultsToTheStartWeekday_GOV_025(t *testing.T) {
	t.Parallel()
	w := []governance.Window{{Name: "w", TimeZone: "UTC", Start: "2026-05-01T10:00", Duration: time.Hour, Recurrence: "FREQ=WEEKLY"}} // A Friday.
	if !held(t, w, "production", utc(2026, 5, 8, 10, 30)) || held(t, w, "production", utc(2026, 5, 9, 10, 30)) {
		t.Error("the window does not recur on Fridays only")
	}
}
