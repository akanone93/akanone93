// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Loads Flutter's CanvasKit build for tests (STU-006): the runtime the
 * pinned Flutter SDK ships (`flutter precache --web`), from
 * `PLUX_CANVASKIT_DIR` or the SDK cache, with the repository's Noto fonts
 * registered under the families the Flutter conformance writer loads.
 */

import { readFileSync } from "node:fs";
import { join } from "node:path";
import type { CanvasKit } from "canvaskit-wasm";
import { CanvasKitParagraphEngine } from "../src/text/canvaskit-engine.ts";

/** The default CanvasKit directory: the web SDK of the pinned Flutter. */
export const defaultCanvasKitDir = "/root/.cache/plux/flutter-3.47.5/bin/cache/flutter_web_sdk/canvaskit";

const fontsDir = join(import.meta.dir, "../../../../schema/goldens/fonts");

/** The test fonts: family name and file, as `test/layout-conformance` loads them. */
export const testFonts: ReadonlyArray<readonly [family: string, file: string]> = [
  ["NotoSans", "NotoSans_400Regular.ttf"],
  ["NotoSans", "NotoSans_700Bold.ttf"],
  ["NotoNaskhArabic", "NotoNaskhArabic_400Regular.ttf"],
];

/** Loads CanvasKit from `PLUX_CANVASKIT_DIR` (default [defaultCanvasKitDir]). */
export async function loadCanvasKit(): Promise<CanvasKit> {
  const { PLUX_CANVASKIT_DIR: dir = defaultCanvasKitDir } = process.env;
  const module = (await import(join(dir, "canvaskit.js"))) as {
    default: (options: { wasmBinary: Uint8Array }) => Promise<CanvasKit>;
  };
  return module.default({ wasmBinary: readFileSync(join(dir, "canvaskit.wasm")) });
}

/** A paragraph engine on `canvasKit` with the test fonts registered. */
export function testParagraphEngine(canvasKit: CanvasKit): CanvasKitParagraphEngine {
  const engine = new CanvasKitParagraphEngine(canvasKit);
  for (const [family, file] of testFonts) {
    engine.registerFont(readFileSync(join(fontsDir, file)), family);
  }
  return engine;
}
