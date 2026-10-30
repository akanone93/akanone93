// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, type IconName } from "@plux/ui";
import { Badge } from "@plux/ui/components/badge";
import { Card, CardAction, CardDescription, CardFooter, CardHeader, CardTitle } from "@plux/ui/components/card";
import type { ReactNode } from "react";
import { CountUp } from "./count-up.tsx";
import { homeMessages } from "./home.messages.ts";
import type { OrgPulse } from "./org-pulse.ts";

/** One figure: shadcn's section card (dashboard-01), the label over the number, a badge, and a note at the foot. */
function Figure({
  icon,
  label,
  note,
  badge,
  children,
}: {
  readonly icon: IconName;
  readonly label: string;
  readonly note: string;
  readonly badge?: ReactNode;
  readonly children: ReactNode;
}) {
  return (
    <Card className="@container/card">
      <CardHeader>
        <CardDescription>{label}</CardDescription>
        <CardTitle className="text-2xl font-semibold tabular-nums @3xs/card:text-3xl">{children}</CardTitle>
        <CardAction>{badge}</CardAction>
      </CardHeader>
      <CardFooter className="flex-col items-start gap-1.5 text-sm">
        <div className="flex items-center gap-2 text-muted-foreground">
          <Icon name={icon} size="pill" />
          {note}
        </div>
      </CardFooter>
    </Card>
  );
}

/**
 * The organisation at a glance (STU-015): four figures summed from the apps' summaries, as shadcn's section cards.
 * Only the crash-free rate has a trend the server reports, so only it carries a trend badge.
 */
export function Pulse({ pulse }: { readonly pulse: OrgPulse }) {
  const { i18n } = useLingui();
  const m = homeMessages(i18n);
  const whole = new Intl.NumberFormat(i18n.locale, { maximumFractionDigits: 0 });
  const percent = new Intl.NumberFormat(i18n.locale, {
    style: "percent",
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  });
  const points = new Intl.NumberFormat(i18n.locale, { maximumFractionDigits: 1, signDisplay: "exceptZero" });
  const trend = pulse.crashFreeTrend;
  return (
    <section
      aria-label={m.pulseLabel}
      className="home-figures grid grid-cols-1 gap-4 px-4 lg:px-6 @xl/main:grid-cols-2 @5xl/main:grid-cols-4"
    >
      <Figure icon="device-mobile" label={m.pulseDevices} note={m.pulseDevicesNote(pulse.apps)}>
        <CountUp value={Number(pulse.devices)} format={(v) => whole.format(Math.round(v))} />
      </Figure>
      <Figure icon="rocket-launch" label={m.pulseLive} note={m.pulseLiveNote(pulse.apps)}>
        <CountUp value={pulse.live} format={(v) => whole.format(Math.round(v))} />
      </Figure>
      <Figure
        icon="pulse"
        label={m.pulseCrashFree}
        note={m.pulseCrashFreeNote}
        badge={
          trend === undefined ? undefined : (
            <Badge variant="outline">
              <Icon name={trend >= 0 ? "caret-up" : "caret-down"} data-icon="inline-start" />
              {m.pulseTrend(points.format(trend * 100))}
            </Badge>
          )
        }
      >
        {pulse.crashFree === undefined ? (
          <span className="text-muted-foreground">{m.pulseNoRate}</span>
        ) : (
          <CountUp value={pulse.crashFree} format={(v) => percent.format(v)} />
        )}
      </Figure>
      <Figure icon="puzzle-piece" label={m.pulsePlugins} note={m.pulsePluginsNote}>
        <CountUp value={Number(pulse.plugins)} format={(v) => whole.format(Math.round(v))} />
      </Figure>
    </section>
  );
}
