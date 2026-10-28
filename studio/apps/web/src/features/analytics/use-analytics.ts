// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import { AnalyticsService } from "@plux/api";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";
import type { AnalyticsSearch } from "./analytics.schema.ts";

const DAY = 86_400_000;

export interface Range {
  readonly from: Date;
  readonly to: Date;
}

const dateOf = (v: string | undefined) => {
  const d = v === undefined ? Number.NaN : Date.parse(`${v}T00:00:00Z`);
  return Number.isNaN(d) ? undefined : new Date(d);
};

/** The range of the search: whole UTC days, by default the last seven including today. */
export function rangeOf(search: AnalyticsSearch, now: Date = new Date()): Range {
  const today = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate());
  const to = dateOf(search.to) ?? new Date(today);
  const from = dateOf(search.from) ?? new Date(to.getTime() - 6 * DAY);
  return { from, to: new Date(to.getTime() + DAY) };
}

function useClient() {
  const { session } = useApp();
  return createClient(AnalyticsService, session.transport);
}

/** One event name's daily rollups, optionally grouped. */
export function useRollups(app: string, env: string | undefined, metric: string, range: Range, groupBy: string[] = []) {
  const client = useClient();
  return useQuery({
    queryKey: ["analytics-rollups", app, env, metric, range.from.getTime(), range.to.getTime(), groupBy],
    queryFn: async () =>
      (
        await client.queryRollups({
          appId: app,
          environment: env ?? "",
          metric,
          granularity: "1d",
          from: timestampFromDate(range.from),
          to: timestampFromDate(range.to),
          groupBy,
        })
      ).points,
  });
}

/** The headline numbers of every app the person may read. */
export function useOverview(range: Range) {
  const client = useClient();
  return useQuery({
    queryKey: ["analytics-overview", range.from.getTime(), range.to.getTime()],
    queryFn: async () =>
      (await client.getOverview({ from: timestampFromDate(range.from), to: timestampFromDate(range.to) })).apps,
  });
}

export function useFunnels(app: string) {
  const client = useClient();
  return useQuery({
    queryKey: ["funnels", app],
    queryFn: async () => (await client.listFunnels({ appId: app })).funnels,
  });
}

export function useFunnelResult(id: string, range: Range) {
  const client = useClient();
  return useQuery({
    queryKey: ["funnel-result", id, range.from.getTime(), range.to.getTime()],
    queryFn: async () =>
      client.getFunnelResult({ id, from: timestampFromDate(range.from), to: timestampFromDate(range.to) }),
  });
}

/** Creates or deletes a funnel and refreshes the list. */
export function useFunnelMutations(app: string) {
  const client = useClient();
  const cache = useQueryClient();
  const done = () => cache.invalidateQueries({ queryKey: ["funnels", app] });
  return {
    create: useMutation({
      mutationFn: (v: { name: string; steps: string[] }) =>
        client.createFunnel({ funnel: { appId: app, name: v.name, steps: v.steps } }),
      onSuccess: done,
    }),
    remove: useMutation({ mutationFn: (id: string) => client.deleteFunnel({ id }), onSuccess: done }),
  };
}

export function useErrorGroups(app: string, days: number) {
  const client = useClient();
  return useQuery({
    queryKey: ["error-groups", app, days],
    queryFn: async () => (await client.listErrorGroups({ appId: app, days })).groups,
  });
}

export function useErrorGroup(id: string) {
  const client = useClient();
  return useQuery({
    queryKey: ["error-group", id],
    enabled: id !== "",
    queryFn: async () => client.getErrorGroup({ id }),
  });
}
