// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Test support: loads the pinned Flutter CanvasKit headless (CPU raster
 * surfaces only), a repository font, scene item builders, and a worker
 * environment whose frame and idle callbacks the test runs by hand.
 */

import { readFileSync } from "node:fs";
import { join } from "node:path";
import type { CanvasKit } from "canvaskit-wasm";
import type { LinkSpec, RectSpec, TextSpec } from "../src/protocol.ts";
import type { GlProbe } from "../src/select.ts";
import type { WorkerDeps } from "../src/worker.ts";

/** The CanvasKit of the pinned Flutter SDK. */
export const defaultCanvasKitDir = "/root/.cache/plux/flutter-3.47.5/bin/cache/flutter_web_sdk/canvaskit";

/** Loads CanvasKit from `PLUX_CANVASKIT_DIR` (default [defaultCanvasKitDir]). */
export async function loadCanvasKit(): Promise<CanvasKit> {
  const { PLUX_CANVASKIT_DIR: dir = defaultCanvasKitDir } = process.env;
  const module = (await import(join(dir, "canvaskit.js"))) as {
    default: (options: { wasmBinary: Uint8Array }) => Promise<CanvasKit>;
  };
  return module.default({ wasmBinary: readFileSync(join(dir, "canvaskit.wasm")) });
}

/** Noto Sans from the repository's golden fonts. */
export function fontBytes(): ArrayBuffer {
  const bytes = readFileSync(join(import.meta.dir, "../../../../schema/goldens/fonts/NotoSans_400Regular.ttf"));
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

/** A box. */
export function box(id: string, x: number, y: number, fill = 0x3366ccff, z = 1): RectSpec {
  return { kind: "rect", id, x, y, w: 100, h: 50, radius: 6, fill, stroke: { color: 0x222222ff, width: 1 }, z };
}

/** A text run. */
export function text(id: string, x: number, y: number, value = "Label", z = 2): TextSpec {
  return { kind: "text", id, x, y, w: 120, text: value, size: 14, color: 0x111111ff, z };
}

/** A curved link. */
export function link(id: string, x0: number, y0: number, x1: number, y1: number, color = 0x888888ff): LinkSpec {
  return {
    kind: "link",
    id,
    from: { x: x0, y: y0 },
    to: { x: x1, y: y1 },
    c1: { x: x0 + 40, y: y0 },
    c2: { x: x1 - 40, y: y1 },
    stroke: { color, width: 2 },
    z: 0,
  };
}

/** A software-GL probe, as on a machine without a GPU. */
export const softwareProbe: GlProbe = {
  available: true,
  software: true,
  renderer: "ANGLE (Google, Vulkan 1.3.0 (SwiftShader Device (Subzero)), SwiftShader driver)",
  majorPerformanceCaveat: true,
};

/** A worker environment on CPU raster surfaces with hand-run frames and idle slices. */
export class TestEnvironment {
  readonly frames: (() => void)[] = [];
  readonly idles: (() => void)[] = [];
  probe: GlProbe = softwareProbe;
  surfaces = true;
  webglSurfaces = true;

  constructor(readonly ck: CanvasKit) {}

  /** The dependencies to pass to the worker core. */
  deps(): WorkerDeps {
    const ck = this.ck;
    return {
      loadCanvasKit: () => Promise.resolve(ck),
      now: () => performance.now(),
      requestFrame: (callback) => this.frames.push(callback),
      requestIdle: (callback) => this.idles.push(callback),
      probeGl: () => this.probe,
      makeSurface: (_ck, canvas, kind) =>
        this.surfaces && (this.webglSurfaces || kind === "raster") ? ck.MakeSurface(canvas.width, canvas.height) : null,
    };
  }

  /** Runs the pending frame and idle callbacks until none remain. */
  drain(limit = 50): number {
    let runs = 0;
    while (runs < limit && (this.frames.length > 0 || this.idles.length > 0)) {
      for (const callback of this.frames.splice(0)) {
        callback();
      }
      for (const callback of this.idles.splice(0)) {
        callback();
      }
      runs++;
    }
    return runs;
  }
}

/** Lets pending promises and timers run. */
export function tick(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}
