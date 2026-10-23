// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package governance

import (
	"context"
	"errors"
	"time"
)

// Errors an extension returns, wrapped with %w. The core maps them to the
// error codes of the API (Appendix F).
var (
	// ErrInvalidWindow reports a window that cannot be evaluated: an unknown
	// time zone, a start that is not a wall-clock time, a recurrence outside
	// the supported subset or a duration that is not positive.
	ErrInvalidWindow = errors.New("governance: the freeze window is not valid")
	// ErrUnavailable reports a feature the running edition does not
	// provide (PLX-8600).
	ErrUnavailable = errors.New("governance: the feature is not part of this edition")
)

// StartLayout is the layout of Window.Start: a wall-clock date and time
// without a zone.
const StartLayout = "2006-01-02T15:04"

// Window is one change-freeze window as an organisation defined it.
type Window struct {
	// ID identifies the window; the core assigns it.
	ID string
	// Name is a label for people, unique in the organisation.
	Name string
	// EnvironmentKey is the key of the environment the window freezes; empty
	// freezes every production environment. A freeze only ever holds
	// changes to production environments.
	EnvironmentKey string
	// TimeZone is an IANA time zone name, such as "Europe/Berlin".
	TimeZone string
	// Start is the wall-clock start in TimeZone, in StartLayout. For a
	// one-off window it is the only start; for a recurring one it is the
	// first start, and the series begins on its date.
	Start string
	// Duration is how long each occurrence lasts, in elapsed time.
	Duration time.Duration
	// Recurrence is empty for a one-off window, and otherwise
	// "FREQ=DAILY" or "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR,SA,SU" (the days in
	// any order), each optionally followed by ";UNTIL=YYYYMMDD".
	Recurrence string
	// Reason says why the window exists, for the person it refuses.
	Reason string
}

// Occurrence is one stretch of time a window is in force.
type Occurrence struct {
	Window Window
	// Start and End bound the occurrence: it is in force from Start,
	// inclusive, until End, exclusive.
	Start, End time.Time
}

// FreezeCalendar decides which freeze windows hold a change back.
//
// The core stores the windows and asks the calendar, under the licence's
// FeatureChangeFreezes, whenever a production change is made.
type FreezeCalendar interface {
	// Validate checks a window before the core stores it. The error wraps
	// ErrInvalidWindow, with a reason a person can act on, or
	// ErrUnavailable.
	Validate(ctx context.Context, w Window) error
	// InForce returns the occurrence of the first window, in the order
	// given, that holds changes to the environment at the instant at, and
	// false when none does. A window with an EnvironmentKey applies only to
	// that environment.
	InForce(ctx context.Context, windows []Window, environmentKey string, at time.Time) (Occurrence, bool, error)
}

// NoFreezes is the community calendar: it accepts no window and holds no
// change back (GOV-025 is an enterprise feature).
type NoFreezes struct{}

var _ FreezeCalendar = NoFreezes{}

// Validate implements FreezeCalendar.
func (NoFreezes) Validate(context.Context, Window) error { return ErrUnavailable }

// InForce implements FreezeCalendar.
func (NoFreezes) InForce(context.Context, []Window, string, time.Time) (Occurrence, bool, error) {
	return Occurrence{}, false, nil
}
