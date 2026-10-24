// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Axis-aligned rectangles in world or device units. Rectangles are mutable
 * so the draw path can reuse them instead of allocating per frame.
 */

/** A rectangle by its edges: left, top, right, bottom. */
export interface Rect {
  l: number;
  t: number;
  r: number;
  b: number;
}

/** A new rectangle from its edges. */
export function makeRect(l: number, t: number, r: number, b: number): Rect {
  return { l, t, r, b };
}

/** Sets `out` to the given edges and returns it. */
export function setRect(out: Rect, l: number, t: number, r: number, b: number): Rect {
  out.l = l;
  out.t = t;
  out.r = r;
  out.b = b;
  return out;
}

/** A copy of `rect`. */
export function copyRect(rect: Rect): Rect {
  return { l: rect.l, t: rect.t, r: rect.r, b: rect.b };
}

/** Whether `a` and `b` overlap with a non-empty area. */
export function intersects(a: Rect, b: Rect): boolean {
  return a.l < b.r && b.l < a.r && a.t < b.b && b.t < a.b;
}

/** Grows `target` to also cover `other`, and returns it. */
export function unionInto(target: Rect, other: Rect): Rect {
  return setRect(
    target,
    Math.min(target.l, other.l),
    Math.min(target.t, other.t),
    Math.max(target.r, other.r),
    Math.max(target.b, other.b),
  );
}

/** Sets `out` to the overlap of `a` and `b`; false when they do not overlap. */
export function intersectInto(out: Rect, a: Rect, b: Rect): boolean {
  setRect(out, Math.max(a.l, b.l), Math.max(a.t, b.t), Math.min(a.r, b.r), Math.min(a.b, b.b));
  return out.l < out.r && out.t < out.b;
}
