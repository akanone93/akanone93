// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Where the plugin's screens sit on the canvas (STU-050, L-10). The saved positions live in the
 * editor-only `canvas` document (kind "canvas"); when none is saved, or the kind is not yet
 * served, screens are laid out left to right in navigation-flow order. This module is the one
 * place that knows the document's shape.
 */

export interface Point {
  readonly x: number;
  readonly y: number;
}

/** Horizontal space between auto-laid-out screens, in world units. */
export const SCREEN_GAP = 160;

/** The kind of the editor-only positions document. */
export const CANVAS_KIND = "canvas";

/** A document as far as positions need it. */
export interface KindedDocument {
  readonly kind: string;
  readonly content: Uint8Array;
}

/** Saved positions by screen ID, or undefined when the plugin has no usable `canvas` document. */
export function readSavedPositions(documents: readonly KindedDocument[]): ReadonlyMap<string, Point> | undefined {
  const doc = documents.find((d) => d.kind === CANVAS_KIND);
  if (doc === undefined) {
    return undefined;
  }
  try {
    const parsed: unknown = JSON.parse(new TextDecoder().decode(doc.content));
    const list = (parsed as { positions?: unknown }).positions;
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
  } catch {
    return undefined;
  }
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

/** One row, left to right in `order`, each screen `widths` wide with a gap between. */
export function autoLayout(order: readonly string[], widths: ReadonlyMap<string, number>): Map<string, Point> {
  const out = new Map<string, Point>();
  let x = 0;
  for (const id of order) {
    out.set(id, { x, y: 0 });
    x += (widths.get(id) ?? 0) + SCREEN_GAP;
  }
  return out;
}

/** Saved positions where present, auto-layout for the rest (placed after the rightmost saved screen). */
export function placeScreens(
  order: readonly string[],
  widths: ReadonlyMap<string, number>,
  saved: ReadonlyMap<string, Point> | undefined,
): Map<string, Point> {
  const auto = autoLayout(order, widths);
  if (saved === undefined) {
    return auto;
  }
  const out = new Map<string, Point>();
  let right = 0;
  for (const id of order) {
    const p = saved.get(id);
    if (p !== undefined) {
      out.set(id, p);
      right = Math.max(right, p.x + (widths.get(id) ?? 0) + SCREEN_GAP);
    }
  }
  for (const id of order) {
    if (!out.has(id)) {
      out.set(id, { x: right, y: 0 });
      right += (widths.get(id) ?? 0) + SCREEN_GAP;
    }
  }
  return out;
}
