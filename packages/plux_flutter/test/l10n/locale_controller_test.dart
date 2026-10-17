// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:isolate';
import 'dart:math';
import 'dart:ui';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/src/assets/assets.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/core/config.dart' show PluxConsent;
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/l10n/calendar/calendar.dart';
import 'package:plux_flutter/src/l10n/cldr_index.g.dart';
import 'package:plux_flutter/src/l10n/l10n.dart';
import 'package:plux_flutter/src/l10n/locale_controller.dart';
import 'package:plux_flutter/src/state/providers.dart';
import 'package:plux_flutter/src/telemetry/recorder.dart';

import '../support/locale_bundle.dart';

Future<R> _inline<R>(R Function() work) async => work();

/// What the controller reported.
final class _Seen {
  final List<PluxException> errors = [];
  final List<(String, Map<String, Object?>)> events = [];
}

LocaleRelease _release({
  List<String> supported = const ['en', 'de', 'ar', 'am'],
  String calendar = 'gregorian',
  String? secondaryCalendar,
  Set<String> nativeDigits = const {},
  Map<String, String>? dePlux,
}) {
  final payloads = {
    for (final t in supported)
      t: localePayload(t, plux: t == 'de' ? dePlux : null),
  };
  return LocaleRelease(
    id: 1,
    defaultLocale: 'en',
    supported: supported,
    index: cldrLocaleIndex,
    payload: (tag) => payloads[tag],
    calendar: calendar,
    secondaryCalendar: secondaryCalendar,
    nativeDigits: nativeDigits,
  );
}

ProviderContainer _container(
  LocaleRelease? release,
  _Seen seen, {
  LocaleOffload offload = _inline,
  Locale? host,
  TelemetryRecorder? recorder,
}) {
  final c = ProviderContainer(
    overrides: [
      localeReleaseProvider.overrideWithValue(release),
      localeOffloadProvider.overrideWithValue(offload),
      localeSinkProvider.overrideWithValue(
        LocaleSink(
          report: seen.errors.add,
          record: (name, fields) {
            seen.events.add((name, fields));
            recorder?.record(name, fields: fields);
          },
        ),
      ),
    ],
  );
  addTearDown(c.dispose);
  if (host != null) {
    c.read(environmentProvider.notifier).replace(PluxEnvironment(locale: host));
  }
  return c;
}

Future<PluxLocaleState> _settled(ProviderContainer c) async {
  c.listen(localeControllerProvider, (_, _) {});
  await pumpEventQueue();
  return c.read(localeControllerProvider);
}

void main() {
  test('the requested locale resolves along the CLDR parent chain to a '
      'supported one and its data is parsed [I18N-005] [I18N-003]', () async {
    final c = _container(_release(), _Seen(), host: const Locale('de', 'AT'));
    final s = await _settled(c);
    expect(s.tag, 'de');
    expect(s.chain, ['de', 'en']);
    expect(s.direction, TextDirection.ltr);
    expect(s.data, isNotNull);
    expect(s.flutter['material']!['cancelButtonLabel'], 'Abbrechen');
  });

  test('a host locale the release lacks falls back to the app default '
      '[I18N-005]', () async {
    final c = _container(_release(), _Seen(), host: const Locale('fr'));
    final s = await _settled(c);
    expect(s.tag, 'en');
    expect(s.chain, ['en']);
  });

  test('a locale the release has no translations for is reported and '
      'changes nothing [I18N-005] [PLX-4400]', () async {
    final seen = _Seen();
    final c = _container(_release(), seen, host: const Locale('de'));
    expect((await _settled(c)).tag, 'de');
    final ctl = c.read(localeControllerProvider.notifier);
    expect(ctl.request(const Locale('fr')), isFalse);
    await pumpEventQueue();
    expect(c.read(localeControllerProvider).tag, 'de');
    expect(c.read(environmentProvider).locale, const Locale('de'));
    expect(seen.errors.map((e) => e.code), [PluxErrorCode.localeNotInRelease]);
    expect(seen.errors.single.details['locale'], 'fr');
    expect(ctl.request(const Locale('ar')), isTrue);
    expect((await _settled(c)).tag, 'ar');
    expect(ctl.request(null), isTrue);
    expect(c.read(environmentProvider).locale, isNull);
  });

  test('locale data is parsed once per release and locale, by the '
      'off-isolate runner [I18N-005] [I18N-011]', () async {
    var runs = 0;
    Future<R> counting<R>(R Function() work) async {
      runs++;
      return work();
    }

    final c = _container(
      _release(),
      _Seen(),
      offload: counting,
      host: const Locale('en'),
    );
    expect((await _settled(c)).tag, 'en');
    expect(runs, 1);
    final ctl = c.read(localeControllerProvider.notifier)
      ..request(const Locale('de'));
    expect((await _settled(c)).tag, 'de');
    expect(runs, 2);
    ctl
      ..request(const Locale('en'))
      ..request(const Locale('de'));
    expect((await _settled(c)).tag, 'de');
    expect(runs, 2, reason: 'en and de are cached');
  });

  test('the bytes parse in a real isolate [I18N-005]', () async {
    final c = _container(
      _release(),
      _Seen(),
      offload: Isolate.run,
      host: const Locale('ar'),
    );
    c.listen(localeControllerProvider, (_, _) {});
    for (var i = 0; i < 200; i++) {
      if (c.read(localeControllerProvider).data != null) break;
      await Future<void>.delayed(const Duration(milliseconds: 25));
    }
    final s = c.read(localeControllerProvider);
    expect(s.tag, 'ar');
    expect(s.data, isNotNull);
    expect(s.direction, TextDirection.rtl);
  });

  test('German numbers and dates are formatted from the locale data '
      '[I18N-011]', () async {
    final c = _container(_release(), _Seen(), host: const Locale('de'));
    final data = (await _settled(c)).data!;
    expect(data.formatNumber(PluxDecimal.parse('1234.5')), '1.234,5');
    final jdn = calendarSystem(gregorianId)!
        .toJdn(const CalendarYmd(2024, 3, 11));
    expect(
      data.formatDate(
        pluxCalendarDate(calendarSystem(gregorianId)!, jdn),
        gregorianId,
        const PluxDateOptions(dateStyle: 'full'),
      ),
      'Montag, 11. März 2024',
    );
  });

  test('the app calendar reaches the state, and am-ET shows the Ethiopian '
      'date [I18N-010] [I18N-011]', () async {
    final c = _container(
      _release(calendar: 'ethiopic', secondaryCalendar: 'gregorian'),
      _Seen(),
      host: const Locale('am', 'ET'),
    );
    final s = await _settled(c);
    expect(s.tag, 'am');
    expect(s.calendar, 'ethiopic');
    expect(s.secondaryCalendar, 'gregorian');
    final gregorian = calendarSystem(gregorianId)!;
    final jdn = gregorian.toJdn(const CalendarYmd(2024, 3, 11));
    final ethiopic = calendarSystem(s.calendar)!;
    final text = s.data!.formatDate(
      pluxCalendarDate(ethiopic, jdn),
      s.calendar,
      const PluxDateOptions(dateStyle: 'long'),
    );
    // 2 Megabit 2016 in the Ethiopian calendar.
    expect(text, contains('መጋቢት'));
    expect(text, contains('2016'));
  });

  test(
    'digits are Latin unless the app opts the locale in [I18N-011]',
    () async {
      final latin = _container(_release(), _Seen(), host: const Locale('ar'));
      expect((await _settled(latin)).nativeDigits, isFalse);
      final native = _container(
        _release(nativeDigits: {'ar'}),
        _Seen(),
        host: const Locale('ar'),
      );
      expect((await _settled(native)).nativeDigits, isTrue);
    },
  );

  test('the runtime strings are English unless the locale brings a plux '
      'group [I18N-004] [ACT-020]', () async {
    final c = _container(
      _release(dePlux: {'fallback.unavailable': 'Inhalt nicht verfügbar'}),
      _Seen(),
      host: const Locale('de'),
    );
    final de = await _settled(c);
    expect(de.strings['plux.fallback.unavailable'], 'Inhalt nicht verfügbar');
    expect(de.strings['plux.router.notFound'], 'Page not found');
    c.read(localeControllerProvider.notifier).request(const Locale('en'));
    expect(
      (await _settled(c)).strings['plux.fallback.unavailable'],
      'Content unavailable',
    );
  });

  test('a message supplied by a later locale of the chain is reported '
      'once as PLX-4401 and sampled as l10n_missing [I18N-006]', () async {
    final seen = _Seen();
    final posted = <String>[];
    final recorder = TelemetryRecorder(
      post: posted.addAll,
      withdraw: (_) {},
      consent: const PluxConsent(analytics: true),
      hostSampling: const {},
      random: Random(1),
      clock: () => DateTime.utc(2026),
    );
    final c = _container(
      _release(),
      seen,
      host: const Locale('de'),
      recorder: recorder,
    );
    await _settled(c);
    final ctl = c.read(localeControllerProvider.notifier)
      ..missing('k1', 'de', 'en')
      ..missing('k1', 'de', 'en')
      ..missing('k2', 'de', 'en');
    expect(seen.errors.map((e) => e.code), [
      PluxErrorCode.translationFallback,
      PluxErrorCode.translationFallback,
    ]);
    expect(seen.events.map((e) => e.$1), ['l10n_missing', 'l10n_missing']);
    expect(seen.events.first.$2, {
      'key': 'k1',
      'requested': 'de',
      'resolved': 'en',
    });
    expect(posted, hasLength(2));
    expect(posted.first, contains('l10n_missing'));
    // The app's sampling rate applies to the event (ANL-003).
    recorder.appSampling = const {'l10n_missing': 0.0};
    ctl.missing('k3', 'de', 'en');
    expect(posted, hasLength(2));
  });

  test('the embedded parent table is the one of schema/l10n/cldr/index.json '
      '[I18N-003]', () {
    final json = jsonDecode(
      File('../../schema/l10n/cldr/index.json').readAsStringSync(),
    ) as Map<String, Object?>;
    expect(
      cldrParentLocales,
      (json['parentLocales']! as Map<String, Object?>).cast<String, String>(),
    );
    expect(
      cldrLikelyScripts,
      (json['likelyScripts']! as Map<String, Object?>).cast<String, String>(),
    );
  });

  group('a file per locale [I18N-012]', () {
    List<int> h(String s) => List<int>.generate(32, (i) => s.codeUnitAt(0) + i);
    String hex(String s) =>
        h(s).map((b) => b.toRadixString(16).padLeft(2, '0')).join();

    fbs.Asset asset() {
      final bytes = fbs.AssetIndexObjectBuilder(
        assets: [
          fbs.AssetObjectBuilder(
            id: fbs.UuidObjectBuilder(hi: 1, lo: 2),
            key: 'banner.png',
            mediaType: 'image/png',
            hash: h('a'),
            size: 7,
            localeFiles: [
              fbs.AssetLocaleFileObjectBuilder(
                tag: 'ar',
                hash: h('b'),
                size: 11,
              ),
              fbs.AssetLocaleFileObjectBuilder(
                tag: 'de',
                hash: h('c'),
                size: 13,
              ),
            ],
          ),
        ],
      ).toBytes();
      return fbs.AssetIndex(bytes).assets!.single;
    }

    test('the file of the effective locale, or of the nearest on its chain, '
        'comes before the default file', () {
      final a = asset();
      expect(preferredFiles(a, AssetDevice.plain), [hex('a')]);
      expect(preferredFiles(a, AssetDevice.plain, locales: ['de', 'en']), [
        hex('c'),
        hex('a'),
      ]);
      expect(
        preferredFiles(a, AssetDevice.plain, locales: ['de-AT', 'de', 'en']),
        [hex('c'), hex('a')],
      );
      expect(preferredFiles(a, AssetDevice.plain, locales: ['ar', 'en']), [
        hex('b'),
        hex('a'),
      ]);
      expect(preferredFiles(a, AssetDevice.plain, locales: ['en']), [
        hex('a'),
      ], reason: 'a locale without a file shows the default one');
      expect(fileSize(a, hex('c')), 13);
    });
  });
}
