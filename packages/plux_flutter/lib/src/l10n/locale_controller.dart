// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The effective locale of Plux pages (ADR-0066 § Runtime, I18N-005,
/// I18N-006): the [LocaleController] resolves the locale the host,
/// `PluxConfig.locale`, `Plux.setLocale` or the `setLocale` action asks for
/// against the release's supported locales along the CLDR parent chain, and
/// holds its parsed data. A request for a locale the release lacks is
/// reported (PLX-4400) and changes nothing. The locale data and Flutter
/// strings are parsed once per release and locale, off the UI isolate.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:isolate';
import 'dart:typed_data';
import 'dart:ui' show Locale, PlatformDispatcher, TextDirection;

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/l10n/l10n.dart';
import 'package:plux_flutter/src/l10n/runtime_strings.dart';
import 'package:plux_flutter/src/pxl/vm.dart' show PxlLocaleContext;
import 'package:plux_flutter/src/state/providers.dart';

/// The bytes the app bundle carries for one supported locale.
final class LocalePayload {
  /// Creates the payload.
  const LocalePayload({this.data, this.flutter});

  /// Canonical JSON in the shape of `schema/l10n/cldr/<locale>.json`.
  final Uint8List? data;

  /// JSON map of Flutter's Material, Cupertino and Widgets ARB keys (and
  /// the `plux.` runtime keys) to the locale's strings.
  final Uint8List? flutter;
}

/// What the controller needs of a release.
final class LocaleRelease {
  /// Creates the description.
  const LocaleRelease({
    required this.id,
    required this.defaultLocale,
    required this.supported,
    this.index = const PluxLocaleIndex(),
    this.payload = _noPayload,
    this.calendar = 'gregorian',
    this.secondaryCalendar,
    this.nativeDigits = const {},
  });

  /// Tells releases apart for the parse cache.
  final Object id;

  /// The app's default locale: the end of every fallback chain.
  final String defaultLocale;

  /// The locales the release has translations for.
  final List<String> supported;

  /// The CLDR parent locales.
  final PluxLocaleIndex index;

  /// The payload of a supported locale, or null.
  final LocalePayload? Function(String tag) payload;

  /// The app's calendar (`localisation.calendar`).
  final String calendar;

  /// The app's secondary calendar (`localisation.secondaryCalendar`).
  final String? secondaryCalendar;

  /// The locales whose native digits the app opts into
  /// (`localisation.numberingSystem`).
  final Set<String> nativeDigits;

  static LocalePayload? _noPayload(String tag) => null;
}

/// The release's locale description; null until a release is active.
final localeReleaseProvider = Provider<LocaleRelease?>((ref) {
  final r = ref.watch(activeReleaseProvider);
  if (r == null) return null;
  final meta = r.meta('');
  final defaultLocale = meta.defaultLocale ?? 'en';
  return LocaleRelease(
    id: r.record.sequence,
    defaultLocale: defaultLocale,
    supported: [
      ...?meta.supportedLocales,
      if (!(meta.supportedLocales ?? const <String>[]).contains(defaultLocale))
        defaultLocale,
    ],
  );
}, dependencies: [activeReleaseProvider]);

/// Runs [work] off the UI isolate; tests replace it.
typedef LocaleOffload = Future<R> Function<R>(R Function() work);

/// The off-isolate runner of the runtime.
final localeOffloadProvider = Provider<LocaleOffload>(
  (ref) =>
      <R>(R Function() work) => Isolate.run(work),
  dependencies: const [],
);

/// Where the controller reports problems and telemetry.
final class LocaleSink {
  /// Creates the sink.
  const LocaleSink({required this.report, required this.record});

  /// Reports a problem (PLX-4400, PLX-4401).
  final void Function(PluxException error) report;

  /// Records a telemetry event of [name] with [fields].
  final void Function(String name, Map<String, Object?> fields) record;
}

/// The runtime's reporting and telemetry; nothing before it exists.
final localeSinkProvider = Provider<LocaleSink>((ref) {
  final rt = ref.watch(pluxRuntimeProvider);
  return LocaleSink(
    report: (e) => rt?.reportProblem(e),
    record: (name, fields) => rt?.telemetry.record(name, fields: fields),
  );
}, dependencies: [pluxRuntimeProvider]);

/// Everything the widgets below a page read about the locale.
final class PluxLocaleState {
  /// Creates the state.
  const PluxLocaleState({
    required this.tag,
    required this.chain,
    this.data,
    this.flutter = const {},
    this.calendar = 'gregorian',
    this.secondaryCalendar,
    this.nativeDigits = false,
  });

  /// The state before a release is known: the host's locale, English
  /// formatting.
  factory PluxLocaleState.host(Locale locale) {
    final tag = locale.toLanguageTag();
    return PluxLocaleState(tag: tag, chain: [tag]);
  }

  /// The resolved locale: a supported one of the release.
  final String tag;

  /// The locales to look a missing message up in, [tag] first and the app
  /// default last.
  final List<String> chain;

  /// The CLDR data of [tag]; null formats with the built-in fallback.
  final PluxLocaleData? data;

  /// Flutter's ARB keys and the `plux.` runtime keys for [tag].
  final Map<String, String> flutter;

  /// The app's calendar.
  final String calendar;

  /// The app's secondary calendar.
  final String? secondaryCalendar;

  /// Whether digits are the locale's native ones.
  final bool nativeDigits;

  /// The locale as Flutter's `Locale`.
  Locale get locale => localeOfTag(tag);

  /// The reading direction.
  TextDirection get direction => (data?.isRtl ?? _rtlLanguage(tag))
      ? TextDirection.rtl
      : TextDirection.ltr;

  /// The runtime's own strings in this locale.
  RuntimeStrings get strings => RuntimeStrings(flutter);

  /// The context PXL's `format` and `l10n` groups use, or null without
  /// data.
  PxlLocaleContext? get pxl {
    final d = data;
    return d == null ? null : PxlLocaleContext(d);
  }
}

/// Languages written right to left, for the time before data is loaded.
bool _rtlLanguage(String tag) => const {
  'ar',
  'he',
  'fa',
  'ur',
  'ps',
  'sd',
  'ug',
  'yi',
  'dv',
}.contains(tag.split('-').first);

/// The `Locale` of a BCP 47 [tag].
Locale localeOfTag(String tag) {
  final parts = tag.split('-');
  String? script, region;
  for (final p in parts.skip(1)) {
    if (p.length == 4) {
      script = p;
    } else if (p.length == 2 || (p.length == 3 && int.tryParse(p) != null)) {
      region = p;
    }
  }
  return Locale.fromSubtags(
    languageCode: parts.first,
    scriptCode: script,
    countryCode: region,
  );
}

final class _Parsed {
  const _Parsed(this.data, this.flutter);
  final PluxLocaleData? data;
  final Map<String, String> flutter;
}

_Parsed _parse(Uint8List? data, Uint8List? flutter) {
  PluxLocaleData? d;
  if (data != null && data.isNotEmpty) {
    d = PluxLocaleData.fromJson(
      jsonDecode(utf8.decode(data)) as Map<String, Object?>,
    );
  }
  final map = <String, String>{};
  if (flutter != null && flutter.isNotEmpty) {
    final json = jsonDecode(utf8.decode(flutter)) as Map<String, Object?>;
    for (final e in json.entries) {
      final v = e.value;
      if (v is String) map[e.key] = v;
    }
  }
  return _Parsed(d, map);
}

/// Holds the effective locale of Plux pages.
final class LocaleController extends Notifier<PluxLocaleState> {
  final Map<(Object, String), _Parsed> _parsed = {};
  final Set<(Object, String)> _loading = {};
  final Set<String> _missingReported = {};
  PluxLocaleState? _last;

  @override
  PluxLocaleState build() {
    final env = ref.watch(environmentProvider);
    final release = ref.watch(localeReleaseProvider);
    final requested = env.locale ?? PlatformDispatcher.instance.locale;
    if (release == null) {
      return _last = PluxLocaleState.host(requested);
    }
    final tag = _resolve(release, requested.toLanguageTag());
    final key = (release.id, tag);
    final parsed = _parsed[key];
    if (parsed == null) {
      _load(release, tag, key);
      // Until the data is parsed the previous locale's stays on screen.
      return _last ?? _state(release, tag, const _Parsed(null, {}));
    }
    return _last = _state(release, tag, parsed);
  }

  String _resolve(LocaleRelease release, String tag) {
    try {
      return release.index.resolve(
        tag,
        release.supported,
        release.defaultLocale,
      );
    } on L10nException {
      return release.defaultLocale;
    }
  }

  PluxLocaleState _state(LocaleRelease release, String tag, _Parsed p) {
    final chain = <String>[
      ...release.index.chain(tag),
      if (!release.index.chain(tag).contains(release.defaultLocale))
        release.defaultLocale,
    ];
    return PluxLocaleState(
      tag: tag,
      chain: chain,
      data: p.data,
      flutter: p.flutter,
      calendar: release.calendar,
      secondaryCalendar: release.secondaryCalendar,
      nativeDigits: release.nativeDigits.contains(tag),
    );
  }

  void _load(LocaleRelease release, String tag, (Object, String) key) {
    if (!_loading.add(key)) return;
    final payload = release.payload(tag);
    final offload = ref.read(localeOffloadProvider);
    final data = payload?.data, flutter = payload?.flutter;
    unawaited(() async {
      _Parsed parsed;
      try {
        parsed = await offload(() => _parse(data, flutter));
      } on Object catch (e) {
        _report(
          PluxException(
            PluxErrorCode.messageFormatFailed,
            'locale data of $tag is invalid: $e',
          ),
        );
        parsed = const _Parsed(null, {});
      }
      _loading.remove(key);
      _parsed[key] = parsed;
      if (!ref.mounted) return;
      final current = ref.read(localeReleaseProvider);
      if (current?.id != release.id) return;
      state = _last = _state(release, tag, parsed);
    }());
  }

  /// Asks for [locale]; null follows the host. A locale the release has no
  /// translations for is reported (PLX-4400) and changes nothing. Returns
  /// whether the request took effect.
  bool request(Locale? locale) {
    final release = ref.read(localeReleaseProvider);
    if (locale != null && release != null) {
      try {
        release.index.resolve(locale.toLanguageTag(), release.supported, '');
      } on L10nException {
        _report(
          PluxException(
            PluxErrorCode.localeNotInRelease,
            'the release has no locale ${locale.toLanguageTag()}',
            details: {'locale': locale.toLanguageTag()},
          ),
        );
        return false;
      }
    }
    final env = ref.read(environmentProvider);
    ref
        .read(environmentProvider.notifier)
        .replace(
          PluxEnvironment(
            locale: locale,
            themeMode: env.themeMode,
            brand: env.brand,
            consent: env.consent,
            user: env.user,
            authDelegate: env.authDelegate,
          ),
        );
    return true;
  }

  /// Reports that message [key] was not in the requested locale and
  /// another locale of the chain supplied it (PLX-4401), once per key and
  /// locale, with the sampled `l10n_missing` event.
  void missing(String key, String requested, String resolved) {
    if (!_missingReported.add('$requested\u0000$key')) return;
    _report(
      PluxException(
        PluxErrorCode.translationFallback,
        'message $key is missing in $requested; $resolved is shown',
        details: {'key': key, 'requested': requested, 'resolved': resolved},
      ),
    );
    ref
        .read(pluxRuntimeProvider)
        ?.telemetry
        .record(
          'l10n_missing',
          fields: {'key': key, 'requested': requested, 'resolved': resolved},
        );
  }

  void _report(PluxException e) => ref.read(localeSinkProvider).report(e);
}

/// The effective locale of Plux pages.
final localeControllerProvider =
    NotifierProvider<LocaleController, PluxLocaleState>(
      LocaleController.new,
      dependencies: [
        environmentProvider,
        localeReleaseProvider,
        localeOffloadProvider,
        localeSinkProvider,
      ],
    );
