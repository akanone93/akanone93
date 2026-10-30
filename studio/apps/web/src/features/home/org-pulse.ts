// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { AppEntry } from "../apps/apps-model.ts";

/** The environment whose live release counts an app as live in the organisation's pulse. */
export const productionKey = "production";

/** The organisation at a glance, summed from its apps' summaries (STU-015, STU-022). */
export interface OrgPulse {
  readonly apps: number;
  readonly devices: bigint;
  /** Apps with a live release in production. */
  readonly live: number;
  /** The crash-free rate over all active devices, each app weighted by its devices; undefined while no app has any. */
  readonly crashFree: number | undefined;
  readonly plugins: bigint;
}

export function orgPulse(entries: readonly AppEntry[]): OrgPulse {
  let devices = 0n;
  let plugins = 0n;
  let live = 0;
  let weighted = 0;
  for (const { summary } of entries) {
    if (summary === undefined) {
      continue;
    }
    devices += summary.activeDevices;
    plugins += summary.pluginCount;
    weighted += summary.crashFreeRate * Number(summary.activeDevices);
    if (summary.releases.some((r) => r.environmentKey === productionKey && r.liveSequence > 0n)) {
      live += 1;
    }
  }
  return {
    apps: entries.length,
    devices,
    live,
    crashFree: devices > 0n ? weighted / Number(devices) : undefined,
    plugins,
  };
}

/** The apps with the most active devices first, at most `count`; the hero stacks them (STU-015). */
export function busiest(entries: readonly AppEntry[], count: number): AppEntry[] {
  const devices = (e: AppEntry) => e.summary?.activeDevices ?? 0n;
  return [...entries]
    .sort((a, b) => {
      const d = devices(b) - devices(a);
      return d > 0n ? 1 : d < 0n ? -1 : a.app.name.localeCompare(b.app.name);
    })
    .slice(0, count);
}
