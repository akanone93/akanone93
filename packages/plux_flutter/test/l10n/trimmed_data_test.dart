// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/l10n/l10n.dart';

/// Locale data as the compiler writes it for an app that uses no currency,
/// no territory and no date skeleton (I18N-002, ADR-0066 Revision
/// 2026-10-18): those parts are empty.
PluxLocaleData _trimmed() {
  final json = jsonDecode(
    File('../../schema/l10n/cldr/en.json').readAsStringSync(),
  ) as Map<String, Object?>;
  json['currencies'] = <String, Object?>{
    'CHF': {'name': 'Swiss Franc', 'symbol': 'CHF'},
  };
  json['territories'] = <String, Object?>{};
  final gregorian =
      (json['calendars']! as Map<String, Object?>)['gregorian']!
          as Map<String, Object?>;
  gregorian['availableFormats'] = <String, Object?>{'yMd': 'M/d/y'};
  return PluxLocaleData.fromJson(json);
}

void main() {
  // Verifies: I18N-002.
  test('names the data lacks fall back to their codes', () {
    final data = _trimmed();
    expect(data.currencyName('CHF'), 'Swiss Franc');
    expect(data.currencyName('EUR'), 'EUR');
    expect(data.territoryName('FR'), isNull);
    expect(data.territoryCodes, isEmpty);
    expect(data.formatMoney(PluxDecimal.parse('1.5'), 'EUR'), contains('EUR'));
  });

  // Verifies: I18N-002.
  test('styles and kept skeletons still format', () {
    final data = _trimmed();
    final d = PluxCalendarDate.gregorian(2026, 10, 18, 14, 5);
    expect(
      data.formatDate(d, 'gregorian', const PluxDateOptions(skeleton: 'yMd')),
      '10/18/2026',
    );
    expect(
      data.formatDate(d, 'gregorian', const PluxDateOptions(dateStyle: 'long')),
      'October 18, 2026',
    );
  });
}
