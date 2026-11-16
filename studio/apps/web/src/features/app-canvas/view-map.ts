// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** A rectangle in world units. */
export interface Rect {
  readonly l: number;
  readonly t: number;
  readonly r: number;
  readonly b: number;
}

/** A canvas view: the world point at its top start corner and the zoom (canvas pixels per world unit). */
export interface View {
  readonly x: number;
  readonly y: number;
  readonly zoom: number;
}

/** How the map draws the world: a map point is `world * scale + offset`. */
export interface MapFrame {
  readonly scale: number;
  readonly ox: number;
  readonly oy: number;
}

/** The part of the world a view of `size` canvas pixels shows. */
export function viewRect(view: View, size: { readonly w: number; readonly h: number }): Rect {
  return { l: view.x, t: view.y, r: view.x + size.w / view.zoom, b: view.y + size.h / view.zoom };
}

/**
 * Fits the items and the view into a map of `box` pixels, with a margin of `pad` of the content on each side, centred.
 * The view is part of what is fitted, so the frame that marks it never leaves the map.
 */
export function mapFrame(
  items: readonly Rect[],
  view: Rect,
  box: { readonly w: number; readonly h: number },
  pad = 0.08,
): MapFrame {
  const all = [...items, view];
  const l = Math.min(...all.map((r) => r.l));
  const t = Math.min(...all.map((r) => r.t));
  const w = Math.max(1, Math.max(...all.map((r) => r.r)) - l);
  const h = Math.max(1, Math.max(...all.map((r) => r.b)) - t);
  const scale = Math.min(box.w / (w * (1 + 2 * pad)), box.h / (h * (1 + 2 * pad)));
  return { scale, ox: (box.w - w * scale) / 2 - l * scale, oy: (box.h - h * scale) / 2 - t * scale };
}

/** A world rectangle in map pixels. */
export function onMap(f: MapFrame, r: Rect): { x: number; y: number; w: number; h: number } {
  return { x: r.l * f.scale + f.ox, y: r.t * f.scale + f.oy, w: (r.r - r.l) * f.scale, h: (r.b - r.t) * f.scale };
}

/** The world point under a map point. */
export function fromMap(f: MapFrame, p: { readonly x: number; readonly y: number }): { x: number; y: number } {
  return { x: (p.x - f.ox) / f.scale, y: (p.y - f.oy) / f.scale };
}

/**
 * The pan, in canvas pixels as the surface takes it, that puts world point `centre` at the middle of the view. A pan
 * of (dx, dy) moves the content by that much, so the view's world origin moves by (-dx, -dy) / zoom.
 */
export function panTo(
  view: View,
  size: { readonly w: number; readonly h: number },
  centre: { readonly x: number; readonly y: number },
): { dx: number; dy: number } {
  const cx = view.x + size.w / (2 * view.zoom);
  const cy = view.y + size.h / (2 * view.zoom);
  return { dx: (cx - centre.x) * view.zoom, dy: (cy - centre.y) * view.zoom };
}
