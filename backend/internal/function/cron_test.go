// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// at parses a time in the stamp layout.
func at(t *testing.T, value string) time.Time {
	t.Helper()
	v, err := time.Parse(stamp, value)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

const stamp = "2006-01-02 15:04"

func TestFN025CronNext(t *testing.T) {
	for _, c := range []struct {
		expr, zone, after, want string
	}{
		{"* * * * *", "", "2026-03-01 10:00", "2026-03-01 10:01"},
		{"*/15 * * * *", "UTC", "2026-03-01 10:07", "2026-03-01 10:15"},
		{"0 9 * * 1-5", "", "2026-03-06 09:00", "2026-03-09 09:00"}, // Friday to Monday
		{"30 2 29 2 *", "", "2026-01-01 00:00", "2028-02-29 02:30"},
		{"@hourly", "", "2026-03-01 10:00", "2026-03-01 11:00"},
		{"@daily", "", "2026-03-01 10:00", "2026-03-02 00:00"},
		{"@midnight", "", "2026-12-31 23:59", "2027-01-01 00:00"},
		{"@weekly", "", "2026-03-01 10:00", "2026-03-08 00:00"}, // 1 March 2026 is a Sunday
		{"@monthly", "", "2026-03-01 00:00", "2026-04-01 00:00"},
		{"@yearly", "", "2026-03-01 00:00", "2027-01-01 00:00"},
		{"0 0 1,15 * *", "", "2026-03-02 00:00", "2026-03-15 00:00"},
		{"0 12 * jan-mar mon", "", "2026-03-30 12:00", "2027-01-04 12:00"},
		{"5 4 * * 7", "", "2026-03-01 04:05", "2026-03-08 04:05"},  // 7 is Sunday
		{"0 0 13 * 5", "", "2026-03-01 00:00", "2026-03-06 00:00"}, // either day field matches
		{"10-20/5 8 * * *", "", "2026-03-01 08:11", "2026-03-01 08:15"},
		{"0 8 * * *", "Europe/Berlin", "2026-03-01 00:00", "2026-03-01 07:00"}, // CET is UTC+1
		{"0 8 * * *", "Europe/Berlin", "2026-07-01 00:00", "2026-07-01 06:00"}, // CEST is UTC+2
		{"0 8 * * *", "Asia/Kolkata", "2026-03-01 00:00", "2026-03-01 02:30"},
	} {
		s, err := ParseSchedule(c.expr, c.zone)
		if err != nil {
			t.Errorf("%q %q: %v", c.expr, c.zone, err)
			continue
		}
		got := s.Next(at(t, c.after))
		if want := at(t, c.want); !got.Equal(want) {
			t.Errorf("%q in %q after %s = %s, want %s", c.expr, c.zone, c.after, got.Format(stamp), c.want)
		}
	}
}

// FN-025: the schedule is correct across daylight saving.
func TestFN025CronDaylightSaving(t *testing.T) {
	ny := func(expr string, after time.Time, n int) []string {
		s, err := ParseSchedule(expr, "America/New_York")
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for range n {
			after = s.Next(after)
			out = append(out, after.Format("2006-01-02T15:04Z"))
		}
		return out
	}
	eq := func(name string, got, want []string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: %v, want %v", name, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s: %v, want %v", name, got, want)
			}
		}
	}
	// Spring forward, 8 March 2026: 02:00 to 03:00. A 02:30 job has no 02:30
	// that day; the days either side fire at 02:30 local.
	eq("spring", ny("30 2 * * *", at(t, "2026-03-07 00:00"), 3),
		[]string{"2026-03-07T07:30Z", "2026-03-09T06:30Z", "2026-03-10T06:30Z"})
	// Fall back, 1 November 2026: 01:00 to 01:00 again. A 01:30 job fires once.
	eq("fall", ny("30 1 * * *", at(t, "2026-10-31 00:00"), 3),
		[]string{"2026-10-31T05:30Z", "2026-11-01T05:30Z", "2026-11-02T06:30Z"})
	// An hourly job fires in every real hour, the repeated one included.
	eq("hourly", ny("0 * * * *", at(t, "2026-11-01 04:30"), 4),
		[]string{"2026-11-01T05:00Z", "2026-11-01T06:00Z", "2026-11-01T07:00Z", "2026-11-01T08:00Z"})
	// A daily job keeps its local hour across the change.
	eq("daily", ny("0 9 * * *", at(t, "2026-03-06 00:00"), 4),
		[]string{"2026-03-06T14:00Z", "2026-03-07T14:00Z", "2026-03-08T13:00Z", "2026-03-09T13:00Z"})
}

func TestFN025CronNextIsAfterAndMonotonic(t *testing.T) {
	s, err := ParseSchedule("*/7 3-5 * * *", "Australia/Lord_Howe") // a 30-minute DST change
	if err != nil {
		t.Fatal(err)
	}
	last := at(t, "2026-09-25 00:00")
	for range 400 {
		next := s.Next(last)
		if !next.After(last) {
			t.Fatalf("Next(%s) = %s", last, next)
		}
		last = next
	}
	if s.Location().String() != "Australia/Lord_Howe" {
		t.Fatalf("location %s", s.Location())
	}
}

func TestFN025CronInvalid(t *testing.T) {
	for _, c := range [][2]string{
		{"", ""},
		{"* * * *", ""},
		{"* * * * * *", ""},
		{"60 * * * *", ""},
		{"* 24 * * *", ""},
		{"* * 0 * *", ""},
		{"* * 32 * *", ""},
		{"* * * 13 *", ""},
		{"* * * * 8", ""},
		{"*/0 * * * *", ""},
		{"5-1 * * * *", ""},
		{"a * * * *", ""},
		{"@reboot", ""},
		{"@sometimes", ""},
		{"@", ""},
		{"* * * * *", "Mars/Olympus"},
		{"* * * * *", "Local"},
		{"1,,2 * * * *", ""},
		{"- * * * *", ""},
		{"*/100 * * * *", ""},
	} {
		_, err := ParseSchedule(c[0], c[1])
		if got, _ := plxerr.CodeOf(err); got != plxerr.CronExpressionInvalid {
			t.Errorf("%q %q: %v, want PLX-7150", c[0], c[1], err)
		}
	}
	if _, err := ParseSchedule(string(make([]byte, 300)), ""); err == nil {
		t.Error("an over-long expression parsed")
	}
}

func TestFN025CronNoFireWithinHorizon(t *testing.T) {
	s, err := ParseSchedule("0 0 30 2 *", "") // 30 February
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Next(at(t, "2026-01-01 00:00")); !got.IsZero() {
		t.Fatalf("30 February fires at %s", got)
	}
}

func FuzzParseSchedule(f *testing.F) {
	for _, s := range []string{"* * * * *", "@daily", "*/5 1-3 1,15 jan-mar mon-fri", "99 * * * *"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, expr string) {
		s, err := ParseSchedule(expr, "UTC")
		if err != nil {
			return
		}
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if next := s.Next(base); !next.IsZero() && !next.After(base) {
			t.Fatalf("%q: Next(%s) = %s", expr, base, next)
		}
	})
}
