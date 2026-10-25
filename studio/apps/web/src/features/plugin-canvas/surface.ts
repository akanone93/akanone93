// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { CanvasHost, type SceneOp, type WorkerLike } from "@plux/canvas";

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

/** Where the host finds its worker and font; the web build ships both beside the app. */
const WORKER_URL = "/canvas-worker.js";
const FONT_URL = "/canvas-font.ttf";

/** The browser surface: the canvas host on a worker. */
export const browserSurface: SurfaceFactory = (canvas, size) => {
  const fonts: ArrayBuffer[] = [];
  void fetch(FONT_URL)
    .then((r) => r.arrayBuffer())
    .then((buffer) => fonts.push(buffer))
    .catch(() => undefined);
  return new CanvasHost(
    canvas,
    size,
    { renderer: "auto", tileSize: 512, background: 0xf2f4f9ff, fonts, fontFamily: "Noto Sans", measureFrames: false },
    {
      createWorker: () => new Worker(WORKER_URL, { type: "module" }) as unknown as WorkerLike,
      mainThread: () => {
        throw new Error("the plugin canvas needs a worker");
      },
    },
  );
};
