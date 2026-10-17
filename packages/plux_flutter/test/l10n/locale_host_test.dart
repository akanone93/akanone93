// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/l10n/cldr_index.dart';
import 'package:plux_flutter/src/l10n/locale_controller.dart';
import 'package:plux_flutter/src/l10n/locale_host.dart';
import 'package:plux_flutter/src/l10n/scope.dart';
import 'package:plux_flutter/src/state/providers.dart';

import '../support/locale_bundle.dart';

Future<R> _inline<R>(R Function() work) async => work();

/// Page state that must survive a switch of locale.
final class _Counter extends StatefulWidget {
  const _Counter();

  @override
  State<_Counter> createState() => _CounterState();
}

final class _CounterState extends State<_Counter> {
  int taps = 0;

  @override
  Widget build(BuildContext context) => Column(
    children: [
      Text('taps $taps', key: const Key('taps')),
      Text(
        MaterialLocalizations.of(context).cancelButtonLabel,
        key: const Key('cancel'),
      ),
      Text(
        CupertinoLocalizations.of(context).copyButtonLabel,
        key: const Key('copy'),
      ),
      Text('${Directionality.of(context)}', key: const Key('direction')),
      Text(
        MaterialLocalizations.of(context).formatDecimal(1234),
        key: const Key('number'),
      ),
      Text(
        PluxLocaleScope.calendarOf(context) ?? 'none',
        key: const Key('calendar'),
      ),
      TextButton(
        onPressed: () => setState(() => taps++),
        child: const Text('tap'),
      ),
    ],
  );
}

LocaleRelease _release({
  String calendar = 'gregorian',
  Set<String> nativeDigits = const {},
}) {
  const supported = ['en', 'de', 'ar', 'am', 'fa'];
  final payloads = {for (final t in supported) t: localePayload(t)};
  return LocaleRelease(
    id: 1,
    defaultLocale: 'en',
    supported: supported,
    index: cldrLocaleIndex,
    payload: (tag) => payloads[tag],
    calendar: calendar,
    nativeDigits: nativeDigits,
  );
}

ProviderContainer _container(LocaleRelease release, {Locale? host}) {
  final c = ProviderContainer(
    overrides: [
      localeReleaseProvider.overrideWithValue(release),
      localeOffloadProvider.overrideWithValue(_inline),
      localeSinkProvider.overrideWithValue(
        LocaleSink(report: (_) {}, record: (_, _) {}),
      ),
    ],
  );
  addTearDown(c.dispose);
  c
      .read(environmentProvider.notifier)
      .replace(PluxEnvironment(locale: host ?? const Locale('en')));
  return c;
}

Future<void> _show(
  WidgetTester tester,
  ProviderContainer c,
  Widget Function() page,
) async {
  await tester.pumpWidget(
    UncontrolledProviderScope(
      key: UniqueKey(),
      container: c,
      child: MaterialApp(
        home: Consumer(
          builder: (context, ref, _) => PluxLocaleHost(
            state: ref.watch(localeControllerProvider),
            child: Material(child: page()),
          ),
        ),
      ),
    ),
  );
  await tester.pump();
  await tester.pump();
}

String _text(WidgetTester tester, String key) =>
    tester.widget<Text>(find.byKey(Key(key))).data!;

void main() {
  testWidgets('switching from en to ar flips the direction and the texts '
      'without losing page state [I18N-005] [I18N-004]', (tester) async {
    final c = _container(_release());
    await _show(tester, c, _Counter.new);
    expect(_text(tester, 'cancel'), 'Cancel');
    expect(_text(tester, 'direction'), 'TextDirection.ltr');
    await tester.tap(find.text('tap'));
    await tester.tap(find.text('tap'));
    await tester.pump();
    expect(_text(tester, 'taps'), 'taps 2');
    final before = tester.state(find.byType(_Counter));

    c.read(localeControllerProvider.notifier).request(const Locale('ar'));
    await tester.pump();
    await tester.pump();
    await tester.pump();

    expect(_text(tester, 'cancel'), 'الإلغاء');
    expect(_text(tester, 'direction'), 'TextDirection.rtl');
    expect(_text(tester, 'taps'), 'taps 2');
    expect(identical(tester.state(find.byType(_Counter)), before), isTrue);
    expect(find.byType(Directionality).evaluate().isNotEmpty, isTrue);
    expect(
      tester.getTopRight(find.byKey(const Key('cancel'))).dx,
      greaterThan(0),
    );

    c.read(localeControllerProvider.notifier).request(const Locale('en'));
    await tester.pump();
    await tester.pump();
    expect(_text(tester, 'direction'), 'TextDirection.ltr');
    expect(_text(tester, 'taps'), 'taps 2');
  });

  testWidgets('Cupertino strings come from the flutter map too [I18N-004]', (
    tester,
  ) async {
    final c = _container(_release(), host: const Locale('de'));
    await _show(tester, c, _Counter.new);
    expect(_text(tester, 'copy'), 'Kopieren');
  });

  testWidgets('the Material date picker shows German strings and dates from '
      'the flutter map and the engine [I18N-004] [I18N-011]', (tester) async {
    final c = _container(_release(), host: const Locale('de'));
    await _show(
      tester,
      c,
      () => DatePickerDialog(
        initialDate: DateTime(2024, 3, 11),
        firstDate: DateTime(2024),
        lastDate: DateTime(2025),
      ),
    );
    expect(find.text('Abbrechen'), findsOneWidget);
    expect(find.text('OK'), findsOneWidget);
    expect(find.text('Datum auswählen'), findsOneWidget);
    expect(find.text('März 2024'), findsOneWidget);
    expect(find.text('Cancel'), findsNothing);
  });

  testWidgets('numbers are Latin unless the app opts the locale into its '
      'native digits [I18N-011]', (tester) async {
    final latin = _container(_release(), host: const Locale('fa'));
    await _show(tester, latin, _Counter.new);
    expect(_text(tester, 'number'), contains('1'));
    final native = _container(
      _release(nativeDigits: {'fa'}),
      host: const Locale('fa'),
    );
    await _show(tester, native, _Counter.new);
    expect(_text(tester, 'number'), contains('۱'));
  });

  testWidgets('the app calendar is in scope below the host [I18N-011]', (
    tester,
  ) async {
    final c = _container(
      _release(calendar: 'ethiopic'),
      host: const Locale('am', 'ET'),
    );
    await _show(tester, c, _Counter.new);
    expect(_text(tester, 'calendar'), 'ethiopic');
  });

  testWidgets('without a Localizations ancestor the host brings its own '
      '[I18N-004]', (tester) async {
    final c = _container(_release(), host: const Locale('de'));
    late WidgetsLocalizations seen;
    late Locale locale;
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: c,
        child: Consumer(
          builder: (context, ref, _) => PluxLocaleHost(
            state: ref.watch(localeControllerProvider),
            child: Builder(
              builder: (context) {
                seen = WidgetsLocalizations.of(context);
                locale = Localizations.localeOf(context);
                return const SizedBox();
              },
            ),
          ),
        ),
      ),
    );
    await tester.pump();
    await tester.pump();
    expect(locale, const Locale('de'));
    expect(seen, isA<WidgetsLocalizations>());
  });
}
