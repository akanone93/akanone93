// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { assetSource, sourceOf } from "./image-field.tsx";

describe("[AST-002] the image picker's values", () => {
  test("a source is an app asset or a web address; picking an asset keeps the ThumbHash", () => {
    expect(sourceOf({ asset: { $asset: "logo" } })).toEqual({ asset: "logo" });
    expect(sourceOf({ url: "https://example.com/a.png" })).toEqual({ url: "https://example.com/a.png" });
    expect(sourceOf(undefined)).toEqual({});
    expect(assetSource({ url: "https://x", thumbHash: "abc" }, "logo")).toEqual({
      asset: { $asset: "logo" },
      thumbHash: "abc",
    });
  });
});
