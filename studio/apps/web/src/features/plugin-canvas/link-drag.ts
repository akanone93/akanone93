// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Geometry of the link drag shortcut (F6, STU-047): world and canvas points, and the screen under the pointer. */

export interface Point {
  readonly x: number;
  readonly y: number;
}

export interface WorldRect {
  readonly l: number;
  readonly t: number;
  readonly r: number;
  readonly b: number;
}

export interface View {
  readonly x: number;
  readonly y: number;
  readonly zoom: number;
}

/** A point of the canvas element (CSS pixels) as a world point. */
export const toWorld = (v: View, p: Point): Point => ({ x: v.x + p.x / v.zoom, y: v.y + p.y / v.zoom });

/** A world point as a point of the canvas element. */
export const toCanvas = (v: View, p: Point): Point => ({ x: (p.x - v.x) * v.zoom, y: (p.y - v.y) * v.zoom });

/** Where a link starts: the middle of the widget's trailing edge, in world coordinates. */
export const handleAnchor = (box: WorldRect): Point => ({ x: box.r, y: (box.t + box.b) / 2 });

/** The screen whose frame contains the world point, topmost (last) first; undefined over empty canvas. */
export function screenAt(
  p: Point,
  frames: readonly { readonly id: string; readonly box: WorldRect }[],
): string | undefined {
  for (let i = frames.length - 1; i >= 0; i--) {
    const f = frames[i];
    if (f !== undefined && p.x >= f.box.l && p.x <= f.box.r && p.y >= f.box.t && p.y <= f.box.b) {
      return f.id;
    }
  }
  return undefined;
}
