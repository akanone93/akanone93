// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Renderer selection at start-up (ADR-0013 M-12). The canvas probes WebGL
 * with `failIfMajorPerformanceCaveat` and reads the GL renderer string; on a
 * hardware GL it uses WebGL, and on a software GL (SwiftShader, llvmpipe, or
 * a context refused for a major performance caveat) it measures a short
 * workload on both surfaces and keeps WebGL unless CPU raster is clearly
 * faster on this machine.
 */

import type { CanvasKit, Surface } from "canvaskit-wasm";
import type { RendererPreference } from "./protocol.ts";

/** What the WebGL probe found. */
export interface GlProbe {
  readonly available: boolean;
  readonly software: boolean;
  readonly renderer: string;
  readonly majorPerformanceCaveat: boolean;
}

/** CPU raster must take less than this fraction of WebGL's time to be chosen. */
export const RASTER_ADVANTAGE = 0.8;

const softwareGl = /swiftshader|llvmpipe|softpipe|software|basic render/i;

/** Whether a GL renderer string names a software implementation. */
export function isSoftwareGl(renderer: string): boolean {
  return softwareGl.test(renderer);
}

/** The renderer to use for a preference, probe and optional measurement. */
export function chooseRenderer(
  preference: RendererPreference,
  probe: GlProbe,
  measured: { readonly webgl: number; readonly raster: number } | null,
): "webgl" | "raster" {
  if (preference !== "auto") {
    return preference;
  }
  if (!probe.available) {
    return "raster";
  }
  if (measured === null) {
    return "webgl";
  }
  return measured.raster < measured.webgl * RASTER_ADVANTAGE ? "raster" : "webgl";
}

const MEASURE_TILE = 512;
const MEASURE_GRID = 3;
const MEASURE_ITEMS = 40;

/**
 * The median time in ms of a representative pan frame on `surface`: a clear,
 * a 3 x 3 grid of 512-pixel tile blits and a small batched live layer, then a
 * flush and a 1x1 read so GPU work is included. Tile blits dominate a frame,
 * and they are where the renderers differ most (M-12).
 */
export function measureSurface(ck: CanvasKit, surface: Surface, now: () => number, rounds = 5): number {
  const info = {
    width: MEASURE_TILE,
    height: MEASURE_TILE,
    colorType: ck.ColorType.RGBA_8888,
    alphaType: ck.AlphaType.Premul,
    colorSpace: ck.ColorSpace.SRGB,
  };
  const tile = surface.makeSurface(info);
  tile.getCanvas().clear(ck.Color4f(0.9, 0.9, 1, 1));
  const image = tile.makeImageSnapshot();
  const paint = new ck.Paint();
  paint.setAntiAlias(true);
  const builder = new ck.PathBuilder();
  const src = ck.XYWHRect(0, 0, MEASURE_TILE, MEASURE_TILE);
  const times: number[] = [];
  const canvas = surface.getCanvas();
  for (let round = 0; round < rounds; round++) {
    const start = now();
    canvas.clear(ck.WHITE);
    for (let i = 0; i < MEASURE_GRID * MEASURE_GRID; i++) {
      const x = (i % MEASURE_GRID) * MEASURE_TILE - 100 - round;
      const y = Math.floor(i / MEASURE_GRID) * MEASURE_TILE - 100;
      const dst = ck.XYWHRect(x, y, MEASURE_TILE, MEASURE_TILE);
      canvas.drawImageRectOptions(image, src, dst, ck.FilterMode.Nearest, ck.MipmapMode.None);
    }
    for (let i = 0; i < MEASURE_ITEMS; i++) {
      builder.moveTo(i * 4, 0);
      builder.cubicTo(i * 4 + 40, 30, i * 4 + 80, 60, i * 4 + 120, 100);
    }
    const links = builder.detach();
    canvas.drawPath(links, paint);
    links.delete();
    surface.flush();
    canvas.readPixels(0, 0, { ...info, width: 1, height: 1 });
    times.push(now() - start);
  }
  builder.delete();
  paint.delete();
  image.delete();
  tile.delete();
  times.sort((a, b) => a - b);
  return times[Math.floor(times.length / 2)] ?? 0;
}
