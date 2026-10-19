// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';

import '../support/harness.dart';

/// `DualCalendarDatePicker` (I18N-003, A11Y-001), rendered from the gallery
/// page `calendar`.
void main() {
  final g = Harness.goldens;
  late Harness h;
  late List<String> haptics;

  setUp(() async {
    h = await Harness.create();
    haptics = [];
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, (call) async {
          if (call.method.startsWith('HapticFeedback.')) {
            haptics.add(call.method);
          }
          return null;
        });
  });
  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, null);
    return h.close();
  });

  Future<void> open(
    WidgetTester tester, {
    bool rtl = false,
    double scale = 1,
  }) async {
    tester.view.physicalSize = const Size(800, 2000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.runAsync(() async {
      await h.startFrom(g.bundles['widgets/widgets.pxb']!, {
        'gallery': g.bundles['widgets/gallery.pxb']!,
      }, themeSource: PluxThemeSource.plux);
      await h.loadIconFonts();
    });
    await tester.pumpWidget(
      MaterialApp(
        debugShowCheckedModeBanner: false,
        builder: (context, child) => Directionality(
          textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
          child: MediaQuery(
            data: MediaQuery.of(context)
                .copyWith(textScaler: TextScaler.linear(scale)),
            child: child!,
          ),
        ),
        home: const PluxScope(child: PluxView('calendar')),
      ),
    );
    await settle(tester);
  }

  List<PluxException> problems() => [
    for (final e in h.errors)
      if (e.code != PluxErrorCode.syncFailed) e,
  ];

  group('picker in light, dark, right-to-left and 200% text '
      '[I18N-003]', () {
    for (final (name, dark, rtl, scale) in [
      ('light', false, false, 1.0),
      ('dark', true, false, 1.0),
      ('rtl', false, true, 1.0),
      ('text200', false, false, 2.0),
    ]) {
      testWidgets(name, (tester) async {
        await open(tester, rtl: rtl, scale: scale);
        if (dark) {
          Plux.setThemeMode(ThemeMode.dark);
          await settle(tester);
          await tester.pump(const Duration(milliseconds: 300));
        }
        // 11 March 2024 is 1 Ramadan 1445 in Umm al-Qura; 20 March 2024 is
        // 1 Farvardin 1403.
        expect(find.text('March 2024'), findsOneWidget);
        expect(find.text('Shaʻban – Ramadan 1445 AH'), findsOneWidget);
        expect(find.text('Farvardin 1403 AP'), findsOneWidget);
        expect(find.text('March – April 2024'), findsOneWidget);
        expect(problems(), isEmpty);
        await expectLater(
          find.byType(MaterialApp),
          matchesGoldenFile('goldens/calendar_$name.png'),
        );
      });
    }
  });

  testWidgets('each day is a button labelled in both calendars and the '
      'chosen one is selected [I18N-003, A11Y-001]', (tester) async {
    final handle = tester.ensureSemantics();
    await open(tester);
    expect(
      tester.getSemantics(
        find.bySemanticsLabel('Monday, March 11, 2024, 1 Ramadan 1445 AH'),
      ),
      matchesSemantics(
        label: 'Monday, March 11, 2024, 1 Ramadan 1445 AH',
        isButton: true,
        isSelected: true,
        isEnabled: true,
        hasEnabledState: true,
        hasSelectedState: true,
        hasTapAction: true,
      ),
    );
    expect(
      find.bySemanticsLabel('Tuesday, March 12, 2024, 2 Ramadan 1445 AH'),
      findsOneWidget,
    );
    expect(
      find.bySemanticsLabel('1 Farvardin 1403 AP, Wednesday, March 20, 2024'),
      findsOneWidget,
    );
    handle.dispose();
  });

  testWidgets('choosing a day fires onChanged and the arrows change month '
      '[I18N-003]', (tester) async {
    await open(tester);
    await tester.tap(
      find.bySemanticsLabel('Tuesday, March 12, 2024, 2 Ramadan 1445 AH'),
    );
    await tester.pump();
    await tester.pump();
    expect(haptics, hasLength(1));
    await tester.tap(find.byTooltip('Next month').first);
    await tester.pump();
    expect(find.text('April 2024'), findsOneWidget);
    await tester.tap(find.byTooltip('Previous month').first);
    await tester.pump();
    await tester.tap(find.byTooltip('Previous month').first);
    await tester.pump();
    expect(find.text('February 2024'), findsOneWidget);
  });
}
