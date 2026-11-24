// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Pure geometry of canvas interactions (STU-067): nudging and resizing. There are no guides or snapping (ADR-0110). */

export interface Box {
  readonly l: number;
  readonly t: number;
  readonly r: number;
  readonly b: number;
}

export interface Point {
  readonly x: number;
  readonly y: number;
}

/** Arrow keys nudge by 1 px; with Shift by 10 px. */
export const NUDGE_STEP = 1;
export const NUDGE_STEP_SHIFT = 10;

export type ArrowKey = "ArrowLeft" | "ArrowRight" | "ArrowUp" | "ArrowDown";

/** The offset an arrow key moves a node by. */
export function nudgeDelta(key: ArrowKey, shift: boolean): Point {
  const n = shift ? NUDGE_STEP_SHIFT : NUDGE_STEP;
  switch (key) {
    case "ArrowLeft":
      return { x: -n, y: 0 };
    case "ArrowRight":
      return { x: n, y: 0 };
    case "ArrowUp":
      return { x: 0, y: -n };
    default:
      return { x: 0, y: n };
  }
}

export const translate = (b: Box, d: Point): Box => ({ l: b.l + d.x, t: b.t + d.y, r: b.r + d.x, b: b.b + d.y });

export type Handle = "nw" | "n" | "ne" | "e" | "se" | "s" | "sw" | "w";

/** Resizes `b` by dragging `handle` by `d`; the box never inverts and keeps at least `min` size. */
export function resizeBox(b: Box, handle: Handle, d: Point, min = 1): Box {
  let { l, t, r, b: bottom } = b;
  if (handle.includes("w")) {
    l = Math.min(l + d.x, r - min);
  }
  if (handle.includes("e")) {
    r = Math.max(r + d.x, l + min);
  }
  if (handle.includes("n")) {
    t = Math.min(t + d.y, bottom - min);
  }
  if (handle.includes("s")) {
    bottom = Math.max(bottom + d.y, t + min);
  }
  return { l, t, r, b: bottom };
}

/** The pointer is on a resize handle within this many screen pixels of the south-east corner. */
export const HANDLE_ZONE = 8;
