// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:plux_flutter/src/bundle/container.dart';
import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/l10n/locale_controller.dart';
import 'package:plux_flutter/src/render/sections.dart' show uuidOf;
import 'package:plux_flutter/src/render/values.dart' show uuidString;

const _cldr = '../../schema/l10n/cldr';
const _flutter = '../../schema/l10n/flutter';

/// The locale data and Flutter strings of [tag], as the compiler puts them
/// in the bundle: read from `schema/l10n`. [plux] adds a `plux` group of
/// the runtime's own strings.
LocalePayload localePayload(String tag, {Map<String, String>? plux}) {
  final data = File('$_cldr/$tag.json').readAsBytesSync();
  final flutterFile = File('$_flutter/$tag.json');
  final groups = flutterFile.existsSync()
      ? jsonDecode(flutterFile.readAsStringSync()) as Map<String, Object?>
      : <String, Object?>{};
  if (plux != null) groups['plux'] = plux;
  return LocalePayload(
    data: Uint8List.fromList(data),
    flutter: Uint8List.fromList(utf8.encode(jsonEncode(groups))),
  );
}

/// [app] with its `l10n` sections replaced by one section per locale of
/// [texts] (message id to text) and for `en` the app's own messages, each
/// with the locale data and Flutter strings of `schema/l10n`. A locale
/// carries the messages [texts] names, so a message left out falls back.
Uint8List withLocales(
  Uint8List app,
  Map<String, Map<String, String>> texts, {
  Map<String, Map<String, String>> plux = const {},
  Set<String> verbatim = const {},
}) {
  final b = BundleContainer.parse(app);
  final english = [
    for (final s in b.ofKind(SectionKind.l10n))
      if (String.fromCharCodes(s.id.takeWhile((x) => x != 0)) == 'en')
        ...?fbs.Locale(s.data).messages,
  ];
  Section section(String tag, List<fbs.Message> from, Map<String, String>? by) {
    final payload = localePayload(tag, plux: plux[tag]);
    final messages = [
      for (final m in from)
        if (by == null || by.containsKey(uuidString(uuidOf(m.key!))))
          fbs.MessageObjectBuilder(
            key: fbs.UuidObjectBuilder(hi: m.key!.hi, lo: m.key!.lo),
            text: by == null ? m.text : by[uuidString(uuidOf(m.key!))],
            ast: by == null ? m.ast : null,
          ),
    ];
    final data = fbs.LocaleObjectBuilder(
      tag: tag,
      messages: messages,
      data: payload.data,
      flutter: payload.flutter,
      tagResolved: tag,
    ).toBytes('PXLN');
    return Section(
      SectionKind.l10n,
      Uint8List(16)..setAll(0, tag.codeUnits),
      Uint8List(32),
      data,
    );
  }

  return encodeBundle(b.kind, [
    for (final s in b.sections)
      if (s.kind != SectionKind.l10n) s,
    section('en', english, null),
    for (final t in verbatim) section(t, english, null),
    for (final e in texts.entries) section(e.key, english, e.value),
  ]);
}
