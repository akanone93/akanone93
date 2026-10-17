// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

part of 'locale_data.dart';

/// Bounds the number of fraction digits shown: the value is rounded, half to
/// even, to [max] digits and trailing zeros are dropped down to [min].
final class FractionDigits {
  /// Creates the bounds.
  const FractionDigits(this.min, this.max);

  /// The fewest fraction digits shown.
  final int min;

  /// The most fraction digits shown.
  final int max;

  /// The most fraction digits a request may ask for.
  static const maxAllowed = 20;

  void _validate() {
    if (min < 0 || max < min || max > maxAllowed) {
      throw L10nException(
        L10nError.invalidOption,
        'fraction digits $min..$max',
      );
    }
  }
}

/// Options of [PluxLocaleData.formatNumber], [PluxLocaleData.formatPercent]
/// and [PluxLocaleData.formatCompact].
final class NumberOptions {
  /// Creates the options.
  const NumberOptions({
    this.fraction,
    this.numberingSystem = '',
    this.noGrouping = false,
  });

  /// Overrides the pattern's fraction digits; null keeps them.
  final FractionDigits? fraction;

  /// Empty or `latn` for Latin digits (the default), or `default` or the
  /// locale's own system id (such as `arab`) for its native digits.
  final String numberingSystem;

  /// Turns the grouping separator off.
  final bool noGrouping;
}

/// How a currency is written.
enum CurrencyDisplay {
  /// The locale's symbol ($, €, US$).
  symbol,

  /// The narrow symbol ($ for every dollar).
  narrow,

  /// The ISO 4217 code.
  code,
}

/// Options of [PluxLocaleData.formatMoney].
final class MoneyOptions {
  /// Creates the options.
  const MoneyOptions({
    this.display = CurrencyDisplay.symbol,
    this.accounting = false,
    this.fraction,
    this.numberingSystem = '',
    this.noGrouping = false,
  });

  /// How the currency is written.
  final CurrencyDisplay display;

  /// Writes negative amounts in the locale's accounting style, such as
  /// (€1.00).
  final bool accounting;

  /// Overrides the currency's ISO 4217 digits.
  final FractionDigits? fraction;

  /// See [NumberOptions.numberingSystem].
  final String numberingSystem;

  /// Turns the grouping separator off.
  final bool noGrouping;
}

enum _PartKind { text, currency, percent, permille, minus, plus }

final class _Part {
  const _Part(this.kind, [this.text = '']);

  final _PartKind kind;
  final String text;
}

/// A compiled CLDR number pattern such as `#,##0.00 ¤;(#,##0.00 ¤)`.
final class _Pattern {
  _Pattern._(
    this.prefix,
    this.suffix,
    this.negPrefix,
    this.negSuffix,
    this.hasNeg,
    this.minInt,
    this.minFrac,
    this.maxFrac,
    this.primary,
    this.secondary,
    this.scale,
  );

  factory _Pattern.compile(String src) {
    final (pos, neg, hasNeg) = _cutUnquoted(src, ';');
    final core = _parseHalf(pos);
    var negPrefix = <_Part>[];
    var negSuffix = <_Part>[];
    if (hasNeg) {
      final n = _parseHalf(neg);
      negPrefix = n.prefix;
      negSuffix = n.suffix;
    }
    var scale = 0;
    for (final p in [...core.prefix, ...core.suffix]) {
      if (p.kind == _PartKind.percent) scale = 2;
      if (p.kind == _PartKind.permille) scale = 3;
    }
    return _Pattern._(
      core.prefix,
      core.suffix,
      negPrefix,
      negSuffix,
      hasNeg,
      core.minInt,
      core.minFrac,
      core.maxFrac,
      core.primary,
      core.secondary,
      scale,
    );
  }

  final List<_Part> prefix;
  final List<_Part> suffix;
  final List<_Part> negPrefix;
  final List<_Part> negSuffix;
  final bool hasNeg;
  final int minInt;
  final int minFrac;
  final int maxFrac;

  /// The grouping sizes; 0 means no grouping.
  final int primary;
  final int secondary;

  /// The power of ten the value is multiplied by: 2 for a percent.
  final int scale;
}

/// Splits [s] at the first [sep] outside single quotes.
(String, String, bool) _cutUnquoted(String s, String sep) {
  var quoted = false;
  for (var i = 0; i < s.length; i++) {
    final c = s[i];
    if (c == "'") {
      quoted = !quoted;
    } else if (c == sep && !quoted) {
      return (s.substring(0, i), s.substring(i + 1), true);
    }
  }
  return (s, '', false);
}

final class _Half {
  _Half(
    this.prefix,
    this.suffix,
    this.minInt,
    this.minFrac,
    this.maxFrac,
    this.primary,
    this.secondary,
  );

  final List<_Part> prefix;
  final List<_Part> suffix;
  final int minInt;
  final int minFrac;
  final int maxFrac;
  final int primary;
  final int secondary;
}

_Half _parseHalf(String s) {
  final runes = s.runes.toList();
  var start = -1;
  var end = -1;
  var quoted = false;
  for (var i = 0; i < runes.length; i++) {
    final r = runes[i];
    if (r == 0x27) {
      quoted = !quoted;
    } else if (!quoted && (r == 0x23 || r == 0x30 || r == 0x2c || r == 0x2e)) {
      if (start < 0) start = i;
      end = i + 1;
    }
  }
  if (start < 0) {
    throw L10nException(
      L10nError.invalidData,
      'pattern "$s" has no digit placeholder',
    );
  }
  final core = String.fromCharCodes(runes.sublist(start, end));
  final dot = core.indexOf('.');
  final ip = dot < 0 ? core : core.substring(0, dot);
  final fp = dot < 0 ? '' : core.substring(dot + 1);
  if (fp.contains(',')) {
    throw L10nException(
      L10nError.invalidData,
      'pattern "$s" groups the fraction',
    );
  }
  var primary = 0;
  var secondary = 0;
  final groups = ip.split(',');
  if (groups.length > 1) {
    primary = groups.last.length;
    secondary = groups.length > 2 ? groups[groups.length - 2].length : primary;
    if (primary == 0 || secondary == 0) {
      throw L10nException(L10nError.invalidData, 'pattern "$s" groups nothing');
    }
  }
  return _Half(
    _parseAffix(String.fromCharCodes(runes.sublist(0, start))),
    _parseAffix(String.fromCharCodes(runes.sublist(end))),
    '0'.allMatches(ip).length,
    '0'.allMatches(fp).length,
    fp.length,
    primary,
    secondary,
  );
}

List<_Part> _parseAffix(String s) {
  final out = <_Part>[];
  final text = StringBuffer();
  void flush() {
    if (text.isNotEmpty) {
      out.add(_Part(_PartKind.text, text.toString()));
      text.clear();
    }
  }

  final runes = s.runes.toList();
  for (var i = 0; i < runes.length; i++) {
    final r = runes[i];
    switch (r) {
      case 0x27: // '
        if (i + 1 < runes.length && runes[i + 1] == 0x27) {
          text.writeCharCode(0x27);
          i++;
          continue;
        }
        var j = i + 1;
        while (j < runes.length) {
          if (runes[j] == 0x27) {
            if (j + 1 < runes.length && runes[j + 1] == 0x27) {
              text.writeCharCode(0x27);
              j += 2;
              continue;
            }
            break;
          }
          text.writeCharCode(runes[j]);
          j++;
        }
        if (j == runes.length) {
          throw L10nException(
            L10nError.invalidData,
            'pattern "$s" has an unterminated quote',
          );
        }
        i = j;
      case 0xa4: // ¤
        flush();
        while (i + 1 < runes.length && runes[i + 1] == 0xa4) {
          i++;
        }
        out.add(const _Part(_PartKind.currency));
      case 0x25: // %
        flush();
        out.add(const _Part(_PartKind.percent));
      case 0x2030: // ‰
        flush();
        out.add(const _Part(_PartKind.permille));
      case 0x2d: // -
        flush();
        out.add(const _Part(_PartKind.minus));
      case 0x2b: // +
        flush();
        out.add(const _Part(_PartKind.plus));
      default:
        text.writeCharCode(r);
    }
  }
  flush();
  return out;
}

/// The short-compact patterns of one magnitude, by plural category.
final class _CompactRow {
  _CompactRow(this.exp, this.patterns);

  /// The power of ten of the magnitude.
  final int exp;
  final Map<String, _Pattern> patterns;
}

/// A numbering system's symbols and compiled patterns.
final class _NumberSystem {
  _NumberSystem._(
    this.symbols,
    this.decimal,
    this.percent,
    this.currency,
    this.accounting,
    this.compact,
  );

  factory _NumberSystem.compile(Map<String, Object?> json) {
    final symbols = _strings(_map(json, 'symbols'));
    for (final k in [
      'decimal',
      'group',
      'minusSign',
      'percentSign',
      'plusSign',
    ]) {
      if ((symbols[k] ?? '').isEmpty) {
        throw L10nException(L10nError.invalidData, 'symbol $k is missing');
      }
    }
    final rows = <_CompactRow>[];
    for (final e in _map(json, 'compactShort').entries) {
      final key = e.key;
      if (!RegExp(r'^10*$').hasMatch(key)) {
        throw L10nException(
          L10nError.invalidData,
          'compact magnitude "$key" is not a power of ten',
        );
      }
      rows.add(
        _CompactRow(key.length - 1, {
          for (final p in (e.value! as Map<String, Object?>).entries)
            p.key: _Pattern.compile(p.value! as String),
        }),
      );
    }
    rows.sort((a, b) => a.exp.compareTo(b.exp));
    return _NumberSystem._(
      symbols,
      _Pattern.compile(_string(json, 'decimal')),
      _Pattern.compile(_string(json, 'percent')),
      _Pattern.compile(_string(json, 'currency')),
      _Pattern.compile(_string(json, 'accounting')),
      rows,
    );
  }

  final Map<String, String> symbols;
  final _Pattern decimal;
  final _Pattern percent;
  final _Pattern currency;
  final _Pattern accounting;
  final List<_CompactRow> compact;

  /// The row for the largest magnitude not above 10^[exp], or null when the
  /// number is below the smallest.
  _CompactRow? compactRow(int exp) {
    _CompactRow? best;
    for (final r in compact) {
      if (r.exp <= exp) best = r;
    }
    return best;
  }
}

/// Everything one formatting call needs besides the number.
final class _Layout {
  _Layout(
    this.pat,
    this.sysId,
    this.ns, {
    required this.grouping,
    this.min = 0,
    this.max = 0,
    this.currency = '',
    this.isMoney = false,
    this.minGrouping = 0,
  });

  final _Pattern pat;
  final String sysId;
  final _NumberSystem ns;
  final bool grouping;
  int min;
  int max;

  /// What ¤ is replaced by; empty outside money.
  final String currency;
  final bool isMoney;

  /// When positive, raises the locale's minimum grouping digits: compact
  /// notation groups only from five digits.
  final int minGrouping;

  void setFractions(FractionDigits? f) {
    min = pat.minFrac;
    max = pat.maxFrac;
    if (f == null) return;
    f._validate();
    min = f.min;
    max = f.max;
  }
}

final _symbolOrSpace = RegExp(r'[\p{S}\p{Z}\s]', unicode: true);

/// The number formatting methods of [PluxLocaleData].
extension PluxNumberFormatting on PluxLocaleData {
  /// Formats [x] with the locale's decimal pattern: grouping (including
  /// secondary grouping), fraction digits rounded half to even, and the
  /// locale's separators (I18N-001).
  String formatNumber(
    PluxDecimal x, [
    NumberOptions opts = const NumberOptions(),
  ]) {
    final (id, ns) = _system(opts.numberingSystem);
    final ly = _Layout(ns.decimal, id, ns, grouping: !opts.noGrouping)
      ..setFractions(opts.fraction);
    return _render(ly, x);
  }

  /// Formats [x], a fraction (0.25), as a percentage (`25%`).
  String formatPercent(
    PluxDecimal x, [
    NumberOptions opts = const NumberOptions(),
  ]) {
    final (id, ns) = _system(opts.numberingSystem);
    final ly = _Layout(ns.percent, id, ns, grouping: !opts.noGrouping)
      ..setFractions(opts.fraction);
    return _render(ly, x);
  }

  /// Formats [amount] in a currency (I18N-011): the currency's ISO 4217
  /// digits unless overridden, the locale's symbol, its placement and
  /// spacing, and its accounting style on request. 1234.56 EUR is
  /// `1.234,56 €` in de and 1234.56 USD is `$1,234.56` in en.
  String formatMoney(
    PluxDecimal amount,
    String currency, [
    MoneyOptions opts = const MoneyOptions(),
  ]) {
    if (!RegExp(r'^[A-Z]{3}$').hasMatch(currency)) {
      throw L10nException(
        L10nError.invalidCurrency,
        'invalid currency "$currency"',
      );
    }
    final (id, ns) = _system(opts.numberingSystem);
    final names = _currencies[currency];
    final symbol = (names?.symbol ?? '').isEmpty ? currency : names!.symbol;
    final shown = switch (opts.display) {
      CurrencyDisplay.symbol => symbol,
      CurrencyDisplay.narrow =>
        (names?.narrow ?? '').isEmpty ? symbol : names!.narrow,
      CurrencyDisplay.code => currency,
    };
    final digits = currencyDigits(currency);
    final ly = _Layout(
      opts.accounting ? ns.accounting : ns.currency,
      id,
      ns,
      grouping: !opts.noGrouping,
      min: digits,
      max: digits,
      currency: shown,
      isMoney: true,
    );
    final f = opts.fraction;
    if (f != null) {
      f._validate();
      ly
        ..min = f.min
        ..max = f.max;
    }
    return _render(ly, amount);
  }

  /// Formats [x] in the short compact notation (`1.2K`, `3 Mio.`, `12万`):
  /// the magnitude's pattern is chosen by the plural category of the rounded
  /// number, which keeps two significant digits below ten and whole numbers
  /// above.
  String formatCompact(
    PluxDecimal x, [
    NumberOptions opts = const NumberOptions(),
  ]) {
    final (id, ns) = _system(opts.numberingSystem);
    if (opts.fraction != null) {
      throw const L10nException(
        L10nError.invalidOption,
        'compact notation fixes its own fraction digits',
      );
    }
    var cur = x.abs();
    var scaled = cur;
    var exp = 0;
    for (var attempt = 0; attempt < 2; attempt++) {
      final row = ns.compactRow(cur.intLen - 1);
      exp = 0;
      final other = row?.patterns['other'];
      if (row != null &&
          other != null &&
          (other.prefix.isNotEmpty || other.suffix.isNotEmpty)) {
        exp = row.exp - (other.minInt - 1);
      }
      scaled = cur.shift(-exp);
      scaled = scaled.intLen >= 2
          ? scaled.roundFrac(0)
          : scaled.roundSignificant(2).trimFrac(0);
      if (exp > 0 && scaled.intLen > row!.patterns['other']!.minInt) {
        cur = scaled.shift(exp); // rounded up to the next magnitude
        continue;
      }
      break;
    }
    var pat = ns.decimal;
    if (exp > 0) {
      final cat = _cardinal.category(PluralOperands(scaled, exp));
      final row = ns.compactRow(cur.intLen - 1);
      if (row == null) {
        throw const L10nException(
          L10nError.invalidData,
          'missing compact patterns',
        );
      }
      pat = row.patterns[cat.name] ?? row.patterns['other']!;
    }
    final ly = _Layout(
      pat,
      id,
      ns,
      grouping: !opts.noGrouping,
      max: scaled.frac.length,
      minGrouping: 2,
    );
    return _render(ly, scaled.withSign(negative: x.isNegative));
  }

  /// Writes [x] with the layout: rounding, grouping, symbols, digits and the
  /// pattern's affixes.
  String _render(_Layout ly, PluxDecimal x) {
    final p = ly.pat;
    final v = x.abs().shift(p.scale).roundFrac(ly.max).trimFrac(ly.min);
    var ints = v.ints;
    if (ints.length < p.minInt) ints = '0' * (p.minInt - ints.length) + ints;
    var decSym = ly.ns.symbols['decimal']!;
    var groupSym = ly.ns.symbols['group']!;
    if (ly.isMoney) {
      final d = ly.ns.symbols['currencyDecimal'];
      if (d != null && d.isNotEmpty) decSym = d;
      final g = ly.ns.symbols['currencyGroup'];
      if (g != null && g.isNotEmpty) groupSym = g;
    }
    var mg = _minGrouping;
    if (ly.minGrouping > mg) mg = ly.minGrouping;
    if (mg < 1) mg = 1;
    if (ly.grouping && p.primary > 0 && ints.length >= p.primary + mg) {
      ints = _group(ints, p.primary, p.secondary, groupSym);
    }
    var core = ints;
    if (v.frac.isNotEmpty) core += decSym + v.frac;
    core = _mapDigits(core, ly.sysId);

    var prefix = p.prefix;
    var suffix = p.suffix;
    var lead = '';
    if (x.isNegative) {
      if (p.hasNeg) {
        prefix = p.negPrefix;
        suffix = p.negSuffix;
      } else {
        lead = ly.ns.symbols['minusSign']!;
      }
    }
    final b = StringBuffer(lead);
    _writeAffix(b, prefix, ly, isPrefix: true);
    b.write(core);
    _writeAffix(b, suffix, ly, isPrefix: false);
    return b.toString();
  }

  /// Writes a prefix or suffix. A currency sign touching the number gets the
  /// locale's currency spacing when the currency text ends (before the
  /// number) or starts (after it) with a letter-like character.
  void _writeAffix(
    StringBuffer b,
    List<_Part> a,
    _Layout ly, {
    required bool isPrefix,
  }) {
    for (var i = 0; i < a.length; i++) {
      final pt = a[i];
      switch (pt.kind) {
        case _PartKind.text:
          b.write(pt.text);
        case _PartKind.currency:
          final cur = ly.currency;
          if (cur.isEmpty) continue;
          final touches = isPrefix ? i == a.length - 1 : i == 0;
          if (touches && isPrefix) {
            b.write(cur);
            if (!_symbolOrSpace.hasMatch(String.fromCharCode(cur.runes.last))) {
              b.write(_spacingAfter);
            }
          } else if (touches) {
            if (!_symbolOrSpace.hasMatch(
              String.fromCharCode(cur.runes.first),
            )) {
              b.write(_spacingBefore);
            }
            b.write(cur);
          } else {
            b.write(cur);
          }
        case _PartKind.percent:
          b.write(ly.ns.symbols['percentSign']);
        case _PartKind.permille:
          b.write(ly.ns.symbols['perMille']);
        case _PartKind.minus:
          b.write(ly.ns.symbols['minusSign']);
        case _PartKind.plus:
          b.write(ly.ns.symbols['plusSign']);
      }
    }
  }
}

/// Inserts [sep] into the integer digits: the first group from the right has
/// [primary] digits, the others [secondary].
String _group(String ints, int primary, int secondary, String sep) {
  final parts = <String>[];
  var end = ints.length;
  var size = primary;
  while (end > size) {
    parts.add(ints.substring(end - size, end));
    end -= size;
    size = secondary;
  }
  parts.add(ints.substring(0, end));
  return parts.reversed.join(sep);
}
