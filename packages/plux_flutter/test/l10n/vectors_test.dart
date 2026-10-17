// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/l10n/l10n.dart';

const _cldr = '../../schema/l10n/cldr';
const _vectors = '../../schema/testdata/l10n';

late final PluxLocaleIndex _index;
late final List<String> _locales;
final _loaded = <String, PluxLocaleData>{};

/// The data of the best locale for [tag], loaded from schema/l10n/cldr.
PluxLocaleData load(String tag) {
  final id = _index.resolve(tag, _locales, '');
  return _loaded.putIfAbsent(id, () {
    final json = jsonDecode(File('$_cldr/$id.json').readAsStringSync());
    return PluxLocaleData.fromJson(json as Map<String, Object?>);
  });
}

List<Map<String, Object?>> _cases(String name) {
  final json = jsonDecode(
    File('$_vectors/$name').readAsStringSync(),
  ) as Map<String, Object?>;
  return (json['cases']! as List<Object?>).cast<Map<String, Object?>>();
}

String _want(Map<String, Object?> c) => (c['ours'] ?? c['expect'])! as String;

/// Runs [run] on every case of [file] and reports the first mismatches.
void _runVectors(
  String file,
  String Function(PluxLocaleData l, Map<String, Object?> c) run,
) {
  final failures = <String>[];
  var n = 0;
  for (final c in _cases(file)) {
    n++;
    String got;
    try {
      got = run(load(c['locale']! as String), c);
    } on L10nException catch (e) {
      got = 'error: $e';
    }
    if (got != _want(c)) {
      failures.add('$file case $n $c: got "$got", want "${_want(c)}"');
    }
  }
  expect(n, greaterThan(0));
  expect(
    failures,
    isEmpty,
    reason: '${failures.length} cases failed:\n${failures.take(25).join('\n')}',
  );
}

NumberOptions _numberOptions(Map<String, Object?> c) {
  final o = (c['options'] as Map<String, Object?>?) ?? const {};
  final min = o['minFraction'] as int?;
  return NumberOptions(
    fraction: min == null
        ? null
        : FractionDigits(min, o['maxFraction']! as int),
    numberingSystem: (o['numbering'] as String?) ?? '',
  );
}

void main() {
  setUpAll(() {
    final idx = jsonDecode(
      File('$_cldr/index.json').readAsStringSync(),
    ) as Map<String, Object?>;
    _index = PluxLocaleIndex.fromJson(idx);
    _locales = (idx['locales']! as List<Object?>).cast<String>();
  });

  test('plural vectors (I18N-001, I18N-002, QA-003)', () {
    _runVectors('plurals.json', (l, c) {
      final n = PluxDecimal.parse(c['n']! as String);
      return (c['type'] == 'ordinal' ? l.pluralOrdinal(n) : l.pluralCardinal(n))
          .name;
    });
  });

  test('number vectors (I18N-001, QA-003)', () {
    _runVectors('numbers.json', (l, c) {
      final x = PluxDecimal.parse(c['value']! as String);
      final opts = _numberOptions(c);
      return switch (c['op']) {
        'decimal' => l.formatNumber(x, opts),
        'percent' => l.formatPercent(x, opts),
        _ => l.formatCompact(x, opts),
      };
    });
  });

  test('money vectors (I18N-011, QA-003)', () {
    _runVectors('money.json', (l, c) {
      final o = (c['options'] as Map<String, Object?>?) ?? const {};
      final min = o['minFraction'] as int?;
      return l.formatMoney(
        PluxDecimal.parse(c['amount']! as String),
        c['currency']! as String,
        MoneyOptions(
          display: switch (c['display']) {
            'narrow' => CurrencyDisplay.narrow,
            'code' => CurrencyDisplay.code,
            _ => CurrencyDisplay.symbol,
          },
          accounting: c['accounting'] == true,
          fraction: min == null
              ? null
              : FractionDigits(min, o['maxFraction']! as int),
          numberingSystem: (o['numbering'] as String?) ?? '',
        ),
      );
    });
  });

  test('date vectors (I18N-001, QA-003)', () {
    _runVectors('dates.json', (l, c) {
      final f = (c['date']! as List<Object?>).cast<int>();
      final got = l.formatDate(
        PluxCalendarDate(
          era: f[0],
          year: f[1],
          month: f[2],
          day: f[3],
          weekday: f[4],
          hour: f[5],
          minute: f[6],
          second: f[7],
        ),
        (c['calendar'] as String?) ?? 'gregorian',
        PluxDateOptions(
          dateStyle: (c['dateStyle'] as String?) ?? '',
          timeStyle: (c['timeStyle'] as String?) ?? '',
          skeleton: (c['skeleton'] as String?) ?? '',
          numberingSystem: (c['numbering'] as String?) ?? '',
        ),
      );
      // JavaScriptCore writes U+0020 where CLDR has U+202F (dates.json
      // "normalize"); the engine keeps CLDR's character.
      return got.replaceAll(' ', ' ');
    });
  });
}
