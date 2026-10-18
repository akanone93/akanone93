// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

const _app = '../../schema/testdata/documents/plux_express';
const _cldr = '../../schema/l10n/cldr';
const _locales = ['en', 'de-DE', 'ar', 'am-ET'];

Map<String, Object?> _json(String path) =>
    jsonDecode(File(path).readAsStringSync()) as Map<String, Object?>;

/// The reading direction CLDR gives the language of [tag], the data the
/// runtime reads its direction from.
TextDirection _direction(String tag) {
  final data = _json('$_cldr/${tag.split('-').first}.json');
  return data['direction'] == 'rtl' ? TextDirection.rtl : TextDirection.ltr;
}

/// A page title in the locale, laid out in the locale's direction.
class _Page extends StatelessWidget {
  const _Page(this.tag, this.messages, this.id);

  final String tag;
  final Map<String, Object?> messages;
  final String id;

  @override
  Widget build(BuildContext context) => Directionality(
    textDirection: _direction(tag),
    child: Builder(
      builder: (context) => Text(
        messages[id]! as String,
        textDirection: Directionality.of(context),
      ),
    ),
  );
}

/// The localisation of the reference app Plux Express (DX-004, I18N-001,
/// I18N-002, I18N-008): four locales, every key translated, Arabic and
/// Amharic drafted and marked `translated`, not `reviewed` (D15).
void main() {
  final keys = [
    for (final k in (_json('$_app/translations/keys.json')['keys']! as List))
      (k as Map<String, Object?>)['id']! as String,
  ];
  final byLocale = {
    for (final l in _locales) l: _json('$_app/translations/$l.json'),
  };

  test('the app supports en, de-DE, ar and am-ET with en the default '
      '[DX-004] [I18N-001]', () {
    final app = _json('$_app/app.json');
    expect(app['supportedLocales'], _locales);
    expect(app['defaultLocale'], 'en');
  });

  for (final l in _locales) {
    test('$l translates every key under its own tag [I18N-001] [I18N-008]', () {
      final doc = byLocale[l]!;
      final messages = doc['messages']! as Map<String, Object?>;
      expect(doc['locale'], l);
      expect(messages.keys.toSet(), keys.toSet());
      expect(messages.values.every((m) => (m! as String).isNotEmpty), isTrue);
      if (l == 'ar' || l == 'am-ET') {
        final states = doc['states']! as Map<String, Object?>;
        expect(states.keys.toSet(), keys.toSet());
        expect(states.values.toSet(), {'translated'});
      }
    });
  }

  testWidgets('switching the locale en to ar to am-ET changes the text and '
      'the direction [I18N-002] [I18N-005]', (tester) async {
    const title = '01a14cd1-ca0d-753c-9aed-14f6ccf98f5c';
    String tag = 'en';
    late StateSetter set;
    await tester.pumpWidget(
      StatefulBuilder(
        builder: (context, setState) {
          set = setState;
          return _Page(
            tag,
            byLocale[tag]!['messages']! as Map<String, Object?>,
            title,
          );
        },
      ),
    );
    Directionality dir() => tester.widget<Directionality>(
      find
          .descendant(
            of: find.byType(_Page),
            matching: find.byType(Directionality),
          )
          .first,
    );
    expect(find.text('Catalogue'), findsOneWidget);
    expect(dir().textDirection, TextDirection.ltr);

    set(() => tag = 'ar');
    await tester.pump();
    expect(find.text('الكتالوج'), findsOneWidget);
    expect(dir().textDirection, TextDirection.rtl);

    set(() => tag = 'am-ET');
    await tester.pump();
    expect(find.text('ካታሎግ'), findsOneWidget);
    expect(dir().textDirection, TextDirection.ltr);

    set(() => tag = 'de-DE');
    await tester.pump();
    expect(find.text('Katalog'), findsOneWidget);
  });
}
