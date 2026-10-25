// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Stages the pinned Flutter CanvasKit (ADR-0013) for the web build: copies
// canvaskit.js and canvaskit.wasm from the pinned SDK (or PLUX_CANVASKIT_DIR)
// beside the canvas worker, after checking each against its pinned SHA-256.
// Vite then emits both as hashed assets. The staged files are not committed.

import { createHash } from "node:crypto";
import { copyFileSync, mkdirSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

/** Flutter 3.47.5's CanvasKit; changes together with the Flutter pin (ci.md). */
export const PINNED: Readonly<Record<string, string>> = {
  "canvaskit.js": "bb559f6080c7d312ac2a912b4abec9f68ff3d3022d4a603c7796b9b31460642b",
  "canvaskit.wasm": "fbed517a43e82452404446683f00f2e876d835aed84410695759e67b6bb01cd3",
};

export function stage(source: string, target: string): void {
  mkdirSync(target, { recursive: true });
  for (const [name, sha] of Object.entries(PINNED)) {
    const from = join(source, name);
    const actual = createHash("sha256").update(readFileSync(from)).digest("hex");
    if (actual !== sha) {
      throw new Error(`${from}: SHA-256 ${actual} does not match the pin ${sha}`);
    }
    copyFileSync(from, join(target, name));
  }
}

if (import.meta.main) {
  const source =
    process.env["PLUX_CANVASKIT_DIR"] ??
    join(homedir(), ".cache/plux/flutter-3.47.5/bin/cache/flutter_web_sdk/canvaskit");
  stage(source, new URL("../apps/web/src/features/plugin-canvas/canvaskit", import.meta.url).pathname);
}
