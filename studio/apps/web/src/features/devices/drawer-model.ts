// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type DevEvent, DevLogLevel } from "@plux/api";

/** The nine tabs of the bottom drawer, in display order (STU-066). */
export const drawerTabs = [
  "problems",
  "logs",
  "flow",
  "state",
  "network",
  "functions",
  "sync",
  "performance",
  "console",
] as const;
export type DrawerTab = (typeof drawerTabs)[number];

/**
 * Events the drawer keeps in view. The limits registry holds no limit for a viewer's window (the server
 * bounds retention), so this is the view's own cap, not a policy limit.
 */
export const viewWindow = 1000;

const tabByCase: Readonly<Record<string, DrawerTab>> = {
  log: "logs",
  error: "logs",
  drop: "logs",
  actionTrace: "flow",
  stateDiff: "state",
  networkCall: "network",
  functionCall: "functions",
  sync: "sync",
  hotApply: "sync",
  frameTiming: "performance",
  commandResult: "console",
};

/** The tab an event belongs to; events no tab shows (screenshots, inspect, security) have none. */
export function tabOf(event: DevEvent): DrawerTab | undefined {
  const kind = event.payload.case;
  return kind === undefined ? undefined : tabByCase[kind];
}

type Summaries = { [K in NonNullable<DevEvent["payload"]["case"]>]?: (value: never) => string };

const summaries: Summaries = {
  log: (v: { source: string; message: string }) => `${v.source} ${v.message}`,
  error: (v: { error?: { code: string; message: string } }) => `${v.error?.code ?? ""} ${v.error?.message ?? ""}`,
  drop: (v: { dropped: bigint }) => `dropped ${v.dropped}`,
  actionTrace: (v: { graph: string; trigger: string }) => `${v.graph} ${v.trigger}`,
  stateDiff: (v: { scope: string; changes: { path: string }[] }) =>
    `${v.scope} ${v.changes.map((c) => c.path).join(", ")}`,
  networkCall: (v: { method: string; urlTemplate: string; status: number }) =>
    `${v.method} ${v.urlTemplate} ${v.status}`,
  functionCall: (v: { function: string; placement: string }) => `${v.function} ${v.placement}`,
  sync: (v: { plugin: string; releaseSequence: bigint }) => `${v.plugin} #${v.releaseSequence}`,
  hotApply: (v: { pagesApplied: string[] }) => v.pagesApplied.join(", "),
  frameTiming: (v: { route: string; frames: unknown[] }) => `${v.route} ${v.frames.length} frames`,
  commandResult: (v: { commandId: string; status: number }) => `${v.commandId} status ${v.status}`,
};

/** A one-line, plain-text summary of an event; it is what a row shows and what search matches. */
export function summarise(event: DevEvent): string {
  const kind = event.payload.case;
  const make = kind === undefined ? undefined : (summaries[kind] as ((value: unknown) => string) | undefined);
  return make === undefined ? "" : make(event.payload.value).trim();
}

export type LevelFilter = "all" | "warning" | "error";

export interface DrawerFilter {
  /** Empty means every device. */
  readonly deviceId: string;
  readonly level: LevelFilter;
  readonly search: string;
}

const floor: Record<LevelFilter, number> = { all: 0, warning: DevLogLevel.WARNING, error: DevLogLevel.ERROR };

/** The level filter narrows log lines only; an error event is an error at every level. */
function passesLevel(event: DevEvent, level: LevelFilter): boolean {
  if (event.payload.case === "log") {
    return event.payload.value.level >= floor[level];
  }
  return true;
}

/** Whether the event passes the drawer's source device, level and search filters. */
export function matches(event: DevEvent, filter: DrawerFilter): boolean {
  if (filter.deviceId !== "" && event.deviceId !== filter.deviceId) {
    return false;
  }
  if (!passesLevel(event, filter.level)) {
    return false;
  }
  const needle = filter.search.trim().toLowerCase();
  return needle === "" || summarise(event).toLowerCase().includes(needle);
}

/** The retained events and the point at which the view was frozen, if it is paused. */
export interface Buffer {
  readonly events: readonly DevEvent[];
  readonly frozenAt: number | undefined;
}

export const emptyBuffer: Buffer = { events: [], frozenAt: undefined };

/** Appends an event, dropping the oldest beyond the window; a frozen view keeps showing what it showed. */
export function push(buffer: Buffer, event: DevEvent, window = viewWindow): Buffer {
  const events = [...buffer.events, event];
  const over = Math.max(0, events.length - window);
  const frozenAt = buffer.frozenAt === undefined ? undefined : Math.max(0, buffer.frozenAt - over);
  return { events: over === 0 ? events : events.slice(over), frozenAt };
}

export function pause(buffer: Buffer): Buffer {
  return { ...buffer, frozenAt: buffer.events.length };
}

export function resume(buffer: Buffer): Buffer {
  return { ...buffer, frozenAt: undefined };
}

/** What the drawer shows: everything, or only what arrived before the pause. */
export function visible(buffer: Buffer): readonly DevEvent[] {
  return buffer.frozenAt === undefined ? buffer.events : buffer.events.slice(0, buffer.frozenAt);
}

/** Events that arrived since the pause. */
export function pending(buffer: Buffer): number {
  return buffer.frozenAt === undefined ? 0 : buffer.events.length - buffer.frozenAt;
}
