// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Field, Input } from "@plux/ui";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { useScope } from "../security/use-security.ts";
import { analyticsMessages } from "./analytics.messages.ts";
import type { AnalyticsSearch } from "./analytics.schema.ts";
import { type Range, rangeOf } from "./use-analytics.ts";

/** The scope of an analytics page: org, app, environment and the range of its search. */
export function useAnalyticsScope() {
  const { app } = useScope();
  const { org } = useParams({ strict: false }) as { org: string };
  const search = useSearch({ strict: false }) as AnalyticsSearch;
  const range: Range = rangeOf(search);
  return { org, app, search, range, env: search.env };
}

const routes = [
  ["/o/$org/apps/$app/analytics", "navOverview"],
  ["/o/$org/apps/$app/analytics/usage", "navUsage"],
  ["/o/$org/apps/$app/analytics/funnels", "navFunnels"],
  ["/o/$org/apps/$app/analytics/errors", "navErrors"],
  ["/o/$org/apps/$app/analytics/performance", "navPerformance"],
] as const;

/** Links between the pages of the area and the range, kept in the address. */
export function AreaNav(): ReactElement {
  const m = analyticsMessages(useLingui().i18n);
  const { org, app, search } = useAnalyticsScope();
  const navigate = useNavigate();
  const set = (key: "from" | "to", value: string) =>
    navigate({ to: ".", search: (prev: AnalyticsSearch) => ({ ...prev, [key]: value === "" ? undefined : value }) });
  return (
    <div className="flex flex-wrap items-end gap-4">
      <nav aria-label={m.overviewTitle} className="flex gap-3">
        {routes.map(([to, key]) => (
          <Link key={to} to={to} params={{ org, app }} search={{ env: search.env }} className="text-body">
            {m[key]}
          </Link>
        ))}
      </nav>
      <Field label={m.rangeFrom}>
        <Input type="date" value={search.from ?? ""} onChange={(e) => set("from", e.target.value)} />
      </Field>
      <Field label={m.rangeTo}>
        <Input type="date" value={search.to ?? ""} onChange={(e) => set("to", e.target.value)} />
      </Field>
    </div>
  );
}
