// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';

import '../support/harness.dart';

/// The scrolling, layout and display widgets of the gallery's `scrolling`
/// page (WGT-020, ADR-0071): `Table`, `BackdropFilter`, `RefreshIndicator`,
/// `ReorderableListView`, `NestedScrollView`, `SliverPersistentHeader`,
/// `DataTable`, `Stepper` and `MaterialBanner`.
void main() {
  final g = Harness.goldens;
  late Harness h;
  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  Future<void> open(
    WidgetTester tester, {
    PluxThemeSource themeSource = PluxThemeSource.host,
    TransitionBuilder? builder,
  }) async {
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.runAsync(() async {
      await h.startFrom(g.bundles['widgets/widgets.pxb']!, {
        'gallery': g.bundles['widgets/gallery.pxb']!,
      }, themeSource: themeSource);
      await h.loadIconFonts();
    });
    await tester.pumpWidget(
      MaterialApp(
        debugShowCheckedModeBanner: false,
        builder: builder,
        home: PluxScope(child: const PluxView('scrolling')),
      ),
    );
    await settle(tester);
  }

  List<PluxException> problems() => [
    for (final e in h.errors)
      if (e.code != PluxErrorCode.syncFailed) e,
  ];

  testWidgets('every widget of the group renders without a problem '
      '[WGT-020]', (tester) async {
    await open(tester);
    for (final type in [
      Table,
      BackdropFilter,
      RefreshIndicator,
      ReorderableListView,
      NestedScrollView,
      SliverPersistentHeader,
      DataTable,
      Stepper,
      MaterialBanner,
    ]) {
      expect(find.byType(type), findsWidgets, reason: "$type");
    }
    expect(problems(), isEmpty);
  });

  testWidgets('Table lays its children out row by row in the given column '
      'widths [WGT-020]', (tester) async {
    await open(tester);
    final a1 = tester.getTopLeft(find.text('a1'));
    final b1 = tester.getTopLeft(find.text('b1'));
    final a2 = tester.getTopLeft(find.text('a2'));
    expect(b1.dx - a1.dx, 80, reason: 'first column is fixed at 80');
    expect(a2.dx, a1.dx);
    expect(a2.dy, greaterThan(a1.dy));
    expect(find.text('a3'), findsOneWidget, reason: 'the last row is short');
  });

  testWidgets('BackdropFilter blurs what is behind it [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    final filter = tester.widget<BackdropFilter>(find.byType(BackdropFilter));
    expect(filter.enabled, isTrue);
    expect(filter.filter.toString(), contains('blur'));
  });

  testWidgets('RefreshIndicator fires onRefresh when the list is pulled '
      'down [WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('refreshed 0'), findsOneWidget);
    await tester.fling(
      find.descendant(
        of: find.byType(RefreshIndicator),
        matching: find.text('a'),
      ),
      const Offset(0, 300),
      1000,
    );
    await tester.pump(const Duration(seconds: 1));
    await settle(tester);
    expect(find.text('refreshed 1'), findsOneWidget);
  });

  testWidgets('ReorderableListView fires onReorder with the old and the new '
      'index after a drag [WGT-020]', (tester) async {
    await open(tester);
    final tile = find.text('first').first;
    final gesture = await tester.startGesture(tester.getCenter(tile));
    await tester.pump(kLongPressTimeout + const Duration(milliseconds: 50));
    for (var i = 0; i < 6; i++) {
      await gesture.moveBy(const Offset(0, 20));
      await tester.pump(const Duration(milliseconds: 50));
    }
    await gesture.up();
    await settle(tester);
    expect(find.text('moved 0>1'), findsOneWidget);
  });

  testWidgets('NestedScrollView scrolls its header away with the body '
      '[WGT-020]', (tester) async {
    await open(tester);
    final before = tester.getTopLeft(find.text('nested header')).dy;
    await tester.drag(
      find.descendant(
        of: find.byType(NestedScrollView),
        matching: find.text('c'),
      ),
      const Offset(0, -30),
    );
    await settle(tester);
    expect(tester.getTopLeft(find.text('nested header')).dy, lessThan(before));
  });

  testWidgets('SliverPersistentHeader stays pinned at its minimum extent '
      '[WGT-020]', (tester) async {
    await open(tester);
    final header = find.text('pinned header');
    final expanded = tester.getSize(
      find.ancestor(of: header, matching: find.byType(ColoredBox)).first,
    );
    expect(expanded.height, 80);
    await tester.drag(
      find.descendant(
        of: find.byType(CustomScrollView),
        matching: find.text('d'),
      ),
      const Offset(0, -150),
    );
    await settle(tester);
    expect(header, findsOneWidget);
    expect(
      tester
          .getSize(
            find.ancestor(of: header, matching: find.byType(ColoredBox)).first,
          )
          .height,
      40,
    );
  });

  testWidgets('DataTable sorts by a heading and reports row selection '
      '[WGT-020] [A11Y-001]', (tester) async {
    await open(tester);
    expect(find.text('sort 1 true'), findsOneWidget);
    await tester.tap(find.text('Name'));
    await settle(tester);
    expect(find.text('sort 0 true'), findsOneWidget);
    await tester.tap(find.text('Name'));
    await settle(tester);
    expect(find.text('sort 0 false'), findsOneWidget);
    await tester.tap(find.byType(Checkbox).at(2));
    await settle(tester);
    expect(find.text('selected 1:true'), findsOneWidget);
    expect(find.text('Pears'), findsOneWidget);
    expect(problems(), isEmpty);
  });

  testWidgets('Stepper opens the tapped step and follows Continue and '
      'Cancel [WGT-020] [A11Y-001]', (tester) async {
    await open(tester);
    expect(find.text('step 0'), findsOneWidget);
    expect(find.text('content one'), findsOneWidget);
    await tester.tap(find.text('Two'));
    await settle(tester);
    expect(find.text('step 1'), findsOneWidget);
    expect(find.text('content two'), findsOneWidget);
    await tester.tap(find.text('Continue').hitTestable());
    await settle(tester);
    expect(find.text('step 2'), findsOneWidget);
    await tester.tap(find.text('Cancel').hitTestable());
    await settle(tester);
    expect(find.text('step 1'), findsOneWidget);
  });

  testWidgets('MaterialBanner shows its message and runs its action '
      '[WGT-020] [A11Y-001]', (tester) async {
    final semantics = tester.ensureSemantics();
    await open(tester);
    expect(find.text('Connection restored'), findsOneWidget);
    expect(
      tester.getSemantics(find.text('Connection restored')),
      isSemantics(label: 'Connection restored'),
    );
    expect(
      tester.getSemantics(find.text('dismiss')),
      isSemantics(
        label: 'dismiss',
        isButton: true,
        hasTapAction: true,
        isEnabled: true,
        hasEnabledState: true,
        isFocusable: true,
      ),
    );
    await tester.tap(find.text('dismiss'));
    await settle(tester);
    expect(find.text('banner dismissed'), findsOneWidget);
    semantics.dispose();
  });

  testWidgets('interactive widgets expose their controls to a screen '
      'reader [A11Y-001]', (tester) async {
    final semantics = tester.ensureSemantics();
    await open(tester);
    expect(
      tester.getSemantics(find.text('Continue').hitTestable()),
      isSemantics(
        label: 'Continue',
        isButton: true,
        hasTapAction: true,
        isEnabled: true,
        hasEnabledState: true,
        isFocusable: true,
      ),
    );
    expect(
      tester.getSemantics(find.text('Name')),
      isSemantics(label: 'Name', hasTapAction: true),
    );
    semantics.dispose();
  });

  group('the group in light, dark, right-to-left and 200% text '
      '[WGT-020]', () {
    for (final (name, dark, rtl, scale) in [
      ('light', false, false, 1.0),
      ('dark', true, false, 1.0),
      ('rtl', false, true, 1.0),
      ('text200', false, false, 2.0),
    ]) {
      testWidgets(name, (tester) async {
        await open(
          tester,
          themeSource: PluxThemeSource.plux,
          builder: (context, child) => Directionality(
            textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
            child: MediaQuery(
              data: MediaQuery.of(context)
                  .copyWith(textScaler: TextScaler.linear(scale)),
              child: child!,
            ),
          ),
        );
        if (dark) {
          Plux.setThemeMode(ThemeMode.dark);
          await settle(tester);
          await tester.pump(const Duration(milliseconds: 300));
        }
        expect(problems(), isEmpty);
        await expectLater(
          find.byType(MaterialApp),
          matchesGoldenFile('goldens/scrolling_$name.png'),
        );
      });
    }
  });
}
