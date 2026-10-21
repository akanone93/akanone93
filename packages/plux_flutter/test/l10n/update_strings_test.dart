// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/core/update_screen.dart';
import 'package:plux_flutter/src/l10n/runtime_strings.dart';

/// The text of the mandatory-update screen in every locale the runtime
/// ships (REL-070, I18N-004).
void main() {
  RuntimeStrings table(String locale) {
    final raw = jsonDecode(
      File('../../schema/l10n/plux/$locale.json').readAsStringSync(),
    ) as Map<String, Object?>;
    return RuntimeStrings({for (final e in raw.entries) e.key: '${e.value}'});
  }

  const key = 'plux.update.required';
  final english = englishRuntimeStrings[key]!;

  test('every shipped locale translates the update title, and the English '
      'file says what the built-in text does [REL-070] [I18N-004]', () {
    expect(table('en')[key], english);
    for (final (locale, text) in [
      ('de', 'Zum Fortfahren ist ein Update erforderlich'),
      ('ar', 'التحديث مطلوب للمتابعة'),
      ('am', 'ለመቀጠል ዝማኔ ያስፈልጋል'),
    ]) {
      expect(table(locale).table[key], text, reason: locale);
      expect(table(locale)[key], isNot(english), reason: locale);
    }
  });

  for (final (locale, direction) in [
    ('de', TextDirection.ltr),
    ('ar', TextDirection.rtl),
    ('am', TextDirection.ltr),
  ]) {
    testWidgets('the update screen shows the $locale title [REL-070] '
        '[I18N-004]', (tester) async {
      final strings = table(locale);
      await tester.pumpWidget(
        MaterialApp(
          home: Directionality(
            textDirection: direction,
            child: PluxRuntimeStringsScope(
              strings: strings,
              child: const PluxDefaultUpdateScreen(messageKey: 'update.now'),
            ),
          ),
        ),
      );
      expect(find.text(strings.table[key]!), findsOneWidget);
      expect(find.text(english), findsNothing);
    });
  }
}
