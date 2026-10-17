// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package calendar

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/l10n"
)

// TestVectors verifies I18N-003: every ICU-generated case converts from the
// day number to the date and back, in each non-Gregorian system.
func TestVectors(t *testing.T) {
	b, err := os.ReadFile("../../../../schema/testdata/l10n/calendars.json")
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]json.RawMessage
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{Ethiopic, IslamicUmalqura, Persian} {
		var cases [][5]int
		if err := json.Unmarshal(file[id], &cases); err != nil {
			t.Fatal(err)
		}
		if len(cases) < 2000 {
			t.Errorf("%s: %d cases, want at least 2000", id, len(cases))
		}
		sys, err := ForID(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cases {
			jdn := c[0]
			d := sys.FromJDN(jdn)
			era, y := sys.EraOf(d.Year)
			if era != c[1] || y != c[2] || d.Month != c[3] || d.Day != c[4] {
				t.Fatalf("%s FromJDN(%d) = era %d %d-%d-%d, ICU says era %d %d-%d-%d",
					id, jdn, era, y, d.Month, d.Day, c[1], c[2], c[3], c[4])
			}
			back, err := sys.ToJDN(d)
			if err != nil || back != jdn {
				t.Fatalf("%s ToJDN(%v) = %d, %v; want %d", id, d, back, err, jdn)
			}
		}
	}
}

// TestRoundTrip verifies I18N-003: day numbers and dates are inverse, month
// lengths add up to the year, and consecutive days advance by one, for every
// system over a wide range including the Umm al-Qura table edges.
func TestRoundTrip(t *testing.T) {
	for _, id := range IDs() {
		sys, _ := ForID(id)
		prev := sys.FromJDN(2000000)
		for jdn := 2000001; jdn < 2700000; jdn++ {
			d := sys.FromJDN(jdn)
			got, err := sys.ToJDN(d)
			if err != nil || got != jdn {
				t.Fatalf("%s: %d -> %v -> %d, %v", id, jdn, d, got, err)
			}
			next := d.Day == prev.Day+1 && d.Month == prev.Month && d.Year == prev.Year
			rolled := d.Day == 1 && (d.Month == prev.Month+1 && d.Year == prev.Year || d.Month == 1 && d.Year == prev.Year+1)
			if !next && !rolled {
				t.Fatalf("%s: day %d is %v after %v", id, jdn, d, prev)
			}
			if rolled && prev.Day != sys.DaysInMonth(prev.Year, prev.Month) {
				t.Fatalf("%s: %v is not the last day of its month", id, prev)
			}
			prev = d
		}
	}
}

// TestGregorian verifies I18N-003 for the proleptic Gregorian calendar.
func TestGregorian(t *testing.T) {
	g, _ := ForID(Gregorian)
	for _, c := range []struct {
		d   Date
		jdn int
	}{
		{Date{2000, 1, 1}, 2451545},
		{Date{1970, 1, 1}, 2440588},
		{Date{1582, 10, 15}, 2299161},
		{Date{-4713, 11, 24}, 0},
		{Date{0, 12, 31}, 1721425},
	} {
		if got, err := g.ToJDN(c.d); err != nil || got != c.jdn {
			t.Errorf("ToJDN(%v) = %d, %v; want %d", c.d, got, err, c.jdn)
		}
		if got := g.FromJDN(c.jdn); got != c.d {
			t.Errorf("FromJDN(%d) = %v; want %v", c.jdn, got, c.d)
		}
	}
	for _, c := range []struct {
		y    int
		leap bool
	}{{1900, false}, {2000, true}, {2024, true}, {2023, false}, {0, true}, {-4, true}} {
		if g.IsLeapYear(c.y) != c.leap {
			t.Errorf("IsLeapYear(%d) = %v", c.y, !c.leap)
		}
	}
	if era, y := g.EraOf(0); era != 0 || y != 1 {
		t.Errorf("EraOf(0) = %d, %d; want BC 1", era, y)
	}
}

// TestEthiopicEras verifies I18N-003: Amete Alem before year 1, a 6-day
// Pagume in leap years.
func TestEthiopicEras(t *testing.T) {
	e, _ := ForID(Ethiopic)
	if era, y := e.EraOf(0); era != 0 || y != 5500 {
		t.Errorf("EraOf(0) = %d, %d; want AA 5500", era, y)
	}
	if era, y := e.EraOf(1); era != 1 || y != 1 {
		t.Errorf("EraOf(1) = %d, %d; want AM 1", era, y)
	}
	if e.DaysInMonth(2015, 13) != 6 || e.DaysInMonth(2016, 13) != 5 || e.MonthsInYear(2016) != 13 {
		t.Error("Pagume lengths")
	}
	if got := e.FromJDN(1724221); got != (Date{1, 1, 1}) {
		t.Errorf("FromJDN(1724221) = %v", got)
	}
}

// TestUmalquraOutOfTable verifies I18N-003 and PLX-1615's premise: years
// outside 1300-1600 AH are flagged and use the tabular civil arithmetic.
func TestUmalquraOutOfTable(t *testing.T) {
	u, _ := ForID(IslamicUmalqura)
	for y, want := range map[int]bool{1299: true, 1300: false, 1445: false, 1600: false, 1601: true, 1: true} {
		if u.OutOfTable(y) != want {
			t.Errorf("OutOfTable(%d) = %v", y, !want)
		}
	}
	// 1 Muharram 1 AH is 16 July 622 (Julian) in the civil calendar.
	if jdn, err := u.ToJDN(Date{1, 1, 1}); err != nil || jdn != 1948440 {
		t.Errorf("ToJDN(1-1-1) = %d, %v", jdn, err)
	}
	if era, y := u.EraOf(0); era != 1 || y != 1 {
		t.Errorf("EraOf(0) = %d, %d; want BH 1", era, y)
	}
	if _, err := u.ToJDN(Date{1445, 1, 31}); !errors.Is(err, ErrInvalidDate) {
		t.Errorf("ToJDN(1445-1-31) err = %v", err)
	}
}

// TestPersianLeap verifies I18N-003: the 33-year rule and ICU's corrections.
func TestPersianLeap(t *testing.T) {
	p, _ := ForID(Persian)
	for _, c := range []struct {
		y    int
		leap bool
	}{{1399, true}, {1403, true}, {1404, false}, {1502, false}, {1503, true}} {
		if p.IsLeapYear(c.y) != c.leap {
			t.Errorf("IsLeapYear(%d) = %v", c.y, !c.leap)
		}
	}
	if p.DaysInMonth(1403, 12) != 30 || p.DaysInMonth(1404, 12) != 29 {
		t.Error("Esfand lengths")
	}
}

// TestUnknownSystem verifies the error for an id that is not a system.
func TestUnknownSystem(t *testing.T) {
	if _, err := ForID("hebrew"); !errors.Is(err, ErrUnknownSystem) {
		t.Errorf("err = %v", err)
	}
}

// TestFormatDate verifies I18N-003: FormatDate formats a converted date with
// the locale's names for that calendar.
func TestFormatDate(t *testing.T) {
	en, err := l10n.Load("en")
	if err != nil {
		t.Fatal(err)
	}
	g, _ := ForID(Gregorian)
	jdn, err := g.ToJDN(Date{2024, 3, 11})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ cal, want string }{
		{Gregorian, "Monday, March 11, 2024"},
		{IslamicUmalqura, "Monday, Ramadan 1, 1445 AH"},
		{Persian, "Monday, Esfand 21, 1402 AP"},
		{Ethiopic, "Monday, Megabit 2, 2016 AM"},
	} {
		sys, _ := ForID(c.cal)
		s, err := en.FormatDate(CalendarDate(sys, jdn, 0, 0, 0), c.cal, l10n.DateOptions{DateStyle: "full"})
		if err != nil {
			t.Fatalf("%s: %v", c.cal, err)
		}
		if s != c.want {
			t.Errorf("%s: %q, want %q", c.cal, s, c.want)
		}
	}
}
