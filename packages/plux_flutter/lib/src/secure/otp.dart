// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The format of a one-time code (`OtpInput`, SEC-090): its length and
/// alphabet, and the check that typed, autofilled and pasted text pass.
library;

import 'package:flutter/services.dart';

/// The characters a one-time code may contain (`OtpAlphabet`).
enum OtpCharset {
  /// The digits 0 to 9.
  numeric,

  /// The digits and the letters A to Z, upper and lower case.
  alphanumeric;

  /// The charset the registry names [name] (`OtpAlphabet`), or [numeric]
  /// for anything else.
  static OtpCharset named(String? name) =>
      name == 'alphanumeric' ? alphanumeric : numeric;

  /// Whether [unit], a UTF-16 code unit, belongs to the charset.
  bool allows(int unit) {
    final digit = unit >= 0x30 && unit <= 0x39;
    if (this == numeric) return digit;
    return digit ||
        (unit >= 0x41 && unit <= 0x5A) ||
        (unit >= 0x61 && unit <= 0x7A);
  }
}

/// The shape of a code: [length] characters of [charset].
final class OtpFormat {
  /// Creates the format.
  const OtpFormat({required this.length, this.charset = OtpCharset.numeric});

  /// The number of characters.
  final int length;

  /// The characters allowed.
  final OtpCharset charset;

  /// [text] as the code it stands for, or null when it is not one: spaces
  /// and hyphens that group a code (`123 456`, `123-456`) are dropped, and
  /// what remains must be at most [length] characters of the [charset].
  /// Nothing is dropped silently beyond those separators, so text with any
  /// other character is refused whole.
  String? normalize(String text) {
    final out = StringBuffer();
    for (final unit in text.codeUnits) {
      if (unit == 0x20 || unit == 0x2D || unit == 0x09 || unit == 0xA0) {
        continue;
      }
      if (!charset.allows(unit)) return null;
      out.writeCharCode(unit);
    }
    final code = out.toString();
    return code.length <= length ? code : null;
  }
}

/// Refuses edits that would put text outside the [format] into a code
/// field. A typed character outside the alphabet, or one past the length,
/// is dropped. Text inserted in one go, as a paste or an autofill does, is
/// checked as a whole (SEC-090): grouped codes such as `123 456` are
/// accepted as the code, and anything else, or anything too long, is
/// refused entirely and reported to [onRefused].
final class OtpFormatter extends TextInputFormatter {
  /// Creates the formatter.
  const OtpFormatter(this.format, {this.onRefused});

  /// The code's format.
  final OtpFormat format;

  /// Called when an edit other than one typed character is refused.
  final void Function()? onRefused;

  @override
  TextEditingValue formatEditUpdate(
    TextEditingValue oldValue,
    TextEditingValue newValue,
  ) {
    if (newValue.text == oldValue.text) return newValue;
    final code = format.normalize(newValue.text);
    if (code == null) {
      // A single typed character is dropped quietly; anything else was
      // inserted whole, and the user is told it was refused.
      if (newValue.text.length - oldValue.text.length != 1) onRefused?.call();
      return oldValue;
    }
    if (code == newValue.text) return newValue;
    return TextEditingValue(
      text: code,
      selection: TextSelection.collapsed(offset: code.length),
    );
  }
}
