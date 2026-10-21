// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package governance

import (
	"context"
	"fmt"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/governance"
)

// Calendar is the enterprise change-freeze calendar (GOV-025). It holds no
// state; the zero value is ready to use.
type Calendar struct{}

var _ governance.FreezeCalendar = Calendar{}

// Validate implements governance.FreezeCalendar. It needs the licence's
// FeatureChangeFreezes for writing, and refuses a window it could not
// evaluate later.
func (Calendar) Validate(ctx context.Context, w governance.Window) error {
	if err := edition.Require(ctx, edition.FeatureChangeFreezes); err != nil {
		return fmt.Errorf("governance: %w", err)
	}
	_, err := parse(w)
	return err
}

// InForce implements governance.FreezeCalendar. It needs the licence's
// FeatureChangeFreezes for reading. A window that cannot be evaluated is an
// error, not a window that is out of force.
func (Calendar) InForce(ctx context.Context, windows []governance.Window, environmentKey string, at time.Time) (governance.Occurrence, bool, error) {
	if err := edition.RequireRead(ctx, edition.FeatureChangeFreezes); err != nil {
		return governance.Occurrence{}, false, fmt.Errorf("governance: %w", err)
	}
	for _, w := range windows {
		if w.EnvironmentKey != "" && w.EnvironmentKey != environmentKey {
			continue
		}
		p, err := parse(w)
		if err != nil {
			return governance.Occurrence{}, false, fmt.Errorf("window %q: %w", w.Name, err)
		}
		if occ, held := p.occurrenceAt(at); held {
			return governance.Occurrence{Window: w, Start: occ.start, End: occ.end}, true, nil
		}
	}
	return governance.Occurrence{}, false, nil
}

// parsed is a window ready to evaluate.
type parsed struct {
	loc      *time.Location
	first    date
	hour     int
	minute   int
	duration time.Duration
	// recurring is set when the window has a rule.
	recurring bool
	rule      rule
}

// occurrence is one start and end, as instants.
type occurrence struct{ start, end time.Time }

// parse reads a window, or returns an error wrapping
// governance.ErrInvalidWindow.
func parse(w governance.Window) (parsed, error) {
	if w.TimeZone == "" || w.TimeZone == "Local" {
		return parsed{}, invalid("the time zone is an IANA name such as Europe/Berlin")
	}
	loc, err := time.LoadLocation(w.TimeZone)
	if err != nil {
		return parsed{}, invalid("%q is not a time zone the server knows", w.TimeZone)
	}
	start, err := time.Parse(governance.StartLayout, w.Start)
	if err != nil {
		return parsed{}, invalid("the start %q is not a wall-clock time as %s", w.Start, governance.StartLayout)
	}
	if w.Duration <= 0 {
		return parsed{}, invalid("the duration is not positive")
	}
	p := parsed{loc: loc, first: dateOf(start), hour: start.Hour(), minute: start.Minute(), duration: w.Duration}
	if w.Recurrence == "" {
		return p, nil
	}
	p.recurring = true
	if p.rule, err = parseRule(w.Recurrence, p.first); err != nil {
		return parsed{}, err
	}
	return p, nil
}

// occurrenceAt returns the occurrence that holds the instant, if any. An
// occurrence that began on an earlier date reaches it when the window is
// longer than a day, so the dates that could have started it are searched,
// the latest first.
func (p parsed) occurrenceAt(at time.Time) (occurrence, bool) {
	if !p.recurring {
		return p.within(p.first, at)
	}
	today := dateOf(at.In(p.loc))
	for back := 0; back <= int(p.duration/(24*time.Hour))+1; back++ {
		d := today.plusDays(-back)
		if !p.rule.startsOn(d, p.first) {
			continue
		}
		if occ, held := p.within(d, at); held {
			return occ, true
		}
	}
	return occurrence{}, false
}

// within returns the occurrence that starts on the date, if it holds the
// instant.
func (p parsed) within(d date, at time.Time) (occurrence, bool) {
	start := startOn(d, p.hour, p.minute, p.loc)
	occ := occurrence{start: start, end: start.Add(p.duration)}
	return occ, !at.Before(occ.start) && at.Before(occ.end)
}

// startOn resolves a wall-clock time on a date to an instant. A time the
// clocks jumped over is the moment the gap ends; a time the clocks repeated
// is its first occurrence; every other time is itself.
func startOn(d date, hour, minute int, loc *time.Location) time.Time {
	wall := time.Date(d.year, d.month, d.day, hour, minute, 0, 0, time.UTC)
	_, before := wall.Add(-24 * time.Hour).In(loc).Zone()
	_, after := wall.Add(24 * time.Hour).In(loc).Zone()
	first := wall.Add(-time.Duration(before) * time.Second)
	second := wall.Add(-time.Duration(after) * time.Second)
	if second.Before(first) {
		first, second = second, first
	}
	switch {
	case showsWall(first, d, hour, minute, loc):
		return first
	case showsWall(second, d, hour, minute, loc):
		return second
	}
	_, end := first.In(loc).ZoneBounds()
	return end
}

// showsWall reports whether the clocks of loc read the given date and time at
// the instant.
func showsWall(t time.Time, d date, hour, minute int, loc *time.Location) bool {
	local := t.In(loc)
	h, m, _ := local.Clock()
	return dateOf(local) == d && h == hour && m == minute
}
