// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/actions/handlers.dart';
import 'package:plux_flutter/src/device/guard.dart';
import 'package:plux_flutter/src/l10n/calendar/calendar.dart';
import 'package:plux_flutter/src/l10n/locale_controller.dart';
import 'package:plux_flutter/src/l10n/locale_host.dart';
import 'package:plux_flutter/src/render/builders/calendar.dart';
import 'package:plux_flutter/src/render/scope.dart';

import '../actions/engine_test.dart' show FakeNavigator;
import '../support/harness.dart';
import '../support/locale_bundle.dart';

const _title = '01c0c450-6c00-7000-8000-000000000050';
const _count = '01c0c450-6c00-7000-8000-000000000051';
const _ok = '01c0c450-6c00-7000-8000-000000000052';

/// The runtime switching the locale of a rendered page (I18N-004,
/// I18N-005, I18N-006, ACT-020): the gallery's `text` page over a release
/// whose app bundle carries German, Arabic and English.
void main() {
  final g = Harness.goldens;
  late Harness h;
  late List<(String, Map<String, Object?>)> events;
  ProviderContainer? hostContainer;

  setUp(() async {
    h = await Harness.create();
    events = [];
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, (_) async => null);
  });
  tearDown(() {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, null);
    return h.close().then((_) => hostContainer?.dispose());
  });

  Future<void> open(
    WidgetTester tester,
    String route, {
    Map<String, Map<String, String>> plux = const {},
    Uint8List Function(Uint8List app)? tweak,
    Widget Function(Widget page)? wrap,
  }) async {
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final container = hostContainer = ProviderContainer(
      overrides: [
        localeSinkProvider.overrideWithValue(
          LocaleSink(
            report: (e) => h.runtime.reportProblem(e),
            record: (name, fields) => events.add((name, fields)),
          ),
        ),
      ],
    );
    await tester.runAsync(() async {
      final app = withLocales(
        g.bundles['widgets/widgets.pxb']!,
        {
          // German translates the plural only: the title falls back.
          'de': {_count: '{count, plural, one {# Widget} other {# Widgets}}'},
          'ar': {
            _title: 'المعرض',
            _count: '{count, plural, other {# عنصر}}',
            _ok: 'حسنا',
          },
        },
        plux: plux,
        verbatim: plux.keys.toSet().difference({'de', 'ar'}),
      );
      await h.startFrom(tweak == null ? app : tweak(app), {
        'gallery': g.bundles['widgets/gallery.pxb']!,
      }, container: container);
      await h.loadIconFonts();
    });
    await tester.pumpWidget(
      MaterialApp(
        debugShowCheckedModeBanner: false,
        home: PluxScope(
          child: (wrap ?? (w) => w)(
            PluxView(
              route,
              inputs: route == 'text' ? const {'who': 'Ada'} : const {},
            ),
          ),
        ),
      ),
    );
    await settle(tester);
  }

  Future<void> switchTo(WidgetTester tester, Locale? locale) async {
    await tester.runAsync(() async {
      Plux.setLocale(locale);
      await Plux.container.read(localeControllerProvider.notifier).settled();
    });
    await settle(tester);
  }

  PageStateNotifier state(WidgetTester tester, Finder finder) {
    final scope = RenderScope.of(tester.element(finder));
    return Plux.container.read(pageStateProvider(scope.state!).notifier);
  }

  List<PluxException> problems() => [
    for (final e in h.errors)
      if (e.code != PluxErrorCode.syncFailed) e,
  ];

  testWidgets('a switch of locale changes texts and direction in place and '
      'keeps the page state [I18N-005] [I18N-004]', (tester) async {
    await open(tester, 'text');
    expect(find.text('3 widgets'), findsOneWidget);
    expect(find.text('Gallery'), findsOneWidget);
    final page = state(tester, find.text('3 widgets'))..set('count', 5);
    await tester.pump();
    expect(find.text('5 widgets'), findsOneWidget);
    final before = tester.state(find.byType(PluxView));

    await switchTo(tester, const Locale('ar'));
    expect(find.text('5 عنصر'), findsOneWidget);
    expect(find.text('المعرض'), findsOneWidget);
    expect(
      Directionality.of(tester.element(find.text('المعرض'))),
      TextDirection.rtl,
    );
    expect(
      MaterialLocalizations.of(tester.element(find.text('المعرض')))
          .okButtonLabel,
      isNot('OK'),
    );
    expect(identical(tester.state(find.byType(PluxView)), before), isTrue);
    expect(page.state['count'], 5);

    await switchTo(tester, const Locale('en'));
    expect(find.text('5 widgets'), findsOneWidget);
    expect(
      Directionality.of(tester.element(find.text('5 widgets'))),
      TextDirection.ltr,
    );
    expect(problems(), isEmpty);
  });

  testWidgets('a message missing in the requested locale shows the default '
      "locale's text, marked in debug, and is reported and sampled "
      '[I18N-006] [PLX-4401]', (tester) async {
    await open(tester, 'text');
    await switchTo(tester, const Locale('de', 'AT'));
    expect(find.text('3 Widgets'), findsOneWidget);
    // The key is never shown: the app default's text stands in for it.
    expect(kDebugMode, isTrue);
    expect(find.text('⚠ Gallery'), findsOneWidget);
    await tester.pump();
    final missing = [
      for (final e in events)
        if (e.$1 == 'l10n_missing') e.$2,
    ];
    expect(missing, [
      {'key': _title, 'requested': 'de', 'resolved': 'en'},
    ]);
    expect(
      problems().map((e) => e.code),
      contains(PluxErrorCode.translationFallback),
    );
  });

  testWidgets('a locale the release does not have is refused with PLX-4400 '
      'and the page stays as it was [I18N-005] [PLX-4400]', (tester) async {
    await open(tester, 'text');
    await switchTo(tester, const Locale('fr'));
    expect(find.text('3 widgets'), findsOneWidget);
    expect(problems().map((e) => e.code), [PluxErrorCode.localeNotInRelease]);
    expect(Plux.container.read(localeControllerProvider).tag, 'en');
  });

  testWidgets('the setLocale action switches the locale like '
      'Plux.setLocale [I18N-005] [ACT-020]', (tester) async {
    await open(tester, 'text');
    final handler = builtInHandlers['setLocale']!;
    StepContext ctx() => StepContext(
      navigator: FakeNavigator(),
      emit: (_, _) {},
      nativeActions: const NoNativeActions(),
      device: DeviceScope(
        plugin: '',
        guard: DeviceGuard(
          capabilities: (_) => (deviceApis: const [], networkDomains: const []),
          deepLinks: () => null,
          report: (_) {},
        ),
        context: () => null,
      ),
    );
    await tester.runAsync(() async {
      await handler.run(ctx(), {'locale': 'de'});
      await Plux.container.read(localeControllerProvider.notifier).settled();
    });
    await settle(tester);
    expect(find.text('3 Widgets'), findsOneWidget);
    expect(
      () => handler.run(ctx(), {'locale': ''}),
      throwsA(isA<ActionError>()),
    );
    expect(() => handler.run(ctx(), {}), throwsA(isA<ActionError>()));
  });

  // Verifies: I18N-011, I18N-003.
  testWidgets("the app bundle's calendar settings reach the date picker "
      'under am-ET [I18N-011] [I18N-003]', (tester) async {
    await open(
      tester,
      'text',
      tweak: (app) => withLocalisation(
        app,
        calendar: 'ethiopic',
        secondary: 'gregorian',
        nativeDigits: {'am'},
      ),
      plux: {'am': const {}},
    );
    await switchTo(tester, const Locale('am', 'ET'));
    final settings = Plux.container.read(appLocalisationProvider);
    expect(settings.calendar, 'ethiopic');
    expect(settings.secondaryCalendar, 'gregorian');
    expect(settings.nativeDigits, {'am'});
    final state = Plux.container.read(localeControllerProvider);
    expect(state.tag, 'am');
    // The picker has no calendar of its own: the app's apply.
    await tester.pumpWidget(
      MaterialApp(
        home: PluxLocaleHost(
          state: state,
          child: Material(
            child: Builder(
              builder: (context) => DualCalendarView(
                primary: calendarSystem(PluxLocaleScope.calendarOf(context)!)!,
                secondary: calendarSystem(
                  PluxLocaleScope.secondaryCalendarOf(context)!,
                ),
                value: null,
                firstJdn: 2460565,
                lastJdn: 2461000,
                onChanged: null,
                // 11 September 2024 is 1 Meskerem: the grid opens on today.
                now: () => DateTime(2024, 9, 11),
              ),
            ),
          ),
        ),
      ),
    );
    await settle(tester);
    expect(find.textContaining('Meskerem'), findsOneWidget);
  });

  // Verifies: I18N-005.
  testWidgets('a locale that follows the host follows its changes '
      '[I18N-005]', (tester) async {
    await open(tester, 'text');
    await switchTo(tester, null);
    tester.platformDispatcher.localesTestValue = const [Locale('ar')];
    addTearDown(tester.platformDispatcher.clearLocalesTestValue);
    await tester.runAsync(() async {
      await Future<void>.delayed(const Duration(milliseconds: 50));
      Plux.container.read(localeControllerProvider);
      await Plux.container.read(localeControllerProvider.notifier).settled();
    });
    await settle(tester);
    await tester.runAsync(() async {
      await Plux.container.read(localeControllerProvider.notifier).settled();
    });
    await settle(tester);
    expect(Plux.container.read(localeControllerProvider).tag, 'ar');
    expect(
      Plux.container.read(localeControllerProvider).direction,
      TextDirection.rtl,
    );
  });

  // Verifies: ACT-020, I18N-005.
  testWidgets('the not-found page and the fallback use the runtime strings '
      'of the effective locale [ACT-020] [I18N-005]', (tester) async {
    await open(
      tester,
      'text',
      plux: {
        'de': {
          'plux.router.notFound': 'Seite nicht gefunden',
          'plux.fallback.unavailable': 'Inhalt nicht verfügbar',
        },
      },
    );
    await switchTo(tester, const Locale('de'));
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: Plux.container,
        child: const MaterialApp(
          home: Column(
            children: [
              Expanded(child: PluxNotFoundPage(route: 'x')),
              Expanded(
                child: PluxDefaultFallback(
                  PluxException(PluxErrorCode.routeNotFound, 'x'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
    expect(find.text('Seite nicht gefunden'), findsOneWidget);
    expect(find.bySemanticsLabel('Inhalt nicht verfügbar'), findsOneWidget);
  });
}
