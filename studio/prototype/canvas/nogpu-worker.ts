// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Worker for the no-GPU techniques (M-12): one renderer per canvas, because an
// OffscreenCanvas keeps the first context type it was given.

import type { CanvasKit, CanvasKitInitOptions } from "canvaskit-wasm";
import { type Content, makeSceneA, makeSceneB, runVariant, type VariantResult } from "./nogpu";
import type { PageDoc } from "./scenes";

type CanvasKitInit = (options: CanvasKitInitOptions) => Promise<CanvasKit>;
const importModule = (url: string): Promise<{ default: CanvasKitInit }> => import(url);

const scope = self as unknown as {
  onmessage: ((event: MessageEvent<{ canvas: OffscreenCanvas; renderer: "gl" | "sw" }>) => void) | null;
  postMessage(message: unknown): void;
  requestAnimationFrame(cb: (t: number) => void): number;
};

scope.onmessage = async (event) => {
  const { canvas, renderer } = event.data;
  try {
    const { default: init } = await importModule("/canvaskit/canvaskit.js");
    const ck = await init({ locateFile: (file: string) => `/canvaskit/${file}` });
    const surface = renderer === "gl" ? ck.MakeWebGLCanvasSurface(canvas) : ck.MakeSWCanvasSurface(canvas);
    if (!surface) {
      scope.postMessage({ type: "error", reason: `${renderer} surface is null` });
      return;
    }
    const [font, pages] = await Promise.all([
      fetch("/font.ttf").then((r) => r.arrayBuffer()),
      fetch("/pages.json").then((r) => r.json() as Promise<PageDoc[]>),
    ]);
    const env = { ck, width: canvas.width, height: canvas.height, now: () => performance.now() };
    const raf = (cb: (t: number) => void): void => {
      scope.requestAnimationFrame(cb);
    };
    const a: Content = makeSceneA(ck, font);
    const b: Content = makeSceneB(ck, font, pages);
    const results: VariantResult[] = [];
    const run = async (content: Content, o: Parameters<typeof runVariant>[4]): Promise<void> => {
      results.push(...(await runVariant(env, surface, raf, content, o)));
    };
    if (renderer === "gl") {
      await run(a, { name: "gl baseline", batch: false, tile: false, lod: false, renderer });
      await run(a, { name: "gl batch", batch: true, tile: false, lod: false, renderer });
      await run(a, { name: "gl batch+tile+lod", batch: true, tile: true, lod: true, renderer });
      await run(b, { name: "gl baseline", batch: false, tile: false, lod: false, renderer });
      await run(b, { name: "gl tile+lod", batch: false, tile: true, lod: true, renderer });
    } else {
      await run(a, { name: "sw baseline", batch: false, tile: false, lod: false, renderer });
      await run(a, { name: "sw batch", batch: true, tile: false, lod: false, renderer });
      await run(a, { name: "sw tile", batch: false, tile: true, lod: false, renderer });
      await run(a, { name: "sw lod", batch: false, tile: false, lod: true, renderer });
      await run(a, { name: "sw batch+tile+lod", batch: true, tile: true, lod: true, renderer });
      await run(b, { name: "sw baseline", batch: false, tile: false, lod: false, renderer });
      await run(b, { name: "sw tile", batch: false, tile: true, lod: false, renderer });
      await run(b, { name: "sw lod", batch: false, tile: false, lod: true, renderer });
      await run(b, { name: "sw tile+lod", batch: false, tile: true, lod: true, renderer });
    }
    scope.postMessage({ type: "result", renderer, results });
  } catch (error) {
    scope.postMessage({ type: "error", reason: String(error) });
  }
};
