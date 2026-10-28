// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import { Link, useParams } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { Frame } from "../releases/frame.tsx";
import { Table } from "../security/parts.tsx";
import { analyticsMessages } from "./analytics.messages.ts";
import { Sparkline } from "./charts.tsx";
import { AreaNav, useAnalyticsScope } from "./parts.tsx";
import { useErrorGroup, useErrorGroups } from "./use-analytics.ts";

const DAY = 86_400_000;
const when = (t: Timestamp | undefined) =>
  t === undefined ? "" : timestampDate(t).toISOString().slice(0, 16).replace("T", " ");

/** The days the range spans, at least one. */
const daysOf = (from: Date, to: Date) => Math.max(1, Math.round((to.getTime() - from.getTime()) / DAY));

/** A28: error groups by fingerprint, newest activity first. */
export function AnalyticsErrorsPage(): ReactElement {
  const m = analyticsMessages(useLingui().i18n);
  const { org, app, range, search } = useAnalyticsScope();
  const q = useErrorGroups(app, daysOf(range.from, range.to));
  const groups = q.data ?? [];
  return (
    <Frame id="errors-title" title={m.errorsTitle} caption={m.errorsCaption} loading={q.isPending} failed={q.isError}>
      <AreaNav />
      {groups.length === 0 && <p className="text-body text-secondary">{m.noErrors}</p>}
      <Table
        caption={m.errorsTitle}
        heads={[
          m.codeColumn,
          m.pluginColumn,
          m.routeColumn,
          m.eventsColumn,
          m.devicesColumn,
          m.lastSeenColumn,
          m.trendColumn,
        ]}
        rows={groups.map((g) => ({
          key: g.id,
          cells: [
            <Link
              key="open"
              to="/o/$org/apps/$app/analytics/errors/$fingerprint"
              params={{ org, app, fingerprint: g.fingerprint } as never}
              search={search}
            >
              {g.fatal ? `${g.code} (${m.fatalLabel})` : g.code}
            </Link>,
            g.pluginKey,
            g.route,
            g.events.toString(),
            g.devices.toString(),
            when(g.lastSeen),
            <Sparkline key="trend" label={m.trendLabel} values={g.trend.map((d) => Number(d.events))} />,
          ],
        }))}
      />
    </Frame>
  );
}

/** A29: one group: occurrences, the symbolicated path, and a link that opens the failing node on the canvas. */
export function AnalyticsErrorPage(): ReactElement {
  const m = analyticsMessages(useLingui().i18n);
  const { org, app, range, search } = useAnalyticsScope();
  const { fingerprint } = useParams({ strict: false }) as { fingerprint: string };
  const list = useErrorGroups(app, daysOf(range.from, range.to));
  const id = list.data?.find((g) => g.fingerprint === fingerprint)?.id ?? "";
  const detail = useErrorGroup(id);
  const group = detail.data?.group;
  const missing = !list.isPending && id === "";
  return (
    <Frame
      id="error-title"
      title={group?.code ?? m.errorTitle}
      loading={list.isPending || (id !== "" && detail.isPending)}
      failed={list.isError || detail.isError || missing}
    >
      <AreaNav />
      <Link to="/o/$org/apps/$app/analytics/errors" params={{ org, app }} search={search}>
        {m.backErrors}
      </Link>
      {group !== undefined && (
        <>
          <h2 className="text-section">{m.symbolicated}</h2>
          <dl className="m-0 flex flex-col gap-x-4 gap-y-1 text-body">
            <dt className="text-secondary">{m.nodePathLabel}</dt>
            <dd className="m-0 font-mono">{group.nodePath}</dd>
            <dt className="text-secondary">{m.jsonPathLabel}</dt>
            <dd className="m-0 font-mono">{group.jsonPath}</dd>
            <dt className="text-secondary">{m.sourceFileLabel}</dt>
            <dd className="m-0 font-mono">{group.sourceFile}</dd>
            <dt className="text-secondary">{m.framesLabel}</dt>
            <dd className="m-0 font-mono">{group.frames.join(" > ")}</dd>
          </dl>
          <Link
            to="/o/$org/apps/$app/plugins/$plugin"
            params={{ org, app, plugin: group.pluginKey }}
            search={{ focus: group.route }}
          >
            {m.openNode}
          </Link>
          <Table
            caption={m.occurrences}
            heads={[m.timeColumn, m.eventColumn]}
            rows={(detail.data?.samples ?? []).map((s) => ({ key: s.eventId, cells: [when(s.occurredAt), s.eventId] }))}
          />
        </>
      )}
    </Frame>
  );
}
