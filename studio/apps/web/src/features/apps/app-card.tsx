// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { AppSummary, EnvironmentRelease } from "@plux/api";
import { Button, cx, Icon, MetricStrip, StatusPill } from "@plux/ui";
import { Link } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { homeMessages } from "../home/home.messages.ts";
import { type AppActionHandlers, AppActions, type AppPermissions } from "./app-actions.tsx";
import { appsMessages } from "./apps.messages.ts";
import { type AppEntry, healthOf } from "./apps-model.ts";
import { CardVisual } from "./card-visual.tsx";
import { CardLimits } from "./limits-usage.tsx";

type Messages = ReturnType<typeof appsMessages>;

/** What a card or a row needs besides the app itself. */
export interface AppCardProps {
  readonly entry: AppEntry;
  readonly org: string;
  /** The name of the owning team, or undefined when no team owns the app. */
  readonly team: string | undefined;
  readonly pinned: boolean;
  readonly permissions: AppPermissions;
  readonly handlers: AppActionHandlers;
}

/** The environment badges: the release each environment serves (STU-022). */
function Environments({ releases, m }: { readonly releases: readonly EnvironmentRelease[]; readonly m: Messages }) {
  if (releases.length === 0) {
    return <p className="text-caption text-secondary">{m.noEnvironments}</p>;
  }
  return (
    <ul aria-label={m.environments} className="flex flex-wrap gap-1">
      {releases.map((r) => (
        <li key={r.environmentKey} className="rounded-full bg-raised px-2 py-0.5 font-mono text-tag text-secondary">
          {r.liveSequence > 0n ? m.live(r.environmentKey, Number(r.liveSequence)) : m.notLive(r.environmentKey)}
        </li>
      ))}
    </ul>
  );
}

/** The health badge with its error trend (STU-022): the crash-free rate, and its change against the week before. */
function Health({
  summary,
  m,
  locale,
}: {
  readonly summary: AppSummary;
  readonly m: Messages;
  readonly locale: string;
}) {
  const health = healthOf(summary);
  if (health.kind === "none") {
    return <StatusPill status="neutral" label={m.healthNone} />;
  }
  const percent = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 1 });
  const points = new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(Math.abs(health.trend) * 100);
  const word = { good: m.healthGood, warn: m.healthWarn, bad: m.healthBad }[health.kind];
  const status = { good: "success", warn: "warning", bad: "danger" }[health.kind] as "success" | "warning" | "danger";
  const flat = Math.abs(health.trend) < 0.0005;
  return (
    <span className="flex flex-wrap items-center gap-2">
      <StatusPill status={status} label={word} value={m.crashFree(percent.format(health.rate))} />
      <span className="flex items-center gap-1 text-caption text-secondary">
        {!flat && <Icon name={health.trend > 0 ? "caret-up" : "caret-down"} size="pill" />}
        <span>{flat ? m.trendFlat : health.trend > 0 ? m.trendUp(points) : m.trendDown(points)}</span>
      </span>
    </span>
  );
}

/** The date and time of the newest release, or a word when there is none. */
function Published({
  summary,
  m,
  locale,
}: {
  readonly summary: AppSummary;
  readonly m: Messages;
  readonly locale: string;
}) {
  const at = summary.lastPublishedAt;
  if (at === undefined || (at.seconds === 0n && at.nanos === 0)) {
    return <p className="text-caption text-secondary">{m.neverPublished}</p>;
  }
  const when = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(timestampDate(at));
  return <p className="text-caption text-secondary">{m.published(when)}</p>;
}

/** The pin toggle: pinned apps come first in every order (STU-023). */
function PinButton({ entry, pinned, handlers }: Pick<AppCardProps, "entry" | "pinned" | "handlers">): ReactElement {
  const m = appsMessages(useLingui().i18n);
  const name = entry.app.name;
  return (
    <Button
      size="sm"
      variant="ghost"
      aria-pressed={pinned}
      aria-label={pinned ? m.unpin(name) : m.pin(name)}
      onClick={() => handlers.onPin(entry.app.id)}
    >
      {pinned ? m.pinnedShort : m.pinShort}
    </Button>
  );
}

/** The name as a link to the app's canvas, with the key beside it. */
function NameLink({ entry, org }: Pick<AppCardProps, "entry" | "org">): ReactElement {
  const home = homeMessages(useLingui().i18n);
  const { app } = entry;
  return (
    <Link
      to="/o/$org/apps/$app"
      params={{ org, app: app.id }}
      aria-label={home.openApp(app.name)}
      className="min-w-0 truncate font-serif text-title"
    >
      {app.name}
    </Link>
  );
}

/** One app as a card (STU-020 to STU-022, STU-025). */
export function AppCard(props: AppCardProps): ReactElement {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  const { entry, team } = props;
  const { app, summary } = entry;
  return (
    <li
      className={cx(
        "flex flex-col gap-3 rounded-lg bg-card p-(--density-card-padding-x) shadow-e1",
        "transition-transform duration-enter ease-standard hover:-translate-y-0.5 motion-reduce:transition-none motion-reduce:hover:translate-y-0",
      )}
    >
      <CardVisual app={app} />
      <div className="flex items-start gap-2">
        <div className="flex min-w-0 flex-1 flex-col">
          <NameLink entry={entry} org={props.org} />
          <span className="font-mono text-data text-tertiary">{app.key}</span>
        </div>
        <PinButton entry={entry} pinned={props.pinned} handlers={props.handlers} />
        <AppActions entry={entry} org={props.org} permissions={props.permissions} handlers={props.handlers} />
      </div>
      {app.description !== "" && <p className="line-clamp-2 text-body text-secondary">{app.description}</p>}
      <p className="text-caption text-secondary">{team === undefined ? m.noTeam : m.owner(team)}</p>
      {app.archived && <StatusPill status="neutral" label={m.archivedBadge} />}
      {summary !== undefined && (
        <>
          <Environments releases={summary.releases} m={m} />
          <Published summary={summary} m={m} locale={i18n.locale} />
          <Health summary={summary} m={m} locale={i18n.locale} />
          <MetricStrip
            metrics={[
              { label: m.devices, value: new Intl.NumberFormat(i18n.locale).format(summary.activeDevices) },
              { label: m.plugins, value: new Intl.NumberFormat(i18n.locale).format(summary.pluginCount) },
            ]}
          />
          <CardLimits limits={summary.limits} />
        </>
      )}
    </li>
  );
}

/** One app as a row of the list view (STU-020): the same facts, side by side. */
export function AppRow(props: AppCardProps): ReactElement {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  const { entry, team } = props;
  const { app, summary } = entry;
  return (
    <li className="flex flex-wrap items-center gap-4 rounded-lg bg-card p-3 shadow-e1">
      <CardVisual app={app} compact />
      <div className="flex min-w-40 flex-1 flex-col">
        <NameLink entry={entry} org={props.org} />
        <span className="text-caption text-secondary">{team === undefined ? m.noTeam : m.owner(team)}</span>
      </div>
      {app.archived && <StatusPill status="neutral" label={m.archivedBadge} />}
      {summary !== undefined && (
        <>
          <Environments releases={summary.releases} m={m} />
          <Health summary={summary} m={m} locale={i18n.locale} />
          <MetricStrip
            metrics={[
              { label: m.devices, value: new Intl.NumberFormat(i18n.locale).format(summary.activeDevices) },
              { label: m.plugins, value: new Intl.NumberFormat(i18n.locale).format(summary.pluginCount) },
            ]}
          />
          <Published summary={summary} m={m} locale={i18n.locale} />
          <CardLimits limits={summary.limits} />
        </>
      )}
      <PinButton entry={entry} pinned={props.pinned} handlers={props.handlers} />
      <AppActions entry={entry} org={props.org} permissions={props.permissions} handlers={props.handlers} />
    </li>
  );
}
