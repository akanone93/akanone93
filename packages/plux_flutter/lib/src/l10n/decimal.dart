// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'package:plux_flutter/src/l10n/errors.dart';

/// An exact base-10 number: sign, integer digits and the fraction digits as
/// written, trailing zeros included (`1.50` keeps its two digits, which plural
/// rules and formatting see). Money and every other quantity the localisation
/// engine formats use [PluxDecimal], never `double`.
final class PluxDecimal {
  const PluxDecimal._(this.isNegative, this.ints, this.frac);

  /// Parses an optionally signed decimal such as `-1234.50` or `1.5e3`.
  ///
  /// Throws an [L10nException] with [L10nError.invalidDecimal] otherwise.
  factory PluxDecimal.parse(String s) {
    L10nException bad() =>
        L10nException(L10nError.invalidDecimal, 'invalid decimal "$s"');
    var rest = s;
    var neg = false;
    if (rest.isNotEmpty && (rest[0] == '-' || rest[0] == '+')) {
      neg = rest[0] == '-';
      rest = rest.substring(1);
    }
    var exp = 0;
    final e = rest.indexOf(RegExp('[eE]'));
    if (e >= 0) {
      var digits = rest.substring(e + 1);
      rest = rest.substring(0, e);
      var sign = 1;
      if (digits.isNotEmpty && (digits[0] == '-' || digits[0] == '+')) {
        if (digits[0] == '-') sign = -1;
        digits = digits.substring(1);
      }
      if (digits.isEmpty || digits.length > 3 || !_allDigits(digits)) {
        throw bad();
      }
      exp = int.parse(digits);
      if (exp > _maxExponent) throw bad();
      exp *= sign;
    }
    final dot = rest.indexOf('.');
    final ip = dot < 0 ? rest : rest.substring(0, dot);
    final fp = dot < 0 ? '' : rest.substring(dot + 1);
    if ((ip.isEmpty && fp.isEmpty) || !_allDigits(ip) || !_allDigits(fp)) {
      throw bad();
    }
    var d = PluxDecimal._(neg, _trimLeft(ip), fp);
    if (exp != 0) {
      final shifted = d.shift(exp);
      d = PluxDecimal._(neg, shifted.ints, shifted.frac);
    }
    return d;
  }

  /// Returns [n] as a [PluxDecimal].
  factory PluxDecimal.fromInt(int n) {
    if (n == 0) return zero;
    return PluxDecimal._(n < 0, n.abs().toString(), '');
  }

  /// The number 0.
  static const zero = PluxDecimal._(false, '', '');

  static const _maxExponent = 400;

  /// Whether the sign is negative; `-0` is negative.
  final bool isNegative;

  /// The integer digits without leading zeros; empty is 0.
  final String ints;

  /// The fraction digits as written.
  final String frac;

  /// The integer digits, `0` for none.
  String get intDigits => ints.isEmpty ? '0' : ints;

  /// Whether every digit is zero.
  bool get isZero => ints.isEmpty && _trimRight(frac).isEmpty;

  /// The number of integer digits, 0 when the number is below 1.
  int get intLen => ints.length;

  /// The absolute value.
  PluxDecimal abs() => PluxDecimal._(false, ints, frac);

  /// Returns this number with [negative] as its sign.
  PluxDecimal withSign({required bool negative}) =>
      PluxDecimal._(negative, ints, frac);

  /// Multiplies by 10^[n] by moving the decimal point; the fraction digits are
  /// kept as far as they still exist.
  PluxDecimal shift(int n) {
    if (n == 0) return this;
    var all = ints + frac;
    var point = ints.length + n;
    if (point < 0) {
      all = '0' * -point + all;
      point = 0;
    }
    if (point > all.length) all += '0' * (point - all.length);
    return PluxDecimal._(
      isNegative,
      _trimLeft(all.substring(0, point)),
      all.substring(point),
    );
  }

  /// Rounds to [n] fraction digits, half to even, and returns the number with
  /// exactly [n] digits; [n] may be negative to round to tens, hundreds and
  /// so on.
  PluxDecimal roundFrac(int n) {
    if (n < 0) return shift(n).roundFrac(0).shift(-n);
    if (frac.length <= n) {
      return PluxDecimal._(isNegative, ints, frac + '0' * (n - frac.length));
    }
    final keep = frac.substring(0, n);
    final rest = frac.substring(n);
    final digits = (ints + keep).codeUnits.toList();
    var up = false;
    if (rest.codeUnitAt(0) > 0x35) {
      up = true;
    } else if (rest.codeUnitAt(0) == 0x35) {
      if (_trimRight(rest.substring(1)).isNotEmpty) {
        up = true;
      } else {
        final last = digits.isEmpty ? 0x30 : digits.last;
        up = (last - 0x30).isOdd;
      }
    }
    if (up) {
      var i = digits.length - 1;
      for (; i >= 0; i--) {
        if (digits[i] == 0x39) {
          digits[i] = 0x30;
          continue;
        }
        digits[i]++;
        break;
      }
      if (i < 0) digits.insert(0, 0x31);
    }
    final all = String.fromCharCodes(digits);
    final split = all.length - n;
    return PluxDecimal._(
      isNegative,
      _trimLeft(all.substring(0, split)),
      all.substring(split),
    );
  }

  /// Rounds to [sig] significant digits, half to even.
  PluxDecimal roundSignificant(int sig) {
    if (isZero) return this;
    return roundFrac(sig - 1 - exponent);
  }

  /// The power of ten of the first non-zero digit.
  int get exponent {
    if (ints.isNotEmpty) return ints.length - 1;
    for (var i = 0; i < frac.length; i++) {
      if (frac[i] != '0') return -(i + 1);
    }
    return 0;
  }

  /// Removes trailing zero fraction digits down to [min] digits.
  PluxDecimal trimFrac(int min) {
    var f = frac;
    while (f.length > min && f.endsWith('0')) {
      f = f.substring(0, f.length - 1);
    }
    return PluxDecimal._(isNegative, ints, f);
  }

  @override
  String toString() =>
      '${isNegative ? '-' : ''}$intDigits${frac.isEmpty ? '' : '.$frac'}';

  @override
  bool operator ==(Object other) =>
      other is PluxDecimal &&
      other.isNegative == isNegative &&
      other.ints == ints &&
      other.frac == frac;

  @override
  int get hashCode => Object.hash(isNegative, ints, frac);
}

bool _allDigits(String s) {
  for (var i = 0; i < s.length; i++) {
    final c = s.codeUnitAt(i);
    if (c < 0x30 || c > 0x39) return false;
  }
  return true;
}

String _trimLeft(String s) {
  var i = 0;
  while (i < s.length && s[i] == '0') {
    i++;
  }
  return s.substring(i);
}

String _trimRight(String s) {
  var i = s.length;
  while (i > 0 && s[i - 1] == '0') {
    i--;
  }
  return s.substring(0, i);
}

/// What operands saturate to; no plural rule compares with a value this large.
const int satMax = 9007199254740991;

/// Returns the digit string as a number, modulo [mod] when [mod] is positive,
/// and saturated at [satMax] otherwise.
int digitsValue(String s, int mod) {
  var v = 0;
  for (var i = 0; i < s.length; i++) {
    final c = s.codeUnitAt(i) - 0x30;
    if (mod > 0) {
      v = (v * 10 + c) % mod;
      continue;
    }
    if (v > (satMax - c) ~/ 10) return satMax;
    v = v * 10 + c;
  }
  return v;
}
