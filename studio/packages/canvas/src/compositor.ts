// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Compositor-assisted pan and zoom, and the interaction layer (STU-045,
 * STU-120, ADR-0013 M-12). On a machine without a GPU, re-blitting the tiles
 * of the whole view every frame costs more than a frame. Instead the worker
 * renders the base once into an overscanned surface — the view plus one tile
 * of margin on each side — and the host moves and scales the canvas element
 * with a CSS transform, which the browser compositor applies without any GL
 * work. The worker re-renders the base only when the view nears the edge of
 * the overscan, the zoom settles, or the content changes. Dragged items are
 * drawn each frame into a small interaction layer above the base, sized to
 * the bounds of what moves.
 *
 * Everything here is pure arithmetic shared by the host and the worker.
 * Coordinates are physical CSS pixels and world units: the canvas does not
 * mirror for right-to-left layouts (rule book §8.5).
 */

/** The fraction of the margin left on a side at which the base is re-rendered. */
export const RECENTRE_FRACTION = 0.25;

/** The step, in CSS pixels, in which the interaction layer is positioned and sized. */
export const LAYER_STEP = 128;

/** The world origin and zoom of the rendered base surface. */
export interface BaseOrigin {
  readonly x: number;
  readonly y: number;
  readonly zoom: number;
}

/** The view as the compositor needs it: world top-left, zoom and CSS size. */
export interface ViewLike {
  readonly x: number;
  readonly y: number;
  readonly zoom: number;
  readonly width: number;
  readonly height: number;
}

/** A CSS transform of the base element: translate by (`tx`, `ty`) CSS pixels, then scale. */
export interface ElementTransform {
  readonly tx: number;
  readonly ty: number;
  readonly scale: number;
}

/** A rectangle in CSS pixels of the base surface. */
export interface LayerBox {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
}

/** The overscan margin in CSS pixels: one tile on each side. */
export function overscanMargin(tileSize: number, dpr: number): number {
  return Math.ceil(tileSize / dpr);
}

/** The CSS size of the base surface for a view of `width` × `height`. */
export function baseSize(width: number, height: number, margin: number): { width: number; height: number } {
  return { width: width + 2 * margin, height: height + 2 * margin };
}

/**
 * The origin of a base centred on `view`, snapped so the origin falls on a
 * device pixel and the renderer's own pixel snapping is a no-op.
 */
export function centredOrigin(view: ViewLike, margin: number, dpr: number): BaseOrigin {
  const unit = view.zoom * dpr;
  return {
    x: Math.round((view.x - margin / view.zoom) * unit) / unit,
    y: Math.round((view.y - margin / view.zoom) * unit) / unit,
    zoom: view.zoom,
  };
}

/** The transform that shows a base rendered at `origin` as `view`. */
export function elementTransform(view: ViewLike, origin: BaseOrigin): ElementTransform {
  return {
    tx: (origin.x - view.x) * view.zoom,
    ty: (origin.y - view.y) * view.zoom,
    scale: view.zoom / origin.zoom,
  };
}

/** `transform` as a CSS `transform` value; the element's transform origin is its top-left corner. */
export function transformCss(transform: ElementTransform, offsetX = 0, offsetY = 0): string {
  const { tx, ty, scale } = transform;
  const x = tx + offsetX * scale;
  const y = ty + offsetY * scale;
  return scale === 1 ? `translate3d(${x}px, ${y}px, 0)` : `translate3d(${x}px, ${y}px, 0) scale(${scale})`;
}

/**
 * Whether the base must be re-rendered for `view`: the zoom changed outside a
 * zoom gesture, or the view comes within a quarter of the margin of the edge
 * of what was rendered.
 */
export function needsRender(view: ViewLike, origin: BaseOrigin, margin: number, zooming: boolean): boolean {
  if (view.zoom !== origin.zoom && !zooming) {
    return true;
  }
  const size = baseSize(view.width, view.height, margin);
  const reserve = (margin * RECENTRE_FRACTION) / origin.zoom;
  const left = view.x - origin.x;
  const top = view.y - origin.y;
  const right = origin.x + size.width / origin.zoom - (view.x + view.width / view.zoom);
  const bottom = origin.y + size.height / origin.zoom - (view.y + view.height / view.zoom);
  return Math.min(left, top, right, bottom) < reserve;
}

/** Whether the base at `origin` is the centred base of `view`. */
export function isCentred(view: ViewLike, origin: BaseOrigin, margin: number, dpr: number): boolean {
  const centred = centredOrigin(view, margin, dpr);
  return centred.x === origin.x && centred.y === origin.y && centred.zoom === origin.zoom;
}

/**
 * The interaction layer's box for moving content with CSS `bounds` on the
 * base: the current box while it still holds the bounds, otherwise the bounds
 * grown by one step on each side and snapped outward to the step, so the
 * layer is re-allocated in steps and not every frame.
 */
export function layerBox(bounds: LayerBox, current: LayerBox | null, step = LAYER_STEP): LayerBox {
  if (
    current &&
    bounds.x >= current.x &&
    bounds.y >= current.y &&
    bounds.x + bounds.w <= current.x + current.w &&
    bounds.y + bounds.h <= current.y + current.h
  ) {
    return current;
  }
  const x = Math.floor((bounds.x - step) / step) * step;
  const y = Math.floor((bounds.y - step) / step) * step;
  const r = Math.ceil((bounds.x + bounds.w + step) / step) * step;
  const b = Math.ceil((bounds.y + bounds.h + step) / step) * step;
  return { x, y, w: r - x, h: b - y };
}
