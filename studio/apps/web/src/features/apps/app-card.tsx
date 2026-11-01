// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useLingui } from "@lingui/react";
import type { AppSummary, EnvironmentRelease } from "@plux/api";
import { Icon, StatusPill } from "@plux/ui";
import { Badge } from "@plux/ui/components/badge";
import { Button } from "@plux/ui/components/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@plux/ui/components/card";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@plux/ui/components/hover-card";
import { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle } from "@plux/ui/components/item";
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
      <Badge variant="outline" className="text-muted-foreground">
        {m.noEnvironments}
      </Badge>
    );
  }
  return (
    <ul aria-label={m.environments} className="flex flex-wrap gap-1.5">
      {releases.map((r) => (
        <li key={r.environmentKey}>
          <Badge variant="outline">
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

/**
 * The health badge with its error trend (STU-022): the crash-free rate, and its change against the week before. The
 * compact form shows the trend as an arrow and keeps its words for screen readers; the detail card spells it out.
 */
function Health({
  summary,
  m,
  locale,
  compact = false,
}: {
  readonly summary: AppSummary;
  readonly m: Messages;
  readonly locale: string;
  readonly compact?: boolean;
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
  const trend = flat ? m.trendFlat : health.trend > 0 ? m.trendUp(points) : m.trendDown(points);
  return (
    <span className="flex flex-wrap items-center gap-2">
      <StatusPill status={status} label={word} value={m.crashFree(percent.format(health.rate))} />
      <span className="flex items-center gap-1 text-xs text-muted-foreground">
        {!flat && <Icon name={health.trend > 0 ? "caret-up" : "caret-down"} size="pill" />}
        <span className={compact ? "sr-only" : undefined}>{trend}</span>
      </span>
    </span>
  );
}

/** The date, time and publisher of the newest release, or a word when there is none (STU-022). */
function Published({
  summary,
  m,
  locale,
  compact = false,
}: {
  readonly summary: AppSummary;
  readonly m: Messages;
  readonly locale: string;
  readonly compact?: boolean;
}) {
  const at = summary.lastPublishedAt;
  if (at === undefined || (at.seconds === 0n && at.nanos === 0)) {
    return <p className="text-xs">{m.neverPublished}</p>;
  }
  const when = new Intl.DateTimeFormat(
    locale,
    compact ? { dateStyle: "medium" } : { dateStyle: "medium", timeStyle: "short" },
  ).format(timestampDate(at));
  const who = summary.lastPublisher?.display ?? "";
  return <p className="truncate text-xs">{who === "" ? m.published(when) : m.publishedBy(when, who)}</p>;
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

/** The name as a link to the app's canvas; it stretches over the whole card, so the card opens the app. */
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

/** The two figures of the detail card: active devices and plugins, side by side in one band, value over label. */
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
    <dl className="app-figures grid grid-cols-2 rounded-lg border">
      {[
        { label: m.devices, value: n.format(summary.activeDevices) },
        { label: m.plugins, value: n.format(summary.pluginCount) },
      ].map((f) => (
        <div key={f.label} className="flex flex-col-reverse gap-0.5 px-3 py-2">
          <dt className="text-xs text-muted-foreground">{f.label}</dt>
          <dd className="text-lg font-semibold tracking-tight tabular-nums">{f.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** The figures of a compact card: an icon and the number each, their names for screen readers. */
function CompactFigures({
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
    <dl className="flex items-center gap-3">
      {[
        { label: m.devices, value: summary.activeDevices, icon: "device-mobile" as const },
        { label: m.plugins, value: summary.pluginCount, icon: "puzzle-piece" as const },
      ].map((f) => (
        <div key={f.label} className="flex items-center gap-1" title={f.label}>
          <Icon name={f.icon} size="pill" />
          <dt className="sr-only">{f.label}</dt>
          <dd className="font-medium text-foreground tabular-nums">{n.format(f.value)}</dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * The detail view of an app on hover or focus: its picture, everything the card shortens, and its limits. The whole
 * view is a link to the app, like the card it details, so a click lands on the app wherever the pointer rests.
 */
function AppDetail(props: AppCardProps): ReactElement {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  const home = homeMessages(i18n);
  const { entry, team } = props;
  const { app, summary } = entry;
  return (
    <Link
      to="/o/$org/apps/$app"
      params={{ org: props.org, app: app.id }}
      aria-label={home.openApp(app.name)}
      className="app-detail flex flex-col outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
    >
      <CardVisual app={app} />
      <div className="flex flex-col gap-3 p-4">
        <div className="flex flex-col gap-0.5">
          <p className="font-semibold">{app.name}</p>
          <p className="text-xs text-muted-foreground">
            <span className="font-mono">{app.key}</span> · {team === undefined ? m.noTeam : m.owner(team)}
          </p>
        </div>
        {app.description !== "" && <p className="text-sm text-muted-foreground">{app.description}</p>}
        {summary !== undefined && (
          <>
            <Environments releases={summary.releases} m={m} />
            <Figures summary={summary} m={m} locale={i18n.locale} />
            <Health summary={summary} m={m} locale={i18n.locale} />
            {summary.limits.length > 0 && <CardLimits limits={summary.limits} />}
            <div className="flex items-center gap-2 border-t pt-3 text-muted-foreground">
              <Icon name="clock" size="pill" />
              <Published summary={summary} m={m} locale={i18n.locale} />
            </div>
          </>
        )}
        <span className="app-detail-open flex items-center gap-1 text-sm font-medium">
          {m.openCanvas}
          <Icon name="arrow-right" size="pill" />
        </span>
      </div>
    </Link>
  );
}

/**
 * One app as a card (STU-020 to STU-022, STU-025): shadcn's Card, compact. Its picture, name, key and team head it,
 * with pin and actions; then a line of what it is for, its health and live releases, and at the foot its devices,
 * plugins and newest release. Resting on the name, or anywhere on the card, opens the detail card with the full
 * picture, figures and limits. The whole card opens the app; pin and actions stay their own buttons.
 */
export function AppCard(props: AppCardProps): ReactElement {
  const { i18n } = useLingui();
  const m = appsMessages(i18n);
  const { entry, team } = props;
  const { app, summary } = entry;
  // The whole card is the hover card's anchor, so the detail opens beside it, never over the pointer.
  return (
    <HoverCard>
      <HoverCardTrigger delay={450} closeDelay={150} render={<div className="relative h-full" />}>
        <Card
          size="sm"
          className="app-card h-full gap-3 transition hover:shadow-md has-focus-visible:ring-3 has-focus-visible:ring-ring/50"
        >
          <CardHeader className="flex items-center gap-3">
            <CardVisual app={app} compact />
            <div className="flex min-w-0 flex-1 flex-col">
              <CardTitle className="flex min-w-0 items-center gap-2 text-sm">
                <NameLink entry={entry} org={props.org} />
                {app.archived && <StatusPill status="neutral" label={m.archivedBadge} />}
              </CardTitle>
              <p className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
                <span className="truncate font-mono">{app.key}</span>
                <span aria-hidden="true">·</span>
                <span className="truncate">{team === undefined ? m.noTeam : m.owner(team)}</span>
              </p>
            </div>
            <div className="relative z-10 -me-1 flex shrink-0 gap-0.5">
              <PinButton entry={entry} pinned={props.pinned} handlers={props.handlers} />
              <AppActions entry={entry} org={props.org} permissions={props.permissions} handlers={props.handlers} />
            </div>
          </CardHeader>
          <CardContent className="flex flex-1 flex-col gap-2.5">
            <CardDescription className="truncate text-xs">{app.description}</CardDescription>
            {summary !== undefined && (
              <div className="flex flex-wrap items-center gap-1.5">
                <Health summary={summary} m={m} locale={i18n.locale} compact />
                <Environments releases={summary.releases} m={m} />
              </div>
            )}
            {summary !== undefined && summary.limits.length > 0 && <CardLimits limits={summary.limits} compact />}
          </CardContent>
          {summary !== undefined && (
            <CardFooter className="gap-3 text-xs text-muted-foreground">
              <CompactFigures summary={summary} m={m} locale={i18n.locale} />
              <span className="ms-auto flex min-w-0 items-center gap-1">
                <Icon name="clock" size="pill" />
                <Published summary={summary} m={m} locale={i18n.locale} compact />
              </span>
            </CardFooter>
          )}
        </Card>
      </HoverCardTrigger>
      {/* Beside the card, not over it, so the card stays under the pointer and both open the app. */}
      <HoverCardContent side="inline-end" align="start" sideOffset={12} className="w-80 overflow-hidden p-0">
        <AppDetail {...props} />
      </HoverCardContent>
    </HoverCard>
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
    <Item variant="outline" className="relative bg-card hover:bg-muted/40">
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
