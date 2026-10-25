// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Pure geometry of canvas interactions (STU-067): nudging, smart guides, resize and distance labels. */

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
/** Guides snap within this many screen pixels. */
export const SNAP_PX = 6;

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

/** A guide line: `axis` x is a vertical line at `at`; `from`..`to` is its extent on the other axis. */
export interface Guide {
  readonly axis: "x" | "y";
  readonly at: number;
  readonly from: number;
  readonly to: number;
}

export interface SnapResult {
  readonly delta: Point;
  readonly guides: readonly Guide[];
}

const xs = (b: Box): number[] => [b.l, (b.l + b.r) / 2, b.r];
const ys = (b: Box): number[] => [b.t, (b.t + b.b) / 2, b.b];

function bestOffset(mine: readonly number[], theirs: readonly number[], limit: number): number | null {
  let best: number | null = null;
  for (const a of mine) {
    for (const c of theirs) {
      const d = c - a;
      if (Math.abs(d) <= limit && (best === null || Math.abs(d) < Math.abs(best))) {
        best = d;
      }
    }
  }
  return best;
}

/**
 * Snaps `moving` (already at its dragged position) to the edges and centres of `others`. `zoom` converts the
 * pixel threshold to world units. Returns the correction to add and the guides to draw on the overlay.
 */
export function snapBox(moving: Box, others: readonly Box[], zoom: number): SnapResult {
  const limit = SNAP_PX / zoom;
  const dx = bestOffset(xs(moving), others.flatMap(xs), limit) ?? 0;
  const dy = bestOffset(ys(moving), others.flatMap(ys), limit) ?? 0;
  const snapped = translate(moving, { x: dx, y: dy });
  const guides: Guide[] = [];
  for (const o of others) {
    for (const at of xs(snapped)) {
      if (xs(o).some((v) => Math.abs(v - at) < 1e-6)) {
        guides.push({ axis: "x", at, from: Math.min(snapped.t, o.t), to: Math.max(snapped.b, o.b) });
      }
    }
    for (const at of ys(snapped)) {
      if (ys(o).some((v) => Math.abs(v - at) < 1e-6)) {
        guides.push({ axis: "y", at, from: Math.min(snapped.l, o.l), to: Math.max(snapped.r, o.r) });
      }
    }
  }
  return { delta: { x: dx, y: dy }, guides };
}

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

export interface DistanceLabel {
  readonly axis: "x" | "y";
  readonly from: number;
  readonly to: number;
  /** Position on the other axis, where the label sits. */
  readonly across: number;
  readonly value: number;
}

/** Gaps between `a` and `b` along each axis where they do not overlap, for the distance labels. */
export function distances(a: Box, b: Box): DistanceLabel[] {
  const out: DistanceLabel[] = [];
  const midY = (Math.max(a.t, b.t) + Math.min(a.b, b.b)) / 2;
  const midX = (Math.max(a.l, b.l) + Math.min(a.r, b.r)) / 2;
  if (a.r <= b.l || b.r <= a.l) {
    const [lo, hi] = a.r <= b.l ? [a.r, b.l] : [b.r, a.l];
    out.push({ axis: "x", from: lo, to: hi, across: a.t < b.b && b.t < a.b ? midY : (a.t + a.b) / 2, value: hi - lo });
  }
  if (a.b <= b.t || b.b <= a.t) {
    const [lo, hi] = a.b <= b.t ? [a.b, b.t] : [b.b, a.t];
    out.push({ axis: "y", from: lo, to: hi, across: a.l < b.r && b.l < a.r ? midX : (a.l + a.r) / 2, value: hi - lo });
  }
  return out;
}

/** The pointer is on a resize handle within this many screen pixels of the south-east corner. */
export const HANDLE_ZONE = 8;
