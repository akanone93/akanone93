# 0067. Calendar systems behind one interface

- **Status:** Proposed
- **Date:** 2026-10-16
- **Requirements:** `I18N-003`, `PXL-006`

## Context and problem

`I18N-003` requires non-Gregorian calendars with conversion and a per-app and per-field choice.
Ethiopia uses the Ethiopian calendar; Islamic banking shows Hijri dates beside Gregorian ones and
reckons Zakat by the Hijri year; Iran and Afghanistan use the Persian calendar (maintainer, P8
plan D5). PXL declares `toCalendar` and `fromCalendar`; `DualCalendarDatePicker` is in
Appendix C.

## Decision drivers

- Identical results in Go and Dart; deterministic; no platform calendar APIs.
- One interface, so a further calendar is one implementation plus vectors.
- An honest answer for Hijri dates, whose observed start differs by country.

## Considered options

1. Platform calendars (ICU on Android, `NSCalendar` on iOS).
2. **Our own arithmetic implementations behind one interface, with a pinned Umm al-Qura table.**

## Decision

Chosen option: **2**.

- **Interface** (Go `backend/internal/l10n/calendar`, Dart `lib/src/l10n/calendar/`): `toJdn`
  and `fromJdn` (Julian day number), `monthsInYear`, `daysInMonth`, `isLeapYear`, `eraOf`.
  Conversion between calendars goes through the Julian day number.
- **Gregorian:** proleptic.
- **Ethiopian:** Amete Mihret era, Amete Alem before year 1; 13 months, the 13th of 5 or 6 days.
- **Islamic Umm al-Qura:** month lengths for 1300–1600 AH from the table in a pinned ICU release
  (`islamcal.cpp`, Unicode-3.0), committed as data; outside that range tabular Islamic
  arithmetic, flagged at compile time with `PLX-1615`. The table is Saudi Arabia's civil
  calendar; local moon sighting can differ by a day, which the guide states.
- **Persian:** the arithmetic of the same pinned ICU release.
- **Choice:** `localisation.calendar` and `localisation.secondaryCalendar` per app, a `calendar`
  prop per date field and picker; `format.date` and the pickers use it; names come from the
  locale data (ADR-0066).
- **Vectors:** 2,000 dates per calendar across the table range, leap years and era edges, generated
  once from the pinned ICU and committed under `schema/testdata/l10n/calendars/`.

## Consequences

- **Positive:** the same dates on every platform and in the compiler; no dependency.
- **Negative:** Umm al-Qura beyond 1600 AH is tabular, and observational calendars are out of
  scope.
- **Follow-up:** L3; a per-app ±1-day Hijri adjustment can be added later without changing the
  interface.
