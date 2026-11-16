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

/** How much farther than the snap distance a held snap lets go: past it the box moves freely again (no flicker). */
export const SNAP_RELEASE = 2;

/** One axis's snap: the line it sits on and which of the moving box's lines (start, centre, end) sits there. */
export interface SnapLock {
  readonly target: number;
  readonly edge: 0 | 1 | 2;
}

export interface SnapResult {
  readonly delta: Point;
  readonly guides: readonly Guide[];
  /** The snap of each axis, for the next move to hold (hysteresis). */
  readonly locks: { readonly x?: SnapLock | undefined; readonly y?: SnapLock | undefined };
}

const xs = (b: Box): number[] => [b.l, (b.l + b.r) / 2, b.r];
const ys = (b: Box): number[] => [b.t, (b.t + b.b) / 2, b.b];

/**
 * The offset that puts one of `mine` on the nearest of `theirs` within `limit`. A snap held from the previous move stays
 * while its line is within `limit × SNAP_RELEASE`: near the threshold the box no longer flips between snapped and free
 * on every pixel, which read as a vibration while dragging.
 */
function bestOffset(
  mine: readonly number[],
  theirs: readonly number[],
  limit: number,
  held: SnapLock | undefined,
): SnapLock | undefined {
  if (held !== undefined && theirs.some((t) => Math.abs(t - held.target) < 1e-6)) {
    const at = mine[held.edge] ?? 0;
    if (Math.abs(held.target - at) <= limit * SNAP_RELEASE) {
      return held;
    }
  }
  let best: { lock: SnapLock; d: number } | undefined;
  mine.forEach((a, edge) => {
    for (const c of theirs) {
      const d = c - a;
      if (Math.abs(d) <= limit && (best === undefined || Math.abs(d) < Math.abs(best.d))) {
        best = { lock: { target: c, edge: edge as 0 | 1 | 2 }, d };
      }
    }
  });
  return best?.lock;
}

/**
 * The guides of a snapped box: one line per aligned position, spanning the box and every other box on it, so a row of
 * aligned screens draws one line rather than one per screen.
 */
function mergedGuides(snapped: Box, others: readonly Box[]): Guide[] {
  const lines = new Map<string, Guide>();
  const add = (g: Guide) => {
    const key = `${g.axis}:${Math.round(g.at * 1000)}`;
    const was = lines.get(key);
    lines.set(key, was === undefined ? g : { ...was, from: Math.min(was.from, g.from), to: Math.max(was.to, g.to) });
  };
  for (const o of others) {
    for (const at of xs(snapped)) {
      if (xs(o).some((v) => Math.abs(v - at) < 1e-6)) {
        add({ axis: "x", at, from: Math.min(snapped.t, o.t), to: Math.max(snapped.b, o.b) });
      }
    }
    for (const at of ys(snapped)) {
      if (ys(o).some((v) => Math.abs(v - at) < 1e-6)) {
        add({ axis: "y", at, from: Math.min(snapped.l, o.l), to: Math.max(snapped.r, o.r) });
      }
    }
  }
  return [...lines.values()];
}

/**
 * Snaps `moving` (already at its dragged position) to the edges and centres of `others`. `zoom` converts the
 * pixel threshold to world units; `held` is the previous move's snap, kept until the pointer is clearly away.
 * Returns the correction to add, the guides to draw on the overlay and the snaps to hold.
 */
export function snapBox(moving: Box, others: readonly Box[], zoom: number, held: SnapResult["locks"] = {}): SnapResult {
  const limit = SNAP_PX / zoom;
  const lx = bestOffset(xs(moving), others.flatMap(xs), limit, held.x);
  const ly = bestOffset(ys(moving), others.flatMap(ys), limit, held.y);
  const dx = lx === undefined ? 0 : lx.target - (xs(moving)[lx.edge] ?? 0);
  const dy = ly === undefined ? 0 : ly.target - (ys(moving)[ly.edge] ?? 0);
  const snapped = translate(moving, { x: dx, y: dy });
  return { delta: { x: dx, y: dy }, guides: mergedGuides(snapped, others), locks: { x: lx, y: ly } };
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
