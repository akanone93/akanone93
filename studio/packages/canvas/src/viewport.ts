// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type Rect, setRect } from "./geometry.ts";
import type { Point, ViewportChange } from "./protocol.ts";

/** The smallest zoom: 10 % (ADR-0013). */
export const MIN_ZOOM = 0.1;
/** The largest zoom: 400 % (ADR-0013). */
export const MAX_ZOOM = 4;

/** `zoom` clamped to 10–400 %. */
export function clampZoom(zoom: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoom));
}

/**
 * The view onto the world: `x`, `y` are the world coordinates of the top-left
 * corner, `zoom` is CSS pixels per world unit, `width` and `height` are the
 * canvas size in CSS pixels.
 */
export class Viewport {
  x = 0;
  y = 0;
  zoom = 1;

  constructor(
    public width: number,
    public height: number,
  ) {}

  /** Applies a change from the protocol. */
  apply(change: ViewportChange): void {
    switch (change.op) {
      case "pan":
        this.pan(change.dx, change.dy);
        break;
      case "zoom":
        this.zoomAbout(change.factor, change.x, change.y);
        break;
      case "set":
        this.x = change.x;
        this.y = change.y;
        this.zoom = clampZoom(change.zoom);
        break;
    }
  }

  /** Moves the content by `dx`, `dy` CSS pixels. */
  pan(dx: number, dy: number): void {
    this.x -= dx / this.zoom;
    this.y -= dy / this.zoom;
  }

  /** Zooms by `factor` keeping the world point under CSS point (`px`, `py`) fixed. */
  zoomAbout(factor: number, px: number, py: number): void {
    const wx = this.x + px / this.zoom;
    const wy = this.y + py / this.zoom;
    this.zoom = clampZoom(this.zoom * factor);
    this.x = wx - px / this.zoom;
    this.y = wy - py / this.zoom;
  }

  /** The world point under CSS point (`px`, `py`). */
  toWorld(px: number, py: number): Point {
    return { x: this.x + px / this.zoom, y: this.y + py / this.zoom };
  }

  /** The visible world rectangle, written into `out`. */
  worldRect(out: Rect): Rect {
    return setRect(out, this.x, this.y, this.x + this.width / this.zoom, this.y + this.height / this.zoom);
  }
}
