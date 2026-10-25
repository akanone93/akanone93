// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { CanvasHost, type FontSource, type SceneOp, type WorkerLike } from "@plux/canvas";
import { canvasWorkerScriptURL } from "../../lib/trusted-worker.ts";
import canvasWorkerURL from "./canvas-worker.ts?worker&url";

/** The part of the canvas host the plugin canvas uses; a stub in tests. */
export interface CanvasSurface {
  patch(ops: readonly SceneOp[]): void;
  setViewport(x: number, y: number, zoom: number): void;
  pan(dx: number, dy: number): void;
  zoomAbout(factor: number, x: number, y: number): void;
  readonly viewport: { readonly x: number; readonly y: number; readonly zoom: number };
  dispose(): void;
}

export interface SurfaceSize {
  readonly width: number;
  readonly height: number;
  readonly dpr: number;
}

/** Starts a surface on `canvas`. */
export type SurfaceFactory = (canvas: HTMLCanvasElement, size: SurfaceSize) => CanvasSurface;

/** Roboto and Noto Sans, regular and bold; Vite emits them as hashed assets. */
const FONT_FILES: readonly (readonly [string, string])[] = [
  ["Roboto", new URL("./fonts/Roboto_400Regular.ttf", import.meta.url).href],
  ["Roboto", new URL("./fonts/Roboto_700Bold.ttf", import.meta.url).href],
];
const NOTO_FILES: readonly (readonly [string, string])[] = [
  ["Noto Sans", new URL("./fonts/NotoSans_400Regular.ttf", import.meta.url).href],
  ["Noto Sans", new URL("./fonts/NotoSans_700Bold.ttf", import.meta.url).href],
];

const toSource = ([family, url]: readonly [string, string]): FontSource => ({ family, url });

/** The browser surface: the canvas host on a bundled worker, started once its fonts are loaded. */
export const browserSurface: SurfaceFactory = (canvas, size) => {
  let host: CanvasHost | null = null;
  let disposed = false;
  let view = { x: 0, y: 0, zoom: 1 };
  const queued: ((h: CanvasHost) => void)[] = [];
  const run = (action: (h: CanvasHost) => void): void => {
    if (host) {
      action(host);
    } else if (!disposed) {
      queued.push(action);
    }
  };
  CanvasHost.create(
    canvas,
    size,
    {
      renderer: "auto",
      tileSize: 512,
      background: 0xf2f4f9ff,
      fonts: [],
      fontFamily: "Noto Sans",
      measureFrames: false,
    },
    {
      createWorker: () =>
        new Worker(canvasWorkerScriptURL(canvasWorkerURL, window, import.meta.env.DEV), {
          type: "module",
        }) as unknown as WorkerLike,
      mainThread: () => {
        throw new Error("the plugin canvas needs a worker");
      },
    },
    {
      request: {
        device: [],
        roboto: toSource(FONT_FILES[0] as readonly [string, string]),
        noto: [...FONT_FILES.slice(1), ...NOTO_FILES].map(toSource),
        sfPro: false,
      },
      deps: { fetchFont: (url) => fetch(url).then((r) => r.arrayBuffer()) },
    },
  )
    .then((created) => {
      if (disposed) {
        created.dispose();
        return;
      }
      host = created;
      for (const action of queued.splice(0)) {
        action(created);
      }
    })
    .catch(() => undefined);
  return {
    patch: (ops) => run((h) => h.patch(ops)),
    setViewport: (x, y, zoom) => {
      view = { x, y, zoom };
      run((h) => h.setViewport(x, y, zoom));
    },
    pan: (dx, dy) => run((h) => h.pan(dx, dy)),
    zoomAbout: (factor, x, y) => run((h) => h.zoomAbout(factor, x, y)),
    get viewport() {
      return host ? host.viewport : view;
    },
    dispose: () => {
      disposed = true;
      host?.dispose();
    },
  };
};
