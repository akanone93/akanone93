// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Reading the top-level sections of a JSON object on demand: a locale
/// file is mostly currency and territory names that most pages never read,
/// so decoding it whole costs time and memory at every start.
library;

import 'dart:convert';
import 'dart:typed_data';

/// The top-level sections of a locale file, read on demand.
abstract interface class JsonSections {
  /// The sections of an already decoded object.
  const factory JsonSections.of(Map<String, Object?> json) = _DecodedSections;

  /// The sections of the JSON object in [bytes]: one pass finds where each
  /// top-level value starts and ends, and a section is decoded only when
  /// read. Throws a [FormatException] when [bytes] are not a JSON object.
  factory JsonSections.scan(Uint8List bytes) = _ByteSections;

  /// The names of the sections.
  Iterable<String> get keys;

  /// The decoded value of section [key], or null.
  Object? operator [](String key);
}

/// Sections of an already decoded object.
final class _DecodedSections implements JsonSections {
  const _DecodedSections(this._json);

  final Map<String, Object?> _json;

  @override
  Iterable<String> get keys => _json.keys;

  @override
  Object? operator [](String key) => _json[key];
}

/// Sections found in JSON bytes.
final class _ByteSections implements JsonSections {
  _ByteSections(this._bytes) {
    _scan();
  }

  final Uint8List _bytes;
  final Map<String, (int, int)> _ranges = {};

  @override
  Iterable<String> get keys => _ranges.keys;

  @override
  Object? operator [](String key) {
    final r = _ranges[key];
    if (r == null) return null;
    return jsonDecode(utf8.decode(Uint8List.sublistView(_bytes, r.$1, r.$2)));
  }

  void _scan() {
    var i = _skipSpace(0);
    if (i >= _bytes.length || _bytes[i] != _brace) _bad(i);
    i = _skipSpace(i + 1);
    if (i < _bytes.length && _bytes[i] == _closeBrace) return;
    while (true) {
      if (i >= _bytes.length || _bytes[i] != _quote) _bad(i);
      final keyEnd = _skipString(i);
      final key = jsonDecode(String.fromCharCodes(_bytes, i, keyEnd));
      if (key is! String) _bad(i);
      i = _skipSpace(keyEnd);
      if (i >= _bytes.length || _bytes[i] != _colon) _bad(i);
      final start = _skipSpace(i + 1);
      final end = _skipValue(start);
      _ranges[key] = (start, end);
      i = _skipSpace(end);
      if (i < _bytes.length && _bytes[i] == _comma) {
        i = _skipSpace(i + 1);
        continue;
      }
      if (i < _bytes.length && _bytes[i] == _closeBrace) return;
      _bad(i);
    }
  }

  Never _bad(int at) =>
      throw FormatException('not a JSON object of sections', _bytes, at);

  int _skipSpace(int i) {
    while (i < _bytes.length &&
        (_bytes[i] == 0x20 ||
            _bytes[i] == 0x0a ||
            _bytes[i] == 0x0d ||
            _bytes[i] == 0x09)) {
      i++;
    }
    return i;
  }

  /// The index after the string that starts at [i].
  int _skipString(int i) {
    i++;
    while (i < _bytes.length) {
      final b = _bytes[i++];
      if (b == _backslash) {
        i++;
      } else if (b == _quote) {
        return i;
      }
    }
    return _bad(i);
  }

  /// The index after the value that starts at [i].
  int _skipValue(int i) {
    if (i >= _bytes.length) _bad(i);
    if (_bytes[i] == _quote) return _skipString(i);
    var depth = 0;
    while (i < _bytes.length) {
      final b = _bytes[i];
      if (b == _quote) {
        i = _skipString(i);
        if (depth == 0) return i;
        continue;
      }
      if (b == _brace || b == _bracket) {
        depth++;
      } else if (b == _closeBrace || b == _closeBracket) {
        if (depth == 0) return i;
        depth--;
        if (depth == 0) return i + 1;
      } else if (depth == 0 && (b == _comma || _isSpace(b))) {
        return i;
      }
      i++;
    }
    return depth == 0 ? i : _bad(i);
  }

  static bool _isSpace(int b) =>
      b == 0x20 || b == 0x0a || b == 0x0d || b == 0x09;

  static const _quote = 0x22;
  static const _backslash = 0x5c;
  static const _brace = 0x7b;
  static const _closeBrace = 0x7d;
  static const _bracket = 0x5b;
  static const _closeBracket = 0x5d;
  static const _colon = 0x3a;
  static const _comma = 0x2c;
}
