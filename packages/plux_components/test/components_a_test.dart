// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_components/plux_components.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/render/scope.dart';

import '../../plux_flutter/test/support/harness.dart';

/// The Layer 2 components of P5, group A (ADR-0071): ConsentCheckbox,
/// SearchField, FilterBar, SortSheet, InfiniteList, WizardForm,
/// OnboardingPager, SettingsList, ProfileHeader and StatusTimeline, on the
/// gallery page `components-a`.
void main() {
  final g = Harness.goldens;
  late Harness h;
  setUp(() async {
    h = await Harness.create();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, (call) async {
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
    PluxThemeSource themeSource = PluxThemeSource.host,
    TransitionBuilder? builder,
  }) async {
    tester.view.physicalSize = const Size(800, 3400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.runAsync(() async {
      await h.startFrom(
        g.bundles['widgets/widgets.pxb']!,
        {'gallery': g.bundles['widgets/gallery.pxb']!},
        themeSource: themeSource,
        widgetPackages: const [PluxComponents()],
      );
      await h.loadIconFonts();
    });
    await tester.pumpWidget(
      MaterialApp(
        debugShowCheckedModeBanner: false,
        builder: builder,
        home: PluxScope(child: PluxView('components-a')),
      ),
    );
    await settle(tester);
  }

  /// Problems other than the sync that finds no server release.
  List<PluxException> problems() => [
    for (final e in h.errors)
      if (e.code != PluxErrorCode.syncFailed) e,
  ];

  /// The page's state.
  Map<String, Object?> state(WidgetTester tester) {
    final scope = RenderScope.of(tester.element(find.text('Ordered')));
    return Plux.container.read(pageStateProvider(scope.state!));
  }

  Future<void> tapText(WidgetTester tester, String text) async {
    await tester.tap(find.text(text));
    await settle(tester);
  }

  group('Group A components in light, dark, right-to-left and 200% text '
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
        for (final t in [
          'I accept the terms',
          'Read the terms',
          'Open',
          'Closed',
          'Sort by',
          'Newest',
          'Price',
          'one',
          'Account step',
          'Welcome',
          'Profile',
          'Privacy',
          'Ada Lovelace',
          'Mathematician',
          'Follow',
          'Shipped',
          'In transit',
        ]) {
          expect(find.text(t), findsWidgets, reason: t);
        }
        expect(tester.takeException(), isNull);
        expect(problems(), isEmpty);
        await expectLater(
          find.byType(MaterialApp),
          matchesGoldenFile('goldens/components_a_$name.png'),
        );
      });
    }
  });

  testWidgets('ConsentCheckbox fires onChanged and, when checked, onAccepted '
      'with the document version [WGT-020]', (tester) async {
    await open(tester);
    expect(state(tester)['accepted'], '');
    await tapText(tester, 'I accept the terms');
    expect(tester.widget<Checkbox>(find.byType(Checkbox)).value, isTrue);
    expect(state(tester)['accepted'], '2026-01');
    await tapText(tester, 'I accept the terms');
    expect(tester.widget<Checkbox>(find.byType(Checkbox)).value, isFalse);
  });

  testWidgets('SearchField fires onSearch after the debounce and '
      'onSuggestionSelected for a suggestion [WGT-020]', (tester) async {
    await open(tester);
    await tester.enterText(find.byType(TextField).first, 'ap');
    await tester.pump(const Duration(milliseconds: 20));
    expect(state(tester)['query'], '', reason: 'not before the debounce');
    await tester.pump(const Duration(milliseconds: 100));
    await settle(tester);
    expect(state(tester)['query'], 'ap');
    expect(find.text('apricot'), findsOneWidget);
    await tapText(tester, 'apricot');
    expect(state(tester)['picked'], 'apricot');
    expect(find.text('apple'), findsNothing, reason: 'suggestions close');
    expect(
      tester.widget<TextField>(find.byType(TextField).first).controller!.text,
      'apricot',
    );
  });

  testWidgets('FilterBar toggles chips and fires onToggle [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    FilterChip chip(String t) => tester.widget<FilterChip>(
      find.ancestor(of: find.text(t), matching: find.byType(FilterChip)),
    );
    expect(chip('Open').selected, isFalse);
    expect(chip('Closed').selected, isTrue);
    await tapText(tester, 'Open');
    expect(chip('Open').selected, isTrue);
    expect(state(tester)['toggled'], 'opentrue');
    await tapText(tester, 'Closed');
    expect(state(tester)['toggled'], 'closedfalse');
    final bar = tester.widget<SingleChildScrollView>(
      find
          .ancestor(
            of: find.byType(FilterChip).first,
            matching: find.byType(SingleChildScrollView),
          )
          .first,
    );
    expect(bar.scrollDirection, Axis.horizontal);
  });

  testWidgets('SortSheet selects one option and fires onSelected [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    expect(find.byIcon(Icons.radio_button_checked), findsOneWidget);
    await tapText(tester, 'Price');
    expect(state(tester)['sort'], 'price');
    expect(find.byIcon(Icons.radio_button_checked), findsOneWidget);
    final row = find.ancestor(
      of: find.text('Price'),
      matching: find.byType(InkWell),
    );
    expect(
      find.descendant(
        of: row.first,
        matching: find.byIcon(Icons.radio_button_checked),
      ),
      findsOneWidget,
    );
  });

  testWidgets('InfiniteList builds its items and fires onEndReached while '
      'more exist [WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('one'), findsOneWidget);
    expect(find.text('three'), findsOneWidget);
    expect(state(tester)['ended'], isTrue);
  });

  testWidgets('WizardForm validates each step through its form before it '
      'goes on [WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('Account step'), findsOneWidget);
    expect(find.text('Contact step'), findsNothing);
    expect(find.text('Back'), findsNothing, reason: 'nothing before step 1');
    await tapText(tester, 'Next');
    expect(find.text('Account step'), findsOneWidget, reason: 'a is empty');
    expect(state(tester)['step'], 0);
    await tester.enterText(find.byType(TextFormField), 'x');
    await settle(tester);
    await tapText(tester, 'Next');
    expect(find.text('Contact step'), findsOneWidget);
    expect(state(tester)['step'], 1);
    await tapText(tester, 'Back');
    expect(find.text('Account step'), findsOneWidget);
    expect(state(tester)['step'], 0);
    await tapText(tester, 'Next');
    await tapText(tester, 'Next');
    expect(find.text('Contact step'), findsOneWidget, reason: 'b is empty');
    await tester.enterText(find.byType(TextFormField), 'y');
    await settle(tester);
    await tapText(tester, 'Next');
    expect(find.text('Review step'), findsOneWidget);
    expect(state(tester)['submitted'], isFalse);
    await tapText(tester, 'Submit');
    expect(state(tester)['submitted'], isTrue);
  });

  testWidgets('OnboardingPager pages, reports the page and finishes with '
      'onDone [WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('Welcome'), findsOneWidget);
    await tapText(tester, 'Continue');
    await tester.pump(const Duration(milliseconds: 600));
    expect(state(tester)['pageIndex'], 1);
    expect(find.text('Stay in sync'), findsOneWidget);
    await tapText(tester, 'Continue');
    await tester.pump(const Duration(milliseconds: 600));
    expect(find.text('All set'), findsOneWidget);
    expect(state(tester)['done'], isFalse);
    await tapText(tester, 'Finish');
    expect(state(tester)['done'], isTrue);
  });

  testWidgets('SettingsList groups its rows under headers [WGT-020]', (
    tester,
  ) async {
    await open(tester);
    double top(String t) => tester.getTopLeft(find.text(t)).dy;
    expect(top('Account') < top('Profile'), isTrue);
    expect(top('Password') < top('Privacy'), isTrue);
    expect(top('Privacy') < top('Analytics'), isTrue);
  });

  testWidgets('ProfileHeader shows the initial without an avatar image '
      '[WGT-020]', (tester) async {
    await open(tester);
    expect(find.text('A'), findsOneWidget);
    expect(find.text('Follow'), findsOneWidget);
  });

  testWidgets('Group A components read as their screen-reader script says '
      '[WGT-020] [A11Y-001]', (tester) async {
    final semantics = tester.ensureSemantics();
    await open(tester);
    String label(String l) =>
        tester.getSemantics(find.bySemanticsLabel(l).first).label;
    // The consent reads as an unchecked checkbox with its text.
    final consent = tester.getSemantics(
      find.bySemanticsLabel('I accept the terms'),
    );
    expect(consent.label, 'I accept the terms');
    expect(
      consent,
      isSemantics(
        label: 'I accept the terms',
        hasCheckedState: true,
        isChecked: false,
        hasTapAction: true,
        hasEnabledState: false,
      ),
    );
    // Sort rows read as one radio group, the chosen one checked.
    expect(
      tester.getSemantics(find.bySemanticsLabel('Newest')),
      isSemantics(
        label: 'Newest',
        hasCheckedState: true,
        isChecked: true,
        isInMutuallyExclusiveGroup: true,
        hasTapAction: true,
      ),
    );
    expect(
      tester.getSemantics(find.bySemanticsLabel('Price')),
      isSemantics(
        label: 'Price',
        hasCheckedState: true,
        isChecked: false,
        isInMutuallyExclusiveGroup: true,
        hasTapAction: true,
      ),
    );
    for (final t in ['Sort by', 'Account', 'Privacy', 'Ada Lovelace']) {
      expect(
        tester.getSemantics(find.text(t)),
        isSemantics(label: t, isHeader: true),
        reason: t,
      );
    }
    // The timeline names each step's state; only the current one is live.
    expect(
      tester.getSemantics(
        find.bySemanticsLabel('Shipped, In transit, current step'),
      ),
      isSemantics(
        label: 'Shipped, In transit, current step',
        isLiveRegion: true,
      ),
    );
    expect(label('Ordered, Mon, done'), 'Ordered, Mon, done');
    expect(label('Returned, failed'), 'Returned, failed');
    expect(
      tester.getSemantics(find.bySemanticsLabel('Delivered, pending')),
      isSemantics(label: 'Delivered, pending'),
    );
    // The wizard announces its step and progress.
    expect(
      tester.getSemantics(find.bySemanticsLabel('Account 1 / 3')),
      isSemantics(label: 'Account 1 / 3', isLiveRegion: true),
    );
    semantics.dispose();
  });
}
