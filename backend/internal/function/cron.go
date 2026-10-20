// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"strconv"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// maxCronLength bounds an expression: nothing legitimate is longer.
const maxCronLength = 256

// cronHorizon is how far Next looks: a 29 February schedule can be eight
// years away.
const cronHorizon = 9 * 366 * 24 * time.Hour

// macros are the descriptors a schedule may use instead of five fields.
var macros = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

var (
	monthNames = []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}
	dayNames   = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
)

// Schedule is a parsed cron schedule in a time zone (FN-025). It is a value
// and safe to share.
type Schedule struct {
	loc                      *time.Location
	minute, hour, dom, month uint64
	dow                      uint64
	domAny, dowAny, hourAny  bool
}

// ParseSchedule reads a five-field cron expression — minute, hour, day of
// month, month, day of week — or one of @hourly, @daily, @midnight,
// @weekly, @monthly, @yearly and @annually, in a zone: "UTC" (the default
// when empty) or an IANA name such as "Europe/Berlin". A field is "*", a
// number, a range "a-b", a list "a,b", and a step "*/n" or "a-b/n"; months
// and weekdays may be named. When both day fields are restricted a day
// matches if either does, as in Vixie cron. An invalid expression or zone is
// PLX-7150.
func ParseSchedule(expr, zone string) (Schedule, error) {
	if len(expr) == 0 || len(expr) > maxCronLength {
		return Schedule{}, plxerr.New(plxerr.CronExpressionInvalid, "a cron expression has 1 to %d characters", maxCronLength)
	}
	s := Schedule{}
	switch zone {
	case "", "UTC":
		s.loc = time.UTC
	case "Local":
		return Schedule{}, plxerr.New(plxerr.CronExpressionInvalid, "a schedule names UTC or an IANA zone, not the server's local zone")
	default:
		loc, err := time.LoadLocation(zone)
		if err != nil {
			return Schedule{}, plxerr.New(plxerr.CronExpressionInvalid, "%q is not a time zone", zone)
		}
		s.loc = loc
	}
	if strings.HasPrefix(expr, "@") {
		m, ok := macros[strings.ToLower(expr)]
		if !ok {
			return Schedule{}, plxerr.New(plxerr.CronExpressionInvalid, "%q is not a schedule descriptor; use @hourly, @daily, @midnight, @weekly, @monthly or @yearly", expr)
		}
		expr = m
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return Schedule{}, plxerr.New(plxerr.CronExpressionInvalid, "a cron expression has 5 fields (minute hour day-of-month month day-of-week), not %d", len(fields))
	}
	var err error
	if s.minute, _, err = parseField(fields[0], "minute", 0, 59, nil); err != nil {
		return Schedule{}, err
	}
	if s.hour, s.hourAny, err = parseField(fields[1], "hour", 0, 23, nil); err != nil {
		return Schedule{}, err
	}
	if s.dom, s.domAny, err = parseField(fields[2], "day of month", 1, 31, nil); err != nil {
		return Schedule{}, err
	}
	if s.month, _, err = parseField(fields[3], "month", 1, 12, monthNames); err != nil {
		return Schedule{}, err
	}
	if s.dow, s.dowAny, err = parseField(fields[4], "day of week", 0, 7, dayNames); err != nil {
		return Schedule{}, err
	}
	if s.dow&(1<<7) != 0 { // 7 is Sunday too
		s.dow = s.dow&^(1<<7) | 1
	}
	return s, nil
}

// parseField reads one field into a bit set; any is true when it starts
// with "*". names, when given, are the names of min, min+1, ….
func parseField(field, what string, lo, hi int, names []string) (bits uint64, wild bool, err error) {
	bad := func(format string, args ...any) error {
		return plxerr.New(plxerr.CronExpressionInvalid, "the "+what+" field "+format, args...)
	}
	wild = strings.HasPrefix(field, "*")
	for _, part := range strings.Split(field, ",") {
		rng, stepText, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			n, perr := strconv.Atoi(stepText)
			if perr != nil || n <= 0 || n > hi {
				return 0, false, bad("has the step %q; a step is a positive number up to %d", stepText, hi)
			}
			step = n
		}
		from, to := lo, hi
		if rng != "*" {
			a, b, isRange := strings.Cut(rng, "-")
			var perr error
			if from, perr = cronValue(a, lo, hi, names); perr != nil {
				return 0, false, bad("has %q, which is not %d to %d", a, lo, hi)
			}
			to = from
			if isRange {
				if to, perr = cronValue(b, lo, hi, names); perr != nil {
					return 0, false, bad("has %q, which is not %d to %d", b, lo, hi)
				}
			} else if hasStep {
				to = hi
			}
			if from > to {
				return 0, false, bad("has the range %q running backwards", rng)
			}
		}
		for v := from; v <= to; v += step {
			bits |= 1 << uint(v)
		}
	}
	return bits, wild, nil
}

// cronValue reads a number or a name within bounds.
func cronValue(text string, lo, hi int, names []string) (int, error) {
	if n, err := strconv.Atoi(text); err == nil {
		if n < lo || n > hi {
			return 0, plxerr.New(plxerr.CronExpressionInvalid, "out of range")
		}
		return n, nil
	}
	for i, name := range names {
		if strings.EqualFold(text, name) {
			return i + lo, nil
		}
	}
	return 0, plxerr.New(plxerr.CronExpressionInvalid, "unknown")
}

// Location is the schedule's zone.
func (s Schedule) Location() *time.Location { return s.loc }

// Next returns the first fire time strictly after the given instant, in
// UTC, or the zero time if there is none within nine years. Times are
// matched on the zone's wall clock, and daylight saving is handled as cron
// implementations do: a wall-clock time that a spring-forward skips does not
// occur that day, a fixed hour that a fall-back repeats fires once, and a
// schedule with a wildcard hour ("*", "*/2", @hourly) fires in every real
// hour.
func (s Schedule) Next(after time.Time) time.Time {
	t := after.UTC().Truncate(time.Minute).Add(time.Minute)
	limit := after.Add(cronHorizon)
	for t.Before(limit) {
		l := t.In(s.loc)
		switch {
		case s.month&(1<<uint(l.Month())) == 0:
			t = jump(t, time.Date(l.Year(), l.Month()+1, 1, 0, 0, 0, 0, s.loc))
		case !s.dayMatches(l):
			t = jump(t, time.Date(l.Year(), l.Month(), l.Day()+1, 0, 0, 0, 0, s.loc))
		case s.hour&(1<<uint(l.Hour())) == 0:
			t = t.Add(time.Duration(60-l.Minute()) * time.Minute)
		case s.minute&(1<<uint(l.Minute())) == 0:
			t = t.Add(time.Minute)
		case !s.hourAny && !time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute(), 0, 0, s.loc).Equal(t):
			// The second pass through a repeated wall-clock time.
			t = t.Add(time.Minute)
		default:
			return t.UTC()
		}
	}
	return time.Time{}
}

// jump moves to the target, never backwards, so a zone's odd transition
// cannot loop.
func jump(from, to time.Time) time.Time {
	if !to.After(from) {
		return from.Add(time.Minute)
	}
	return to
}

// dayMatches applies the day-of-month and day-of-week fields.
func (s Schedule) dayMatches(l time.Time) bool {
	dom := s.dom&(1<<uint(l.Day())) != 0
	dow := s.dow&(1<<uint(l.Weekday())) != 0
	if !s.domAny && !s.dowAny {
		return dom || dow
	}
	return dom && dow
}
