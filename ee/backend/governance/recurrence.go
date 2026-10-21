// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package governance

import (
	"fmt"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/ext/governance"
)

// untilLayout is the layout of a recurrence's UNTIL: a local date.
const untilLayout = "20060102"

// weekdays names the days BYDAY accepts.
var weekdays = map[string]time.Weekday{
	"MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday, "TH": time.Thursday,
	"FR": time.Friday, "SA": time.Saturday, "SU": time.Sunday,
}

// rule is a parsed recurrence.
type rule struct {
	// weekly is whether the window recurs on days of the week, and not every
	// day; days holds them.
	weekly bool
	days   map[time.Weekday]bool
	// until is the last local date a window may start on, when hasUntil.
	until    date
	hasUntil bool
}

// date is a calendar date without a zone.
type date struct {
	year, day int
	month     time.Month
}

// dateOf returns the date of t on its own wall clock.
func dateOf(t time.Time) date {
	y, m, d := t.Date()
	return date{year: y, month: m, day: d}
}

// less reports whether d is before o.
func (d date) less(o date) bool {
	if d.year != o.year {
		return d.year < o.year
	}
	if d.month != o.month {
		return d.month < o.month
	}
	return d.day < o.day
}

// plusDays returns the date n days after d, n negative for before.
func (d date) plusDays(n int) date {
	return dateOf(time.Date(d.year, d.month, d.day+n, 12, 0, 0, 0, time.UTC))
}

// weekday returns the day of the week of d.
func (d date) weekday() time.Weekday {
	return time.Date(d.year, d.month, d.day, 12, 0, 0, 0, time.UTC).Weekday()
}

// parseRule reads a recurrence. first is the date of the first start, whose
// weekday a weekly rule without BYDAY recurs on. It returns an error wrapping
// governance.ErrInvalidWindow for anything outside the supported subset.
func parseRule(s string, first date) (rule, error) {
	var (
		r    rule
		freq string
		seen = map[string]bool{}
	)
	for _, part := range strings.Split(s, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || value == "" || seen[key] {
			return rule{}, invalid("the recurrence %q is not KEY=VALUE pairs separated by semicolons", s)
		}
		seen[key] = true
		switch key {
		case "FREQ":
			freq = value
		case "BYDAY":
			days, err := parseDays(value)
			if err != nil {
				return rule{}, err
			}
			r.days = days
		case "UNTIL":
			t, err := time.Parse(untilLayout, value)
			if err != nil {
				return rule{}, invalid("UNTIL=%s is not a date as YYYYMMDD", value)
			}
			r.until, r.hasUntil = dateOf(t), true
		default:
			return rule{}, invalid("the recurrence part %s is not supported; use FREQ, BYDAY and UNTIL", key)
		}
	}
	return finish(r, freq, first)
}

// finish checks that the parts fit the frequency.
func finish(r rule, freq string, first date) (rule, error) {
	switch freq {
	case "DAILY":
		if r.days != nil {
			return rule{}, invalid("BYDAY goes with FREQ=WEEKLY, not FREQ=DAILY")
		}
	case "WEEKLY":
		r.weekly = true
		if r.days == nil {
			r.days = map[time.Weekday]bool{first.weekday(): true}
		}
	default:
		return rule{}, invalid("FREQ is DAILY or WEEKLY")
	}
	if r.hasUntil && r.until.less(first) {
		return rule{}, invalid("UNTIL is before the first start")
	}
	return r, nil
}

// parseDays reads a BYDAY list.
func parseDays(s string) (map[time.Weekday]bool, error) {
	days := map[time.Weekday]bool{}
	for _, name := range strings.Split(s, ",") {
		day, ok := weekdays[name]
		if !ok {
			return nil, invalid("%q is not a day; use MO, TU, WE, TH, FR, SA or SU", name)
		}
		days[day] = true
	}
	return days, nil
}

// startsOn reports whether a window of the series starts on the date.
func (r rule) startsOn(d, first date) bool {
	switch {
	case d.less(first), r.hasUntil && r.until.less(d):
		return false
	case r.weekly:
		return r.days[d.weekday()]
	}
	return true
}

// invalid builds an error that wraps governance.ErrInvalidWindow.
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", governance.ErrInvalidWindow, fmt.Sprintf(format, args...))
}
