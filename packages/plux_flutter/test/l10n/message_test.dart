// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/l10n/l10n.dart';
import 'package:plux_flutter/src/l10n/message.dart';

import '../support/cldr.dart';

/// Converts a vector argument: {"decimal": "1.0"} is a number whose fraction
/// digits count and {"date": "..."} a Gregorian date and time of day.
Object? _arg(Object? v) {
  if (v is Map<String, Object?>) {
    if (v['decimal'] case final String s) return PluxDecimal.parse(s);
    final d = DateTime.parse(v['date']! as String);
    return PluxCalendarDate.gregorian(
      d.year,
      d.month,
      d.day,
      d.hour,
      d.minute,
      d.second,
    );
  }
  return v;
}

void main() {
  final file = jsonDecode(
    File('../../schema/testdata/l10n/messages.json').readAsStringSync(),
  ) as Map<String, Object?>;
  final cases = (file['cases']! as List<Object?>).cast<Map<String, Object?>>();

  group('messages.json [I18N-001] [I18N-011] [QA-003]', () {
    for (final c in cases) {
      if (c['ast'] == null) continue; // a syntax error case, Go only
      test(c['name']! as String, () {
        final nodes = decodeMessage(base64.decode(c['ast']! as String));
        expect(nodes, isNotNull);
        final args = {
          for (final e
              in ((c['args'] ?? <String, Object?>{}) as Map<String, Object?>)
                  .entries)
            e.key: _arg(e.value),
        };
        final got = evaluateMessage(
          nodes!,
          loadCldr(c['locale']! as String),
          args,
        );
        expect(got, c['want']);
      });
    }
  });

  test('the vectors cover at least 60 evaluated cases', () {
    expect(
      cases.where((c) => c['ast'] != null).length,
      greaterThanOrEqualTo(60),
    );
  });

  group('a malformed AST decodes to null and never throws [I18N-001]', () {
    final good = base64.decode(
      cases.firstWhere((c) => c['name'] == 'nested-select-plural')['ast']!
          as String,
    );
    test('every truncation', () {
      for (var n = 0; n < good.length; n++) {
        expect(decodeMessage(good.sublist(0, n)), isNull, reason: 'cut at $n');
      }
    });
    test('trailing bytes, an unknown version and an unknown node', () {
      expect(decodeMessage([...good, 0]), isNull);
      expect(decodeMessage([2, 0]), isNull);
      expect(decodeMessage([1, 1, 99]), isNull);
      expect(decodeMessage(const []), isNull);
    });
    test('deep nesting is refused', () {
      // select x { a { select x { a { ... {text} } } } }, depth levels deep.
      List<int> nested(int depth) => depth == 0
          ? [1, 1, 1, 0x74] // one text node "t"
          : [1, 7, 1, 0x78, 1, 1, 0x61, ...nested(depth - 1)];
      expect(decodeMessage([1, ...nested(10)]), isNotNull);
      final deep = [1, ...nested(40)];
      expect(decodeMessage(deep), isNull);
    });
    test('every single-byte corruption yields null or a message', () {
      for (var i = 0; i < good.length; i++) {
        final bad = List<int>.of(good)..[i] ^= 0xff;
        decodeMessage(bad);
      }
    });
  });

  test('without locale data the English fallback formats [I18N-001]', () {
    final nodes = decodeMessage(
      base64.decode(
        cases.firstWhere((c) => c['name'] == 'plural-basic-en-1')['ast']!
            as String,
      ),
    )!;
    expect(evaluateMessage(nodes, null, {'n': 1}), '1 item');
    expect(evaluateMessage(nodes, null, {'n': 4}), '4 items');
    expect(evaluateMessage(nodes, null, {'n': 0}), 'none');
  });
}
