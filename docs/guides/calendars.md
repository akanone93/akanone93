<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Calendars Guide

How a Plux app shows dates in the Ethiopian, Hijri or Persian calendar, alone or beside the
Gregorian one (`I18N-003`). The design is [ADR-0067](../adr/0067-calendar-systems.md).
Locales and formatting are in the [localisation guide](localisation.md).

## 1. The four calendars

| Value | Calendar | Notes |
|---|---|---|
| `gregorian` | Gregorian | Proleptic. The default. |
| `ethiopic` | Ethiopian | Amete Mihret era, Amete Alem before year 1; 13 months, the 13th of 5 or 6 days. |
| `islamicUmalqura` | Hijri, Umm al-Qura | Saudi Arabia's civil calendar, from a table. |
| `persian` | Persian (Solar Hijri) | Calculated. |

All four are our own arithmetic behind one interface and convert through the Julian day
number, in Go and in Dart, so the compiler, Studio previews and the device show the same
date. No platform calendar API is used. Conformance vectors (2,000 dates per calendar) are
in `schema/testdata/l10n/calendars/`.

## 2. Choose the calendar

For the whole app, in `app.json`:

```json
"localisation": {"calendar": "ethiopic", "secondaryCalendar": "gregorian"}
```

`calendar` is the calendar dates are shown in; `secondaryCalendar` shows a second one
beside it. An unsupported value is refused (`PLX-1611`). `format.date` and the date
pickers use these settings, and month and day names come from the locale's data.

## 3. DualCalendarDatePicker

`DualCalendarDatePicker` is a month grid in one calendar with the day numbers of a second
beside each day, such as Gregorian beside Hijri. It has `value`, `firstDate`, `lastDate`,
`calendar` and `secondaryCalendar`, and fires `onChanged` with the chosen `date`. A
`calendar` prop overrides the app's setting for this picker only
([widget reference](../reference/widgets.md#dualcalendardatepicker)):

```json
{"type": "DualCalendarDatePicker", "testId": "due",
 "props": {"value": {"$expr": "page.form.values.due"},
           "calendar": "islamicUmalqura", "secondaryCalendar": "gregorian"},
 "events": {"onChanged": {"steps": [{"id": "write", "action": "setState",
   "input": {"path": "page.form.values.due", "value": {"$expr": "event"}}}]}}}
```

The chosen value is always a `date`; the calendar changes only how it is shown and picked.

## 4. PXL

The `calendar` group (`pxl.calendar.v1`) converts in expressions:
`toCalendar(d, system)` gives a map with the year, month and day of a `date` in another
calendar, and `fromCalendar(parts, system)` gives the `date` of such a map. `system` is one
of the values above. For example, an Islamic bank reckons a Zakat year with
`toCalendar(today, "islamicUmalqura")`. See the
[PXL standard library](../reference/pxl-stdlib.md).

## 5. Umm al-Qura: the table and its range

Hijri month lengths are observed, not calculated. The Umm al-Qura calendar uses a table of
month lengths for 1300 to 1600 AH from a pinned ICU release, committed as data. Outside
that range the calendar falls back to tabular Islamic arithmetic, and the compiler flags a
date that falls there with the warning `PLX-1615`.

The table is Saudi Arabia's civil calendar. Local moon sighting can differ by a day, so a
date shown for a religious observance in another country may be a day off. A per-app
adjustment of one day is possible later without changing the interface; it is not
built.

## 6. Numerals

Digits follow the locale's `numberingSystem` setting, not the calendar: a Hijri date in
`ar` shows Western digits unless the app sets `"numberingSystem": {"ar": "native"}`.
Ge'ez numerals for `am-ET` are not supported.
