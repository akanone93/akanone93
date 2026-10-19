// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Package calendar converts dates between calendar systems through the
// Julian day number (I18N-003, ADR-0067). It is plain integer arithmetic, so
// Go and Dart agree to the day and the compiler needs no platform calendar.
//
// Years are extended years: Gregorian year 0 is 1 BC, and Ethiopic year 0 is
// the last Amete Alem year. EraOf maps an extended year to the era index and
// year within the era that package l10n formats.
package calendar

import (
	"errors"
	"fmt"

	"github.com/nightCode42/plux3/backend/internal/l10n"
)

//go:generate go run ./internal/gen ../../../../schema/l10n/calendars tables_gen.go ../../../../packages/plux_flutter/lib/src/l10n/calendar/tables.g.dart

// ErrInvalidDate is a month or day outside its calendar's range.
var ErrInvalidDate = errors.New("calendar: invalid date")

// ErrUnknownSystem is a calendar system id this package does not implement.
var ErrUnknownSystem = errors.New("calendar: unknown system")

// The system ids, as used by localisation.calendar and FormatDate.
const (
	Gregorian       = "gregorian"
	Ethiopic        = "ethiopic"
	IslamicUmalqura = "islamic-umalqura"
	Persian         = "persian"
)

// Date is a date in some calendar: Year is the extended year, Month and Day
// start at 1.
type Date struct {
	Year, Month, Day int
}

// System is one calendar.
type System interface {
	// ID is the system id.
	ID() string
	// ToJDN returns the Julian day number of d, or ErrInvalidDate.
	ToJDN(d Date) (int, error)
	// FromJDN returns the date of a Julian day number.
	FromJDN(jdn int) Date
	// MonthsInYear is the number of months of the year.
	MonthsInYear(year int) int
	// DaysInMonth is the length of a month, or 0 if the month does not exist.
	DaysInMonth(year, month int) int
	// IsLeapYear reports whether the year has the extra day.
	IsLeapYear(year int) bool
	// EraOf returns the era index (as l10n indexes the calendar's era names)
	// and the year within that era.
	EraOf(year int) (era, eraYear int)
	// YearOf is the inverse of EraOf: the extended year of a year within an
	// era, or false when the era does not exist.
	YearOf(era, eraYear int) (year int, ok bool)
	// OutOfTable reports whether the year lies outside the pinned table the
	// system relies on, so that its dates are an approximation (PLX-1615).
	// Systems with no table always return false.
	OutOfTable(year int) bool
}

// IDs lists the implemented systems.
func IDs() []string {
	return []string{Gregorian, Ethiopic, IslamicUmalqura, Persian}
}

// ForID returns the system of an id.
func ForID(id string) (System, error) {
	switch id {
	case Gregorian:
		return gregorian{}, nil
	case Ethiopic:
		return ethiopic{}, nil
	case IslamicUmalqura:
		return umalqura{}, nil
	case Persian:
		return persian{}, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownSystem, id)
}

// Convert returns the date in system to of a date in system from.
func Convert(from System, d Date, to System) (Date, error) {
	jdn, err := from.ToJDN(d)
	if err != nil {
		return Date{}, err //nolint:wrapcheck // reported as is
	}
	return to.FromJDN(jdn), nil
}

// Weekday returns the weekday of a Julian day number, 0 for Sunday.
func Weekday(jdn int) int {
	return floorMod(jdn+1, 7)
}

// FormatInput converts a Julian day number and time of day to the
// l10n.CalendarDate that Locale.FormatDate formats in the system's calendar.
func FormatInput(sys System, jdn, hour, minute, second int) l10n.CalendarDate {
	d := sys.FromJDN(jdn)
	era, y := sys.EraOf(d.Year)
	return l10n.CalendarDate{
		Era: era, Year: y, Month: d.Month, Day: d.Day, Weekday: Weekday(jdn),
		Hour: hour, Minute: minute, Second: second,
	}
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func floorMod(a, b int) int {
	return a - floorDiv(a, b)*b
}

// checkDate validates the month and day of d against s.
func checkDate(s System, d Date) error {
	if d.Month < 1 || d.Month > s.MonthsInYear(d.Year) || d.Day < 1 || d.Day > s.DaysInMonth(d.Year, d.Month) {
		return fmt.Errorf("%w: %s %d-%d-%d", ErrInvalidDate, s.ID(), d.Year, d.Month, d.Day)
	}
	return nil
}
