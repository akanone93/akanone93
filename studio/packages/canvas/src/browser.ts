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
import type { SurfaceHandlers, WorkerDeps } from "./worker.ts";

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
  readonly MessageChannel?: new () => MessageChannelLike;
}

/** The parts of a `MessageChannel` idle slices are scheduled through. */
interface MessageChannelLike {
  readonly port1: { onmessage: (() => void) | null };
  readonly port2: { postMessage(message: unknown): void };
}

/**
 * Schedules idle slices as tasks posted through a `MessageChannel`, which,
 * unlike `setTimeout(callback, 0)`, browsers do not clamp to 4 ms once
 * timeouts nest five deep: a chain of slices would otherwise lose 3–4 ms
 * before each 8 ms slice, a third of the throughput the work after a drop
 * drains at (M-13). Frames still run between the tasks, and idle work yields
 * to a frame that has something to draw. Falls back to `setTimeout` without
 * one.
 */
function idleScheduler(scope: BrowserScope): (callback: () => void) => void {
  const Channel = scope.MessageChannel;
  if (!Channel) {
    return (callback) => {
      scope.setTimeout(callback, 0);
    };
  }
  const channel = new Channel();
  const queued: (() => void)[] = [];
  channel.port1.onmessage = () => {
    queued.shift()?.();
  };
  return (callback) => {
    queued.push(callback);
    channel.port2.postMessage(null);
  };
}

type SurfaceTarget = Parameters<CanvasKit["MakeSWCanvasSurface"]>[0];

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

interface Context2dLike {
  putImageData(data: ImageData, dx: number, dy: number, x?: number, y?: number, w?: number, h?: number): void;
}

/**
 * A CPU raster surface presented through the canvas's 2D context. Unlike
 * `MakeSWCanvasSurface`, whose unpremultiplied pixels force Skia off its fast
 * blitters, it renders premultiplied RGBA: the base is opaque, so the bytes
 * are the same as unpremultiplied ones and go to `putImageData` unchanged. A
 * flush copies only its dirty rectangle [l, t, r, b] when one is given.
 */
function rasterOn(ck: CanvasKit, canvas: OffscreenCanvasLike): Surface | null {
  // A CPU-backed 2D context: an accelerated one would hand every copy to the software GL this path avoids.
  const context =
    (
      canvas as { getContext?(type: "2d", attributes: { willReadFrequently: boolean }): Context2dLike | null }
    ).getContext?.("2d", { willReadFrequently: true }) ?? null;
  const { width, height } = canvas;
  if (!context || width <= 0 || height <= 0) {
    return null;
  }
  const pixels = ck.Malloc(Uint8Array, width * height * 4);
  const info = {
    width,
    height,
    colorType: ck.ColorType.RGBA_8888,
    alphaType: ck.AlphaType.Premul,
    colorSpace: ck.ColorSpace.SRGB,
  };
  const surface = ck.MakeRasterDirectSurface(info, pixels, width * 4);
  if (!surface) {
    ck.Free(pixels);
    return null;
  }
  const flush = surface.flush.bind(surface);
  const remove = surface.delete.bind(surface);
  surface.flush = ((dirty?: Int32Array) => {
    flush();
    const bytes = pixels.toTypedArray();
    // A view of CanvasKit's heap, not a copy; the heap is a plain (not shared) ArrayBuffer.
    const heap = bytes.buffer as ArrayBuffer;
    const image = new ImageData(new Uint8ClampedArray(heap, bytes.byteOffset, bytes.length), width, height);
    if (dirty) {
      context.putImageData(
        image,
        0,
        0,
        dirty[0],
        dirty[1],
        (dirty[2] ?? 0) - (dirty[0] ?? 0),
        (dirty[3] ?? 0) - (dirty[1] ?? 0),
      );
    } else {
      context.putImageData(image, 0, 0);
    }
  }) as Surface["flush"];
  surface.delete = () => {
    remove();
    ck.Free(pixels);
  };
  return surface;
}

function surfaceOn(ck: CanvasKit, canvas: OffscreenCanvasLike, kind: "webgl" | "raster" | "layer"): Surface | null {
  const target = canvas as unknown as SurfaceTarget;
  if (kind === "layer") {
    // Transparent: a 2D canvas takes unpremultiplied pixels, which this surface renders.
    return ck.MakeSWCanvasSurface(target);
  }
  if (kind === "raster") {
    return rasterOn(ck, canvas);
  }
  return ck.MakeWebGLCanvasSurface(target, ck.ColorSpace.SRGB, {
    majorVersion: 2,
    // Skia antialiases itself; MSAA on the default framebuffer would only add a resolve pass.
    antialias: 0,
    failIfMajorPerformanceCaveat: 0,
    preserveDrawingBuffer: 0,
  });
}

interface EventTargetLike {
  addEventListener(type: string, listener: (event: { preventDefault(): void }) => void): void;
}

/**
 * Calls `handlers` when `canvas` loses or regains its context. Preventing the
 * default of the loss event is what lets the browser restore a WebGL context.
 */
export function watchCanvasContext(canvas: OffscreenCanvasLike, handlers: SurfaceHandlers): void {
  const target = canvas as Partial<EventTargetLike>;
  if (typeof target.addEventListener !== "function") {
    return;
  }
  const lost = (event: { preventDefault(): void }): void => {
    event.preventDefault();
    handlers.lost();
  };
  const restored = (): void => handlers.restored();
  target.addEventListener("webglcontextlost", lost);
  target.addEventListener("contextlost", lost);
  target.addEventListener("webglcontextrestored", restored);
  target.addEventListener("contextrestored", restored);
}

/** Worker dependencies for a browser scope; `loadCanvasKit` loads the pinned Flutter CanvasKit. */
export function browserWorkerDeps(scope: BrowserScope, loadCanvasKit: () => Promise<CanvasKit>): WorkerDeps {
  return {
    loadCanvasKit,
    now: () => scope.performance.now(),
    requestFrame: (callback) => {
      scope.requestAnimationFrame(() => callback());
    },
    requestIdle: idleScheduler(scope),
    probeGl: () => probe(scope),
    makeSurface: surfaceOn,
    watchContext: watchCanvasContext,
  };
}
