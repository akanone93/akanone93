// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { AppSummary, EnvironmentRelease } from "@plux/api";
import { Icon, StatusPill } from "@plux/ui";
import { Badge } from "@plux/ui/components/badge";
import { Button } from "@plux/ui/components/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@plux/ui/components/card";
import { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle } from "@plux/ui/components/item";
import { Separator } from "@plux/ui/components/separator";
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
    return (
      <Badge variant="secondary" className="bg-background/80 backdrop-blur-sm">
        {m.noEnvironments}
      </Badge>
    );
  }
  return (
    <ul aria-label={m.environments} className="flex flex-wrap gap-1.5">
      {releases.map((r) => (
        <li key={r.environmentKey}>
          <Badge variant="secondary" className="bg-background/80 backdrop-blur-sm">
            <span
              aria-hidden="true"
              className={
                r.liveSequence > 0n ? "size-1.5 rounded-full bg-success" : "size-1.5 rounded-full bg-muted-foreground"
              }
            />
            {r.liveSequence > 0n ? m.live(r.environmentKey, Number(r.liveSequence)) : m.notLive(r.environmentKey)}
          </Badge>
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
      <span className="flex items-center gap-1 text-xs text-muted-foreground">
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
    return <p className="text-xs text-muted-foreground">{m.neverPublished}</p>;
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
      variant={pinned ? "secondary" : "ghost"}
      size="icon-sm"
      aria-pressed={pinned}
      aria-label={pinned ? m.unpin(name) : m.pin(name)}
      title={pinned ? m.pinnedShort : m.pinShort}
      className="relative z-10"
      onClick={() => handlers.onPin(entry.app.id)}
    >
      <Icon name="push-pin" />
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
      className="truncate outline-none after:absolute after:inset-0"
    >
      {app.name}
    </Link>
  );
}

/** The two figures of a card: active devices and plugins, label over value. */
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
    <dl className="grid grid-cols-2 gap-2">
      {[
        { label: m.devices, value: n.format(summary.activeDevices) },
        { label: m.plugins, value: n.format(summary.pluginCount) },
      ].map((f) => (
        <div key={f.label} className="flex flex-col-reverse gap-0.5 rounded-lg bg-muted/50 px-3 py-2">
          <dt className="text-xs text-muted-foreground">{f.label}</dt>
          <dd className="text-lg font-semibold tabular-nums">{f.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * One app as a card (STU-020 to STU-022, STU-025): shadcn's Card with the app's picture and live releases on top, its
 * name and description, health, figures and limits, and the newest release at the foot. The whole card opens the app;
 * pin and actions stay their own buttons.
 */
export function AppCard(props: AppCardProps): ReactElement {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  const { entry, team } = props;
  const { app, summary } = entry;
  return (
    <li className="relative">
      <Card className="h-full gap-0 pt-0 transition-shadow hover:shadow-md has-focus-visible:ring-3 has-focus-visible:ring-ring/50">
        <div className="relative">
          <CardVisual app={app} />
          <div className="absolute inset-x-0 top-0 flex items-start gap-2 p-3">
            {summary !== undefined && <Environments releases={summary.releases} m={m} />}
            <div className="relative z-10 ms-auto flex gap-1 rounded-lg bg-background/80 backdrop-blur-sm">
              <PinButton entry={entry} pinned={props.pinned} handlers={props.handlers} />
              <AppActions entry={entry} org={props.org} permissions={props.permissions} handlers={props.handlers} />
            </div>
          </div>
        </div>
        <CardHeader className="pt-4">
          <CardTitle className="flex min-w-0 items-baseline gap-2 text-base">
            <NameLink entry={entry} org={props.org} />
            <span className="truncate font-mono text-xs font-normal text-muted-foreground">{app.key}</span>
          </CardTitle>
          <CardDescription className="line-clamp-2">
            {app.description === "" ? (team === undefined ? m.noTeam : m.owner(team)) : app.description}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4 pt-4">
          <div className="flex flex-wrap items-center gap-2">
            {app.archived && <StatusPill status="neutral" label={m.archivedBadge} />}
            {summary !== undefined && <Health summary={summary} m={m} locale={i18n.locale} />}
          </div>
          {summary !== undefined && <Figures summary={summary} m={m} locale={i18n.locale} />}
          {summary !== undefined && summary.limits.length > 0 && (
            <>
              <Separator />
              <CardLimits limits={summary.limits} />
            </>
          )}
        </CardContent>
        <CardFooter className="mt-4 flex-wrap justify-between gap-x-4 gap-y-1 border-t py-3">
          {app.description !== "" && (
            <p className="text-xs text-muted-foreground">{team === undefined ? m.noTeam : m.owner(team)}</p>
          )}
          {summary !== undefined && <Published summary={summary} m={m} locale={i18n.locale} />}
        </CardFooter>
      </Card>
    </li>
  );
}

/** One app as a row of the list view (STU-020): shadcn's Item with the same facts, side by side. */
export function AppRow(props: AppCardProps): ReactElement {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  const { entry, team } = props;
  const { app, summary } = entry;
  const n = new Intl.NumberFormat(i18n.locale);
  return (
    <Item variant="outline" render={<li />} className="relative bg-card hover:bg-muted/40">
      <ItemMedia>
        <CardVisual app={app} compact />
      </ItemMedia>
      <ItemContent className="min-w-40">
        <ItemTitle>
          <NameLink entry={entry} org={props.org} />
        </ItemTitle>
        <ItemDescription>{team === undefined ? m.noTeam : m.owner(team)}</ItemDescription>
      </ItemContent>
      {app.archived && <StatusPill status="neutral" label={m.archivedBadge} />}
      {summary !== undefined && (
        <div className="flex flex-wrap items-center gap-3">
          <Environments releases={summary.releases} m={m} />
          <Health summary={summary} m={m} locale={i18n.locale} />
          <dl className="flex gap-4 text-sm">
            <div className="flex items-baseline gap-1">
              <dt className="text-muted-foreground">{m.devices}</dt>
              <dd className="font-medium tabular-nums">{n.format(summary.activeDevices)}</dd>
            </div>
            <div className="flex items-baseline gap-1">
              <dt className="text-muted-foreground">{m.plugins}</dt>
              <dd className="font-medium tabular-nums">{n.format(summary.pluginCount)}</dd>
            </div>
          </dl>
          <Published summary={summary} m={m} locale={i18n.locale} />
        </div>
      )}
      <ItemActions className="relative z-10">
        <PinButton entry={entry} pinned={props.pinned} handlers={props.handlers} />
        <AppActions entry={entry} org={props.org} permissions={props.permissions} handlers={props.handlers} />
      </ItemActions>
    </Item>
  );
}
