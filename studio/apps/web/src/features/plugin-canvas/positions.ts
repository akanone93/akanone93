// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Where the plugin's screens sit on the canvas (STU-050, L-10). The saved positions live in the
 * editor-only `canvas` document (kind "canvas"); when none is saved, or the kind is not yet
 * served, screens are arranged by flow (`arrangeScreens`, #241). This module is the one place
 * that knows the document's shape.
 */

import { arrange, VIEW_ASPECT } from "../../lib/arrange.ts";

export interface Point {
  readonly x: number;
  readonly y: number;
}

/**
 * Space Arrange keeps between screens, in world units (#241): across, room for the arrows to run round them; down,
 * room for a screen's title above it and the line under the one above, at the zooms the canvas opens at.
 */
export const SCREEN_GAP = 200;
export const SCREEN_ROW_GAP = 320;

/** The kind of the editor-only positions document. */
export const CANVAS_KIND = "canvas";

/** A document as far as positions need it. */
export interface KindedDocument {
  readonly kind: string;
  readonly content: Uint8Array;
  /** The content already parsed (the editor's local state); wins over `content`. */
  readonly parsed?: unknown;
}

/** Saved positions by screen ID, or undefined when the plugin has no usable `canvas` document. */
export function readSavedPositions(documents: readonly KindedDocument[]): ReadonlyMap<string, Point> | undefined {
  const doc = documents.find((d) => d.kind === CANVAS_KIND);
  if (doc === undefined) {
    return undefined;
  }
  try {
    return positionsOf(doc.parsed !== undefined ? doc.parsed : JSON.parse(new TextDecoder().decode(doc.content)));
  } catch {
    return undefined;
  }
}

/** The positions of a parsed `canvas` document, or undefined when it holds none. */
export function positionsOf(parsed: unknown): ReadonlyMap<string, Point> | undefined {
  const list =
    typeof parsed === "object" && parsed !== null ? (parsed as { positions?: unknown }).positions : undefined;
  if (!Array.isArray(list)) {
    return undefined;
  }
  const out = new Map<string, Point>();
  for (const p of list as { id?: unknown; x?: unknown; y?: unknown }[]) {
    if (typeof p?.id === "string" && typeof p.x === "number" && typeof p.y === "number") {
      if (Number.isFinite(p.x) && Number.isFinite(p.y)) {
        out.set(p.id, { x: p.x, y: p.y });
      }
    }
  }
  return out.size > 0 ? out : undefined;
}

/** The path of a plugin's `canvas` document beside its `plugin.json` (`plugins/<key>/canvas.json`, canvas schema). */
export function canvasPathOf(pluginPath: string): string {
  return `${pluginPath.slice(0, pluginPath.length - "plugin.json".length)}canvas.json`;
}

/** The saved form of `positions`: whole world units, in ID order, so the document does not churn. */
export function positionList(positions: ReadonlyMap<string, Point>): { id: string; x: number; y: number }[] {
  return [...positions.entries()]
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([id, p]) => ({ id, x: Math.round(p.x), y: Math.round(p.y) }));
}

/** A new plugin `canvas` document holding `positions`. */
export function positionsDocument(id: string, positions: ReadonlyMap<string, Point>) {
  return { schemaVersion: "1.0.0", kind: CANVAS_KIND, id, positions: positionList(positions) };
}

/** The edit that stores `positions` in an existing `canvas` document, keeping everything else in it. */
export function positionsRecipe(positions: ReadonlyMap<string, Point>): (draft: Record<string, unknown>) => void {
  return (draft) => {
    draft["positions"] = positionList(positions);
  };
}

/**
 * Screens in flow order: breadth-first from `entry` along `edges`; screens the flow does not reach
 * follow in their listed order. Edge targets that are not screens are ignored.
 */
export function flowOrder(
  ids: readonly string[],
  entry: string | undefined,
  edges: ReadonlyMap<string, readonly string[]>,
): string[] {
  const known = new Set(ids);
  const seen = new Set<string>();
  const order: string[] = [];
  const walk = (start: string) => {
    const queue = [start];
    while (queue.length > 0) {
      const id = queue.shift();
      if (id === undefined || seen.has(id) || !known.has(id)) {
        continue;
      }
      seen.add(id);
      order.push(id);
      queue.push(...(edges.get(id) ?? []));
    }
  };
  if (entry !== undefined) {
    walk(entry);
  }
  for (const id of ids) {
    walk(id);
  }
  return order;
}

/** A screen's size for arranging: the device's width, and its laid-out height (at least the device's). */
export type ScreenSize = (id: string) => { readonly w: number; readonly h: number };

/**
 * The screens arranged by flow (STU-050, #241): a layered layout from the entry along the links, wrapped and packed
 * towards the view's shape, the screens no link reaches in a grid beside them (`arrange`).
 */
export function arrangeScreens(
  ids: readonly string[],
  entry: string | undefined,
  edges: ReadonlyMap<string, readonly string[]>,
  size: ScreenSize,
): Map<string, Point> {
  const links = ids.flatMap((id) => (edges.get(id) ?? []).map((to) => ({ from: id, to })));
  return arrange(
    ids.map((id) => ({ id, ...size(id) })),
    links,
    { gapX: SCREEN_GAP, gapY: SCREEN_ROW_GAP, groupGap: SCREEN_ROW_GAP, aspect: VIEW_ASPECT, entry },
  );
}

/** Saved positions where present; the other screens arranged by flow beside them, to the right of the saved ones. */
export function placeScreens(
  ids: readonly string[],
  entry: string | undefined,
  edges: ReadonlyMap<string, readonly string[]>,
  size: ScreenSize,
  saved: ReadonlyMap<string, Point> | undefined,
): Map<string, Point> {
  const unsaved = ids.filter((id) => saved?.get(id) === undefined);
  const out = new Map<string, Point>();
  let right = 0;
  let top = Number.POSITIVE_INFINITY;
  for (const id of ids) {
    const p = saved?.get(id);
    if (p !== undefined) {
      out.set(id, p);
      right = Math.max(right, p.x + size(id).w + SCREEN_GAP);
      top = Math.min(top, p.y);
    }
  }
  if (unsaved.length > 0) {
    const laid = arrangeScreens(unsaved, entry, edges, size);
    const y = Number.isFinite(top) ? top : 0;
    for (const id of unsaved) {
      const p = laid.get(id) ?? { x: 0, y: 0 };
      out.set(id, { x: right + p.x, y: y + p.y });
    }
  }
  return out;
}
