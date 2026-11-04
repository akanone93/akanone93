// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, mock, test } from "bun:test";
import { assetRefsOf, type ImageAsset, imageUrlOf, loadAssetImages } from "./screen-images.ts";

const asset = (id: string, mediaType: string, variants: ImageAsset["variants"]): ImageAsset => ({
  id,
  mediaType,
  variants,
});
const form = (kind: string, density: number, url: string) => ({ kind, density, artifact: { urls: [url] } });

describe("[AST-002] the images the canvas loads", () => {
  test("the assets a tree names, anywhere in it", () => {
    const tree = [
      { type: "Image", props: { source: { asset: { $asset: "a" } } } },
      {
        type: "CircleAvatar",
        props: { backgroundImage: { asset: { $asset: "b" } } },
        children: [{ x: { $asset: "a" } }],
      },
    ];
    expect([...assetRefsOf(tree)].sort()).toEqual(["a", "b"]);
  });

  test("a raster form CanvasKit decodes, PNG first at the highest density; none for a vector", () => {
    const photo = asset("p", "image/jpeg", [
      form("avif", 2, "/avif"),
      form("webp", 2, "/webp2"),
      form("png", 1, "/png1"),
      form("png", 3, "/png3"),
    ]);
    expect(imageUrlOf(photo)).toBe("/png3");
    expect(imageUrlOf(asset("o", "image/jpeg", [form("original", 0, "/orig")]))).toBe("/orig");
    expect(imageUrlOf(asset("v", "image/svg+xml", [form("original", 0, "/svg")]))).toBeUndefined();
    expect(imageUrlOf(asset("f", "font/ttf", [form("original", 0, "/font")]))).toBeUndefined();
  });

  test("only the named images the canvas lacks are fetched, under their asset keys; a failed fetch is left out", async () => {
    const assets = [
      asset("a", "image/png", [form("png", 1, "/a")]),
      asset("b", "image/png", [form("png", 1, "/b")]),
      asset("c", "image/png", [form("png", 1, "/c")]),
    ];
    const fetchBytes = mock(async (url: string) => (url === "/c" ? null : new Uint8Array([1])));
    const got = await loadAssetImages(assets, new Set(["a", "b", "c"]), fetchBytes, new Set(["asset:b"]));
    expect(got).toEqual([{ key: "asset:a", bytes: new Uint8Array([1]) }]);
    expect(fetchBytes.mock.calls.map((c) => c[0])).toEqual(["/a", "/c"]);
  });
});
