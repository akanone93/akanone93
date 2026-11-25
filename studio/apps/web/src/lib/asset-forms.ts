// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { objectUrl } from "./object-url.ts";

/**
 * The slice of an asset Studio reads to show it: its media type and its forms, the processed variants (`webp`,
 * `avif`, `png`, `vector`, `dotlottie`) and the stored file itself (`original`), as asset.proto names them.
 */
export interface AssetForms {
  readonly mediaType: string;
  readonly variants: readonly {
    readonly kind: string;
    readonly density: number;
    readonly artifact?: { readonly urls: readonly string[] } | undefined;
  }[];
}

/** The media type of an SVG asset. */
export const SVG_TYPE = "image/svg+xml";

/** Whether an asset is an SVG: a custom icon or a vector picture, drawn on a device from its `vector_graphics` form. */
export const isSvg = (a: Pick<AssetForms, "mediaType">): boolean => a.mediaType === SVG_TYPE;

/**
 * Where Studio reads the first of `kinds` an asset has, same-origin (`objectUrl`): at the highest density, or the
 * lowest with `smallest`; undefined when it has none of them.
 */
export function formUrl(asset: AssetForms, kinds: readonly string[], smallest = false): string | undefined {
  for (const kind of kinds) {
    const forms = asset.variants
      .filter((v) => v.kind === kind && (v.artifact?.urls[0] ?? "") !== "")
      .sort((a, b) => (smallest ? a.density - b.density : b.density - a.density));
    const url = forms[0]?.artifact?.urls[0];
    if (url !== undefined) {
      return objectUrl(url);
    }
  }
  return undefined;
}

/**
 * Where a browser shows an image asset as a small picture (`<img>`, a thumbnail or a picker): an SVG's stored file,
 * which a browser draws as an image with no script and nothing fetched; a raster image's smallest WebP or PNG, or its
 * stored file before they are made. Undefined for a file that is not an image.
 */
export function previewUrlOf(asset: AssetForms): string | undefined {
  if (isSvg(asset)) {
    return formUrl(asset, ["original"]);
  }
  return asset.mediaType.startsWith("image/") ? formUrl(asset, ["webp", "png", "original"], true) : undefined;
}
