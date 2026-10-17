// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The `calendar` group of the PXL standard library: `toCalendar` and
/// `fromCalendar` (PXL-006, I18N-003), identical to Go's
/// `backend/internal/pxl/builtins_calendar.go`.
library;

import 'package:plux_flutter/src/l10n/calendar/calendar.dart';
import 'package:plux_flutter/src/pxl/builtins.dart';
import 'package:plux_flutter/src/pxl/tables.g.dart';
import 'package:plux_flutter/src/pxl/values.dart';
import 'package:plux_flutter/src/pxl/vm.dart';

/// The Julian day number of 1970-01-01, day 0 of a PXL date.
const int _unixEpochJdn = 2440588;

PxlError _invalid(String message) =>
    PxlError(PxlErrorKind.invalidArgument, message);

CalendarSystem _system(Object? id) =>
    calendarSystem(id! as String) ??
    (throw _invalid('"$id" is not a calendar system'));

/// The implementations of the calendar group, keyed by overload.
Map<String, Builtin> calendarFns() => {
  'toCalendar(date,string)': (_, a) {
    final sys = _system(a[1]);
    final d = sys.fromJdn((a[0]! as PxlDate).days + _unixEpochJdn);
    final e = sys.eraOf(d.year);
    return <String, Object?>{
      'era': e.era,
      'year': e.year,
      'month': d.month,
      'day': d.day,
    };
  },
  'fromCalendar(map<string,int>,string)': (_, a) {
    final sys = _system(a[1]);
    final parts = a[0]! as Map<String, Object?>;
    final year = parts['year'];
    final month = parts['month'];
    final day = parts['day'];
    if (year is! int || month is! int || day is! int) {
      throw _invalid('parts need year, month and day');
    }
    const limit = 1 << 31;
    if (year < -limit ||
        year > limit ||
        month < 0 ||
        month > limit ||
        day < 0 ||
        day > limit) {
      throw _invalid('the date is out of range');
    }
    var y = year;
    final era = parts['era'];
    if (era is int) {
      y =
          sys.yearOf(era, y) ??
          (throw _invalid('$era is not an era of ${sys.id}'));
    }
    final int jdn;
    try {
      jdn = sys.toJdn(CalendarYmd(y, month, day));
    } on InvalidCalendarDate {
      throw _invalid(
        '$year-$month-$day is not a date in the ${sys.id} calendar',
      );
    }
    final days = jdn - _unixEpochJdn;
    if (!validDate(days)) throw _invalid('the date leaves years 1–9999');
    return PxlDate(days);
  },
};
