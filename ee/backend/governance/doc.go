// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

// Package governance is the enterprise edition's change-freeze calendar and
// compliance report writer (GOV-025, GOV-026, ADR-0076).
//
// Calendar implements governance.FreezeCalendar: it reads the windows an
// organisation defined, with their time zones, wall-clock starts and
// recurrences, and says which one holds a production change at an instant.
// Reports implements governance.ReportRenderer: it lays a signed
// compliance report out as a PDF 1.7 document, written by this package with
// the standard Helvetica font and no dependency.
//
// Both refuse unless the licence lists their feature: the calendar's
// Validate needs FeatureChangeFreezes for writing and InForce for reading,
// and the report writer needs FeatureComplianceReports for reading
// (GOV-030). The core checks the same licence before it calls either; the
// check here is the second one, so that a mistaken caller cannot use the
// feature without it.
//
// # Recurrence
//
// A window recurs by a subset of RFC 5545's RRULE: FREQ=DAILY, or
// FREQ=WEEKLY with BYDAY (MO, TU, WE, TH, FR, SA, SU, in any order; the weekday
// of the first start when it is left out), each with an optional
// UNTIL=YYYYMMDD, the last local date a window may start on. Anything else
// (INTERVAL, COUNT, BYMONTHDAY, and so on) is refused when the window is
// written rather than approximated: a freeze that silently differs from what
// its author wrote is worse than none.
//
// # Daylight-saving time
//
// A start is a wall-clock time in the window's zone, so a "Friday 18:00"
// window starts at 18:00 on the wall before and after the clocks change. A
// wall-clock start that does not exist on a date, because the clocks jumped
// over it, begins at the moment the gap ends; one that exists twice, because
// the clocks fell back, takes its first occurrence. An occurrence lasts its
// Duration of elapsed time, so across a change of the clocks its end shifts
// by the hour on the wall. The zone database is the host's (time.LoadLocation),
// as everywhere in Plux.
//
// Invariants:
//   - A window that cannot be evaluated is an error, never "not in force".
//   - Evaluation depends only on its arguments, never on the clock or the
//     environment.
//   - The PDF is a function of the report and its envelope: the same input
//     gives the same bytes.
//
// Specification: GOV-025, GOV-026, GOV-030; ADR-0076.
package governance
