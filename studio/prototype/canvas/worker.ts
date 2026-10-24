// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Canvas worker: CanvasKit draws into an OffscreenCanvas transferred from the
// page. When no WebGL surface can be made the worker says so and the page
// falls back to the main thread (recorded in the results).

import type { CanvasKit, CanvasKitInitOptions } from "canvaskit-wasm";
import { type Env, type PageDoc, runScenes } from "./run";

type CanvasKitInit = (options: CanvasKitInitOptions) => Promise<CanvasKit>;

const importModule = (url: string): Promise<{ default: CanvasKitInit }> => import(url);

interface StartMessage {
  canvas: OffscreenCanvas;
  scenes: string[];
}

const scope = self as unknown as {
  onmessage: ((event: MessageEvent<StartMessage>) => void) | null;
  postMessage(message: unknown): void;
  requestAnimationFrame?: (cb: (t: number) => void) => number;
};

function glRenderer(): string {
  const probe = new OffscreenCanvas(1, 1);
  const gl = probe.getContext("webgl2");
  if (!gl) {
    return "no webgl2 context in worker";
  }
  const ext = gl.getExtension("WEBGL_debug_renderer_info");
  return ext ? String(gl.getParameter(ext.UNMASKED_RENDERER_WEBGL)) : "renderer hidden";
}

scope.onmessage = async (event) => {
  const { canvas, scenes } = event.data;
  const features = {
    offscreenCanvas: typeof OffscreenCanvas !== "undefined",
    workerRequestAnimationFrame: typeof scope.requestAnimationFrame === "function",
    glRenderer: glRenderer(),
  };
  try {
    const { default: init } = await importModule("/canvaskit/canvaskit.js");
    const ck = await init({ locateFile: (file: string) => `/canvaskit/${file}` });
    const surface = ck.MakeWebGLCanvasSurface(canvas);
    if (!surface) {
      scope.postMessage({ type: "fallback", reason: "MakeWebGLCanvasSurface returned null", features });
      return;
    }
    const [font, pages] = await Promise.all([
      fetch("/font.ttf").then((r) => r.arrayBuffer()),
      fetch("/pages.json").then((r) => r.json() as Promise<PageDoc[]>),
    ]);
    const raf = scope.requestAnimationFrame;
    if (!raf) {
      scope.postMessage({ type: "fallback", reason: "no requestAnimationFrame in worker", features });
      return;
    }
    const env: Env = {
      ck,
      font,
      pages,
      width: canvas.width,
      height: canvas.height,
      raf: (cb) => {
        raf.call(self, cb);
      },
      now: () => performance.now(),
    };
    scope.postMessage({ type: "result", where: "worker", features, scenes: await runScenes(env, surface, scenes) });
  } catch (error) {
    scope.postMessage({ type: "fallback", reason: String(error), features });
  }
};
