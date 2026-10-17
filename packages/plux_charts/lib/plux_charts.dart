// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Chart widgets for Plux pages: `LineChart`, `BarChart`, `PieChart` and
/// `Sparkline`, drawn in house (ADR-0071). Register [PluxCharts] in
/// `PluxConfig.widgetPackages`.
library;

export 'src/charts.dart'
    show
        PluxCartesianChart,
        PluxPieChart,
        PluxSparkline,
        chartPalette,
        defaultSeriesSummary,
        defaultSliceSummary,
        defaultSparklineSummary;
export 'src/numbers.dart' show formatNumber;
export 'src/package.dart' show PluxChartIds, PluxCharts;
export 'src/painters.dart' show ChartSeriesData;
