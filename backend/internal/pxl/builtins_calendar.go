// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package pxl

import (
	"github.com/nightCode42/plux3/backend/internal/l10n/calendar"
)

// unixEpochJDN is the Julian day number of 1970-01-01, day 0 of a PXL date.
const unixEpochJDN = 2440588

// calendarFns implements the calendar group: toCalendar and fromCalendar
// (PXL-006, I18N-003). An unknown system is an invalidArgument error.
func calendarFns() map[string]builtinFn {
	return map[string]builtinFn{
		"toCalendar(date,string)": func(_ *vm, a []Value) (Value, *EvalError) {
			sys, err := calendar.ForID(arg[string](a, 1))
			if err != nil {
				return nil, evalErr(ErrorInvalidArgument, "%q is not a calendar system", a[1])
			}
			jdn := int(arg[Date](a, 0)) + unixEpochJDN
			d := sys.FromJDN(jdn)
			era, year := sys.EraOf(d.Year)
			return Map{
				"era":   int64(era),
				"year":  int64(year),
				"month": int64(d.Month),
				"day":   int64(d.Day),
			}, nil
		},
		"fromCalendar(map<string,int>,string)": func(_ *vm, a []Value) (Value, *EvalError) {
			sys, err := calendar.ForID(arg[string](a, 1))
			if err != nil {
				return nil, evalErr(ErrorInvalidArgument, "%q is not a calendar system", a[1])
			}
			parts := arg[Map](a, 0)
			get := func(k string) (int64, bool) { v, ok := parts[k].(int64); return v, ok }
			year, okY := get("year")
			month, okM := get("month")
			day, okD := get("day")
			if !okY || !okM || !okD {
				return nil, evalErr(ErrorInvalidArgument, "parts need year, month and day")
			}
			if year < -1<<31 || year > 1<<31 || month < 0 || month > 1<<31 || day < 0 || day > 1<<31 {
				return nil, evalErr(ErrorInvalidArgument, "the date is out of range")
			}
			y := int(year)
			if era, ok := get("era"); ok {
				ey, valid := sys.YearOf(int(era), y)
				if !valid {
					return nil, evalErr(ErrorInvalidArgument, "%d is not an era of %s", era, sys.ID())
				}
				y = ey
			}
			jdn, derr := sys.ToJDN(calendar.Date{Year: y, Month: int(month), Day: int(day)})
			if derr != nil {
				return nil, evalErr(ErrorInvalidArgument, "%d-%d-%d is not a date in the %s calendar", year, month, day, sys.ID())
			}
			days := int64(jdn - unixEpochJDN)
			if !validDate(days) {
				return nil, evalErr(ErrorInvalidArgument, "the date leaves years 1–9999")
			}
			return Date(days), nil
		},
	}
}
