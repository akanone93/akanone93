// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:plux_flutter/src/l10n/decimal.dart';
import 'package:plux_flutter/src/l10n/errors.dart';

/// A CLDR plural category.
enum PluralCategory {
  /// The `zero` category.
  zero,

  /// The `one` category.
  one,

  /// The `two` category.
  two,

  /// The `few` category.
  few,

  /// The `many` category.
  many,

  /// The `other` category, which every locale has.
  other,
}

/// One `operand [% mod] (=|!=) ranges` test of a plural rule.
final class _Relation {
  _Relation(this.operand, this.mod, this.negate, this.ranges);

  final String operand;
  final int mod;
  final bool negate;
  final List<(int, int)> ranges;

  bool matches(PluralOperands o) {
    final (v, isInt) = o.value(operand, mod);
    var inRange = false;
    if (isInt) {
      for (final (lo, hi) in ranges) {
        if (v >= lo && v <= hi) {
          inRange = true;
          break;
        }
      }
    }
    return inRange != negate;
  }
}

/// A plural rule in disjunctive normal form: any of its conjunctions.
final class PluralRule {
  PluralRule._(this._or);

  /// Parses a rule in CLDR's plural rule syntax, without samples.
  factory PluralRule.parse(String text) {
    final or = <List<_Relation>>[];
    var and = <_Relation>[];
    final toks = text.trim().split(RegExp(r'\s+'));
    if (text.trim().isEmpty) throw _bad(text, 'empty rule');
    var i = 0;
    while (i < toks.length) {
      final (rel, n) = _parseRelation(text, toks.sublist(i));
      and.add(rel);
      i += n;
      if (i == toks.length) break;
      switch (toks[i]) {
        case 'and':
          break;
        case 'or':
          or.add(and);
          and = [];
        default:
          throw _bad(text, 'unexpected "${toks[i]}"');
      }
      i++;
      if (i == toks.length) {
        throw _bad(text, 'rule ends after "${toks[i - 1]}"');
      }
    }
    or.add(and);
    return PluralRule._(or);
  }

  final List<List<_Relation>> _or;

  /// Whether the rule holds for [o].
  bool matches(PluralOperands o) {
    for (final and in _or) {
      if (and.every((r) => r.matches(o))) return true;
    }
    return false;
  }
}

L10nException _bad(String rule, String why) =>
    L10nException(L10nError.invalidData, 'plural rule "$rule": $why');

(_Relation, int) _parseRelation(String rule, List<String> toks) {
  if (toks[0].length != 1 || !'nivwftce'.contains(toks[0])) {
    throw _bad(rule, 'unknown operand "${toks[0]}"');
  }
  final operand = toks[0];
  var i = 1;
  var mod = 0;
  if (i < toks.length && toks[i] == '%') {
    if (i + 1 >= toks.length) throw _bad(rule, 'missing modulus');
    final m = int.tryParse(toks[i + 1]);
    if (m == null || m <= 0) throw _bad(rule, 'bad modulus "${toks[i + 1]}"');
    mod = m;
    i += 2;
  }
  if (i + 1 >= toks.length) throw _bad(rule, 'incomplete relation');
  var negate = false;
  switch (toks[i]) {
    case '=':
      break;
    case '!=':
      negate = true;
    default:
      throw _bad(rule, 'unknown operator "${toks[i]}"');
  }
  final ranges = <(int, int)>[];
  for (final part in toks[i + 1].split(',')) {
    final dots = part.indexOf('..');
    final a = int.tryParse(dots < 0 ? part : part.substring(0, dots));
    if (a == null) throw _bad(rule, 'bad range "$part"');
    var b = a;
    if (dots >= 0) {
      final parsed = int.tryParse(part.substring(dots + 2));
      if (parsed == null || parsed < a) throw _bad(rule, 'bad range "$part"');
      b = parsed;
    }
    ranges.add((a, b));
  }
  return (_Relation(operand, mod, negate, ranges), i + 2);
}

/// The CLDR plural operands (n i v w f t c e) of a number.
final class PluralOperands {
  /// Creates the operands of [d]; [c] is the compact decimal exponent, 0 for
  /// a plain number.
  const PluralOperands(this.d, [this.c = 0]);

  /// The number (its absolute value).
  final PluxDecimal d;

  /// The compact decimal exponent (operands c and e).
  final int c;

  /// The value of [op], reduced modulo [mod] when [mod] is positive, and
  /// whether it is an integer (a fraction never matches an integer range).
  (int, bool) value(String op, int mod) {
    final trimmed = _trimZeros(d.frac);
    switch (op) {
      case 'n':
        if (trimmed.isNotEmpty) return (0, false);
        return (digitsValue(d.intDigits, mod), true);
      case 'i':
        return (digitsValue(d.intDigits, mod), true);
      case 'v':
        return (d.frac.length, true);
      case 'w':
        return (trimmed.length, true);
      case 'f':
        return (digitsValue(d.frac, mod), true);
      case 't':
        return (digitsValue(trimmed, mod), true);
      default:
        return (mod > 0 ? c % mod : c, true);
    }
  }
}

String _trimZeros(String s) {
  var i = s.length;
  while (i > 0 && s[i - 1] == '0') {
    i--;
  }
  return s.substring(0, i);
}

/// The compiled rules of one plural type: a category to its rule; `other` has
/// none.
final class PluralRules {
  PluralRules._(this._rules);

  /// Compiles [src], a category name to rule text map from locale data.
  factory PluralRules.compile(Map<String, String> src) {
    final out = <PluralCategory, PluralRule>{};
    src.forEach((name, text) {
      final cat = PluralCategory.values
          .where((c) => c.name == name)
          .firstOrNull;
      if (cat == null || cat == PluralCategory.other) {
        throw L10nException(
          L10nError.invalidData,
          'unknown plural category "$name"',
        );
      }
      out[cat] = PluralRule.parse(text);
    });
    return PluralRules._(out);
  }

  final Map<PluralCategory, PluralRule> _rules;

  /// The first category, in CLDR order, whose rule matches [o].
  PluralCategory category(PluralOperands o) {
    for (final c in PluralCategory.values) {
      final r = _rules[c];
      if (r != null && r.matches(o)) return c;
    }
    return PluralCategory.other;
  }

  /// The categories with a rule, in CLDR order, `other` last.
  List<PluralCategory> get categories => [
    for (final c in PluralCategory.values)
      if (_rules.containsKey(c)) c,
    PluralCategory.other,
  ];
}
