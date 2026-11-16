// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Design tokens as props read them (THM-001, ADR-0032): the token at a
/// path, overlaid by the high-contrast group and then by the selected
/// brand, in the current mode, converted from its W3C form to the PXL
/// value of the prop type it binds (compiler.md §3).
library;

import 'package:plux_flutter/src/bundle/fbs/bundle_fbs_generated.dart' as fbs;
import 'package:plux_flutter/src/pxl/values.dart';
import 'package:plux_flutter/src/render/sections.dart';
import 'package:plux_flutter/src/render/values.dart';

/// Reads the app bundle's tokens for one mode, brand and contrast.
final class TokenReader {
  /// Creates a reader over the [app] bundle's tokens.
  TokenReader({
    required this.app,
    required this.dark,
    required this.brand,
    required this.highContrast,
  });

  /// The app bundle.
  final BundleView app;

  /// Whether dark values apply (THM-002).
  final bool dark;

  /// The brand overlay, or null (THM-003).
  final String? brand;

  /// Whether the high-contrast group applies.
  final bool highContrast;

  /// The token at [path] as a PXL value, or null when no token has it.
  Object? read(String path, ValueResolver resolver) {
    final token = followAliases(path, (p) => _value(p, resolver));
    return token == null ? null : convertToken(token.type, token.value);
  }

  /// The W3C value of the token at [path], before conversion, or null.
  Object? raw(String path, ValueResolver resolver) =>
      followAliases(path, (p) => _value(p, resolver))?.value;

  TokenValue? _value(String path, ValueResolver resolver) {
    final token = _token(path);
    if (token == null) return null;
    return (type: token.type ?? '', value: _raw(token, resolver));
  }

  fbs.Token? _token(String path) =>
      (brand == null ? null : app.token('brands.$brand.$path')) ??
      (highContrast ? app.token('contrast.high.$path') : null) ??
      app.token(path);

  Object? _raw(fbs.Token token, ValueResolver resolver) =>
      resolver.resolve((dark ? token.dark : null) ?? token.light, app.string);

  /// Whether the app has a token at [path].
  bool has(String path) => app.token(path) != null;
}

/// Converts a W3C token value of [type] into the PXL value of the prop
/// type it binds: `color` → color, `dimension` and `number` → double,
/// `fontFamily` → string, `fontWeight` → a FontWeight member, `duration`
/// → duration, `typography` → a TextStyle object, `shadow` → a BoxShadow
/// object. Unknown shapes convert to null.
Object? convertToken(String type, Object? v) => switch (type) {
  'color' => v is String ? parseColor(v) : null,
  'dimension' => _dimension(v),
  'number' => v is num ? v.toDouble() : null,
  'fontFamily' => switch (v) {
    String() => v,
    List<Object?>() when v.isNotEmpty && v.first is String => v.first,
    _ => null,
  },
  'fontWeight' => _fontWeight(v),
  'duration' => _duration(v),
  'typography' => _typography(v),
  'shadow' => _shadow(v),
  _ => null,
};

double? _dimension(Object? v) => switch (v) {
  num() => v.toDouble(),
  {'value': final num n, 'unit': 'rem'} => n * 16,
  {'value': final num n} => n.toDouble(),
  String() => double.tryParse(v.replaceAll(RegExp(r'(px|dp|pt)$'), '')),
  _ => null,
};

String? _fontWeight(Object? v) {
  const names = {
    'thin': 100,
    'hairline': 100,
    'extra-light': 200,
    'ultra-light': 200,
    'light': 300,
    'normal': 400,
    'regular': 400,
    'book': 400,
    'medium': 500,
    'semi-bold': 600,
    'demi-bold': 600,
    'bold': 700,
    'extra-bold': 800,
    'ultra-bold': 800,
    'black': 900,
    'heavy': 900,
  };
  final n = switch (v) {
    int() => v,
    double() => v.round(),
    String() => names[v],
    _ => null,
  };
  if (n == null) return null;
  final w = ((n.clamp(100, 900) + 50) ~/ 100) * 100;
  return 'w$w';
}

PxlDuration? _duration(Object? v) => switch (v) {
  {'value': final num n, 'unit': 's'} => PxlDuration((n * 1000).round()),
  {'value': final num n} => PxlDuration(n.round()),
  String() when v.endsWith('ms') => switch (num.tryParse(
    v.substring(0, v.length - 2),
  )) {
    final n? => PxlDuration(n.round()),
    null => null,
  },
  String() when v.endsWith('s') => switch (num.tryParse(
    v.substring(0, v.length - 1),
  )) {
    final n? => PxlDuration((n * 1000).round()),
    null => null,
  },
  _ => null,
};

Map<String, Object?>? _typography(Object? v) {
  if (v is! Map<String, Object?>) return null;
  final family = v['fontFamily'];
  final families = switch (family) {
    String() => [family],
    List<Object?>() => [
      for (final f in family)
        if (f is String) f,
    ],
    _ => const <String>[],
  };
  return {
    if (families.isNotEmpty) 'fontFamily': families.first,
    if (families.length > 1) 'fontFamilyFallback': families.sublist(1),
    'fontSize': ?_dimension(v['fontSize']),
    'fontWeight': ?_fontWeight(v['fontWeight']),
    'letterSpacing': ?_dimension(v['letterSpacing']),
    if (v['lineHeight'] case final num h) 'height': h.toDouble(),
  };
}

Map<String, Object?>? _shadow(Object? v) {
  if (v is! Map<String, Object?>) return null;
  return {
    if (v['color'] case final String c) 'color': parseColor(c),
    'offset': {
      'dx': _dimension(v['offsetX']) ?? 0.0,
      'dy': _dimension(v['offsetY']) ?? 0.0,
    },
    'blurRadius': ?_dimension(v['blur']),
    'spreadRadius': ?_dimension(v['spread']),
  };
}

/// How many aliases deep a token may refer, as the compiler's contrast check
/// and Studio's canvas follow them.
const maxAliasDepth = 8;

/// How many tokens one read may look up while following aliases, so a theme
/// whose composites refer to each other cannot make a read expensive.
const maxAliasLookups = 64;

final RegExp _alias = RegExp(
  r'^\{([a-zA-Z0-9][a-zA-Z0-9-]*(\.[a-zA-Z0-9][a-zA-Z0-9-]*)*)\}$',
);

/// A token's type and W3C value.
typedef TokenValue = ({String type, Object? value});

/// The token at [path] with its DTCG aliases followed (THM-001): a value
/// `{color.primary}` is the value of the token at `color.primary`, read
/// through the same brand, contrast and mode as the token itself, and so is
/// an alias inside a composite value (`typography`, `shadow`, …). A token
/// without a type of its own takes the type of the token it refers to. An
/// alias to a missing token, deeper than [maxAliasDepth] (a cycle), or past
/// [maxAliasLookups] reads as no token at the top and as null inside a
/// composite.
TokenValue? followAliases(String path, TokenValue? Function(String) lookup) =>
    _AliasWalk(lookup).follow(path, 0);

final class _AliasWalk {
  _AliasWalk(this.lookup);

  final TokenValue? Function(String) lookup;
  int lookups = 0;

  TokenValue? follow(String path, int depth) {
    if (depth > maxAliasDepth || ++lookups > maxAliasLookups) return null;
    final token = lookup(path);
    if (token == null) return null;
    final value = token.value;
    if (value is String) {
      final target = _alias.firstMatch(value)?.group(1);
      if (target == null) return token;
      final resolved = follow(target, depth + 1);
      if (resolved == null) return null;
      return (
        type: token.type.isEmpty ? resolved.type : token.type,
        value: resolved.value,
      );
    }
    return (type: token.type, value: inner(value, depth));
  }

  Object? inner(Object? v, int depth) => switch (v) {
    String() => switch (_alias.firstMatch(v)?.group(1)) {
      null => v,
      final target => follow(target, depth + 1)?.value,
    },
    Map<String, Object?>() => {
      for (final e in v.entries) e.key: inner(e.value, depth),
    },
    List<Object?>() => [for (final x in v) inner(x, depth)],
    _ => v,
  };
}
