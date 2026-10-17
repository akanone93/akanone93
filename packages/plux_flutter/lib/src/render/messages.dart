// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Formats translated messages (ADR-0066, I18N-001, I18N-011). A message
/// that arrives with its pre-parsed AST is evaluated by the AST evaluator of
/// `l10n/message.dart` over the locale's CLDR data, the same as Go's
/// `l10n/mf`. A message without an AST (a bundle from before the AST, or an
/// AST that does not decode) is read from its text by the small formatter
/// below: simple arguments, `select`, `plural` with exact (`=n`) cases, `#`
/// and apostrophe quoting, with English plural rules.
library;

import 'package:plux_flutter/src/l10n/l10n.dart';
import 'package:plux_flutter/src/l10n/message.dart';
import 'package:plux_flutter/src/pxl/decimal.dart' as pxl;
import 'package:plux_flutter/src/pxl/values.dart';

/// Formats [pattern] with [args]; an argument that is missing renders as
/// an empty string, and a malformed pattern renders as written. It never
/// throws.
///
/// When [ast] is given it is decoded and evaluated with the CLDR data
/// [locale] (null: the English fallback of `evaluateMessage`) instead of
/// reading [pattern]. An AST that does not decode is reported to [onError]
/// (PLX-4402, the runtime's message-format failure) and [pattern] is used.
String formatMessage(
  String pattern,
  Map<String, Object?> args, {
  List<int>? ast,
  PluxLocaleData? locale,
  void Function(String problem)? onError,
}) {
  if (ast != null) {
    final nodes = decodeMessage(ast);
    if (nodes != null) {
      return evaluateMessage(nodes, locale, {
        for (final e in args.entries) e.key: _messageArg(e.value),
      });
    }
    onError?.call('the message AST is malformed');
  }
  try {
    final p = _Parser(pattern, args);
    final out = p.message(null);
    return p.pos == pattern.length ? out : pattern;
  } on FormatException {
    return pattern;
  }
}

/// Converts a PXL value to an argument of the message evaluator.
Object? _messageArg(Object? v) {
  switch (v) {
    case final pxl.Decimal d:
      return PluxDecimal.parse(d.toString());
    case final PxlDate d:
      final (y, m, day) = civilFromDays(d.days);
      return PluxCalendarDate.gregorian(y, m, day);
    case final PxlDateTime t:
      final (days, ms) = t.local;
      final (y, m, day) = civilFromDays(days);
      return PluxCalendarDate.gregorian(
        y,
        m,
        day,
        ms ~/ msPerHour,
        ms % msPerHour ~/ msPerMinute,
        ms % msPerMinute ~/ 1000,
      );
    case String() || bool() || int() || double():
      return v;
    default:
      return null;
  }
}

final class _Parser {
  _Parser(this.src, this.args);

  final String src;
  final Map<String, Object?> args;
  int pos = 0;

  bool get _done => pos >= src.length;
  String get _c => src[pos];

  /// A message up to an unmatched `}`; [hash] replaces `#` inside plurals.
  String message(String? hash) {
    final b = StringBuffer();
    while (!_done) {
      final c = _c;
      if (c == '}') break;
      if (c == '{') {
        b.write(argument());
      } else if (c == '#' && hash != null) {
        b.write(hash);
        pos++;
      } else if (c == "'") {
        b.write(quoted());
      } else {
        b.write(c);
        pos++;
      }
    }
    return b.toString();
  }

  /// `''` is an apostrophe; `'{...}'` quotes syntax characters.
  String quoted() {
    pos++;
    if (!_done && _c == "'") {
      pos++;
      return "'";
    }
    if (_done || !'{}#|'.contains(_c)) return "'";
    final end = src.indexOf("'", pos);
    if (end < 0) {
      final rest = src.substring(pos);
      pos = src.length;
      return rest;
    }
    final text = src.substring(pos, end);
    pos = end + 1;
    return text;
  }

  String argument() {
    pos++; // {
    skip();
    final name = word();
    skip();
    if (!_done && _c == '}') {
      pos++;
      return _text(args[name]);
    }
    expect(',');
    skip();
    final kind = word();
    skip();
    if (!_done && _c == '}') {
      pos++;
      return _text(args[name]);
    }
    expect(',');
    final cases = <String, String>{};
    final value = args[name];
    final hash = kind == 'plural' ? _text(value) : null;
    skip();
    if (kind == 'plural' && src.startsWith('offset:', pos)) {
      throw const FormatException('plural offsets are not supported');
    }
    while (!_done && _c != '}') {
      final key = word();
      skip();
      expect('{');
      cases[key] = message(hash);
      expect('}');
      skip();
    }
    expect('}');
    return switch (kind) {
      'plural' =>
        cases['=${_text(value)}'] ??
            cases[value is num && value == 1 ? 'one' : 'other'] ??
            cases['other'] ??
            '',
      'select' => cases[_text(value)] ?? cases['other'] ?? '',
      _ => throw FormatException('unsupported argument type $kind'),
    };
  }

  String word() {
    final start = pos;
    while (!_done && !' ,{}\t\n'.contains(_c)) {
      pos++;
    }
    if (start == pos) throw const FormatException('name expected');
    return src.substring(start, pos);
  }

  void skip() {
    while (!_done && ' \t\n'.contains(_c)) {
      pos++;
    }
  }

  void expect(String c) {
    if (_done || _c != c) throw FormatException('$c expected');
    pos++;
  }

  static String _text(Object? v) => switch (v) {
    null => '',
    double() when v == v.truncateToDouble() && v.abs() < 1e15 =>
      v.toInt().toString(),
    _ => v.toString(),
  };
}
