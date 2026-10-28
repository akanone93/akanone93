// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { type DeviceFontAsset, deviceFontSources } from "./device-fonts.ts";
import { fontRequest } from "./surface.ts";

function ttf(family: string): Uint8Array {
  const text = [...family].flatMap((c) => [0, c.charCodeAt(0)]);
  const name = new Uint8Array(18 + text.length);
  const nv = new DataView(name.buffer);
  nv.setUint16(2, 1);
  nv.setUint16(4, 18);
  nv.setUint16(6, 3);
  nv.setUint16(8, 1);
  nv.setUint16(10, 0x409);
  nv.setUint16(12, 1);
  nv.setUint16(14, text.length);
  name.set(text, 18);
  const out = new Uint8Array(28 + name.length);
  const ov = new DataView(out.buffer);
  ov.setUint32(0, 0x00010000);
  ov.setUint16(4, 1);
  ov.setUint32(12, 0x6e616d65);
  ov.setUint32(20, 28);
  ov.setUint32(24, name.length);
  out.set(name, 28);
  return out;
}

const asset = (mediaType: string, url: string | undefined, size = 100): DeviceFontAsset => ({
  mediaType,
  size: BigInt(size),
  variants: url === undefined ? [] : [{ artifact: { urls: [url] } }],
});

const files: Record<string, Uint8Array> = {
  "https://files/a.ttf": ttf("Brand Sans"),
  "https://files/b.otf": ttf("Brand Serif"),
  "https://files/junk.ttf": new Uint8Array([1, 2, 3]),
};
const read = async (url: string): Promise<Uint8Array> => {
  const bytes = files[url];
  if (bytes === undefined) {
    throw new Error("unavailable");
  }
  return bytes;
};

describe("STU-006 device fonts for the canvas", () => {
  test("each font asset is named by the family its file declares, in order", async () => {
    const assets = [asset("font/ttf", "https://files/a.ttf"), asset("font/otf", "https://files/b.otf")];
    expect(await deviceFontSources(assets, read)).toEqual([
      { family: "Brand Sans", url: "https://files/a.ttf" },
      { family: "Brand Serif", url: "https://files/b.otf" },
    ]);
  });

  test("other assets, unreadable, malformed, oversized and file-less fonts are skipped", async () => {
    const assets = [
      asset("image/png", "https://files/a.ttf"),
      asset("font/ttf", "https://files/missing.ttf"),
      asset("font/ttf", "https://files/junk.ttf"),
      asset("font/ttf", "https://files/b.otf", 5000),
      asset("font/ttf", undefined),
      asset("font/ttf", "https://files/a.ttf"),
    ];
    expect(await deviceFontSources(assets, read, 1000)).toEqual([{ family: "Brand Sans", url: "https://files/a.ttf" }]);
  });

  test("bytes longer than the asset limit are refused even when the listed size is small", async () => {
    expect(await deviceFontSources([asset("font/ttf", "https://files/a.ttf", 1)], read, 10)).toEqual([]);
  });

  test("the surface passes device fonts to the canvas ahead of the host's", () => {
    const device = [{ family: "Brand Sans", url: "https://files/a.ttf" }];
    const host = [{ family: "Host", url: "https://files/h.ttf" }];
    const request = fontRequest(device, host);
    expect(request.device).toEqual(device);
    expect(request.host).toEqual(host);
    expect(request.roboto?.family).toBe("Roboto");
  });
});
