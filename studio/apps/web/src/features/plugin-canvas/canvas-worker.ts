// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Worker entry of the plugin canvas (STU-003): serves the canvas protocol with
// the pinned Flutter CanvasKit, which the build stages beside this file
// (studio/scripts/stage-canvaskit.ts) and Vite emits as hashed assets.

import { type BrowserScope, browserWorkerDeps } from "@plux/canvas";
import { serveCanvasWorker, type WorkerScopeLike } from "@plux/canvas/worker";
import type { CanvasKit, CanvasKitInitOptions } from "canvaskit-wasm";

type CanvasKitInit = (options: CanvasKitInitOptions) => Promise<CanvasKit>;

const scriptUrl = new URL("./canvaskit/canvaskit.js", import.meta.url).href;
const wasmUrl = new URL("./canvaskit/canvaskit.wasm", import.meta.url).href;
const importModule = (url: string): Promise<{ default: CanvasKitInit }> => import(/* @vite-ignore */ url);

const scope = self as unknown as WorkerScopeLike & BrowserScope;
serveCanvasWorker(
  scope,
  browserWorkerDeps(scope, async () => {
    const { default: init } = await importModule(scriptUrl);
    return init({ locateFile: () => wasmUrl });
  }),
);
