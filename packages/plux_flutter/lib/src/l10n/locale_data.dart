// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The localisation engine's locale data: plural rules and number, percent,
/// compact, money and date formatting over the JSON of schema/l10n/cldr
/// (ADR-0066, I18N-001, I18N-002, I18N-011). It is the same engine as Go's
/// `backend/internal/l10n` and gives the same output for the same data
/// (QA-003); it reads data handed to it and depends on no package.
library;

import 'package:plux_flutter/src/l10n/decimal.dart';
import 'package:plux_flutter/src/l10n/errors.dart';
import 'package:plux_flutter/src/l10n/plural.dart';

part 'date.dart';
part 'number.dart';

/// The formatting data of one CLDR locale, built from the JSON of a locale
/// file with [PluxLocaleData.fromJson]. It is immutable.
final class PluxLocaleData {
  PluxLocaleData._({
    required this.id,
    required this.isRtl,
    required this.defaultNumberingSystem,
    required this._digits,
    required this._minGrouping,
    required this._spacingBefore,
    required this._spacingAfter,
    required this._systems,
    required this._currencies,
    required this._currencyDigits,
    required this._territories,
    required this._calendars,
    required this._cardinal,
    required this._ordinal,
  });

  /// Builds the locale from the decoded JSON of a locale file (the file of
  /// schema/l10n/cldr, or the bundle's `LocaleData` of the same shape).
  ///
  /// Throws an [L10nException] with [L10nError.invalidData] when the data is
  /// incomplete.
  factory PluxLocaleData.fromJson(Map<String, Object?> json) {
    try {
      return _build(json);
    } on L10nException {
      rethrow;
    } on Object catch (e) {
      throw L10nException(L10nError.invalidData, 'locale data: $e');
    }
  }

  static PluxLocaleData _build(Map<String, Object?> json) {
    final id = _string(json, 'locale');
    final numbers = _map(json, 'numbers');
    final calendars = _map(json, 'calendars');
    if (id.isEmpty || numbers.isEmpty || calendars.isEmpty) {
      throw const L10nException(
        L10nError.invalidData,
        'locale file is incomplete',
      );
    }
    final numbering = _map(json, 'numbering');
    final defaultSystem = _string(numbering, 'default');
    final digits = <String, String>{'latn': '0123456789'};
    if (defaultSystem != 'latn') {
      final d = _string(numbering, 'digits');
      if (d.runes.length != 10) {
        throw L10nException(
          L10nError.invalidData,
          'numbering system "$defaultSystem" has no ten digits',
        );
      }
      digits[defaultSystem] = d;
    }
    final systems = <String, _NumberSystem>{
      for (final e in numbers.entries)
        e.key: _NumberSystem.compile(e.value! as Map<String, Object?>),
    };
    if (systems['latn'] == null || systems[defaultSystem] == null) {
      throw const L10nException(
        L10nError.invalidData,
        'numbers lack latn or the default system',
      );
    }
    final spacing = _map(json, 'currencySpacing');
    final plurals = _map(json, 'plurals');
    return PluxLocaleData._(
      id: id,
      isRtl: json['direction'] == 'rtl',
      defaultNumberingSystem: defaultSystem,
      digits: digits,
      minGrouping: (json['minimumGroupingDigits'] as int?) ?? 1,
      spacingBefore: spacing['before'] as String? ?? '',
      spacingAfter: spacing['after'] as String? ?? '',
      systems: systems,
      currencies: {
        for (final e in _map(json, 'currencies').entries)
          e.key: _CurrencyNames.fromJson(e.value! as Map<String, Object?>),
      },
      currencyDigits: {
        for (final e in _map(json, 'currencyDigits').entries)
          e.key: e.value! as int,
      },
      territories: {
        for (final e in _map(json, 'territories').entries)
          e.key: e.value! as String,
      },
      calendars: {
        for (final e in calendars.entries)
          e.key: _Calendar.compile(e.value! as Map<String, Object?>),
      },
      cardinal: PluralRules.compile(_strings(_map(plurals, 'cardinal'))),
      ordinal: PluralRules.compile(_strings(_map(plurals, 'ordinal'))),
    );
  }

  /// The CLDR locale the data is for (`de` for a request for `de-DE`).
  final String id;

  /// Whether the layout direction is right to left.
  final bool isRtl;

  /// The locale's own numbering system, such as `latn` or `arabext`.
  /// Formatting uses `latn` unless asked for another.
  final String defaultNumberingSystem;

  final Map<String, String> _digits;
  final int _minGrouping;
  final String _spacingBefore;
  final String _spacingAfter;
  final Map<String, _NumberSystem> _systems;
  final Map<String, _CurrencyNames> _currencies;
  final Map<String, int> _currencyDigits;
  final Map<String, String> _territories;
  final Map<String, _Calendar> _calendars;
  final PluralRules _cardinal;
  final PluralRules _ordinal;

  /// The cardinal plural category of [n] (I18N-001). The fraction digits of
  /// [n] count: 1 is `one` in English, 1.0 is `other`.
  PluralCategory pluralCardinal(PluxDecimal n) =>
      _cardinal.category(PluralOperands(n.abs()));

  /// The ordinal plural category of [n].
  PluralCategory pluralOrdinal(PluxDecimal n) =>
      _ordinal.category(PluralOperands(n.abs()));

  /// The cardinal categories the locale uses, in CLDR order, `other` last.
  List<PluralCategory> get categories => _cardinal.categories;

  /// The locale's name for an ISO 3166 region code, or null.
  String? territoryName(String code) => _territories[code];

  /// The locale's name for a currency, falling back to the code itself.
  String currencyName(String code) {
    final n = _currencies[code]?.name ?? '';
    return n.isEmpty ? code : n;
  }

  /// The ISO 4217 minor-unit digits of a currency.
  int currencyDigits(String code) => _currencyDigits[code] ?? 2;

  /// Resolves a numbering system option: empty and `latn` are Latin digits,
  /// `default` or the locale's own id its native digits.
  (String, _NumberSystem) _system(String opt) {
    final id = switch (opt) {
      '' || 'latn' => 'latn',
      'default' => defaultNumberingSystem,
      _ => opt,
    };
    final ns = _systems[id];
    if (ns == null) {
      throw L10nException(
        L10nError.unsupported,
        'numbering system "$opt" for locale $id',
      );
    }
    return (id, ns);
  }

  /// Replaces the ASCII digits of [s] by those of numbering system [system].
  String _mapDigits(String s, String system) {
    final d = _digits[system];
    if (system == 'latn' || d == null) return s;
    final runes = d.runes.toList();
    final out = StringBuffer();
    for (final r in s.runes) {
      out.writeCharCode(r >= 0x30 && r <= 0x39 ? runes[r - 0x30] : r);
    }
    return out.toString();
  }
}

/// The display strings of one currency; the symbol and narrow symbol default
/// to the code and the name to the code.
final class _CurrencyNames {
  const _CurrencyNames(this.name, this.symbol, this.narrow);

  factory _CurrencyNames.fromJson(Map<String, Object?> json) => _CurrencyNames(
    json['name'] as String? ?? '',
    json['symbol'] as String? ?? '',
    json['narrow'] as String? ?? '',
  );

  final String name;
  final String symbol;
  final String narrow;
}

String _string(Map<String, Object?> m, String key) => m[key] as String? ?? '';

Map<String, Object?> _map(Map<String, Object?> m, String key) =>
    (m[key] as Map<String, Object?>?) ?? const {};

Map<String, String> _strings(Map<String, Object?> m) => {
  for (final e in m.entries) e.key: e.value! as String,
};
