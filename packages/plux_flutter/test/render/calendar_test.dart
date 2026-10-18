// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/l10n/calendar/calendar.dart';
import 'package:plux_flutter/src/render/builders/calendar.dart';

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

  // The grid opens on today when there is no value; the 1900-01-01 lower
  // bound stays the selectable range start.
  group('opens on the current month without a value [I18N-003]', () {
    int jdn(int y, int m, int d) =>
        calendarSystem(gregorianId)!.toJdn(CalendarYmd(y, m, d));

    Future<void> pumpPicker(
      WidgetTester tester, {
      required DateTime Function() now,
      int? value,
      int? first,
      int? last,
    }) async {
      await tester.pumpWidget(
        MaterialApp(
          home: Material(
            child: DualCalendarView(
              primary: calendarSystem(gregorianId)!,
              secondary: null,
              value: value,
              firstJdn: first ?? jdn(1900, 1, 1),
              lastJdn: last ?? jdn(2100, 12, 31),
              onChanged: (_) {},
              now: now,
            ),
          ),
        ),
      );
      await tester.pump();
    }

    testWidgets('shows the month of today', (tester) async {
      await pumpPicker(tester, now: () => DateTime(2024, 7, 15));
      expect(find.text('July 2024'), findsOneWidget);
    });

    testWidgets('a value wins over today', (tester) async {
      await pumpPicker(
        tester,
        now: () => DateTime(2024, 7, 15),
        value: jdn(2024, 3, 11),
      );
      expect(find.text('March 2024'), findsOneWidget);
      expect(find.text('July 2024'), findsNothing);
    });

    testWidgets('today before the lower bound opens on the lower bound', (
      tester,
    ) async {
      await pumpPicker(tester, now: () => DateTime(1850, 5, 1));
      expect(find.text('January 1900'), findsOneWidget);
      // The range is unchanged: 1900 is still selectable from its month.
      expect(
        tester
            .widget<IconButton>(
              find.ancestor(
                of: find.byTooltip('Previous month'),
                matching: find.byType(IconButton),
              ),
            )
            .onPressed,
        isNull,
      );
    });

    testWidgets('today after the upper bound opens on the upper bound', (
      tester,
    ) async {
      await pumpPicker(tester, now: () => DateTime(2150, 1, 1));
      expect(find.text('December 2100'), findsOneWidget);
    });

    testWidgets('a given range is honoured when it excludes today', (
      tester,
    ) async {
      await pumpPicker(
        tester,
        now: () => DateTime(2024, 7, 15),
        first: jdn(2010, 1, 1),
        last: jdn(2012, 6, 30),
      );
      expect(find.text('June 2012'), findsOneWidget);
    });
  });
}
