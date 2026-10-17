// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// ICU MessageFormat on the device (ADR-0066, I18N-001, I18N-011): the decoder
/// of the compact binary AST the compiler stores in the bundle, and its
/// evaluator. It is the Dart twin of Go's `backend/internal/l10n/mf` and gives
/// the same text for the same AST, locale data and arguments (QA-003). The
/// device never parses message text.
library;

import 'dart:convert';
import 'dart:typed_data';

import 'package:plux_flutter/src/l10n/l10n.dart';

/// The kind of a message node; the values are the tags of the encoding.
enum MessageNodeKind {
  /// Literal text.
  text,

  /// A plain argument, `{name}`.
  arg,

  /// `{name, number[, style]}`.
  number,

  /// `{name, date[, style]}`.
  date,

  /// `{name, time[, style]}`.
  time,

  /// `plural` or, with [MessageNode.ordinal], `selectordinal`.
  plural,

  /// `select`.
  select,

  /// `#` inside a plural.
  pound,
}

/// One element of a decoded message.
final class MessageNode {
  /// Creates a node.
  MessageNode(
    this.kind, {
    this.text = '',
    this.name = '',
    this.style = '',
    this.ordinal = false,
    this.offset = 0,
    this.cases = const [],
  });

  /// The kind.
  final MessageNodeKind kind;

  /// The literal of a text node.
  final String text;

  /// The argument name.
  final String name;

  /// The style: empty, a named style, or `::` and a skeleton.
  final String style;

  /// Whether a plural uses ordinal rules.
  final bool ordinal;

  /// The plural offset.
  final int offset;

  /// The branches of a plural or select.
  final List<MessageCase> cases;
}

/// One branch of a plural or select.
final class MessageCase {
  /// Creates a branch.
  const MessageCase(this.selector, this.body);

  /// `other`, `one`, `=0`, or a select word.
  final String selector;

  /// The branch's message.
  final List<MessageNode> body;
}

const _astVersion = 1;
const _maxDepth = 17;

/// Decodes a message AST written by the compiler, or returns null when the
/// bytes are malformed. It never throws.
List<MessageNode>? decodeMessage(List<int> bytes) {
  try {
    if (bytes.isEmpty || bytes[0] != _astVersion) return null;
    final r = _Reader(Uint8List.fromList(bytes));
    r.pos = 1;
    final nodes = r.nodes(0);
    return r.pos == bytes.length ? nodes : null;
  } on _Malformed {
    return null;
  } on Object {
    return null;
  }
}

final class _Malformed implements Exception {
  const _Malformed();
}

final class _Reader {
  _Reader(this.b);

  final Uint8List b;
  int pos = 0;

  int uvarint() {
    var v = 0;
    var shift = 0;
    for (var i = 0; i < 9; i++) {
      if (pos >= b.length) throw const _Malformed();
      final c = b[pos++];
      v |= (c & 0x7f) << shift;
      if (c < 0x80) return v;
      shift += 7;
    }
    throw const _Malformed();
  }

  int byte() {
    if (pos >= b.length) throw const _Malformed();
    return b[pos++];
  }

  String string() {
    final n = uvarint();
    if (n > b.length - pos) throw const _Malformed();
    final s = utf8.decode(
      Uint8List.sublistView(b, pos, pos + n),
      allowMalformed: false,
    );
    pos += n;
    return s;
  }

  List<MessageNode> nodes(int depth) {
    if (depth > _maxDepth) throw const _Malformed();
    final count = uvarint();
    if (count > b.length - pos) throw const _Malformed();
    final out = <MessageNode>[];
    for (var i = 0; i < count; i++) {
      final tag = byte();
      switch (tag) {
        case 1:
          out.add(MessageNode(MessageNodeKind.text, text: string()));
        case 2:
          out.add(MessageNode(MessageNodeKind.arg, name: string()));
        case 3 || 4 || 5:
          final name = string();
          final style = string();
          out.add(
            MessageNode(
              tag == 3
                  ? MessageNodeKind.number
                  : tag == 4
                  ? MessageNodeKind.date
                  : MessageNodeKind.time,
              name: name,
              style: style,
            ),
          );
        case 6:
          final name = string();
          final ordinal = byte() == 1;
          final offset = uvarint();
          if (offset > 0x7fffffff) throw const _Malformed();
          out.add(
            MessageNode(
              MessageNodeKind.plural,
              name: name,
              ordinal: ordinal,
              offset: offset,
              cases: cases(depth),
            ),
          );
        case 7:
          final name = string();
          out.add(
            MessageNode(
              MessageNodeKind.select,
              name: name,
              cases: cases(depth),
            ),
          );
        case 8:
          out.add(MessageNode(MessageNodeKind.pound));
        default:
          throw const _Malformed();
      }
    }
    return out;
  }

  List<MessageCase> cases(int depth) {
    final count = uvarint();
    if (count > b.length - pos) throw const _Malformed();
    final out = <MessageCase>[];
    for (var i = 0; i < count; i++) {
      final sel = string();
      out.add(MessageCase(sel, nodes(depth + 1)));
    }
    return out;
  }
}

/// Evaluates a decoded message. [args] hold `String`, `bool`, `int`,
/// `double`, [PluxDecimal] (a number whose fraction digits count) and
/// [PluxCalendarDate]; a string is also read as a number where a number is
/// needed. An argument that is missing, or a node that cannot be evaluated,
/// renders as nothing; the message always has a rendering and this never
/// throws.
///
/// [locale] is the CLDR data to format with. Until the runtime loads locale
/// data (L4), a null [locale] formats in the plain English fallback: digits
/// as written, `one` for exactly 1, ISO dates.
String evaluateMessage(
  List<MessageNode> nodes,
  PluxLocaleData? locale,
  Map<String, Object?> args,
) {
  final b = StringBuffer();
  _Eval(locale, args).nodes(b, nodes, null);
  return b.toString();
}

final class _Eval {
  _Eval(this.loc, this.args);

  final PluxLocaleData? loc;
  final Map<String, Object?> args;

  void nodes(StringBuffer b, List<MessageNode> nodes, String? pound) {
    for (final n in nodes) {
      switch (n.kind) {
        case MessageNodeKind.text:
          b.write(n.text);
        case MessageNodeKind.pound:
          if (pound != null) b.write(pound);
        case MessageNodeKind.arg:
          _plain(b, n);
        case MessageNodeKind.number:
          _number(b, n);
        case MessageNodeKind.date || MessageNodeKind.time:
          _date(b, n);
        case MessageNodeKind.plural:
          _plural(b, n);
        case MessageNodeKind.select:
          _select(b, n, pound);
      }
    }
  }

  String _digits(PluxDecimal d, [NumberOptions o = const NumberOptions()]) =>
      loc == null ? d.toString() : loc!.formatNumber(d, o);

  void _plain(StringBuffer b, MessageNode n) {
    final v = args[n.name];
    if (v == null) return;
    try {
      if (v is String) {
        b.write(v);
      } else if (v is bool) {
        b.write(v ? 'true' : 'false');
      } else if (v is PluxCalendarDate) {
        _date(b, MessageNode(MessageNodeKind.date, name: n.name));
      } else {
        final d = _decimal(v);
        if (d != null) b.write(_digits(d));
      }
    } on L10nException {
      return;
    }
  }

  void _number(StringBuffer b, MessageNode n) {
    final d = _decimal(args[n.name]);
    if (d == null) return;
    try {
      b.write(_formatNumber(d, n.style));
    } on L10nException {
      return;
    }
  }

  String _formatNumber(PluxDecimal d, String style) {
    final l = loc;
    if (l == null) return d.toString();
    if (style == 'integer') {
      return l.formatNumber(
        d,
        const NumberOptions(fraction: FractionDigits(0, 0)),
      );
    }
    if (style == 'percent') return l.formatPercent(d);
    if (!style.startsWith('::')) return l.formatNumber(d);
    final sk = _NumberSkeleton.parse(style.substring(2));
    if (sk == null) return l.formatNumber(d);
    final frac = sk.fractionSet ? FractionDigits(sk.min, sk.max) : null;
    final opts = NumberOptions(fraction: frac, noGrouping: sk.noGroup);
    if (sk.compact) {
      return l.formatCompact(d, NumberOptions(noGrouping: sk.noGroup));
    }
    if (sk.percent) return l.formatPercent(d, opts);
    if (sk.currency.isNotEmpty) {
      return l.formatMoney(
        d,
        sk.currency,
        MoneyOptions(fraction: frac, noGrouping: sk.noGroup),
      );
    }
    return l.formatNumber(d, opts);
  }

  void _date(StringBuffer b, MessageNode n) {
    final v = args[n.name];
    if (v is! PluxCalendarDate) return;
    final l = loc;
    if (l == null) {
      b.write(_isoDate(v, n.kind));
      return;
    }
    final PluxDateOptions opts;
    if (n.style.startsWith('::')) {
      opts = PluxDateOptions(skeleton: n.style.substring(2));
    } else if (n.kind == MessageNodeKind.date) {
      opts = PluxDateOptions(dateStyle: n.style.isEmpty ? 'medium' : n.style);
    } else {
      opts = PluxDateOptions(timeStyle: n.style.isEmpty ? 'medium' : n.style);
    }
    try {
      b.write(l.formatDate(v, 'gregorian', opts));
    } on L10nException {
      return;
    }
  }

  void _select(StringBuffer b, MessageNode n, String? pound) {
    final v = args[n.name];
    final String key;
    if (v is String) {
      key = v;
    } else if (v is bool) {
      key = v ? 'true' : 'false';
    } else {
      return;
    }
    for (final want in [key, 'other']) {
      for (final c in n.cases) {
        if (c.selector == want) {
          nodes(b, c.body, pound);
          return;
        }
      }
    }
  }

  void _plural(StringBuffer b, MessageNode n) {
    final d = _decimal(args[n.name]);
    if (d == null) return;
    final shifted = _subtract(d, n.offset);
    String text;
    try {
      text = _digits(shifted);
    } on L10nException {
      text = '';
    }
    final whole = _whole(d);
    if (whole != null) {
      for (final c in n.cases) {
        if (c.selector.startsWith('=') &&
            _canonical(c.selector.substring(1)) == whole) {
          nodes(b, c.body, text);
          return;
        }
      }
    }
    final cat = _category(shifted, ordinal: n.ordinal);
    for (final want in [cat, 'other']) {
      for (final c in n.cases) {
        if (c.selector == want) {
          nodes(b, c.body, text);
          return;
        }
      }
    }
  }

  String _category(PluxDecimal d, {required bool ordinal}) {
    final l = loc;
    if (l != null) {
      return (ordinal ? l.pluralOrdinal(d) : l.pluralCardinal(d)).name;
    }
    // The English fallback until locale data is loaded.
    final whole = d.frac.isEmpty && !d.isNegative ? d.intDigits : null;
    if (!ordinal) return whole == '1' ? 'one' : 'other';
    if (whole == null) return 'other';
    final n = int.tryParse(whole);
    if (n == null) return 'other';
    final m10 = n % 10;
    final m100 = n % 100;
    if (m10 == 1 && m100 != 11) return 'one';
    if (m10 == 2 && m100 != 12) return 'two';
    if (m10 == 3 && m100 != 13) return 'few';
    return 'other';
  }
}

String _canonical(String digits) {
  final t = digits.replaceFirst(RegExp('^0+'), '');
  return t.isEmpty ? '0' : t;
}

/// The digits of [d] when it is a non-negative integer value, 1.0 included.
String? _whole(PluxDecimal d) {
  if (d.isNegative) return null;
  if (d.frac.replaceAll('0', '').isNotEmpty) return null;
  return _canonical(d.intDigits);
}

/// [d] less a whole [offset], keeping d's fraction digits.
PluxDecimal _subtract(PluxDecimal d, int offset) {
  if (offset == 0) return d;
  final scale = d.frac.length;
  var u = BigInt.parse('${d.intDigits}${d.frac}');
  if (d.isNegative) u = -u;
  u -= BigInt.from(offset) * BigInt.from(10).pow(scale);
  var digits = u.abs().toString();
  if (digits.length <= scale) {
    digits = '${'0' * (scale - digits.length + 1)}$digits';
  }
  var text = digits.substring(0, digits.length - scale);
  if (scale > 0) text += '.${digits.substring(digits.length - scale)}';
  if (u.isNegative) text = '-$text';
  return PluxDecimal.parse(text);
}

PluxDecimal? _decimal(Object? v) {
  try {
    return switch (v) {
      final PluxDecimal d => d,
      final int i => PluxDecimal.fromInt(i),
      final double x when x.isFinite => PluxDecimal.parse(
        x == x.truncateToDouble() && x.abs() < 1e15
            ? x.toInt().toString()
            : x.toString(),
      ),
      final String s => PluxDecimal.parse(s),
      _ => null,
    };
  } on L10nException {
    return null;
  }
}

String _two(int n) => n.toString().padLeft(2, '0');

String _isoDate(PluxCalendarDate d, MessageNodeKind kind) {
  final date =
      '${d.year.toString().padLeft(4, '0')}-${_two(d.month)}-${_two(d.day)}';
  final time = '${_two(d.hour)}:${_two(d.minute)}:${_two(d.second)}';
  return kind == MessageNodeKind.time ? time : date;
}

/// The subset of ICU number skeletons Plux supports; the same as Go's.
final class _NumberSkeleton {
  bool percent = false;
  bool compact = false;
  bool noGroup = false;
  String currency = '';
  bool fractionSet = false;
  int min = 0;
  int max = 0;

  static _NumberSkeleton? parse(String s) {
    final sk = _NumberSkeleton();
    for (final tok in s.split(RegExp(r'\s+')).where((t) => t.isNotEmpty)) {
      if (tok == 'percent') {
        sk.percent = true;
      } else if (tok == 'compact-short') {
        sk.compact = true;
      } else if (tok == 'group-off') {
        sk.noGroup = true;
      } else if (tok == 'precision-integer') {
        sk
          ..fractionSet = true
          ..min = 0
          ..max = 0;
      } else if (tok.startsWith('currency/')) {
        sk.currency = tok.substring('currency/'.length);
      } else if (tok.startsWith('.') && tok.length > 1) {
        final zeros = tok.substring(1).replaceFirst(RegExp('^0+'), '');
        sk
          ..fractionSet = true
          ..min = tok.length - 1 - zeros.length
          ..max = tok.length - 1;
      } else {
        return null;
      }
    }
    return sk;
  }
}
