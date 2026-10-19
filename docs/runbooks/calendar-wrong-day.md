# Wrong Calendar Day Runbook

A date shows the wrong day in a non-Gregorian calendar (`I18N-003`, ADR-0067).

## How calendars are computed

- Conversion between calendars goes through the Julian day number. Go and Dart share 2,000
  vectors per calendar (`schema/testdata/l10n/calendars/`), so Studio, the compiler and the
  device agree.
- **Ethiopian**: Amete Mihret era (Amete Alem before year 1); 13 months, the 13th of 5 or 6 days.
- **Islamic Umm al-Qura**: month lengths for **1300 to 1600 AH** come from a table (pinned ICU
  release). Outside that range tabular Islamic arithmetic is used and the compiler warns with
  `PLX-1615`.
- The table is Saudi Arabia's civil calendar. Local moon sighting can differ by a day; this is
  not a defect.
- **Persian** uses the arithmetic of the same ICU release. **Gregorian** is proleptic.
- The calendar is chosen per app (`localisation.calendar`, `localisation.secondaryCalendar`)
  and per date field or picker (`calendar` prop).

## Diagnose

1. **Is `PLX-1615` in the publish diagnostics?** The date is outside 1300–1600 AH. Use a date
   within the range, or accept the tabular approximation.
2. **Within the range and off by one day?** Compare with the source the user expects. A
   sighting-based date can differ from the Umm al-Qura civil table; say so in the app, or show
   the Gregorian date as the secondary calendar beside it.
3. **Wrong month names or era?** Names come from the locale data (ADR-0066); check the locale
   and that the app declares the calendar.
4. **Wrong Ethiopian date?** Check the field's `calendar` prop and the Gregorian input. A result
   that disagrees with the shared vectors is a defect: report it with the date and the calendar.
