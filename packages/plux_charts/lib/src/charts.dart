// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The chart widgets (WGT-020, A11Y-001): drawn in house on `CustomPainter`,
/// coloured from the theme. Each chart is one semantics node whose label is
/// a text summary per series; `showTable` adds the values as a table, the
/// alternative for people who cannot read the drawing.
library;

import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:plux_charts/src/numbers.dart';
import 'package:plux_charts/src/painters.dart';

/// The default summary of a series of a line or bar chart.
const String defaultSeriesSummary =
    '{name}: {count} values, first {first}, last {last}, '
    'lowest {min}, highest {max}';

/// The default summary of a slice of a pie chart.
const String defaultSliceSummary = '{label}: {value}, {percent} percent';

/// The default summary of a sparkline.
const String defaultSparklineSummary =
    '{count} values, first {first}, last {last}, lowest {min}, highest {max}';

/// The series colours of [scheme], in order.
List<Color> chartPalette(ColorScheme scheme) => [
  scheme.primary,
  scheme.tertiary,
  scheme.secondary,
  scheme.error,
  scheme.primaryContainer,
  scheme.onSurfaceVariant,
];

Map<String, String> _stats(List<double> v) => v.isEmpty
    ? {'count': '0', 'first': '0', 'last': '0', 'min': '0', 'max': '0'}
    : {
        'count': '${v.length}',
        'first': formatNumber(v.first),
        'last': formatNumber(v.last),
        'min': formatNumber(v.reduce(math.min)),
        'max': formatNumber(v.reduce(math.max)),
      };

/// A line chart or a bar chart of series over shared labels.
final class PluxCartesianChart extends StatelessWidget {
  /// Creates a chart; [bars] chooses bars over lines.
  const PluxCartesianChart({
    required this.series,
    required this.bars,
    this.labels = const [],
    this.title,
    this.summaryTemplate = defaultSeriesSummary,
    this.showTable = false,
    this.height = 220,
    super.key,
  });

  /// The series.
  final List<ChartSeriesData> series;

  /// Whether to draw bars rather than lines.
  final bool bars;

  /// The label of each position along the horizontal axis.
  final List<String> labels;

  /// A heading, also the chart's spoken name.
  final String? title;

  /// What a screen reader hears for each series.
  final String summaryTemplate;

  /// Whether a table of the values is shown below.
  final bool showTable;

  /// The height of the drawing.
  final double height;

  /// The spoken summary: the title, then one sentence per series.
  String get summary => [
    ?title,
    for (final s in series)
      fillTemplate(summaryTemplate, {'name': s.name, ..._stats(s.values)}),
  ].join('. ');

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colors = chartPalette(theme.colorScheme);
    final style = (theme.textTheme.labelSmall ?? const TextStyle()).copyWith(
      color: theme.colorScheme.onSurfaceVariant,
    );
    final rows = math.max(
      labels.length,
      series.fold<int>(0, (m, s) => math.max(m, s.values.length)),
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        Semantics(
          container: true,
          label: summary,
          image: true,
          excludeSemantics: true,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: [
              if (title != null)
                Padding(
                  padding: const EdgeInsets.only(bottom: 8),
                  child: Text(title!, style: theme.textTheme.titleMedium),
                ),
              SizedBox(
                height: height,
                child: CustomPaint(
                  painter: CartesianPainter(
                    series: series,
                    labels: labels,
                    bars: bars,
                    colors: colors,
                    gridColor: theme.colorScheme.outlineVariant,
                    labelStyle: style,
                    textScaler: MediaQuery.textScalerOf(context),
                  ),
                ),
              ),
              if (series.length > 1)
                Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: Wrap(
                    spacing: 16,
                    runSpacing: 4,
                    children: [
                      for (var i = 0; i < series.length; i++)
                        _Key(colors[i % colors.length], series[i].name),
                    ],
                  ),
                ),
            ],
          ),
        ),
        if (showTable)
          Padding(
            padding: const EdgeInsets.only(top: 12),
            child: _DataTable(
              header: [title ?? '', for (final s in series) s.name],
              rows: [
                for (var i = 0; i < rows; i++)
                  [
                    i < labels.length ? labels[i] : '${i + 1}',
                    for (final s in series)
                      i < s.values.length ? formatNumber(s.values[i]) : '',
                  ],
              ],
            ),
          ),
      ],
    );
  }
}

/// A pie chart of labelled slices.
final class PluxPieChart extends StatelessWidget {
  /// Creates a pie chart.
  const PluxPieChart({
    required this.slices,
    this.title,
    this.summaryTemplate = defaultSliceSummary,
    this.showTable = false,
    this.diameter = 180,
    super.key,
  });

  /// The slices: label and size.
  final List<(String, double)> slices;

  /// A heading, also the chart's spoken name.
  final String? title;

  /// What a screen reader hears for each slice.
  final String summaryTemplate;

  /// Whether a table of the values is shown below.
  final bool showTable;

  /// The diameter of the pie.
  final double diameter;

  double get _total => slices.fold(0.0, (t, s) => t + math.max(0, s.$2));

  String _percent(double v) => _total <= 0
      ? '0'
      : formatNumber((math.max(0, v) / _total * 1000).round() / 10);

  /// The spoken summary: the title, then one sentence per slice.
  String get summary => [
    ?title,
    for (final s in slices)
      fillTemplate(summaryTemplate, {
        'label': s.$1,
        'value': formatNumber(s.$2),
        'percent': _percent(s.$2),
      }),
  ].join('. ');

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colors = chartPalette(theme.colorScheme);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        Semantics(
          container: true,
          label: summary,
          image: true,
          excludeSemantics: true,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: [
              if (title != null)
                Padding(
                  padding: const EdgeInsets.only(bottom: 8),
                  child: Text(title!, style: theme.textTheme.titleMedium),
                ),
              Center(
                child: SizedBox.square(
                  dimension: diameter,
                  child: CustomPaint(
                    painter: PiePainter(
                      values: [for (final s in slices) s.$2],
                      colors: colors,
                      emptyColor: theme.colorScheme.outlineVariant,
                      separator: theme.colorScheme.surface,
                    ),
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Wrap(
                  spacing: 16,
                  runSpacing: 4,
                  children: [
                    for (var i = 0; i < slices.length; i++)
                      _Key(colors[i % colors.length], slices[i].$1),
                  ],
                ),
              ),
            ],
          ),
        ),
        if (showTable)
          Padding(
            padding: const EdgeInsets.only(top: 12),
            child: _DataTable(
              header: [title ?? '', '', '%'],
              rows: [
                for (final s in slices)
                  [s.$1, formatNumber(s.$2), _percent(s.$2)],
              ],
            ),
          ),
      ],
    );
  }
}

/// A small line of values with no axes.
final class PluxSparkline extends StatelessWidget {
  /// Creates a sparkline.
  const PluxSparkline({
    required this.values,
    this.semanticLabel,
    this.summaryTemplate = defaultSparklineSummary,
    this.height = 32,
    super.key,
  });

  /// The values.
  final List<double> values;

  /// What the values are, spoken before the summary.
  final String? semanticLabel;

  /// What a screen reader hears.
  final String summaryTemplate;

  /// The height of the line.
  final double height;

  @override
  Widget build(BuildContext context) => Semantics(
    label: [
      ?semanticLabel,
      fillTemplate(summaryTemplate, _stats(values)),
    ].join(': '),
    image: true,
    excludeSemantics: true,
    child: SizedBox(
      height: height,
      width: double.infinity,
      child: CustomPaint(
        painter: SparklinePainter(
          values: values,
          color: Theme.of(context).colorScheme.primary,
        ),
      ),
    ),
  );
}

final class _Key extends StatelessWidget {
  const _Key(this.color, this.label);

  final Color color;
  final String label;

  @override
  Widget build(BuildContext context) => Row(
    mainAxisSize: MainAxisSize.min,
    children: [
      SizedBox.square(
        dimension: 10,
        child: DecoratedBox(
          decoration: BoxDecoration(color: color, shape: BoxShape.circle),
        ),
      ),
      const SizedBox(width: 6),
      Flexible(
        child: Text(label, style: Theme.of(context).textTheme.bodySmall),
      ),
    ],
  );
}

final class _DataTable extends StatelessWidget {
  const _DataTable({required this.header, required this.rows});

  final List<String> header;
  final List<List<String>> rows;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    Widget cell(String s, {bool head = false}) => Padding(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
      child: Text(
        s,
        style: head ? theme.textTheme.labelLarge : theme.textTheme.bodyMedium,
      ),
    );
    return Table(
      border: TableBorder(
        horizontalInside: BorderSide(color: theme.colorScheme.outlineVariant),
      ),
      defaultColumnWidth: const IntrinsicColumnWidth(),
      children: [
        TableRow(children: [for (final h in header) cell(h, head: true)]),
        for (final r in rows) TableRow(children: [for (final c in r) cell(c)]),
      ],
    );
  }
}
