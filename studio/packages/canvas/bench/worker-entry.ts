// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Worker entry point of the canvas benchmark: serves the canvas protocol with
// the pinned Flutter CanvasKit served at /canvaskit/.

import type { CanvasKit, CanvasKitInitOptions } from "canvaskit-wasm";
import { type BrowserScope, browserWorkerDeps } from "../src/browser.ts";
import { serveCanvasWorker, type WorkerScopeLike } from "../src/worker.ts";

type CanvasKitInit = (options: CanvasKitInitOptions) => Promise<CanvasKit>;
const importModule = (url: string): Promise<{ default: CanvasKitInit }> => import(url);

const scope = self as unknown as WorkerScopeLike & BrowserScope;
serveCanvasWorker(
  scope,
  browserWorkerDeps(scope, async () => {
    const { default: init } = await importModule("/canvaskit/canvaskit.js");
    return init({ locateFile: (file: string) => `/canvaskit/${file}` });
  }),
);
