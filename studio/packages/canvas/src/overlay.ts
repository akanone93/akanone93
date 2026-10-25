// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The selection overlay (ADR-0013, M-12): outline and resize handles of the
 * selected items, drawn on the interaction side of the canvas and never into
 * the base. It is a plain 2D canvas the size of the view, redrawn from the
 * host's view mirror and hit index, so selecting, panning or zooming never
 * sends the worker a message and never re-renders the base.
 *
 * Every mark is snapped to whole device pixels and has a size fixed in CSS
 * pixels, so outlines and handles stay crisp and the same size at every
 * zoom from 10 % to 400 %.
 */

import type { Rect } from "./geometry.ts";
import type { ColorRgba } from "./protocol.ts";

/** The outline width in CSS pixels. */
export const OUTLINE_WIDTH = 1;
/** The side of a resize handle in CSS pixels. */
export const HANDLE_SIZE = 8;

/** A rectangle in whole device pixels: x, y, width, height. */
export interface DeviceRect {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
}

/** The marks of one selected item, in device pixels. */
export interface SelectionMarks {
  /** Four bars: top, right, bottom, left. */
  readonly outline: readonly DeviceRect[];
  /** Eight handles: corners and edge midpoints, clockwise from the top-left. */
  readonly handles: readonly DeviceRect[];
}

/** The view the overlay is drawn through: world top-left and zoom. */
export interface OverlayView {
  readonly x: number;
  readonly y: number;
  readonly zoom: number;
}

/** The marks of world rectangle `bounds` seen through `view` at `dpr`. */
export function selectionMarks(view: OverlayView, dpr: number, bounds: Rect): SelectionMarks {
  const scale = view.zoom * dpr;
  const l = Math.round((bounds.l - view.x) * scale);
  const t = Math.round((bounds.t - view.y) * scale);
  const r = Math.max(l + 1, Math.round((bounds.r - view.x) * scale));
  const b = Math.max(t + 1, Math.round((bounds.b - view.y) * scale));
  const line = Math.max(1, Math.round(OUTLINE_WIDTH * dpr));
  const outline: DeviceRect[] = [
    { x: l, y: t, w: r - l, h: line },
    { x: r - line, y: t, w: line, h: b - t },
    { x: l, y: b - line, w: r - l, h: line },
    { x: l, y: t, w: line, h: b - t },
  ];
  const side = Math.max(1, Math.round(HANDLE_SIZE * dpr));
  const half = Math.floor(side / 2);
  const mx = l + Math.floor((r - l) / 2);
  const my = t + Math.floor((b - t) / 2);
  const centres: [number, number][] = [
    [l, t],
    [mx, t],
    [r, t],
    [r, my],
    [r, b],
    [mx, b],
    [l, b],
    [l, my],
  ];
  const handles = centres.map(([cx, cy]) => ({ x: cx - half, y: cy - half, w: side, h: side }));
  return { outline, handles };
}

/** The 2D context calls the overlay uses; a `CanvasRenderingContext2D` in the browser. */
export interface Context2DLike {
  fillStyle: unknown;
  setTransform(a: number, b: number, c: number, d: number, e: number, f: number): void;
  clearRect(x: number, y: number, w: number, h: number): void;
  fillRect(x: number, y: number, w: number, h: number): void;
}

/** The overlay canvas element; an `HTMLCanvasElement` in the browser. */
export interface OverlayCanvasLike {
  width: number;
  height: number;
  getContext(type: "2d"): Context2DLike | null;
  readonly style?: { width: string; height: string; pointerEvents: string; position: string };
}

/** The colours of the overlay. */
export interface OverlayColors {
  readonly outline: ColorRgba;
  readonly handleFill: ColorRgba;
  readonly handleStroke: ColorRgba;
}

function css(color: ColorRgba): string {
  const r = (color >>> 24) & 0xff;
  const g = (color >>> 16) & 0xff;
  const b = (color >>> 8) & 0xff;
  return `rgba(${r},${g},${b},${(color & 0xff) / 255})`;
}

/** Draws selection marks onto an overlay canvas. */
export class SelectionOverlay {
  private readonly context: Context2DLike | null;
  /** Draw passes so far; for tests and diagnostics. */
  draws = 0;

  constructor(
    private readonly canvas: OverlayCanvasLike,
    private readonly colors: OverlayColors,
  ) {
    this.context = canvas.getContext("2d");
    const style = canvas.style;
    if (style) {
      style.position = "absolute";
      style.pointerEvents = "none";
    }
  }

  /** Sizes the overlay to the view: `width` x `height` CSS pixels at `dpr`. */
  resize(width: number, height: number, dpr: number): void {
    this.canvas.width = Math.max(1, Math.round(width * dpr));
    this.canvas.height = Math.max(1, Math.round(height * dpr));
    const style = this.canvas.style;
    if (style) {
      style.width = `${width}px`;
      style.height = `${height}px`;
    }
  }

  /** Clears the overlay and draws the marks of every world rectangle in `boxes`. */
  draw(view: OverlayView, dpr: number, boxes: readonly Rect[]): void {
    const context = this.context;
    if (!context) {
      return;
    }
    this.draws++;
    context.setTransform(1, 0, 0, 1, 0, 0);
    context.clearRect(0, 0, this.canvas.width, this.canvas.height);
    const line = Math.max(1, Math.round(OUTLINE_WIDTH * dpr));
    for (const box of boxes) {
      const marks = selectionMarks(view, dpr, box);
      context.fillStyle = css(this.colors.outline);
      for (const bar of marks.outline) {
        context.fillRect(bar.x, bar.y, bar.w, bar.h);
      }
      for (const handle of marks.handles) {
        context.fillStyle = css(this.colors.handleStroke);
        context.fillRect(handle.x, handle.y, handle.w, handle.h);
        context.fillStyle = css(this.colors.handleFill);
        context.fillRect(handle.x + line, handle.y + line, handle.w - 2 * line, handle.h - 2 * line);
      }
    }
  }
}
