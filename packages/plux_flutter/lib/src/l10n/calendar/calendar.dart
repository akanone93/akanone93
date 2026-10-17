// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Calendar systems behind one interface, converting through the Julian day
/// number (I18N-003, ADR-0067). Plain integer arithmetic, identical to Go's
/// `backend/internal/l10n/calendar`, so every platform and the compiler agree
/// to the day.
///
/// Years are extended years: Gregorian year 0 is 1 BC, and Ethiopic year 0 is
/// the last Amete Alem year. [CalendarSystem.eraOf] maps an extended year to
/// the era index and year within the era that the date formatter writes.
library;

import 'package:plux_flutter/src/l10n/calendar/tables.g.dart';
import 'package:plux_flutter/src/l10n/locale_data.dart';

/// The Gregorian system id.
const String gregorianId = 'gregorian';

/// The Ethiopic system id.
const String ethiopicId = 'ethiopic';

/// The Islamic Umm al-Qura system id.
const String islamicUmalquraId = 'islamic-umalqura';

/// The Persian system id.
const String persianId = 'persian';

/// A month or day outside its calendar's range.
final class InvalidCalendarDate implements Exception {
  /// Creates the exception.
  const InvalidCalendarDate(this.message);

  /// What was invalid.
  final String message;

  @override
  String toString() => 'InvalidCalendarDate: $message';
}

/// A date in some calendar: [year] is the extended year, [month] and [day]
/// start at 1.
final class CalendarYmd {
  /// Creates a date.
  const CalendarYmd(this.year, this.month, this.day);

  /// The extended year.
  final int year;

  /// The month, from 1.
  final int month;

  /// The day of the month, from 1.
  final int day;

  @override
  bool operator ==(Object other) =>
      other is CalendarYmd &&
      other.year == year &&
      other.month == month &&
      other.day == day;

  @override
  int get hashCode => Object.hash(year, month, day);

  @override
  String toString() => '$year-$month-$day';
}

/// The era index and the year within the era of an extended year.
typedef EraYear = ({int era, int year});

/// One calendar system.
abstract interface class CalendarSystem {
  /// The system id.
  String get id;

  /// The Julian day number of [d]; throws [InvalidCalendarDate].
  int toJdn(CalendarYmd d);

  /// The date of a Julian day number.
  CalendarYmd fromJdn(int jdn);

  /// The number of months of [year].
  int monthsInYear(int year);

  /// The length of a month, or 0 if the month does not exist.
  int daysInMonth(int year, int month);

  /// Whether the year has the extra day.
  bool isLeapYear(int year);

  /// The era index (as the date formatter indexes the calendar's era names)
  /// and the year within that era.
  EraYear eraOf(int year);

  /// The extended year of a year within an era, or null when the era does not
  /// exist; the inverse of [eraOf].
  int? yearOf(int era, int eraYear);

  /// Whether [year] lies outside the pinned table the system relies on, so its
  /// dates are an approximation (PLX-1615). Systems with no table return
  /// false.
  bool outOfTable(int year);
}

/// The system ids, in the order of the `calendar` enum.
const List<String> calendarIds = [
  gregorianId,
  ethiopicId,
  islamicUmalquraId,
  persianId,
];

const Map<String, CalendarSystem> _systems = {
  gregorianId: _Gregorian(),
  ethiopicId: _Ethiopic(),
  islamicUmalquraId: _Umalqura(),
  persianId: _Persian(),
};

/// The system of an id, or null when there is none.
CalendarSystem? calendarSystem(String id) => _systems[id];

/// The date in [to] of a date in [from]; throws [InvalidCalendarDate].
CalendarYmd convertCalendar(
  CalendarSystem from,
  CalendarYmd d,
  CalendarSystem to,
) => to.fromJdn(from.toJdn(d));

/// The weekday of a Julian day number, 0 for Sunday.
int calendarWeekday(int jdn) => _mod(jdn + 1, 7);

/// The [PluxCalendarDate] that `PluxLocaleData.formatDate` formats in the
/// system's calendar, for a Julian day number and time of day.
PluxCalendarDate pluxCalendarDate(
  CalendarSystem sys,
  int jdn, {
  int hour = 0,
  int minute = 0,
  int second = 0,
}) {
  final d = sys.fromJdn(jdn);
  final e = sys.eraOf(d.year);
  return PluxCalendarDate(
    era: e.era,
    year: e.year,
    month: d.month,
    day: d.day,
    weekday: calendarWeekday(jdn),
    hour: hour,
    minute: minute,
    second: second,
  );
}

int _div(int a, int b) {
  final q = a ~/ b;
  return (a.remainder(b) != 0 && (a < 0) != (b < 0)) ? q - 1 : q;
}

int _mod(int a, int b) => a - _div(a, b) * b;

void _check(CalendarSystem s, CalendarYmd d) {
  if (d.month < 1 ||
      d.month > s.monthsInYear(d.year) ||
      d.day < 1 ||
      d.day > s.daysInMonth(d.year, d.month)) {
    throw InvalidCalendarDate('${s.id} $d');
  }
}

final class _Gregorian implements CalendarSystem {
  const _Gregorian();

  @override
  String get id => gregorianId;

  @override
  int monthsInYear(int year) => 12;

  @override
  bool outOfTable(int year) => false;

  @override
  bool isLeapYear(int year) =>
      year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);

  @override
  int daysInMonth(int year, int month) => switch (month) {
    2 => isLeapYear(year) ? 29 : 28,
    4 || 6 || 9 || 11 => 30,
    1 || 3 || 5 || 7 || 8 || 10 || 12 => 31,
    _ => 0,
  };

  @override
  EraYear eraOf(int year) =>
      year < 1 ? (era: 0, year: 1 - year) : (era: 1, year: year);

  @override
  int? yearOf(int era, int eraYear) => switch (era) {
    0 => 1 - eraYear,
    1 => eraYear,
    _ => null,
  };

  @override
  int toJdn(CalendarYmd d) {
    _check(this, d);
    final a = (14 - d.month) ~/ 12;
    final y = d.year + 4800 - a;
    final m = d.month + 12 * a - 3;
    return d.day +
        (153 * m + 2) ~/ 5 +
        365 * y +
        _div(y, 4) -
        _div(y, 100) +
        _div(y, 400) -
        32045;
  }

  @override
  CalendarYmd fromJdn(int jdn) {
    final a = jdn + 32044;
    final b = _div(4 * a + 3, 146097);
    final c = a - _div(146097 * b, 4);
    final d = _div(4 * c + 3, 1461);
    final e = c - _div(1461 * d, 4);
    final m = (5 * e + 2) ~/ 153;
    return CalendarYmd(
      100 * b + d - 4800 + m ~/ 10,
      m + 3 - 12 * (m ~/ 10),
      e - (153 * m + 2) ~/ 5 + 1,
    );
  }
}

const int _ethiopicEpochOffset = 1723856;

final class _Ethiopic implements CalendarSystem {
  const _Ethiopic();

  @override
  String get id => ethiopicId;

  @override
  int monthsInYear(int year) => 13;

  @override
  bool outOfTable(int year) => false;

  @override
  bool isLeapYear(int year) => _mod(year, 4) == 3;

  @override
  int daysInMonth(int year, int month) {
    if (month >= 1 && month <= 12) return 30;
    if (month == 13) return isLeapYear(year) ? 6 : 5;
    return 0;
  }

  @override
  EraYear eraOf(int year) =>
      year <= 0 ? (era: 0, year: year + 5500) : (era: 1, year: year);

  @override
  int? yearOf(int era, int eraYear) => switch (era) {
    0 => eraYear - 5500,
    1 => eraYear,
    _ => null,
  };

  @override
  int toJdn(CalendarYmd d) {
    _check(this, d);
    return _ethiopicEpochOffset +
        365 * d.year +
        _div(d.year, 4) +
        30 * (d.month - 1) +
        d.day -
        1;
  }

  @override
  CalendarYmd fromJdn(int jdn) {
    final days = jdn - _ethiopicEpochOffset;
    final c4 = _div(days, 1461);
    final r = _mod(days, 1461);
    final year = 4 * c4 + r ~/ 365 - r ~/ 1460;
    final doy = r == 1460 ? 365 : r % 365;
    return CalendarYmd(year, doy ~/ 30 + 1, doy % 30 + 1);
  }
}

/// The first day of an Islamic civil year, in days after the civil epoch.
int _civilYearStart(int year) => 354 * (year - 1) + _div(3 + 11 * year, 30);

bool _civilLeap(int year) => _mod(14 + 11 * year, 30) < 11;

int _civilDaysInMonth(int year, int month) =>
    29 + month % 2 + (month == 12 && _civilLeap(year) ? 1 : 0);

CalendarYmd _civilFromDays(int days) {
  final year = _div(30 * days + 10646, 10631);
  final ys = _civilYearStart(year);
  final month = (-_div(-2 * (days - 29 - ys), 59) + 1).clamp(1, 12);
  return CalendarYmd(year, month, days - ys - (59 * (month - 1) + 1) ~/ 2 + 1);
}

final class _Umalqura implements CalendarSystem {
  const _Umalqura();

  @override
  String get id => islamicUmalquraId;

  @override
  int monthsInYear(int year) => 12;

  @override
  bool outOfTable(int year) =>
      year < umalquraYearStart || year > umalquraYearEnd;

  @override
  bool isLeapYear(int year) {
    if (outOfTable(year)) return _civilLeap(year);
    final i = year - umalquraYearStart;
    return umalquraYearStarts[i + 1] - umalquraYearStarts[i] == 355;
  }

  @override
  int daysInMonth(int year, int month) {
    if (month < 1 || month > 12) return 0;
    if (outOfTable(year)) return _civilDaysInMonth(year, month);
    return 29 +
        ((umalquraMonthMasks[year - umalquraYearStart] >> (12 - month)) & 1);
  }

  @override
  EraYear eraOf(int year) =>
      year < 1 ? (era: 1, year: 1 - year) : (era: 0, year: year);

  @override
  int? yearOf(int era, int eraYear) => switch (era) {
    0 => eraYear,
    1 => 1 - eraYear,
    _ => null,
  };

  @override
  int toJdn(CalendarYmd d) {
    _check(this, d);
    if (outOfTable(d.year)) {
      return civilEpochJdn +
          _civilYearStart(d.year) +
          (59 * (d.month - 1) + 1) ~/ 2 +
          d.day -
          1;
    }
    var days = umalquraYearStarts[d.year - umalquraYearStart];
    for (var m = 1; m < d.month; m++) {
      days += daysInMonth(d.year, m);
    }
    return civilEpochJdn + days + d.day - 1;
  }

  @override
  CalendarYmd fromJdn(int jdn) {
    final days = jdn - civilEpochJdn;
    if (days < umalquraYearStarts.first || days >= umalquraYearStarts.last) {
      return _civilFromDays(days);
    }
    var lo = 0;
    var hi = umalquraYearStarts.length - 1;
    while (hi - lo > 1) {
      final mid = (lo + hi) ~/ 2;
      if (umalquraYearStarts[mid] > days) {
        hi = mid;
      } else {
        lo = mid;
      }
    }
    final year = umalquraYearStart + lo;
    var rest = days - umalquraYearStarts[lo];
    var month = 1;
    while (rest >= daysInMonth(year, month)) {
      rest -= daysInMonth(year, month);
      month++;
    }
    return CalendarYmd(year, month, rest + 1);
  }
}

const List<int> _persianDaysBefore = [
  0, 31, 62, 93, 124, 155, 186, 216, 246, 276, 306, 336, //
];

bool _persianNonLeap(int year) {
  var lo = 0;
  var hi = persianNonLeapYears.length;
  while (lo < hi) {
    final mid = (lo + hi) ~/ 2;
    if (persianNonLeapYears[mid] < year) {
      lo = mid + 1;
    } else {
      hi = mid;
    }
  }
  return lo < persianNonLeapYears.length && persianNonLeapYears[lo] == year;
}

int _persianFirstDay(int year) {
  final n = 365 * (year - 1) + _div(8 * year + 21, 33);
  return year > persianNonLeapYears.first && _persianNonLeap(year - 1)
      ? n - 1
      : n;
}

final class _Persian implements CalendarSystem {
  const _Persian();

  @override
  String get id => persianId;

  @override
  int monthsInYear(int year) => 12;

  @override
  bool outOfTable(int year) => false;

  @override
  bool isLeapYear(int year) {
    if (year >= persianNonLeapYears.first && _persianNonLeap(year)) {
      return false;
    }
    if (year > persianNonLeapYears.first && _persianNonLeap(year - 1)) {
      return true;
    }
    return _mod(year * 25 + 11, 33) < 8;
  }

  @override
  int daysInMonth(int year, int month) {
    if (month >= 1 && month <= 6) return 31;
    if (month >= 7 && month <= 11) return 30;
    if (month == 12) return isLeapYear(year) ? 30 : 29;
    return 0;
  }

  @override
  EraYear eraOf(int year) => (era: 0, year: year);

  @override
  int? yearOf(int era, int eraYear) => era == 0 ? eraYear : null;

  @override
  int toJdn(CalendarYmd d) {
    _check(this, d);
    return persianEpochJdn +
        _persianFirstDay(d.year) +
        _persianDaysBefore[d.month - 1] +
        d.day -
        1;
  }

  @override
  CalendarYmd fromJdn(int jdn) {
    final days = jdn - persianEpochJdn;
    var year = _div(33 * days + 3, 12053) + 1;
    var doy = days - _persianFirstDay(year);
    if (doy == 365 &&
        year >= persianNonLeapYears.first &&
        _persianNonLeap(year)) {
      year++;
      doy = 0;
    }
    final month = doy < 216 ? doy ~/ 31 + 1 : (doy - 6) ~/ 30 + 1;
    return CalendarYmd(year, month, doy + 1 - _persianDaysBefore[month - 1]);
  }
}
