// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { sha256Hex } from "../plugin-canvas/thumbnails.ts";

/** What a captured screenshot is kept as (STU-134, WGT-031). */
export type ScreenshotUse = "thumbnail" | "slotPreview" | "translationContext";

/** The asset name that holds the screenshot for `target`: a screen ID, a native slot ID or a screen ID. */
export function screenshotAssetName(use: ScreenshotUse, target: string): string {
  switch (use) {
    case "thumbnail":
      return `thumbnails/screen-${target}.png`;
    case "slotPreview":
      return `previews/slot-${target}.png`;
    case "translationContext":
      return `l10n-context/screen-${target}.png`;
  }
}

/** The slice of the upload path that stores a screenshot. */
export interface ScreenshotStore {
  readonly upload: (job: { name: string; file: Blob }) => Promise<unknown>;
}

/** Stores a screenshot through the existing asset upload; returns the asset name and the PNG's SHA-256. */
export async function storeScreenshot(
  store: ScreenshotStore,
  use: ScreenshotUse,
  target: string,
  png: Uint8Array<ArrayBuffer>,
): Promise<{ readonly name: string; readonly sha256: string }> {
  const name = screenshotAssetName(use, target);
  await store.upload({ name, file: new Blob([png], { type: "image/png" }) });
  return { name, sha256: await sha256Hex(png) };
}
