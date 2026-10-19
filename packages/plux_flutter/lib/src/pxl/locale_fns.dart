// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The `format` and `l10n` groups of PXL (PXL-006, I18N-001, I18N-011) on the
/// localisation engine, identical to Go's `backend/internal/pxl/localefns.go`.
library;

import 'package:plux_flutter/src/l10n/l10n.dart';
import 'package:plux_flutter/src/pxl/builtins.dart';
import 'package:plux_flutter/src/pxl/decimal.dart' as pxl;
import 'package:plux_flutter/src/pxl/tables.g.dart';
import 'package:plux_flutter/src/pxl/values.dart';
import 'package:plux_flutter/src/pxl/vm.dart';

PluxLocaleData _locale(Vm vm) {
  final l = vm.locale?.locale;
  if (l == null) {
    throw const PxlError(
      PxlErrorKind.unsupported,
      'the evaluation has no locale',
    );
  }
  return l;
}

PxlError _bad(String message) =>
    PxlError(PxlErrorKind.invalidArgument, message);

/// Converts a number argument for the engine.
PluxDecimal _decimalOf(Object? v) {
  try {
    return switch (v) {
      final int i => PluxDecimal.fromInt(i),
      final double d when d.isFinite => PluxDecimal.parse(pxl.plainShortest(d)),
      final pxl.Decimal d => PluxDecimal.parse(d.toString()),
      _ => throw _bad('not a number'),
    };
  } on L10nException catch (e) {
    throw _bad(e.message);
  }
}

T _guard<T>(T Function() f) {
  try {
    return f();
  } on L10nException catch (e) {
    throw _bad(e.message);
  }
}

Object? _number(Vm vm, List<Object?> a) {
  final loc = _locale(vm);
  final d = _decimalOf(a[0]);
  NumberOptions opts = const NumberOptions();
  if (a.length > 1) {
    final n = a[1]! as int;
    if (n < 0 || n > 20) {
      throw _bad('fraction digits $n outside 0 to 20');
    }
    opts = NumberOptions(fraction: FractionDigits(n, n));
  }
  return _guard(() => loc.formatNumber(d, opts));
}

Object? _date(Vm vm, List<Object?> a) {
  final loc = _locale(vm);
  final style = a.length > 1 ? a[1]! as String : 'medium';
  final int days;
  var secs = 0;
  var zone = '';
  var hasTime = false;
  final v = a[0];
  if (v is PxlDate) {
    days = v.days;
  } else {
    final dt = v! as PxlDateTime;
    final (d, ms) = dt.local;
    days = d;
    secs = ms ~/ 1000;
    hasTime = true;
    final off = dt.offset.abs();
    zone =
        'GMT${dt.offset < 0 ? '-' : '+'}'
        '${(off ~/ 60).toString().padLeft(2, '0')}:'
        '${(off % 60).toString().padLeft(2, '0')}';
  }
  final (y, m, d) = civilFromDays(days);
  final date = PluxCalendarDate.gregorian(
    y,
    m,
    d,
    secs ~/ 3600,
    secs ~/ 60 % 60,
    secs % 60,
  );
  return _guard(
    () => loc.formatDate(
      date,
      'gregorian',
      PluxDateOptions(
        dateStyle: style,
        timeStyle: hasTime ? style : '',
        zoneName: zone,
      ),
    ),
  );
}

Builtin _one(String Function(PluxLocaleData l, PluxDecimal d) f) =>
    (vm, a) => _guard(() => f(_locale(vm), _decimalOf(a[0])));

Object? _translate(Vm vm, List<Object?> a) {
  final translate = vm.locale?.translate;
  if (translate == null) {
    throw const PxlError(
      PxlErrorKind.unsupported,
      'the evaluation has no translations',
    );
  }
  final args = <String, String>{};
  if (a.length > 1) {
    for (final e in (a[1]! as Map<String, Object?>).entries) {
      args[e.key] = e.value is String ? e.value! as String : '';
    }
  }
  final key = a[0]! as String;
  return translate(key, args) ?? (throw _bad('translation "$key" is missing'));
}

/// The implementations of the format and l10n groups, by overload key.
Map<String, Builtin> localeFns() {
  final percent = _one((l, d) => l.formatPercent(d));
  final compact = _one((l, d) => l.formatCompact(d));
  return {
    'format.number(int)': _number,
    'format.number(double)': _number,
    'format.number(decimal)': _number,
    'format.number(double,int)': _number,
    'format.number(decimal,int)': _number,
    'format.percent(double)': percent,
    'format.percent(decimal)': percent,
    'format.compact(int)': compact,
    'format.compact(double)': compact,
    'format.compact(decimal)': compact,
    'format.date(date)': _date,
    'format.date(dateTime)': _date,
    'format.date(date,DateStyle)': _date,
    'format.date(dateTime,DateStyle)': _date,
    'format.money(money)': (vm, a) {
      final loc = _locale(vm);
      final m = a[0]! as Money;
      return _guard(() => loc.formatMoney(_decimalOf(m.amount), m.currency));
    },
    't(string)': _translate,
    't(string,map<string,string>)': _translate,
    'plural(int,map<string,string>)': (vm, a) {
      final loc = _locale(vm);
      final forms = a[1]! as Map<String, Object?>;
      final cat = loc.pluralCardinal(PluxDecimal.fromInt(a[0]! as int)).name;
      for (final k in [cat, 'other']) {
        final s = forms[k];
        if (s is String) return s;
      }
      throw _bad('no form for the plural category "$cat" and no other form');
    },
  };
}
