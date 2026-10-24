// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The browser environment of the canvas worker: animation frames, idle
 * slices, the WebGL probe (`failIfMajorPerformanceCaveat` and the unmasked
 * GL renderer string) and CanvasKit surfaces on an OffscreenCanvas.
 * Exercised by the software-rendering benchmark (`bench/`).
 */

import type { CanvasKit, Surface } from "canvaskit-wasm";
import type { OffscreenCanvasLike } from "./protocol.ts";
import { type GlProbe, isSoftwareGl } from "./select.ts";
import type { WorkerDeps } from "./worker.ts";

interface GlContextLike {
  readonly RENDERER: number;
  getExtension(name: string): unknown;
  getParameter(parameter: number): unknown;
}

interface ProbeCanvas extends OffscreenCanvasLike {
  getContext(type: "webgl2", attributes?: { failIfMajorPerformanceCaveat: boolean }): GlContextLike | null;
}

/** The parts of a worker or window scope the canvas uses. */
export interface BrowserScope {
  requestAnimationFrame(callback: (time: number) => void): number;
  setTimeout(callback: () => void, ms: number): unknown;
  readonly performance: { now(): number };
  readonly OffscreenCanvas?: new (width: number, height: number) => ProbeCanvas;
}

type SurfaceTarget = Parameters<CanvasKit["MakeSWCanvasSurface"]>[0];

const MEASURE_SIZE = 256;

function probe(scope: BrowserScope): GlProbe {
  const Offscreen = scope.OffscreenCanvas;
  const none: GlProbe = { available: false, software: true, renderer: "", majorPerformanceCaveat: false };
  if (!Offscreen) {
    return none;
  }
  let gl = new Offscreen(1, 1).getContext("webgl2", { failIfMajorPerformanceCaveat: true });
  const caveat = gl === null;
  gl ??= new Offscreen(1, 1).getContext("webgl2");
  if (!gl) {
    return none;
  }
  const debug = gl.getExtension("WEBGL_debug_renderer_info") as { UNMASKED_RENDERER_WEBGL: number } | null;
  const renderer = String(gl.getParameter(debug ? debug.UNMASKED_RENDERER_WEBGL : gl.RENDERER));
  (gl.getExtension("WEBGL_lose_context") as { loseContext(): void } | null)?.loseContext();
  return { available: true, software: caveat || isSoftwareGl(renderer), renderer, majorPerformanceCaveat: caveat };
}

function surfaceOn(ck: CanvasKit, canvas: OffscreenCanvasLike, kind: "webgl" | "raster"): Surface | null {
  const target = canvas as unknown as SurfaceTarget;
  if (kind === "raster") {
    return ck.MakeSWCanvasSurface(target);
  }
  return ck.MakeWebGLCanvasSurface(target, ck.ColorSpace.SRGB, {
    majorVersion: 2,
    antialias: 1,
    failIfMajorPerformanceCaveat: 0,
    preserveDrawingBuffer: 0,
  });
}

/** Worker dependencies for a browser scope; `loadCanvasKit` loads the pinned Flutter CanvasKit. */
export function browserWorkerDeps(scope: BrowserScope, loadCanvasKit: () => Promise<CanvasKit>): WorkerDeps {
  return {
    loadCanvasKit,
    now: () => scope.performance.now(),
    requestFrame: (callback) => {
      scope.requestAnimationFrame(() => callback());
    },
    requestIdle: (callback) => {
      scope.setTimeout(callback, 0);
    },
    probeGl: () => probe(scope),
    makeSurface: surfaceOn,
    makeMeasureSurface: (ck, kind) => {
      if (kind === "raster") {
        return ck.MakeSurface(MEASURE_SIZE, MEASURE_SIZE);
      }
      const Offscreen = scope.OffscreenCanvas;
      return Offscreen ? surfaceOn(ck, new Offscreen(MEASURE_SIZE, MEASURE_SIZE), "webgl") : null;
    },
  };
}
