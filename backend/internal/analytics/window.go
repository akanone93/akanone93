// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"errors"
	"fmt"
	"time"
)

// Window is the daily off-peak window in which a scheduled retention change
// takes effect (maintainer decision Q17): from Start to End, both minutes
// after local midnight in Location. A window whose End is not after its
// Start spans midnight.
type Window struct {
	Start, End int
	Location   *time.Location
}

// ParseWindow reads "HH:MM" times and an IANA zone name.
func ParseWindow(start, end, zone string) (Window, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" || zone == "Local" {
		return Window{}, fmt.Errorf("analytics: %q is not an IANA time zone", zone)
	}
	s, err := minutes(start)
	if err != nil {
		return Window{}, err
	}
	e, err := minutes(end)
	if err != nil {
		return Window{}, err
	}
	if s == e {
		return Window{}, errors.New("analytics: the maintenance window is empty")
	}
	return Window{Start: s, End: e, Location: loc}, nil
}

func minutes(hhmm string) (int, error) {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return 0, fmt.Errorf("analytics: %q is not a time of day as HH:MM", hhmm)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// Open reports whether the window is open at t. It reads the wall clock of
// the installation's zone, so on the day a clock moves forward the window
// is open for the minutes of it that exist, and on the day it moves back,
// for the minutes that occur twice, twice.
func (w Window) Open(t time.Time) bool {
	l := t.In(w.Location)
	m := l.Hour()*60 + l.Minute()
	if w.Start < w.End {
		return m >= w.Start && m < w.End
	}
	return m >= w.Start || m < w.End
}

// maxLookahead bounds the search for the next open minute: a window opens
// at least once a day, and two days cover any change of the clock.
const maxLookahead = 49 * time.Hour

// NextOpen returns the first minute at or after t at which the window is
// open: t itself when it is open now.
func (w Window) NextOpen(t time.Time) time.Time {
	t = t.UTC().Truncate(time.Minute)
	if w.Open(t) {
		return t
	}
	for d := time.Minute; d <= maxLookahead; d += time.Minute {
		if c := t.Add(d); w.Open(c) {
			return c
		}
	}
	return t
}
