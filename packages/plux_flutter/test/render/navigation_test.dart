// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';

import '../support/harness.dart';

/// The navigation and menu widgets of the gallery (WGT-020, A11Y-001):
/// `PopupMenuButton`, `MenuAnchor`, `SearchBar`, `SearchAnchor`,
/// `Autocomplete`, `CalendarDatePicker`, `TabBar`, `TabBarView`,
/// `NavigationBar`, `BottomNavigationBar`, `Drawer`, `NavigationDrawer` and
/// `NavigationRail`, compiled by the Go compiler and rendered here.
/// Lets futures complete and every animation end.
Future<void> idle(WidgetTester tester) async {
  await settle(tester);
  await tester.pumpAndSettle(const Duration(milliseconds: 50));
  await settle(tester);
}

void main() {
  final g = Harness.goldens;
  late Harness h;
  setUp(() async => h = await Harness.create());
  tearDown(() => h.close());

  Future<void> open(
    WidgetTester tester, {
    TransitionBuilder? builder,
    PluxThemeSource themeSource = PluxThemeSource.host,
  }) async {
    tester.view.physicalSize = const Size(800, 3400);
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
        home: PluxScope(child: PluxView('navigation')),
      ),
    );
    await idle(tester);
  }

  List<PluxException> problems() => [
    for (final e in h.errors)
      if (e.code != PluxErrorCode.syncFailed) e,
  ];

  Finder within(Type type, Finder what) =>
      find.descendant(of: find.byType(type), matching: what);

  group('the gallery page in light, dark, right-to-left and 200% text '
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
          await idle(tester);
          await tester.pump(const Duration(milliseconds: 300));
        }
        for (final t in [
          'picked: none',
          'query: ',
          'date: 2026-03-10',
          'tab: 0',
          'nav: 0',
          'bottom: 0',
          'panel: 0',
          'rail: 0',
          'pane one',
          'drawer header',
        ]) {
          expect(find.text(t), findsOneWidget, reason: t);
        }
        expect(problems(), isEmpty);
        await expectLater(
          find.byType(MaterialApp),
          matchesGoldenFile('goldens/navigation_$name.png'),
        );
      });
    }
  });

  testWidgets('PopupMenuButton and MenuAnchor fire onSelected with the '
      'chosen value; a disabled entry cannot be chosen [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    await tester.tap(find.byTooltip('Open menu'));
    await idle(tester);
    expect(find.text('Gamma'), findsOneWidget);
    await tester.tap(find.text('Gamma'), warnIfMissed: false);
    await idle(tester);
    expect(find.text('picked: none'), findsOneWidget, reason: 'disabled');
    await tester.tap(find.text('Beta'));
    await idle(tester);
    expect(find.text('picked: b'), findsOneWidget);

    await tester.tap(find.text('anchor'));
    await idle(tester);
    await tester.tap(find.text('Alpha'));
    await idle(tester);
    expect(find.text('picked: a'), findsOneWidget);
    expect(problems(), isEmpty);
  });

  testWidgets('SearchBar writes its text to state; SearchAnchor and '
      'Autocomplete filter ignoring case and fire onSelected [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    await tester.enterText(
      find.descendant(
        of: find.byType(SearchBar).first,
        matching: find.byType(EditableText),
      ),
      'hello',
    );
    await idle(tester);
    expect(find.text('query: hello'), findsOneWidget);

    await tester.enterText(
      within(Autocomplete<String>, find.byType(EditableText)),
      'BAN',
    );
    await idle(tester);
    expect(find.text('Apple'), findsNothing);
    await tester.tap(find.text('Banana'));
    await idle(tester);
    expect(find.text('picked: Banana'), findsOneWidget);

    await tester.tap(find.byType(SearchAnchor));
    await idle(tester);
    await tester.enterText(find.byType(EditableText).last, 'AP');
    await idle(tester);
    expect(
      find.widgetWithText(ListTile, 'Banana'),
      findsNothing,
      reason: 'filtered',
    );
    expect(find.text('Apple'), findsOneWidget);
    await tester.tap(find.text('Apricot'));
    await idle(tester);
    expect(find.text('picked: Apricot'), findsOneWidget);
    expect(problems(), isEmpty);
  });

  testWidgets('CalendarDatePicker fires onDateChanged with the picked date '
      '[WGT-020]', (tester) async {
    await open(tester);
    await tester.tap(within(CalendarDatePicker, find.text('15')));
    await idle(tester);
    expect(find.text('date: 2026-03-15'), findsOneWidget);
    expect(problems(), isEmpty);
  });

  testWidgets('TabBar and TabBarView share one index through state '
      '[WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('pane one'), findsOneWidget);
    await tester.tap(within(TabBar, find.text('Two')));
    await idle(tester);
    expect(find.text('tab: 1'), findsOneWidget);
    expect(find.text('pane two'), findsOneWidget);

    await tester.fling(find.byType(TabBarView), const Offset(-250, 0), 1500);
    await idle(tester);
    expect(find.text('tab: 2'), findsOneWidget, reason: 'swiped');
    expect(find.text('pane three'), findsOneWidget);
    expect(
      tester.widget<TabBar>(find.byType(TabBar)).controller!.index,
      2,
      reason: 'the bar follows the view',
    );
    expect(problems(), isEmpty);
  });

  testWidgets('bars, drawer and rail fire their selection event with the '
      'index [WGT-020]', (tester) async {
    await open(tester);
    await tester.tap(within(NavigationBar, find.text('Search')));
    await idle(tester);
    expect(find.text('nav: 1'), findsOneWidget);

    await tester.tap(within(BottomNavigationBar, find.text('Profile')));
    await idle(tester);
    expect(find.text('bottom: 2'), findsOneWidget);

    await tester.tap(within(NavigationDrawer, find.text('Profile')));
    await idle(tester);
    expect(find.text('panel: 2'), findsOneWidget);

    await tester.tap(within(NavigationRail, find.text('Search')).first);
    await idle(tester);
    expect(find.text('rail: 1'), findsOneWidget);
    expect(problems(), isEmpty);
  });

  testWidgets('Scaffold.drawer holds a Drawer that opens [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    expect(find.text('drawer body'), findsNothing);
    tester.state<ScaffoldState>(find.byType(Scaffold)).openDrawer();
    await idle(tester);
    expect(find.text('drawer body'), findsOneWidget);
    expect(problems(), isEmpty);
  });

  testWidgets('navigation widgets read as their screen-reader script says '
      '[A11Y-001]', (tester) async {
    final semantics = tester.ensureSemantics();
    await open(tester);

    expect(
      tester.getSemantics(find.byTooltip('Open menu')),
      isSemantics(tooltip: 'Open menu', isButton: true, hasTapAction: true),
    );
    expect(
      tester.getSemantics(within(TabBar, find.text('One'))),
      isSemantics(isSelected: true, hasTapAction: true),
      reason: 'the selected tab',
    );
    expect(
      tester.getSemantics(within(TabBar, find.text('Two'))),
      isSemantics(isSelected: false, hasTapAction: true),
    );
    for (final label in [
      'main navigation',
      'bottom navigation',
      'date picker',
      'sections',
    ]) {
      expect(find.bySemanticsLabel(label), findsWidgets, reason: label);
    }
    expect(
      tester.getSemantics(within(NavigationBar, find.text('Home'))),
      isSemantics(isSelected: true, hasTapAction: true),
    );
    expect(problems(), isEmpty);
    semantics.dispose();
  });
}
