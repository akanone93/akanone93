// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { hostFontSources } from "./host-fonts.ts";

const asset = (id: string, url?: string) => ({
  id,
  variants: url === undefined ? [] : [{ artifact: { urls: [url] } }],
});

describe("[STU-006] host fonts for the canvas", () => {
  test("each listed font is the verified file of its asset, in the app's order", () => {
    const app = {
      hostFonts: [
        { family: "Brand Serif", assetId: "b" },
        { family: "Brand Sans", assetId: "a" },
      ],
    };
    expect(hostFontSources(app, [asset("a", "https://files/a.ttf"), asset("b", "https://files/b.ttf")])).toEqual([
      { family: "Brand Serif", url: "https://files/b.ttf" },
      { family: "Brand Sans", url: "https://files/a.ttf" },
    ]);
  });

  test("an entry whose asset is gone or has no file is left out", () => {
    const app = {
      hostFonts: [
        { family: "Gone", assetId: "x" },
        { family: "Pending", assetId: "p" },
        { family: "Kept", assetId: "k" },
      ],
    };
    expect(hostFontSources(app, [asset("p"), asset("k", "https://files/k.ttf")])).toEqual([
      { family: "Kept", url: "https://files/k.ttf" },
    ]);
  });
});
