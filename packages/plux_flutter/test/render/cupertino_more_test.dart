// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';

import '../support/harness.dart';

/// The Cupertino and structural widgets of K1d (ADR-0071): the navigation
/// bar, tab bar, picker, date picker, context menu and data scope, compiled
/// by the Go compiler from the gallery page `cupertino-more`.
void main() {
  final g = Harness.goldens;
  late Harness h;
  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  Future<void> open(
    WidgetTester tester, {
    bool dark = false,
    bool rtl = false,
    double scale = 1,
  }) async {
    tester.view.physicalSize = const Size(800, 1400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.runAsync(() async {
      await h.startFrom(g.bundles['widgets/widgets.pxb']!, {
        'gallery': g.bundles['widgets/gallery.pxb']!,
      }, themeSource: PluxThemeSource.host);
      await h.loadIconFonts();
    });
    await tester.pumpWidget(
      MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: dark ? ThemeData.dark() : ThemeData.light(),
        builder: (context, child) => Directionality(
          textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
          child: MediaQuery(
            data: MediaQuery.of(context)
                .copyWith(textScaler: TextScaler.linear(scale)),
            child: child!,
          ),
        ),
        home: PluxScope(child: PluxView('cupertino-more')),
      ),
    );
    await settle(tester);
  }

  testWidgets('renders every widget of the group without problems '
      '[WGT-020]', (tester) async {
    await open(tester);
    expect(find.byType(CupertinoNavigationBar), findsOneWidget);
    expect(find.byType(CupertinoTabBar), findsOneWidget);
    expect(find.text('Apple'), findsOneWidget);
    expect(find.byType(CupertinoDatePicker), findsOneWidget);
    expect(find.byType(CupertinoContextMenu), findsOneWidget);
    for (final t in ['Back', 'Title', 'Edit', 'Home', 'Search', 'Profile']) {
      expect(find.text(t), findsOneWidget, reason: t);
    }
    // The gallery's static source is a kind the test runtime does not load
    // (PLX-5108): DataScope then shows its error state.
    expect(
      h.errors.where(
        (e) =>
            e.code != PluxErrorCode.syncFailed &&
            e.code != PluxErrorCode.dataSourceUnavailable,
      ),
      isEmpty,
    );
  });

  testWidgets('tapping a tab fires onTap with its index, which the page '
      'binds back to currentIndex [WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('tab 0'), findsOneWidget);
    await tester.tap(find.text('Search'));
    await settle(tester);
    expect(find.text('tab 1'), findsOneWidget);
    await tester.tap(find.text('Profile'));
    await settle(tester);
    expect(find.text('tab 2'), findsOneWidget);
    final bar = tester.widget<CupertinoTabBar>(find.byType(CupertinoTabBar));
    expect(bar.currentIndex, 2);
  });

  testWidgets('scrolling the picker reports the selected item and starts '
      'at the bound index [WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('wheel 1'), findsOneWidget);
    await tester.drag(find.text('Banana'), const Offset(0, -64));
    await settle(tester);
    expect(find.text('wheel 3'), findsOneWidget);
  });

  testWidgets('scrolling the date picker fires onDateTimeChanged with a '
      'dateTime inside the bounds [WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('chosen none'), findsOneWidget);
    await tester.drag(find.byType(CupertinoDatePicker), const Offset(0, -64));
    await settle(tester);
    expect(find.textContaining('chosen 2026-'), findsOneWidget);
  });

  testWidgets('a long press opens the context menu and choosing an action '
      'fires onAction with its index [WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('action -1'), findsOneWidget);
    final press = await tester.startGesture(
      tester.getCenter(find.text('Press and hold')),
    );
    await tester.pump(const Duration(milliseconds: 500));
    await tester.pump(const Duration(seconds: 1));
    await press.up();
    await tester.pumpAndSettle();
    expect(find.text('Share'), findsOneWidget);
    await tester.tap(find.text('Delete'));
    await tester.pumpAndSettle();
    expect(find.text('action 1'), findsOneWidget);
  });

  testWidgets('DataScope shows exactly one of its states [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    expect(find.textContaining('scope '), findsOneWidget);
    expect(find.text('scope error'), findsOneWidget);
  });

  testWidgets('the group reads correctly to a screen reader '
      '[A11Y-001]', (tester) async {
    final semantics = tester.ensureSemantics();
    await open(tester);
    final tabs = tester.getSemantics(find.text('Search'));
    expect(tabs.label, contains('Search'));
    expect(
      tester.getSemantics(find.byType(CupertinoDatePicker)).label,
      contains('pick a date'),
    );
    semantics.dispose();
  });

  group('light, dark, right-to-left and 200% text [WGT-020] [A11Y-001]', () {
    for (final (name, dark, rtl, scale) in [
      ('light', false, false, 1.0),
      ('dark', true, false, 1.0),
      ('rtl', false, true, 1.0),
      ('text200', false, false, 2.0),
    ]) {
      testWidgets(name, (tester) async {
        await open(tester, dark: dark, rtl: rtl, scale: scale);
        await expectLater(
          find.byType(MaterialApp),
          matchesGoldenFile('goldens/cupertino_more_$name.png'),
        );
      });
    }
  });
}
