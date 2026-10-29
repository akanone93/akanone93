// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { MetricStrip } from "@plux/ui";
import { Link } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { Frame } from "../releases/frame.tsx";
import { Table } from "../security/parts.tsx";
import { type AnalyticsMessages, analyticsMessages } from "./analytics.messages.ts";
import { dailyEvents, percentilesByDay, totalsBy } from "./analytics.model.ts";
import { BarList, LineChart } from "./charts.tsx";
import { AreaNav, useAnalyticsScope } from "./parts.tsx";
import { useOverview, useRollups } from "./use-analytics.ts";

function DayTable({ m, days }: { m: AnalyticsMessages; days: readonly { key: string; value: number }[] }) {
  return (
    <Table
      caption={m.dayColumn}
      heads={[m.dayColumn, m.viewsColumn]}
      rows={days.map((d) => ({ key: d.key, cells: [d.key, d.value] }))}
    />
  );
}

/** A24: the headline numbers of the app, a daily chart and every app of the organisation. */
export function AnalyticsOverviewPage(): ReactElement {
  const m = analyticsMessages(useLingui().i18n);
  const { app, env, range } = useAnalyticsScope();
  const overview = useOverview(range);
  const views = useRollups(app, env, "screen_view", range);
  const mine = overview.data?.find((a) => a.appId === app);
  const days = dailyEvents(views.data ?? []);
  return (
    <Frame
      id="analytics-title"
      title={m.overviewTitle}
      caption={m.overviewCaption}
      loading={overview.isPending || views.isPending}
      failed={overview.isError || views.isError}
    >
      <AreaNav />
      <MetricStrip
        metrics={[
          { label: m.sessions, value: String(mine?.sessions ?? 0) },
          { label: m.screenViews, value: String(mine?.screenViews ?? 0) },
          { label: m.errorsLabel, value: String(mine?.errors ?? 0) },
          { label: m.firingAlerts, value: String(mine?.firingAlerts ?? 0) },
        ]}
      />
      {days.length === 0 ? (
        <p className="text-body text-secondary">{m.noData}</p>
      ) : (
        <>
          <LineChart label={m.screenViewsChart} series={[{ name: m.screenViews, values: days.map((d) => d.value) }]} />
          <DayTable m={m} days={days} />
        </>
      )}
      <Table
        caption={m.appColumn}
        heads={[m.appColumn, m.sessions, m.screenViews, m.errorsLabel, m.firingAlerts]}
        rows={(overview.data ?? []).map((a) => ({
          key: a.appId,
          cells: [a.appId, a.sessions.toString(), a.screenViews.toString(), a.errors.toString(), a.firingAlerts],
        }))}
      />
    </Frame>
  );
}

/** A25: usage by plugin, then by page, then one page per day (`?plugin`, `?screen`). */
export function AnalyticsUsagePage(): ReactElement {
  const m = analyticsMessages(useLingui().i18n);
  const { org, app, env, range, search } = useAnalyticsScope();
  const { plugin, screen } = search as { plugin?: string; screen?: string };
  const q = useRollups(app, env, "screen_view", range, ["plugin", "page"]);
  const all = q.data ?? [];
  const inPlugin = plugin === undefined ? all : all.filter((p) => p.group?.pluginKey === plugin);
  const inScreen = screen === undefined ? inPlugin : inPlugin.filter((p) => p.group?.page === screen);
  const link = (to: { plugin?: string | undefined; screen?: string | undefined }, label: string) => (
    <Link to="/o/$org/apps/$app/analytics/usage" params={{ org, app }} search={{ ...search, ...to }}>
      {label}
    </Link>
  );
  const bars =
    plugin === undefined
      ? totalsBy(all, (p) => p.group?.pluginKey ?? "")
      : totalsBy(inPlugin, (p) => p.group?.page ?? "");
  const days = dailyEvents(inScreen);
  return (
    <Frame id="usage-title" title={m.usageTitle} caption={m.usageCaption} loading={q.isPending} failed={q.isError}>
      <AreaNav />
      {plugin !== undefined && link({ plugin: undefined, screen: undefined }, m.allPlugins)}
      {screen === undefined ? (
        <BarList
          label={plugin === undefined ? m.pluginsChart : m.pagesChart}
          bars={bars.map((b) => ({ key: b.key, label: b.key, value: b.value }))}
          render={(b) =>
            plugin === undefined ? link({ plugin: b.key }, b.label) : link({ plugin, screen: b.key }, b.label)
          }
        />
      ) : (
        <>
          {link({ plugin, screen: undefined }, plugin ?? m.allPlugins)}
          <LineChart label={m.dailyChart} series={[{ name: screen, values: days.map((d) => d.value) }]} />
          <DayTable m={m} days={days} />
        </>
      )}
      {bars.length === 0 && <p className="text-body text-secondary">{m.noData}</p>}
    </Frame>
  );
}

/** A30: render time percentiles per day. */
export function AnalyticsPerformancePage(): ReactElement {
  const m = analyticsMessages(useLingui().i18n);
  const { app, env, range } = useAnalyticsScope();
  const q = useRollups(app, env, "render_perf", range);
  const p = percentilesByDay(q.data ?? []);
  return (
    <Frame
      id="performance-title"
      title={m.performanceTitle}
      caption={m.performanceCaption}
      loading={q.isPending}
      failed={q.isError}
    >
      <AreaNav />
      {p.days.length === 0 ? (
        <p className="text-body text-secondary">{m.noData}</p>
      ) : (
        <>
          <LineChart
            label={m.performanceChart}
            series={[
              { name: "p50", values: p.p50 },
              { name: "p90", values: p.p90 },
              { name: "p99", values: p.p99 },
            ]}
          />
          <Table
            caption={m.performanceChart}
            heads={[m.dayColumn, "p50", "p90", "p99"]}
            rows={p.days.map((d, i) => ({ key: d, cells: [d, p.p50[i], p.p90[i], p.p99[i]] }))}
          />
        </>
      )}
    </Frame>
  );
}
