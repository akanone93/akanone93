// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The effective locale of Plux pages (ADR-0066 § Runtime, I18N-005,
/// I18N-006): the [LocaleController] resolves the locale the host,
/// `PluxConfig.locale`, `Plux.setLocale` or the `setLocale` action asks for
/// against the release's supported locales along the CLDR parent chain, and
/// holds its parsed data. A request for a locale the release lacks is
/// reported (PLX-4400) and changes nothing. The locale data and Flutter
/// strings are parsed once per release and locale: a payload within
/// [LocaleRelease.inlineBytes] at once, reading only the sections a page
/// needs (spawning an isolate costs more than that), a larger one off the UI
/// isolate (L-6).
library;

import 'dart:async';
import 'dart:collection';
import 'dart:convert';
import 'dart:isolate';
import 'dart:typed_data';
import 'dart:ui' show Locale, PlatformDispatcher, TextDirection;

import 'package:flutter/widgets.dart'
    show WidgetsBinding, WidgetsBindingObserver;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/l10n/cldr_index.g.dart';
import 'package:plux_flutter/src/l10n/json_sections.dart';
import 'package:plux_flutter/src/l10n/l10n.dart';
import 'package:plux_flutter/src/l10n/runtime_strings.dart';
import 'package:plux_flutter/src/pxl/vm.dart' show PxlLocaleContext;
import 'package:plux_flutter/src/render/sections.dart';
import 'package:plux_flutter/src/state/providers.dart';
import 'package:plux_flutter/src/verify/bundle_verifier.dart' show SectionGate;

/// The bytes the app bundle carries for one supported locale.
final class LocalePayload {
  /// Creates the payload.
  const LocalePayload({this.data, this.flutter});

  /// Canonical JSON in the shape of `schema/l10n/cldr/<locale>.json`.
  final Uint8List? data;

  /// JSON `{"cupertino":{...},"material":{...},"widgets":{...}}` of
  /// Flutter's ARB keys to the locale's strings, with an optional `plux`
  /// group of the runtime's own strings.
  final Uint8List? flutter;
}

/// What the controller needs of a release.
final class LocaleRelease {
  /// Creates the description.
  const LocaleRelease({
    required this.id,
    required this.defaultLocale,
    required this.supported,
    this.index = cldrLocaleIndex,
    this.payload = _noPayload,
    this.inlineBytes = defaultInlineBytes,
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

  /// The most bytes of a locale's payload that are parsed on the calling
  /// isolate; a larger payload is parsed by the off-isolate runner.
  final int inlineBytes;

  /// The most bytes parsed on the UI isolate by default: the figure above
  /// which layering rule L-6 sends hashing off it ([SectionGate]).
  static const defaultInlineBytes = SectionGate.uiIsolateLimit;

  /// The app's calendar (`localisation.calendar`).
  final String calendar;

  /// The app's secondary calendar (`localisation.secondaryCalendar`).
  final String? secondaryCalendar;

  /// The locales whose native digits the app opts into
  /// (`localisation.numberingSystem`).
  final Set<String> nativeDigits;

  static LocalePayload? _noPayload(String tag) => null;
}

/// What the app's `localisation` settings say (I18N-010, I18N-011): its
/// calendars and the locales that show native digits.
final class AppLocalisation {
  /// Creates the settings.
  const AppLocalisation({
    this.calendar = 'gregorian',
    this.secondaryCalendar,
    this.nativeDigits = const {},
  });

  /// The calendar dates are shown in.
  final String calendar;

  /// A second calendar shown beside the first, or null.
  final String? secondaryCalendar;

  /// The locales whose native numbering system the app opts into.
  final Set<String> nativeDigits;
}

/// The `localisation` settings of the active release's app meta
/// (I18N-010, I18N-011); the Gregorian calendar and Latin digits without
/// a release or where the bundle says nothing.
final appLocalisationProvider = Provider<AppLocalisation>((ref) {
  final r = ref.watch(activeReleaseProvider);
  final l = r?.meta('').localisation;
  if (l == null) return const AppLocalisation();
  final second = l.secondaryCalendar;
  return AppLocalisation(
    calendar: (l.calendar ?? '').isEmpty ? 'gregorian' : l.calendar!,
    secondaryCalendar: second == null || second.isEmpty ? null : second,
    nativeDigits: {
      for (final n in l.numberingSystems ?? const <fbs.NumberingSystemEntry>[])
        if (n.system == 'native' && (n.locale ?? '').isNotEmpty) n.locale!,
    },
  );
}, dependencies: [activeReleaseProvider]);

/// The release's locale description; null until a release is active.
final localeReleaseProvider = Provider<LocaleRelease?>((ref) {
  final r = ref.watch(activeReleaseProvider);
  if (r == null) return null;
  final settings = ref.watch(appLocalisationProvider);
  final view = BundleView(r.bundle(''), r.gate);
  final meta = r.meta('');
  final defaultLocale = meta.defaultLocale ?? 'en';
  // The bundle holds one `l10n` section per locale it can show.
  final supported = <String>{
    ...?meta.supportedLocales,
    defaultLocale,
    ...view.locales,
  };
  return LocaleRelease(
    id: r.record.sequence,
    defaultLocale: defaultLocale,
    supported: supported.toList(),
    payload: (tag) {
      final blobs = view.localeBlobs(tag);
      return blobs == null
          ? null
          : LocalePayload(data: blobs.data, flutter: blobs.flutter);
    },
    calendar: settings.calendar,
    secondaryCalendar: settings.secondaryCalendar,
    nativeDigits: settings.nativeDigits,
  );
}, dependencies: [activeReleaseProvider, appLocalisationProvider]);

/// Runs [work] off the UI isolate; tests replace it.
typedef LocaleOffload = Future<R> Function<R>(R Function() work);

/// The off-isolate runner of the runtime.
final localeOffloadProvider = Provider<LocaleOffload>(
  (ref) => Isolate.run,
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
    this.requested = '',
    this.data,
    this.flutter = const {},
    this.strings = RuntimeStrings.english,
    this.calendar = 'gregorian',
    this.secondaryCalendar,
    this.nativeDigits = false,
    this.loading = false,
  });

  /// The state before a release is known: the host's locale, English
  /// formatting.
  factory PluxLocaleState.host(Locale locale) {
    final tag = locale.toLanguageTag();
    return PluxLocaleState(tag: tag, chain: [tag], loading: true);
  }

  /// Whether the locale's data is still being decoded and there is no
  /// earlier locale to show meanwhile: pages wait rather than render in the
  /// wrong formats.
  final bool loading;

  /// The resolved locale: a supported one of the release.
  final String tag;

  /// The locale as requested (`de-AT` for a request resolved to `de`);
  /// empty means [tag]. Its region is the default region of phone numbers.
  final String requested;

  /// The tag the locale was requested with, or [tag].
  String get requestedTag => requested.isEmpty ? tag : requested;

  /// The locales to look a missing message up in, [tag] first and the app
  /// default last.
  final List<String> chain;

  /// The CLDR data of [tag]; null formats with the built-in fallback.
  final PluxLocaleData? data;

  /// Flutter's ARB keys for [tag] by group (`cupertino`, `material`,
  /// `widgets`) and the runtime's own strings (`plux`).
  final Map<String, Map<String, String>> flutter;

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
  final RuntimeStrings strings;

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
  const _Parsed(this.data, this.flutter, this.strings);
  final PluxLocaleData? data;
  final Map<String, Map<String, String>> flutter;
  final RuntimeStrings strings;
}

const _noStrings = _Parsed(null, {}, RuntimeStrings.english);

/// What the off-isolate step hands back: plain maps and strings, which can
/// cross isolates (a [PluxLocaleData] cannot).
typedef _Decoded = ({
  Map<String, Object?>? data,
  Map<String, Map<String, String>> flutter,
});

/// The string entries of a decoded group; anything else is left out.
Map<String, String> _stringsOf(Object? group) => {
  if (group is Map<String, Object?>)
    for (final e in group.entries)
      if (e.value is String) e.key: e.value! as String,
};

/// Flutter's ARB keys by group, from the bytes of the `flutter` blob.
Map<String, Map<String, String>> _groups(Uint8List? flutter) {
  if (flutter == null || flutter.isEmpty) return {};
  final json = jsonDecode(utf8.decode(flutter)) as Map<String, Object?>;
  return {
    for (final g in json.entries)
      if (g.value is Map<String, Object?>) g.key: _stringsOf(g.value),
  };
}

/// The same as [_groups], with each group decoded when its first key is
/// read: most pages read none (Material and Cupertino strings are for
/// pickers, tooltips and context menus), so start-up neither spends time
/// nor holds memory on them. Throws a [FormatException] when the blob is
/// not a JSON object.
Map<String, Map<String, String>> _lazyGroups(Uint8List? flutter) =>
    flutter == null || flutter.isEmpty
    ? const {}
    : _LazyGroups(JsonSections.scan(flutter));

/// A map of strings decoded on first use. A group that cannot be decoded
/// reads as empty, so its strings are Flutter's English ones; the compiler
/// writes the blob and its section is hash-checked before it is read.
final class _LazyStrings extends UnmodifiableMapBase<String, String> {
  _LazyStrings(this._load);

  final Map<String, String> Function() _load;

  late final Map<String, String> _map = () {
    try {
      return _load();
    } on Object {
      return const <String, String>{};
    }
  }();

  @override
  String? operator [](Object? key) => _map[key];

  @override
  Iterable<String> get keys => _map.keys;

  @override
  bool containsKey(Object? key) => _map.containsKey(key);

  @override
  int get length => _map.length;
}

/// Flutter's groups over a blob's sections, each a [_LazyStrings].
final class _LazyGroups
    extends UnmodifiableMapBase<String, Map<String, String>> {
  _LazyGroups(this._sections);

  final JsonSections _sections;
  final Map<String, Map<String, String>> _groups = {};

  @override
  Map<String, String>? operator [](Object? key) {
    if (key is! String || !containsKey(key)) return null;
    return _groups[key] ??= _LazyStrings(() => _stringsOf(_sections[key]));
  }

  @override
  Iterable<String> get keys => _sections.keys;

  @override
  bool containsKey(Object? key) => key is String && keys.contains(key);
}

/// Decodes a locale's bytes; runs off the UI isolate.
_Decoded _decode(Uint8List? data, Uint8List? flutter) => (
  data: data == null || data.isEmpty
      ? null
      : jsonDecode(utf8.decode(data)) as Map<String, Object?>,
  flutter: _groups(flutter),
);

/// Runs [_decode] on [offload]. The closure is made here, in a scope that
/// holds nothing but the bytes, because a closure sent to another isolate
/// takes its whole scope along.
Future<_Decoded> _decodeOff(
  LocaleOffload offload,
  Uint8List? data,
  Uint8List? flutter,
) => offload(() => _decode(data, flutter));

/// Builds the locale from what [_decode] returned; throws an
/// [L10nException] when the data is incomplete.
_Parsed _build(_Decoded d) => _parsedOf(
  d.data == null ? null : PluxLocaleData.fromJson(d.data!),
  d.flutter,
);

/// Parses a payload on this isolate; throws an [L10nException] when the
/// data is incomplete.
_Parsed _buildNow(LocalePayload payload) {
  final data = payload.data;
  return _parsedOf(
    data == null || data.isEmpty ? null : PluxLocaleData.fromBytes(data),
    _lazyGroups(payload.flutter),
  );
}

_Parsed _parsedOf(
  PluxLocaleData? data,
  Map<String, Map<String, String>> groups,
) {
  final plux = groups['plux'];
  return _Parsed(
    data,
    groups,
    plux == null
        ? RuntimeStrings.english
        : RuntimeStrings(
            _LazyStrings(
              () => {
                for (final e in plux.entries)
                  e.key.startsWith('plux.') ? e.key : 'plux.${e.key}': e.value,
              },
            ),
          ),
  );
}

/// The widgets binding, or null where none is initialised (a pure-Dart
/// test of the controller).
WidgetsBinding? _binding() {
  try {
    return WidgetsBinding.instance;
  } on Object {
    return null;
  }
}

/// Tells the controller that the host's locales changed.
final class _HostLocaleObserver with WidgetsBindingObserver {
  _HostLocaleObserver(this.changed);

  final void Function() changed;

  @override
  void didChangeLocales(List<Locale>? locales) => changed();
}

/// Holds the effective locale of Plux pages.
final class LocaleController extends Notifier<PluxLocaleState> {
  final Map<(Object, String), _Parsed> _parsed = {};
  final Map<(Object, String), Future<void>> _loading = {};
  final Set<String> _missingReported = {};
  PluxLocaleState? _last;

  @override
  PluxLocaleState build() {
    final env = ref.watch(environmentProvider);
    final release = ref.watch(localeReleaseProvider);
    final binding = _binding();
    final dispatcher =
        binding?.platformDispatcher ?? PlatformDispatcher.instance;
    if (env.locale == null && binding != null) {
      // A locale that follows the host follows its changes (I18N-005).
      final observer = _HostLocaleObserver(ref.invalidateSelf);
      binding.addObserver(observer);
      ref.onDispose(() => binding.removeObserver(observer));
    }
    final requested =
        env.locale ?? dispatcher.locales.firstOrNull ?? dispatcher.locale;
    if (release == null) {
      return _last = PluxLocaleState.host(requested);
    }
    final tag = _resolve(release, requested.toLanguageTag());
    final key = (release.id, tag);
    _parsed.removeWhere((k, _) => k.$1 != release.id);
    final want = requested.toLanguageTag();
    var parsed = _parsed[key];
    if (parsed == null) {
      final payload = release.payload(tag);
      if (payload?.data == null && payload?.flutter == null) {
        // Nothing to decode: the locale is ready at once.
        parsed = _parsed[key] = _noStrings;
      } else if (payload != null && _size(payload) <= release.inlineBytes) {
        parsed = _parsed[key] = _parseNow(tag, payload);
      }
    }
    if (parsed == null) {
      _load(release, tag, key);
      // Until the data is decoded the previous locale's stays on screen.
      final last = _last;
      if (last != null && !last.loading) return last;
      return _last = _state(
        release,
        tag,
        _noStrings,
        requested: want,
        loading: true,
      );
    }
    return _last = _state(release, tag, parsed, requested: want);
  }

  /// Completes when every locale being decoded is ready; the first page of
  /// a release is built after it.
  Future<void> settled() async {
    while (_loading.isNotEmpty) {
      await Future.wait(_loading.values.toList());
    }
  }

  static int _size(LocalePayload p) =>
      (p.data?.length ?? 0) + (p.flutter?.length ?? 0);

  /// Parses [payload] now; invalid data is reported (after this build) and
  /// leaves the locale without data.
  _Parsed _parseNow(String tag, LocalePayload payload) {
    try {
      return _buildNow(payload);
    } on Object catch (e) {
      final error = PluxException(
        PluxErrorCode.messageFormatFailed,
        'locale data of $tag is invalid: $e',
      );
      scheduleMicrotask(() {
        if (ref.mounted) _report(error);
      });
      return _noStrings;
    }
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

  PluxLocaleState _state(
    LocaleRelease release,
    String tag,
    _Parsed p, {
    String requested = '',
    bool loading = false,
  }) {
    final chain = <String>[
      ...release.index.chain(tag),
      if (!release.index.chain(tag).contains(release.defaultLocale))
        release.defaultLocale,
    ];
    return PluxLocaleState(
      tag: tag,
      chain: chain,
      requested: requested,
      data: p.data,
      flutter: p.flutter,
      strings: p.strings,
      calendar: release.calendar,
      secondaryCalendar: release.secondaryCalendar,
      nativeDigits: release.nativeDigits.contains(tag),
      loading: loading,
    );
  }

  void _load(LocaleRelease release, String tag, (Object, String) key) {
    if (_loading.containsKey(key)) return;
    final payload = release.payload(tag);
    final offload = ref.read(localeOffloadProvider);
    final data = payload?.data, flutter = payload?.flutter;
    _loading[key] = (() async {
      _Parsed parsed;
      try {
        parsed = _build(await _decodeOff(offload, data, flutter));
      } on Object catch (e) {
        _report(
          PluxException(
            PluxErrorCode.messageFormatFailed,
            'locale data of $tag is invalid: $e',
          ),
        );
        parsed = _noStrings;
      }
      unawaited(_loading.remove(key));
      _parsed[key] = parsed;
      // The build finds the parsed locale in the cache, and decides which
      // locale is the effective one now.
      if (ref.mounted) ref.invalidateSelf();
    })();
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
    ref.read(localeSinkProvider).record('l10n_missing', {
      'key': key,
      'requested': requested,
      'resolved': resolved,
    });
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
