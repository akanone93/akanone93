// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:plux_flutter/src/l10n/errors.dart';

/// The deprecated language subtags BCP 47 canonicalisation replaces.
const _languageAliases = {
  'iw': 'he',
  'in': 'id',
  'ji': 'yi',
  'jw': 'jv',
  'mo': 'ro',
  'tl': 'fil',
  'nb': 'no',
};

/// What locale resolution needs from `schema/l10n/cldr/index.json`: CLDR's
/// parent locales and the likely scripts of language-region pairs.
final class PluxLocaleIndex {
  /// Creates an index from its two maps.
  const PluxLocaleIndex({
    this.parentLocales = const {},
    this.likelyScripts = const {},
  });

  /// Reads the `parentLocales` and `likelyScripts` members of the decoded
  /// `index.json`.
  factory PluxLocaleIndex.fromJson(Map<String, Object?> json) {
    Map<String, String> strings(String key) => {
      for (final e
          in ((json[key] as Map<String, Object?>?) ?? const {}).entries)
        e.key: e.value! as String,
    };
    return PluxLocaleIndex(
      parentLocales: strings('parentLocales'),
      likelyScripts: strings('likelyScripts'),
    );
  }

  /// A locale to its CLDR parent; `root` ends the chain.
  final Map<String, String> parentLocales;

  /// A language-region pair to the script it implies (`zh-TW` is `Hant`).
  final Map<String, String> likelyScripts;

  /// The locales to try for a canonical tag, most specific first: the tag
  /// with its likely script (`zh-TW` is `zh-Hant-TW`), then each CLDR parent
  /// (explicit in [parentLocales], else the tag minus its last subtag).
  List<String> chain(String tag) {
    final parts = tag.split('-');
    var cur = tag;
    if (parts.length == 2 && parts[1].length != 4) {
      final s = likelyScripts[tag];
      if (s != null) cur = '${parts[0]}-$s-${parts[1]}';
    }
    final out = <String>[];
    while (cur.isNotEmpty) {
      out.add(cur);
      final p = parentLocales[cur];
      if (p != null) {
        if (p == 'root') break;
        cur = p;
        continue;
      }
      final i = cur.lastIndexOf('-');
      if (i < 0) break;
      cur = cur.substring(0, i);
    }
    return out;
  }

  /// Picks the locale to use for a requested [tag] out of the locales an app
  /// supports: the tag canonicalised (BCP 47), then each locale on its CLDR
  /// parent chain (`de-AT`, `de`), then a supported locale that refines one
  /// of them, then [fallback]. The result is the canonical form of a
  /// supported locale or of [fallback]. With an empty [fallback] and no match
  /// it throws [L10nError.noLocale].
  String resolve(String tag, List<String> supported, String fallback) {
    final want = canonicalizeLocaleTag(tag);
    final sup = [for (final s in supported) canonicalizeLocaleTag(s)];
    final chain = this.chain(want);
    for (final c in chain) {
      if (sup.contains(c)) return c;
    }
    // A supported locale that is a refinement of a locale on the chain
    // (de-DE for a request for de) comes next.
    for (final c in chain) {
      for (final s in sup) {
        if (this.chain(s).contains(c)) return s;
      }
    }
    if (fallback.isEmpty) {
      throw L10nException(L10nError.noLocale, 'no locale for "$tag"');
    }
    return canonicalizeLocaleTag(fallback);
  }
}

/// Picks the locale to use for [tag] out of the [supported] ones, along the
/// CLDR parent chain of [index], then [fallback]; see
/// [PluxLocaleIndex.resolve].
String resolveLocale(
  String tag,
  List<String> supported,
  String fallback, {
  PluxLocaleIndex index = const PluxLocaleIndex(),
}) => index.resolve(tag, supported, fallback);

bool _isAlpha(String s) => RegExp(r'^[A-Za-z]+$').hasMatch(s);

bool _isAlnum(String s) => RegExp(r'^[A-Za-z0-9]+$').hasMatch(s);

bool _isDigits(String s) => RegExp(r'^[0-9]+$').hasMatch(s);

/// Returns the canonical form of a BCP 47 tag: language in lower case, script
/// in title case, region in upper case, `_` read as `-`, deprecated language
/// subtags replaced. Variants and extensions are dropped: they do not select
/// locale data.
///
/// Throws an [L10nException] with [L10nError.invalidTag] for a tag that is not
/// well-formed.
String canonicalizeLocaleTag(String tag) {
  L10nException bad() =>
      L10nException(L10nError.invalidTag, 'invalid locale tag "$tag"');
  final parts = tag.replaceAll('_', '-').split('-');
  if (parts.contains('')) throw bad();
  var lang = parts[0].toLowerCase();
  if ((lang.length != 2 && lang.length != 3) ||
      !_isAlpha(lang) ||
      lang == 'und') {
    throw bad();
  }
  lang = _languageAliases[lang] ?? lang;
  final out = [lang];
  var rest = parts.sublist(1);
  if (rest.isNotEmpty && rest[0].length == 4 && _isAlpha(rest[0])) {
    out.add(rest[0][0].toUpperCase() + rest[0].substring(1).toLowerCase());
    rest = rest.sublist(1);
  }
  if (rest.isNotEmpty &&
      ((rest[0].length == 2 && _isAlpha(rest[0])) ||
          (rest[0].length == 3 && _isDigits(rest[0])))) {
    out.add(rest[0].toUpperCase());
    rest = rest.sublist(1);
  }
  for (final p in rest) {
    if (p.length == 1 && _isAlnum(p)) break; // an extension: ignored
    if (p.length < 2 || p.length > 8 || !_isAlnum(p)) throw bad();
  }
  return out.join('-');
}
