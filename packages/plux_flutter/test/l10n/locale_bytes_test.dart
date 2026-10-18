// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/l10n/l10n.dart';

const _cldr = '../../schema/l10n/cldr';

Uint8List _bytes(Object? json) =>
    Uint8List.fromList(utf8.encode(json is String ? json : jsonEncode(json)));

Matcher _invalid() => throwsA(
  isA<L10nException>().having((e) => e.code, 'code', L10nError.invalidData),
);

/// English with no names: what the sections a test adds come on top of.
Map<String, Object?> _minimal() =>
    (jsonDecode(File('$_cldr/en.json').readAsStringSync())
          as Map<String, Object?>)
      ..remove('currencies')
      ..remove('territories');

const _full = PluxDateOptions(dateStyle: 'full');

void main() {
  test('every locale file gives the same data from bytes as from decoded '
      'JSON [I18N-002] [I18N-011] [QA-007]', () {
    final index = jsonDecode(
      File('$_cldr/index.json').readAsStringSync(),
    ) as Map<String, Object?>;
    final locales = (index['locales']! as List<Object?>).cast<String>();
    expect(locales.length, greaterThan(100));
    final date = PluxCalendarDate.gregorian(2025, 10, 16, 15, 4, 5);
    for (final id in locales) {
      final file = File('$_cldr/$id.json');
      final json = jsonDecode(file.readAsStringSync()) as Map<String, Object?>;
      final want = PluxLocaleData.fromJson(json);
      final got = PluxLocaleData.fromBytes(file.readAsBytesSync());
      expect(got.id, want.id, reason: id);
      expect(got.isRtl, want.isRtl, reason: id);
      expect(got.defaultNumberingSystem, want.defaultNumberingSystem);
      expect(got.categories, want.categories, reason: id);
      expect(got.territoryCodes, want.territoryCodes, reason: id);
      expect(got.currencyCodes, want.currencyCodes, reason: id);
      for (final t in want.territoryCodes) {
        expect(got.territoryName(t), want.territoryName(t), reason: '$id $t');
      }
      for (final c in want.currencyCodes) {
        expect(got.currencyName(c), want.currencyName(c), reason: '$id $c');
        expect(got.currencyDigits(c), want.currencyDigits(c), reason: '$id $c');
      }
      final n = PluxDecimal.parse('-1234567.891');
      expect(got.formatNumber(n), want.formatNumber(n), reason: id);
      expect(got.formatMoney(n, 'EUR'), want.formatMoney(n, 'EUR'), reason: id);
      expect(
        got.formatDate(date, 'gregorian', _full),
        want.formatDate(date, 'gregorian', _full),
        reason: id,
      );
    }
  });

  test('the sections are found whatever the white space, and strings with '
      'braces, quotes and escapes do not end one early [I18N-002]', () {
    final json = _minimal()
      ..['currencies'] = {
        'XXX': {'name': 'A "}" {\\ name', 'symbol': ']'},
      }
      ..['territories'] = {'AA': '{[,]}', 'BB': 'ü  '};
    final compact = PluxLocaleData.fromBytes(_bytes(json));
    final spaced = PluxLocaleData.fromBytes(
      _bytes(const JsonEncoder.withIndent('\t').convert(json)),
    );
    for (final d in [compact, spaced]) {
      expect(d.id, 'en');
      expect(d.currencyName('XXX'), 'A "}" {\\ name');
      expect(d.territoryName('AA'), '{[,]}');
      expect(d.territoryName('BB'), 'ü  ');
      expect(d.territoryCodes, ['AA', 'BB']);
    }
  });

  test('a section the file lacks reads as empty, as with decoded JSON '
      '[I18N-002]', () {
    final d = PluxLocaleData.fromBytes(_bytes(_minimal()));
    expect(d.territoryCodes, isEmpty);
    expect(d.currencyCodes, isEmpty);
    expect(d.currencyName('EUR'), 'EUR');
    expect(d.currencyDigits('EUR'), 2);
  });

  test('malformed or incomplete bytes are invalid data [I18N-002]', () {
    for (final bad in [
      '',
      '[]',
      '{',
      '{"locale"',
      '{"locale":}',
      '{"locale":"x" "numbers":{}}',
      '{"locale":"x","numbers":{"latn":{}}',
      '{"locale":"x\\',
      '{}',
    ]) {
      expect(
        () => PluxLocaleData.fromBytes(_bytes(bad)),
        _invalid(),
        reason: bad,
      );
    }
    expect(
      () => PluxLocaleData.fromBytes(_bytes(_minimal()..remove('calendars'))),
      _invalid(),
    );
  });

  test('a malformed currency or territory section is found when it is '
      'first used [I18N-002]', () {
    final badCurrencies = PluxLocaleData.fromBytes(
      _bytes(_minimal()..['currencies'] = {'EUR': 'not an object'}),
    );
    expect(badCurrencies.territoryCodes, isEmpty);
    expect(() => badCurrencies.currencyName('EUR'), _invalid());
    final badTerritories = PluxLocaleData.fromJson(
      _minimal()..['territories'] = {'DE': 7},
    );
    expect(() => badTerritories.territoryName('DE'), _invalid());
  });
}
