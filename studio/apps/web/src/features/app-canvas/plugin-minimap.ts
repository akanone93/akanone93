// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { autoLayout, type Size } from "./app-canvas.model.ts";
import type { AppGraph } from "./app-canvas.scene.ts";

/** The mini-map of a plugin's screens on its detail card (STU-031): screens as small boxes, navigation as lines. */

export interface MiniScreen {
  readonly id: string;
  readonly title: string;
  readonly x: number;
  readonly y: number;
}

export interface MiniEdge {
  readonly from: string;
  readonly to: string;
}

export interface MiniMap {
  readonly width: number;
  readonly height: number;
  /** The size of every screen box in the map's own units. */
  readonly cell: Size;
  readonly screens: readonly MiniScreen[];
  readonly edges: readonly MiniEdge[];
}

export const MINI_SIZE: Size = { width: 232, height: 72 };
const CELL: Size = { width: 28, height: 20 };
const MARGIN = 4;

/**
 * The screens of plugin `pluginId` and the navigate links among them, laid out left to right and scaled to fit
 * `MINI_SIZE`. Screens are ordered by title and ID so the same app always draws the same map.
 */
export function miniMapOf(graph: AppGraph, pluginId: string): MiniMap {
  const pages = [...graph.pages.values()]
    .filter((p) => p.pluginId === pluginId)
    .sort((a, b) => (a.title === b.title ? (a.id < b.id ? -1 : 1) : a.title < b.title ? -1 : 1));
  const ids = new Set(pages.map((p) => p.id));
  const edges = graph.links
    .filter((l) => l.targetPage !== undefined && ids.has(l.sourcePage) && ids.has(l.targetPage))
    .map((l) => ({ from: l.sourcePage, to: l.targetPage ?? "" }));
  const laid = autoLayout(
    pages.map((p) => p.id),
    edges,
    CELL,
  );
  const xs = [...laid.values()].map((p) => p.x);
  const ys = [...laid.values()].map((p) => p.y);
  const minX = Math.min(...xs, 0);
  const minY = Math.min(...ys, 0);
  const spanX = Math.max(...xs, 0) - minX + CELL.width;
  const spanY = Math.max(...ys, 0) - minY + CELL.height;
  const scale = Math.min(1, (MINI_SIZE.width - 2 * MARGIN) / spanX, (MINI_SIZE.height - 2 * MARGIN) / spanY);
  const screens = pages.map((p) => {
    const at = laid.get(p.id) ?? { x: 0, y: 0 };
    return {
      id: p.id,
      title: p.title,
      x: Math.round(MARGIN + (at.x - minX) * scale),
      y: Math.round(MARGIN + (at.y - minY) * scale),
    };
  });
  return {
    width: MINI_SIZE.width,
    height: MINI_SIZE.height,
    cell: { width: Math.round(CELL.width * scale), height: Math.round(CELL.height * scale) },
    screens,
    edges,
  };
}
