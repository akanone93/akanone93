// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The dot grid on a canvas's background (STU-032: what boxes snap and align on). Pure arithmetic: the renderer asks
 * for the levels to draw at a zoom and the dots of each level in the world rectangle it draws, and hands each level
 * to CanvasKit as one point list. Dots keep their size in CSS pixels at every zoom.
 */

import type { Rect } from "./geometry.ts";
import type { GridStyle } from "./protocol.ts";

/** The most dots drawn for one rectangle and level; a larger request draws none rather than stall a frame. */
export const MAX_GRID_DOTS = 60_000;

/**
 * The world distance between dots at `zoom`: the grid's spacing, doubled until neighbouring dots are at least
 * `minGap` CSS pixels apart. Always a power-of-two multiple of the spacing, so a coarser grid keeps a subset of
 * the finer one's dots and zooming never shifts a dot.
 */
export function gridStep(grid: GridStyle, zoom: number): number {
  let step = grid.spacing;
  if (!(zoom > 0) || !(step > 0)) {
    return step;
  }
  while (step * zoom < grid.minGap) {
    step *= 2;
  }
  return step;
}

/** One level of the grid: its dots every `step` world units, drawn at `alpha` of the grid's colour. */
export interface GridLevel {
  readonly step: number;
  readonly alpha: number;
}

/** Where the finer level starts to fade in, as a fraction of `minGap`: denser grids are never drawn. */
export const GRID_FADE_FROM = 0.7;

/**
 * The levels drawn at `zoom`: the step's dots in full and, between them, the next finer level's dots fading in as
 * they draw apart, from nothing at `GRID_FADE_FROM` of `minGap` to full at `minGap`, where they become the step.
 * Zooming therefore thins or fills the grid gradually; no level appears or vanishes from one frame to the next.
 */
export function gridLevels(grid: GridStyle, zoom: number): GridLevel[] {
  const step = gridStep(grid, zoom);
  const levels: GridLevel[] = [{ step, alpha: 1 }];
  const fine = step / 2;
  if (fine >= grid.spacing && zoom > 0) {
    const from = grid.minGap * GRID_FADE_FROM;
    const alpha = Math.min(1, Math.max(0, (fine * zoom - from) / (grid.minGap - from)));
    if (alpha > 0) {
      levels.push({ step: fine, alpha });
    }
  }
  return levels;
}

/**
 * The world positions of the dots every `step` in `rect`, as x, y pairs, leaving out those on every `skip` (the
 * coarser level, drawn on its own), or null when there are more than `MAX_GRID_DOTS`. The dots sit on multiples of
 * the step, so tiles drawn apart meet without a seam.
 */
export function gridPoints(rect: Rect, step: number, skip?: number): Float32Array | null {
  if (!(step > 0)) {
    return null;
  }
  const x0 = Math.ceil(rect.l / step);
  const x1 = Math.floor(rect.r / step);
  const y0 = Math.ceil(rect.t / step);
  const y1 = Math.floor(rect.b / step);
  const columns = x1 - x0 + 1;
  const rows = y1 - y0 + 1;
  if (columns <= 0 || rows <= 0) {
    return new Float32Array(0);
  }
  if (columns * rows > MAX_GRID_DOTS) {
    return null;
  }
  const ratio = skip === undefined ? 0 : Math.round(skip / step);
  const out = new Float32Array(columns * rows * 2);
  let i = 0;
  for (let row = y0; row <= y1; row++) {
    for (let column = x0; column <= x1; column++) {
      if (ratio > 1 && column % ratio === 0 && row % ratio === 0) {
        continue;
      }
      // `+ 0` turns the -0 of a column left of zero into 0.
      out[i++] = column * step + 0;
      out[i++] = row * step + 0;
    }
  }
  return out.subarray(0, i);
}
