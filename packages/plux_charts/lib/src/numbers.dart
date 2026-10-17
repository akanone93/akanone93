// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Number formatting and axis maths of the charts. Numbers appear as plain
/// digits, with a point as the decimal separator and no grouping: chart
/// labels are not localized money or measures.
library;

import 'dart:math' as math;

/// [v] as plain digits: whole numbers without a fraction, others with up to
/// two fraction digits and no trailing zeros. Non-finite values read as 0.
String formatNumber(double v) {
  if (!v.isFinite) return '0';
  if (v == v.roundToDouble() && v.abs() < 1e15) return '${v.round()}';
  var s = v.toStringAsFixed(2);
  if (s.contains('.')) {
    s = s.replaceFirst(RegExp(r'0+$'), '').replaceFirst(RegExp(r'\.$'), '');
  }
  return s == '-0' ? '0' : s;
}

/// [v] with non-finite values replaced by 0.
double finite(double v) => v.isFinite ? v : 0;

/// The axis of a chart: its bounds and tick values.
final class ChartAxis {
  /// Creates an axis from the bounds and the step between ticks.
  const ChartAxis(this.min, this.max, this.step);

  /// The axis for data between [lo] and [hi]; it always includes zero.
  factory ChartAxis.fit(double lo, double hi) {
    var low = math.min(0.0, lo);
    var high = math.max(0.0, hi);
    if (high == low) high = low + 1;
    final raw = (high - low) / 4;
    final magnitude = math
        .pow(10, (math.log(raw) / math.ln10).floor())
        .toDouble();
    final n = raw / magnitude;
    final step =
        (n <= 1
            ? 1.0
            : n <= 2
            ? 2.0
            : n <= 5
            ? 5.0
            : 10.0) *
        magnitude;
    low = (low / step).floor() * step;
    high = (high / step).ceil() * step;
    return ChartAxis(low, high, step);
  }

  /// The lowest value.
  final double min;

  /// The highest value.
  final double max;

  /// The distance between ticks.
  final double step;

  /// The tick values from [min] to [max].
  List<double> get ticks {
    final count = ((max - min) / step).round();
    return [for (var i = 0; i <= count; i++) min + i * step];
  }

  /// Where [v] lies between 0 (at [min]) and 1 (at [max]).
  double unit(double v) => max == min ? 0 : (v - min) / (max - min);
}

/// [template] with each `{key}` of [values] replaced; unknown keys stay.
String fillTemplate(String template, Map<String, String> values) => template
    .replaceAllMapped(RegExp(r'\{(\w+)\}'), (m) => values[m[1]] ?? m[0]!);
