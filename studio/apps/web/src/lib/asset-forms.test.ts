// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { type AssetForms, formUrl, previewUrlOf } from "./asset-forms.ts";

const form = (kind: string, density: number, url: string) => ({ kind, density, artifact: { urls: [url] } });
const asset = (mediaType: string, variants: AssetForms["variants"]): AssetForms => ({ mediaType, variants });

describe("[AST-002] the forms of an asset Studio shows", () => {
  test("the first of the kinds asked for, at the highest density or the smallest, read same-origin", () => {
    const photo = asset("image/jpeg", [form("webp", 1, "/w1"), form("webp", 3, "/w3"), form("original", 0, "/o")]);
    expect(formUrl(photo, ["png", "webp"])).toBe("/w3");
    expect(formUrl(photo, ["webp"], true)).toBe("/w1");
    expect(formUrl(photo, ["avif"])).toBeUndefined();
    expect(formUrl(asset("image/png", [form("webp", 1, "https://cdn.example/v1/objects/assets/ab")]), ["webp"])).toBe(
      "/api/v1/objects/assets/ab",
    );
  });

  test("a preview is an SVG's stored file, a raster image's smallest WebP, or nothing for another file", () => {
    expect(previewUrlOf(asset("image/svg+xml", [form("vector", 0, "/vec"), form("original", 0, "/svg")]))).toBe("/svg");
    expect(previewUrlOf(asset("image/png", [form("webp", 2, "/w2"), form("webp", 1, "/w1")]))).toBe("/w1");
    expect(previewUrlOf(asset("image/png", [form("original", 0, "/png")]))).toBe("/png");
    expect(previewUrlOf(asset("font/ttf", [form("original", 0, "/ttf")]))).toBeUndefined();
  });
});
