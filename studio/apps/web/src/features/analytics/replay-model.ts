// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReplayEvent, ReplayNode } from "@plux/api";

/** How long a tap stays marked on the canvas while a replay plays. */
export const TAP_WINDOW_MS = 800;

export interface ReplayTap {
  readonly nodePath: string;
  readonly fx: number;
  readonly fy: number;
}

export interface ReplayState {
  readonly type: string;
  readonly value: string;
}

/** What a replay shows at one moment (ANL-050): the last screen and layout, recent taps, and the state so far. */
export interface ReplayFrame {
  readonly route: string;
  readonly plugin: string;
  readonly nodes: readonly ReplayNode[];
  readonly taps: readonly ReplayTap[];
  readonly inputs: readonly string[];
  readonly state: ReadonlyMap<string, ReplayState>;
}

/** The frame at [at] milliseconds: events are applied in order up to and including it. */
export function frameAt(events: readonly ReplayEvent[], at: number): ReplayFrame {
  let route = "";
  let plugin = "";
  let nodes: readonly ReplayNode[] = [];
  const taps: ReplayTap[] = [];
  const inputs: string[] = [];
  const state = new Map<string, ReplayState>();
  for (const e of events) {
    if (e.atMs > at) {
      break;
    }
    switch (e.kind) {
      case "screen":
        route = e.route;
        plugin = e.pluginKey;
        nodes = [];
        taps.length = 0;
        break;
      case "layout":
        nodes = e.nodes;
        break;
      case "tap":
        if (at - e.atMs <= TAP_WINDOW_MS) {
          taps.push({ nodePath: e.nodePath, fx: e.fx, fy: e.fy });
        }
        break;
      case "input":
        if (at - e.atMs <= TAP_WINDOW_MS) {
          inputs.push(e.nodePath);
        }
        break;
      case "state":
        state.set(e.name, { type: e.valueType, value: e.value });
        break;
    }
  }
  return { route, plugin, nodes, taps, inputs, state };
}

/** The width and height that hold every node, at least 1 so that a view box is valid. */
export function extentOf(nodes: readonly ReplayNode[]): { width: number; height: number } {
  let width = 1;
  let height = 1;
  for (const n of nodes) {
    width = Math.max(width, n.x + n.width);
    height = Math.max(height, n.y + n.height);
  }
  return { width, height };
}

/** The point of a tap on the canvas, inside its node. */
export function tapPoint(tap: ReplayTap, nodes: readonly ReplayNode[]): { x: number; y: number } | undefined {
  const n = nodes.find((x) => x.path === tap.nodePath);
  return n === undefined ? undefined : { x: n.x + tap.fx * n.width, y: n.y + tap.fy * n.height };
}
