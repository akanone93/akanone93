// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Button, Field, Input, Textarea } from "@plux/ui";
import { Link, useParams } from "@tanstack/react-router";
import { type ReactElement, useState } from "react";
import { Frame } from "../releases/frame.tsx";
import { Table } from "../security/parts.tsx";
import { usePermissions } from "../security/use-security.ts";
import { analyticsMessages } from "./analytics.messages.ts";
import { BarList } from "./charts.tsx";
import { AreaNav, useAnalyticsScope } from "./parts.tsx";
import { useFunnelMutations, useFunnelResult, useFunnels } from "./use-analytics.ts";

const stepsOf = (text: string) =>
  text
    .split("\n")
    .map((s) => s.trim())
    .filter((s) => s !== "");

/** A26: the funnels of the app, with a form to define one. */
export function AnalyticsFunnelsPage(): ReactElement {
  const m = analyticsMessages(useLingui().i18n);
  const { org, app, search } = useAnalyticsScope();
  const funnels = useFunnels(app);
  const { create, remove } = useFunnelMutations(app);
  const canManage = usePermissions().has("alerts.manage");
  const [name, setName] = useState("");
  const [steps, setSteps] = useState("");
  const parsed = stepsOf(steps);
  return (
    <Frame
      id="funnels-title"
      title={m.funnelsTitle}
      caption={m.funnelsCaption}
      loading={funnels.isPending}
      failed={funnels.isError}
    >
      <AreaNav />
      {(funnels.data ?? []).length === 0 && <p className="text-body text-secondary">{m.noFunnels}</p>}
      <Table
        caption={m.funnelsTitle}
        heads={[m.funnelName, m.stepsColumn, ""]}
        rows={(funnels.data ?? []).map((f) => ({
          key: f.id,
          cells: [
            <Link
              key="open"
              to="/o/$org/apps/$app/analytics/funnels/$funnelId"
              params={{ org, app, funnelId: f.id } as never}
              search={search}
            >
              {f.name}
            </Link>,
            f.steps.length,
            <Button key="delete" variant="danger" size="sm" disabled={!canManage} onClick={() => remove.mutate(f.id)}>
              {m.deleteFunnel}
            </Button>,
          ],
        }))}
      />
      <form
        className="flex max-w-md flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate(
            { name, steps: parsed },
            {
              onSuccess: () => {
                setName("");
                setSteps("");
              },
            },
          );
        }}
      >
        <Field label={m.funnelName}>
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label={m.funnelSteps}>
          <Textarea rows={4} value={steps} onChange={(e) => setSteps(e.target.value)} />
        </Field>
        <Button type="submit" disabled={!canManage || name === "" || parsed.length < 2 || create.isPending}>
          {m.createFunnel}
        </Button>
      </form>
    </Frame>
  );
}

/** A27: the events reaching each step of one funnel, and the share of the first step. */
export function AnalyticsFunnelPage(): ReactElement {
  const m = analyticsMessages(useLingui().i18n);
  const { org, app, range, search } = useAnalyticsScope();
  const { funnelId } = useParams({ strict: false }) as { funnelId: string };
  const q = useFunnelResult(funnelId, range);
  const steps = q.data?.steps ?? [];
  return (
    <Frame id="funnel-title" title={q.data?.funnel?.name ?? m.funnelsTitle} loading={q.isPending} failed={q.isError}>
      <AreaNav />
      <Link to="/o/$org/apps/$app/analytics/funnels" params={{ org, app }} search={search}>
        {m.backFunnels}
      </Link>
      <BarList
        label={m.funnelChart}
        bars={steps.map((s, i) => ({ key: `${i}-${s.event}`, label: s.event, value: Number(s.events) }))}
      />
      <Table
        caption={m.funnelChart}
        heads={[m.stepColumn, m.eventsColumn, m.conversionColumn]}
        rows={steps.map((s, i) => ({
          key: `${i}-${s.event}`,
          cells: [s.event, s.events.toString(), `${Math.round(s.conversion * 100)}%`],
        }))}
      />
    </Frame>
  );
}
