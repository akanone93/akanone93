// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { AppSummary, EnvironmentRelease } from "@plux/api";
import { Button, Icon, MetricStrip, StatusPill } from "@plux/ui";
import { Link } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { homeMessages } from "../home/home.messages.ts";
import { type AppActionHandlers, AppActions, type AppPermissions } from "./app-actions.tsx";
import { appsMessages } from "./apps.messages.ts";
import { type AppEntry, healthOf, toneOf } from "./apps-model.ts";
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
    return <p className="app-env app-env-none">{m.noEnvironments}</p>;
  }
  return (
    <ul aria-label={m.environments} className="flex flex-wrap gap-2">
      {releases.map((r) => (
        <li key={r.environmentKey} className="app-env" data-live={r.liveSequence > 0n}>
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
      <span className="flex items-center gap-1 text-caption text-muted-foreground">
        {!flat && <Icon name={health.trend > 0 ? "caret-up" : "caret-down"} size="pill" />}
        <span>{flat ? m.trendFlat : health.trend > 0 ? m.trendUp(points) : m.trendDown(points)}</span>
      </span>
    </span>
  );
}

/** The date, time and publisher of the newest release, or a word when there is none (STU-022). */
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
    return <p className="text-caption text-muted-foreground">{m.neverPublished}</p>;
  }
  const when = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(timestampDate(at));
  const who = summary.lastPublisher?.display ?? "";
  return (
    <p className="text-caption text-muted-foreground">{who === "" ? m.published(when) : m.publishedBy(when, who)}</p>
  );
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
      title={pinned ? m.pinnedShort : m.pinShort}
      className="app-pin"
      onClick={() => handlers.onPin(entry.app.id)}
    >
      <Icon name="push-pin" size="tab" />
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
      className="app-link min-w-0 truncate text-title font-semibold"
    >
      {app.name}
    </Link>
  );
}

/** The two figures at the foot of a card: active devices and plugins, label over value. */
function Figures({
  summary,
  m,
  locale,
}: {
  readonly summary: AppSummary;
  readonly m: Messages;
  readonly locale: string;
}) {
  const n = new Intl.NumberFormat(locale);
  return (
    <dl className="app-figures">
      <div>
        <dt>{m.devices}</dt>
        <dd>{n.format(summary.activeDevices)}</dd>
      </div>
      <div>
        <dt>{m.plugins}</dt>
        <dd>{n.format(summary.pluginCount)}</dd>
      </div>
    </dl>
  );
}

/**
 * One app as a card (STU-020 to STU-022, STU-025): its picture on the app's colour with the live releases over it, the
 * name, then health, figures and limits. The whole card opens the app; pin and actions stay their own buttons.
 */
export function AppCard(props: AppCardProps): ReactElement {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  const { entry, team } = props;
  const { app, summary } = entry;
  return (
    <li className="app-card" data-tone={toneOf(app)}>
      <div className="relative">
        <CardVisual app={app} />
        <div className="app-card-top">
          {summary !== undefined && <Environments releases={summary.releases} m={m} />}
          <span className="app-card-controls">
            <PinButton entry={entry} pinned={props.pinned} handlers={props.handlers} />
            <AppActions entry={entry} org={props.org} permissions={props.permissions} handlers={props.handlers} />
          </span>
        </div>
      </div>
      <div className="flex flex-1 flex-col gap-3 p-5">
        <div className="flex min-w-0 items-baseline gap-2">
          <NameLink entry={entry} org={props.org} />
          <span className="font-mono text-tag text-tertiary">{app.key}</span>
          <Icon name="arrow-up-right" size="pill" className="app-go ms-auto text-tertiary" />
        </div>
        {app.description !== "" && <p className="line-clamp-2 text-body text-muted-foreground">{app.description}</p>}
        <p className="text-caption text-tertiary">{team === undefined ? m.noTeam : m.owner(team)}</p>
        {app.archived && <StatusPill status="neutral" label={m.archivedBadge} />}
        {summary !== undefined && (
          <>
            <div className="flex">
              <Health summary={summary} m={m} locale={i18n.locale} />
            </div>
            <Figures summary={summary} m={m} locale={i18n.locale} />
            <Published summary={summary} m={m} locale={i18n.locale} />
            <CardLimits limits={summary.limits} />
          </>
        )}
      </div>
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
    <li className="app-row" data-tone={toneOf(app)}>
      <CardVisual app={app} compact />
      <div className="flex min-w-40 flex-1 flex-col">
        <NameLink entry={entry} org={props.org} />
        <span className="text-caption text-muted-foreground">{team === undefined ? m.noTeam : m.owner(team)}</span>
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
