// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The widget package: builders that read the nodes of `LineChart`,
/// `BarChart`, `PieChart` and `Sparkline`. The IDs are those of the
/// descriptors in `schema/widgets/layer2`; a test compares them.
library;

import 'package:flutter/widgets.dart';
import 'package:plux_charts/src/charts.dart';
import 'package:plux_charts/src/numbers.dart';
import 'package:plux_charts/src/painters.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// The widget IDs of the package's descriptors.
abstract final class PluxChartIds {
  /// `LineChart`.
  static const int lineChart = 193;

  /// `BarChart`.
  static const int barChart = 194;

  /// `PieChart`.
  static const int pieChart = 195;

  /// `Sparkline`.
  static const int sparkline = 196;
}

/// The chart widgets of Plux pages, to register in
/// `PluxConfig.widgetPackages`.
final class PluxCharts implements PluxWidgetPackage {
  /// Creates the package.
  const PluxCharts();

  @override
  String get name => 'plux_charts';

  @override
  Map<int, NodeBuilder> get builders => {
    PluxChartIds.lineChart: (c) =>
        _cartesian(c, bars: false, widget: 'LineChart'),
    PluxChartIds.barChart: (c) => _cartesian(c, bars: true, widget: 'BarChart'),
    PluxChartIds.pieChart: _pie,
    PluxChartIds.sparkline: _sparkline,
  };
}

ChartSeriesData? _series(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final name = asString(d, f.get(1, 'name'));
  final values = asDoubles(d, f.get(2, 'values'));
  if (name == null || values == null) return null;
  return ChartSeriesData(name, [for (final x in values) finite(x)]);
}

(String, double)? _slice(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final label = asString(d, f.get(1, 'label'));
  final value = asDouble(d, f.get(2, 'value'));
  return label == null || value == null ? null : (label, finite(value));
}

Widget _cartesian(NodeContext c, {required bool bars, required String widget}) {
  final series =
      c.decode(1, (d, v) => asList(d, v, _series)) ??
      c.missing('$widget.series');
  return PluxCartesianChart(
    series: series,
    bars: bars,
    labels: c.decode(2, asStrings) ?? const [],
    title: c.decode(3, asString),
    summaryTemplate: c.decode(4, asString) ?? defaultSeriesSummary,
    showTable: c.decode(5, asBool) ?? false,
  );
}

Widget _pie(NodeContext c) {
  final slices =
      c.decode(1, (d, v) => asList(d, v, _slice)) ??
      c.missing('PieChart.slices');
  return PluxPieChart(
    slices: slices,
    title: c.decode(2, asString),
    summaryTemplate: c.decode(3, asString) ?? defaultSliceSummary,
    showTable: c.decode(4, asBool) ?? false,
  );
}

Widget _sparkline(NodeContext c) {
  final values = c.decode(1, asDoubles) ?? c.missing('Sparkline.values');
  return PluxSparkline(
    values: [for (final x in values) finite(x)],
    semanticLabel: c.decode(2, asString),
    summaryTemplate: c.decode(3, asString) ?? defaultSparklineSummary,
  );
}
