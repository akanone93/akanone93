// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_charts/plux_charts.dart';
import 'package:plux_flutter/plux_flutter.dart';

/// A node whose props are given by permanent ID; enough of [NodeContext]
/// for builders that only read props.
final class FakeNode implements NodeContext {
  FakeNode(this.context, this.props);

  @override
  final BuildContext context;
  final Map<int, Object?> props;

  @override
  Object? prop(int id) => props[id];

  @override
  T? decode<T>(int id, T? Function(Decoding d, Object? v) decoder) {
    final v = props[id];
    return v == null ? null : decoder(this, v);
  }

  @override
  Never missing(String name) => throw DecodeError('$name is missing');

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

Map<String, Object?> _descriptor(String type) => jsonDecode(
  File('../../schema/widgets/layer2/$type.json').readAsStringSync(),
) as Map<String, Object?>;

const _series = [
  {
    'name': 'Revenue',
    'values': [10.0, 25.5, 18.0, 30.0],
  },
  {
    'name': 'Costs',
    'values': [8.0, 12.0, 15.25, 20.0],
  },
];
const _labels = ['Q1', 'Q2', 'Q3', 'Q4'];

Widget _app(
  Widget child, {
  bool dark = false,
  bool rtl = false,
  double scale = 1,
}) => MaterialApp(
  debugShowCheckedModeBanner: false,
  theme: ThemeData(
    colorScheme: ColorScheme.fromSeed(
      seedColor: Colors.indigo,
      brightness: dark ? Brightness.dark : Brightness.light,
    ),
  ),
  builder: (context, c) => Directionality(
    textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
    child: MediaQuery(
      data: MediaQuery.of(context)
          .copyWith(textScaler: TextScaler.linear(scale)),
      child: c!,
    ),
  ),
  home: Scaffold(
    body: SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: child,
    ),
  ),
);

Widget _gallery({bool table = true}) => Column(
  crossAxisAlignment: CrossAxisAlignment.stretch,
  children: [
    PluxCartesianChart(
      series: [
        for (final s in _series)
          ChartSeriesData(s['name']! as String, (s['values']! as List<double>)),
      ],
      labels: _labels,
      bars: false,
      title: 'Results',
      showTable: table,
    ),
    const SizedBox(height: 16),
    PluxCartesianChart(
      series: [
        ChartSeriesData('Revenue', const [10, -5, 18, 30]),
      ],
      labels: _labels,
      bars: true,
    ),
    const SizedBox(height: 16),
    const PluxPieChart(
      slices: [('Rent', 50), ('Food', 30), ('Fun', 20)],
      title: 'Spending',
      showTable: true,
    ),
    const SizedBox(height: 16),
    const PluxSparkline(values: [1, 3, 2, 5, 4], semanticLabel: 'Trend'),
  ],
);

void main() {
  test('the package IDs are those of the descriptors [WGT-020] [RT-060]', () {
    const ids = {
      'LineChart': PluxChartIds.lineChart,
      'BarChart': PluxChartIds.barChart,
      'PieChart': PluxChartIds.pieChart,
      'Sparkline': PluxChartIds.sparkline,
    };
    for (final e in ids.entries) {
      final d = _descriptor(e.key);
      expect(d['id'], e.value, reason: e.key);
      expect(d['package'], const PluxCharts().name);
    }
    expect(const PluxCharts().builders.keys, unorderedEquals(ids.values));
    final line = (_descriptor('LineChart')['props']! as List<Object?>)
        .cast<Map<String, Object?>>();
    expect(
      [for (final p in line) p['name']],
      ['series', 'labels', 'title', 'summaryTemplate', 'showTable'],
    );
  });

  test('numbers are plain digits and axes include zero [WGT-020]', () {
    expect(formatNumber(3), '3');
    expect(formatNumber(2.5), '2.5');
    expect(formatNumber(1.005 * 100), '100.5');
    expect(formatNumber(double.nan), '0');
    expect(formatNumber(-0.001), '0');
  });

  testWidgets(
    'builders read props by ID and report a missing series [WGT-020]',
    (tester) async {
      late Widget line;
      late Widget pie;
      late Widget spark;
      Object? error;
      await tester.pumpWidget(
        Builder(
          builder: (context) {
            const pkg = PluxCharts();
            line = pkg.builders[PluxChartIds.lineChart]!(
              FakeNode(context, {1: _series, 2: _labels, 3: 'Results'}),
            );
            pie = pkg.builders[PluxChartIds.pieChart]!(
              FakeNode(context, {
                1: [
                  {'label': 'A', 'value': 1.0},
                ],
              }),
            );
            spark = pkg.builders[PluxChartIds.sparkline]!(
              FakeNode(context, {
                1: [1.0, double.infinity, 3.0],
              }),
            );
            try {
              pkg.builders[PluxChartIds.barChart]!(FakeNode(context, {}));
            } on DecodeError catch (e) {
              error = e;
            }
            return const SizedBox();
          },
        ),
      );
      expect(line, isA<PluxCartesianChart>());
      expect((line as PluxCartesianChart).series.map((s) => s.name), [
        'Revenue',
        'Costs',
      ]);
      expect((line as PluxCartesianChart).title, 'Results');
      expect(pie, isA<PluxPieChart>());
      expect((spark as PluxSparkline).values, [1.0, 0.0, 3.0]);
      expect(error, isA<DecodeError>());
    },
  );

  testWidgets('each series has a text summary and the table has the data '
      '[A11Y-001] [WGT-020]', (tester) async {
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(_app(_gallery()));
    expect(
      find.bySemanticsLabel(
        RegExp(
          r'Results\. Revenue: 4 values, first 10, last 30, lowest 10, highest 30\. Costs: 4 values, first 8, last 20, lowest 8, highest 20',
        ),
      ),
      findsOneWidget,
    );
    expect(
      find.bySemanticsLabel(
        'Spending. Rent: 50, 50 percent. Food: 30, 30 percent. Fun: 20, 20 percent',
      ),
      findsOneWidget,
    );
    expect(find.bySemanticsLabel(RegExp(r'^Trend: 5 values')), findsOneWidget);
    expect(find.text('25.5'), findsWidgets);
    expect(find.text('15.25'), findsOneWidget);
    expect(find.text('Q3'), findsWidgets);
    handle.dispose();
  });

  group('goldens in light, dark, right-to-left and 200% text [WGT-020]', () {
    for (final (name, dark, rtl, scale) in [
      ('light', false, false, 1.0),
      ('dark', true, false, 1.0),
      ('rtl', false, true, 1.0),
      ('text200', false, false, 2.0),
    ]) {
      testWidgets(name, (tester) async {
        tester.view.physicalSize = const Size(600, 1900);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.reset);
        await tester.pumpWidget(
          _app(_gallery(), dark: dark, rtl: rtl, scale: scale),
        );
        await expectLater(
          find.byType(Scaffold),
          matchesGoldenFile('goldens/charts_$name.png'),
        );
      });
    }
  });
}
