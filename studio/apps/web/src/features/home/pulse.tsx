// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { useLingui } from "@lingui/react";
import { Icon, type IconName } from "@plux/ui";
import type { ReactNode } from "react";
import { CountUp } from "./count-up.tsx";
import { homeMessages } from "./home.messages.ts";
import type { OrgPulse } from "./org-pulse.ts";

/** One figure of the pulse: an icon, its label, the number and a short note under it. */
function Figure({
  icon,
  label,
  note,
  tone,
  children,
}: {
  icon: IconName;
  label: string;
  note: string;
  tone: "accent" | "success" | "warning" | "neutral";
  children: ReactNode;
}) {
  return (
    <div className="home-figure" data-tone={tone}>
      <dt className="flex items-center gap-2 text-label text-secondary">
        <span className="home-figure-icon">
          <Icon name={icon} size="tab" />
        </span>
        {label}
      </dt>
      <dd className="m-0 flex flex-col gap-1">
        <span className="font-mono text-display-sm tabular-nums">{children}</span>
        <span className="text-caption text-tertiary">{note}</span>
      </dd>
    </div>
  );
}

/** The organisation at a glance (STU-015): four figures summed from the apps' summaries, counting up as they show. */
export function Pulse({ pulse }: { pulse: OrgPulse }) {
  const { i18n } = useLingui();
  const m = homeMessages(i18n);
  const whole = new Intl.NumberFormat(i18n.locale, { maximumFractionDigits: 0 });
  const percent = new Intl.NumberFormat(i18n.locale, {
    style: "percent",
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  });
  return (
    <dl aria-label={m.pulseLabel} className="home-pulse">
      <Figure icon="device-mobile" label={m.pulseDevices} note={m.pulseDevicesNote(pulse.apps)} tone="accent">
        <CountUp value={Number(pulse.devices)} format={(v) => whole.format(Math.round(v))} />
      </Figure>
      <Figure icon="rocket-launch" label={m.pulseLive} note={m.pulseLiveNote(pulse.apps)} tone="warning">
        <CountUp value={pulse.live} format={(v) => whole.format(Math.round(v))} />
      </Figure>
      <Figure icon="pulse" label={m.pulseCrashFree} note={m.pulseCrashFreeNote} tone="success">
        {pulse.crashFree === undefined ? (
          <span className="text-title text-tertiary">{m.pulseNoRate}</span>
        ) : (
          <CountUp value={pulse.crashFree} format={(v) => percent.format(v)} />
        )}
      </Figure>
      <Figure icon="puzzle-piece" label={m.pulsePlugins} note={m.pulsePluginsNote} tone="neutral">
        <CountUp value={Number(pulse.plugins)} format={(v) => whole.format(Math.round(v))} />
      </Figure>
    </dl>
  );
}
