// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Page script: starts the canvas worker (or the main-thread fallback) and the
// validator worker, and hands the numbers to the Playwright script.

import type { CanvasKit, CanvasKitInitOptions } from "canvaskit-wasm";
import { type Env, type PageDoc, runScenes } from "./run";

type CanvasKitInit = (options: CanvasKitInitOptions) => Promise<CanvasKit>;

const importModule = (url: string): Promise<{ default: CanvasKitInit }> => import(url);

interface RunOptions {
  scenes: string[];
  forceMainThread: boolean;
}

const WIDTH = 1280;
const HEIGHT = 800;

function makeCanvas(): HTMLCanvasElement {
  const canvas = document.createElement("canvas");
  canvas.width = WIDTH;
  canvas.height = HEIGHT;
  canvas.style.width = `${WIDTH}px`;
  canvas.style.height = `${HEIGHT}px`;
  document.body.replaceChildren(canvas);
  return canvas;
}

async function runOnMainThread(scenes: string[], reason: string): Promise<unknown> {
  const { default: init } = await importModule("/canvaskit/canvaskit.js");
  const ck = await init({ locateFile: (file: string) => `/canvaskit/${file}` });
  const canvas = makeCanvas();
  const surface = ck.MakeWebGLCanvasSurface(canvas);
  if (!surface) {
    return { type: "error", where: "main thread", reason: "MakeWebGLCanvasSurface returned null on the main thread" };
  }
  const [font, pages] = await Promise.all([
    fetch("/font.ttf").then((r) => r.arrayBuffer()),
    fetch("/pages.json").then((r) => r.json() as Promise<PageDoc[]>),
  ]);
  const env: Env = {
    ck,
    font,
    pages,
    width: WIDTH,
    height: HEIGHT,
    raf: (cb) => {
      requestAnimationFrame(cb);
    },
    now: () => performance.now(),
  };
  return { type: "result", where: `main thread (${reason})`, scenes: await runScenes(env, surface, scenes) };
}

function runInWorker(scenes: string[]): Promise<{ type: string; reason?: string }> {
  const canvas = makeCanvas();
  const offscreen = canvas.transferControlToOffscreen();
  const worker = new Worker("/worker.js", { type: "module" });
  return new Promise((resolve) => {
    worker.onmessage = (event) => {
      worker.terminate();
      resolve(event.data);
    };
    worker.onerror = (event) => {
      worker.terminate();
      resolve({ type: "fallback", reason: `worker error: ${event.message}` });
    };
    worker.postMessage({ canvas: offscreen, scenes }, [offscreen]);
  });
}

const api = {
  async runCanvas(options: RunOptions): Promise<unknown> {
    if (options.forceMainThread) {
      return runOnMainThread(options.scenes, "requested, for comparison");
    }
    const result = await runInWorker(options.scenes);
    if (result.type === "fallback") {
      const fallback = (await runOnMainThread(options.scenes, `fallback: ${result.reason}`)) as object;
      return { ...fallback, workerFailure: result };
    }
    return result;
  },
  validate(calls: number): Promise<unknown> {
    const worker = new Worker("/validator-worker.js");
    return new Promise((resolve) => {
      worker.onmessage = (event) => {
        worker.terminate();
        resolve(event.data);
      };
      worker.postMessage({ calls });
    });
  },
};

(window as unknown as { plux: typeof api }).plux = api;
