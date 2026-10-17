// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/l10n/calendar/calendar.dart';
import 'package:plux_flutter/src/l10n/locale_data.dart';

const _vectors = '../../schema/testdata/l10n/calendars.json';

CalendarSystem _sys(String id) => calendarSystem(id)!;

void main() {
  // I18N-003: every ICU-generated case converts from the day number to the
  // date and back, in each non-Gregorian system.
  test('I18N-003 vectors match ICU', () {
    final file =
        jsonDecode(File(_vectors).readAsStringSync()) as Map<String, Object?>;
    for (final id in [ethiopicId, islamicUmalquraId, persianId]) {
      final cases = (file[id]! as List<Object?>).cast<List<Object?>>();
      expect(cases.length, greaterThanOrEqualTo(2000), reason: id);
      final sys = _sys(id);
      for (final c in cases) {
        final [jdn as int, era as int, year as int, month as int, day as int] =
            c;
        final d = sys.fromJdn(jdn);
        final e = sys.eraOf(d.year);
        expect(
          [e.era, e.year, d.month, d.day],
          [era, year, month, day],
          reason: '$id jdn $jdn',
        );
        expect(sys.toJdn(d), jdn, reason: '$id $d');
      }
    }
  });

  // I18N-003: dates and day numbers are inverse and consecutive days advance
  // by one, over a range that includes the Umm al-Qura table edges.
  test('I18N-003 round trip over 700,000 days', () {
    for (final id in calendarIds) {
      final sys = _sys(id);
      var prev = sys.fromJdn(2000000);
      for (var jdn = 2000001; jdn < 2700000; jdn++) {
        final d = sys.fromJdn(jdn);
        expect(sys.toJdn(d), jdn, reason: '$id $d');
        final next =
            d.day == prev.day + 1 &&
            d.month == prev.month &&
            d.year == prev.year;
        final rolled =
            d.day == 1 &&
            ((d.month == prev.month + 1 && d.year == prev.year) ||
                (d.month == 1 && d.year == prev.year + 1));
        if (!next && !rolled) fail('$id: day $jdn is $d after $prev');
        if (rolled && prev.day != sys.daysInMonth(prev.year, prev.month)) {
          fail('$id: $prev is not the last day of its month');
        }
        prev = d;
      }
    }
  });

  test('I18N-003 gregorian is proleptic', () {
    final g = _sys(gregorianId);
    for (final (CalendarYmd d, int jdn) in [
      (const CalendarYmd(2000, 1, 1), 2451545),
      (const CalendarYmd(1970, 1, 1), 2440588),
      (const CalendarYmd(1582, 10, 15), 2299161),
      (const CalendarYmd(-4713, 11, 24), 0),
      (const CalendarYmd(0, 12, 31), 1721425),
    ]) {
      expect(g.toJdn(d), jdn);
      expect(g.fromJdn(jdn), d);
    }
    expect(g.isLeapYear(1900), isFalse);
    expect(g.isLeapYear(2000), isTrue);
    expect(g.isLeapYear(0), isTrue);
    expect(g.eraOf(0), (era: 0, year: 1));
  });

  test('I18N-003 ethiopic eras and Pagume', () {
    final e = _sys(ethiopicId);
    expect(e.eraOf(0), (era: 0, year: 5500));
    expect(e.eraOf(1), (era: 1, year: 1));
    expect(e.yearOf(0, 5500), 0);
    expect(e.daysInMonth(2015, 13), 6);
    expect(e.daysInMonth(2016, 13), 5);
    expect(e.fromJdn(1724221), const CalendarYmd(1, 1, 1));
  });

  test('I18N-003 umm al-qura table edges are flagged', () {
    final u = _sys(islamicUmalquraId);
    expect(u.outOfTable(1299), isTrue);
    expect(u.outOfTable(1300), isFalse);
    expect(u.outOfTable(1600), isFalse);
    expect(u.outOfTable(1601), isTrue);
    expect(u.toJdn(const CalendarYmd(1, 1, 1)), 1948440);
    expect(u.eraOf(0), (era: 1, year: 1));
    expect(
      () => u.toJdn(const CalendarYmd(1445, 1, 31)),
      throwsA(isA<InvalidCalendarDate>()),
    );
  });

  test('I18N-003 persian leap years follow ICU corrections', () {
    final p = _sys(persianId);
    expect(
      [
        for (final y in [1399, 1403, 1404, 1502, 1503]) p.isLeapYear(y),
      ],
      [true, true, false, false, true],
    );
    expect(p.daysInMonth(1403, 12), 30);
    expect(p.daysInMonth(1404, 12), 29);
  });

  test('I18N-003 unknown system', () {
    expect(calendarSystem('hebrew'), isNull);
  });

  // I18N-003: FormatDate formats a converted date with the locale's names for
  // that calendar.
  test('I18N-003 formatDate in each calendar', () {
    final json = jsonDecode(
      File('../../schema/l10n/cldr/en.json').readAsStringSync(),
    );
    final en = PluxLocaleData.fromJson(json as Map<String, Object?>);
    final jdn = _sys(gregorianId).toJdn(const CalendarYmd(2024, 3, 11));
    for (final (id, want) in [
      (gregorianId, 'Monday, March 11, 2024'),
      (islamicUmalquraId, 'Monday, Ramadan 1, 1445 AH'),
      (persianId, 'Monday, Esfand 21, 1402 AP'),
      (ethiopicId, 'Monday, Megabit 2, 2016 AM'),
    ]) {
      expect(
        en.formatDate(
          pluxCalendarDate(_sys(id), jdn),
          id,
          const PluxDateOptions(dateStyle: 'full'),
        ),
        want,
        reason: id,
      );
    }
  });
}
