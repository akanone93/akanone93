// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The painters of the charts: plain `CustomPainter`s over immutable data,
/// with the colours and text style handed in from the theme.
library;

import 'dart:math' as math;

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:plux_charts/src/numbers.dart';

/// One named series of values.
@immutable
final class ChartSeriesData {
  /// Creates a series.
  const ChartSeriesData(this.name, this.values);

  /// The series' name.
  final String name;

  /// The values, with non-finite ones already replaced.
  final List<double> values;

  @override
  bool operator ==(Object other) =>
      other is ChartSeriesData &&
      other.name == name &&
      listEquals(other.values, values);

  @override
  int get hashCode => Object.hash(name, Object.hashAll(values));
}

/// The extent of the series' values.
(double, double) _extent(List<ChartSeriesData> series) {
  var lo = 0.0;
  var hi = 0.0;
  for (final s in series) {
    for (final v in s.values) {
      lo = math.min(lo, v);
      hi = math.max(hi, v);
    }
  }
  return (lo, hi);
}

TextPainter _text(String s, TextStyle style, TextScaler scaler) => TextPainter(
  text: TextSpan(text: s, style: style),
  textDirection: TextDirection.ltr,
  textScaler: scaler,
  maxLines: 1,
)..layout();

/// Paints a line chart or a bar chart: grid, value axis, label axis and one
/// line or one bar per series and position. Axes read left to right in every
/// text direction, as numbers do.
final class CartesianPainter extends CustomPainter {
  /// Creates the painter.
  CartesianPainter({
    required this.series,
    required this.labels,
    required this.bars,
    required this.colors,
    required this.gridColor,
    required this.labelStyle,
    required this.textScaler,
  });

  /// The series.
  final List<ChartSeriesData> series;

  /// The label of each position; may be shorter than the series.
  final List<String> labels;

  /// Whether to draw bars rather than lines.
  final bool bars;

  /// The colour of each series, cycling.
  final List<Color> colors;

  /// The colour of the grid.
  final Color gridColor;

  /// The style of axis labels.
  final TextStyle labelStyle;

  /// The text scale.
  final TextScaler textScaler;

  @override
  void paint(Canvas canvas, Size size) {
    final (lo, hi) = _extent(series);
    final axis = ChartAxis.fit(lo, hi);
    final yLabels = [
      for (final t in axis.ticks)
        _text(formatNumber(t), labelStyle, textScaler),
    ];
    final left = yLabels.fold<double>(0, (m, p) => math.max(m, p.width)) + 6;
    final n = math.max(
      labels.length,
      series.fold<int>(0, (m, s) => math.max(m, s.values.length)),
    );
    final xLabels = [
      for (var i = 0; i < n; i++)
        if (i < labels.length) _text(labels[i], labelStyle, textScaler),
    ];
    final bottom = xLabels.isEmpty
        ? 6.0
        : xLabels.fold<double>(0, (m, p) => math.max(m, p.height)) + 8;
    final plot = Rect.fromLTRB(
      left,
      yLabels.isEmpty ? 6 : yLabels.first.height / 2 + 2,
      size.width - 8,
      size.height - bottom,
    );
    if (plot.width <= 0 || plot.height <= 0) return;
    double yOf(double v) => plot.bottom - axis.unit(v) * plot.height;

    final grid = Paint()
      ..color = gridColor
      ..strokeWidth = 1;
    final ticks = axis.ticks;
    for (var i = 0; i < ticks.length; i++) {
      final y = yOf(ticks[i]);
      canvas.drawLine(Offset(plot.left, y), Offset(plot.right, y), grid);
      final p = yLabels[i];
      p.paint(canvas, Offset(plot.left - 6 - p.width, y - p.height / 2));
    }

    final slot = n <= 1 ? plot.width : plot.width / (bars ? n : n - 1);
    double xOf(int i) => bars
        ? plot.left + slot * (i + 0.5)
        : (n <= 1 ? plot.center.dx : plot.left + slot * i);

    var stride = 1;
    if (xLabels.isNotEmpty) {
      final widest = xLabels.fold<double>(0, (m, p) => math.max(m, p.width));
      stride = math.max(1, ((widest + 6) / math.max(slot, 1)).ceil());
    }
    for (var i = 0; i < xLabels.length; i += stride) {
      final p = xLabels[i];
      final x = (xOf(i) - p.width / 2)
          .clamp(0.0, math.max(0.0, size.width - p.width))
          .toDouble();
      p.paint(canvas, Offset(x, plot.bottom + 4));
    }

    final zero = yOf(0);
    for (var s = 0; s < series.length; s++) {
      final color = colors[s % colors.length];
      final values = series[s].values;
      if (bars) {
        final width = slot * 0.8 / series.length;
        final paint = Paint()..color = color;
        for (var i = 0; i < values.length; i++) {
          final x = xOf(i) - slot * 0.4 + width * s;
          final y = yOf(values[i]);
          canvas.drawRect(
            Rect.fromLTRB(x, math.min(y, zero), x + width, math.max(y, zero)),
            paint,
          );
        }
      } else {
        final line = Paint()
          ..color = color
          ..style = PaintingStyle.stroke
          ..strokeWidth = 2
          ..strokeJoin = StrokeJoin.round;
        final path = Path();
        for (var i = 0; i < values.length; i++) {
          final o = Offset(xOf(i), yOf(values[i]));
          if (i == 0) {
            path.moveTo(o.dx, o.dy);
          } else {
            path.lineTo(o.dx, o.dy);
          }
        }
        canvas.drawPath(path, line);
        final dot = Paint()..color = color;
        for (var i = 0; i < values.length; i++) {
          canvas.drawCircle(Offset(xOf(i), yOf(values[i])), 3, dot);
        }
      }
    }
  }

  @override
  bool shouldRepaint(CartesianPainter old) =>
      !listEquals(old.series, series) ||
      !listEquals(old.labels, labels) ||
      old.bars != bars ||
      !listEquals(old.colors, colors) ||
      old.gridColor != gridColor ||
      old.labelStyle != labelStyle ||
      old.textScaler != textScaler;
}

/// Paints a pie: one slice per positive value, clockwise from the top.
final class PiePainter extends CustomPainter {
  /// Creates the painter.
  PiePainter({
    required this.values,
    required this.colors,
    required this.emptyColor,
    required this.separator,
  });

  /// The size of each slice; negative values count as zero.
  final List<double> values;

  /// The colour of each slice, cycling.
  final List<Color> colors;

  /// The ring drawn when there is nothing to show.
  final Color emptyColor;

  /// The colour of the lines between slices.
  final Color separator;

  @override
  void paint(Canvas canvas, Size size) {
    final radius = math.min(size.width, size.height) / 2 - 2;
    if (radius <= 0) return;
    final rect = Rect.fromCircle(
      center: size.center(Offset.zero),
      radius: radius,
    );
    final total = values.fold<double>(0, (t, v) => t + math.max(0, v));
    if (total <= 0) {
      canvas.drawCircle(
        rect.center,
        radius,
        Paint()
          ..color = emptyColor
          ..style = PaintingStyle.stroke
          ..strokeWidth = 2,
      );
      return;
    }
    var start = -math.pi / 2;
    for (var i = 0; i < values.length; i++) {
      final sweep = math.max(0, values[i]) / total * 2 * math.pi;
      if (sweep <= 0) continue;
      canvas.drawArc(
        rect,
        start,
        sweep,
        true,
        Paint()..color = colors[i % colors.length],
      );
      canvas.drawArc(
        rect,
        start,
        sweep,
        true,
        Paint()
          ..color = separator
          ..style = PaintingStyle.stroke
          ..strokeWidth = 1.5,
      );
      start += sweep;
    }
  }

  @override
  bool shouldRepaint(PiePainter old) =>
      !listEquals(old.values, values) ||
      !listEquals(old.colors, colors) ||
      old.emptyColor != emptyColor ||
      old.separator != separator;
}

/// Paints a sparkline: the values as one line scaled to the box, the last
/// point marked.
final class SparklinePainter extends CustomPainter {
  /// Creates the painter.
  SparklinePainter({required this.values, required this.color});

  /// The values.
  final List<double> values;

  /// The line's colour.
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    if (values.isEmpty) return;
    final lo = values.reduce(math.min);
    final hi = values.reduce(math.max);
    final span = hi - lo;
    const pad = 3.0;
    final w = size.width - 2 * pad;
    final h = size.height - 2 * pad;
    Offset at(int i) => Offset(
      values.length == 1 ? size.width / 2 : pad + w * i / (values.length - 1),
      span == 0 ? size.height / 2 : pad + h * (1 - (values[i] - lo) / span),
    );
    final path = Path();
    for (var i = 0; i < values.length; i++) {
      final o = at(i);
      if (i == 0) {
        path.moveTo(o.dx, o.dy);
      } else {
        path.lineTo(o.dx, o.dy);
      }
    }
    canvas.drawPath(
      path,
      Paint()
        ..color = color
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2
        ..strokeJoin = StrokeJoin.round
        ..strokeCap = StrokeCap.round,
    );
    canvas.drawCircle(at(values.length - 1), 3, Paint()..color = color);
  }

  @override
  bool shouldRepaint(SparklinePainter old) =>
      !listEquals(old.values, values) || old.color != color;
}
