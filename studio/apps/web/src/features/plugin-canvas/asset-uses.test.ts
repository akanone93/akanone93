// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { assetBudget, assetFileLimit, assetUses, replaceAssetRecipe } from "./asset-uses.ts";

const image = (id: string, asset: string) => ({ id, type: "Image", props: { source: { $asset: asset } } });

describe("PC-36 where assets are used and what they weigh [AST-001] [AST-003]", () => {
  test("each asset's uses: the widget and page, once per widget, slots and children included", () => {
    const pages = [
      {
        id: "p1",
        root: {
          id: "s",
          type: "Scaffold",
          slots: { body: { id: "c", type: "Column", children: [image("i1", "a"), image("i2", "b")] } },
        },
      },
      {
        id: "p2",
        root: { id: "av", type: "CircleAvatar", props: { image: { $asset: "a" }, fallback: { $asset: "a" } } },
      },
    ];
    const uses = assetUses(pages);
    expect(uses.get("a")).toEqual([
      { node: "i1", page: "p1" },
      { node: "av", page: "p2" },
    ]);
    expect(uses.get("b")).toEqual([{ node: "i2", page: "p1" }]);
    expect(uses.has("z")).toBe(false);
  });

  test("a replacement moves every reference to the old asset and leaves the others", () => {
    const root = {
      id: "c",
      type: "Column",
      children: [image("i1", "a"), image("i2", "b"), { id: "l", props: { list: [{ $asset: "a" }] } }],
    };
    replaceAssetRecipe("a", "a2")(root);
    expect(JSON.stringify(root)).not.toContain('"a"');
    expect(
      assetUses([{ id: "p", root }])
        .get("a2")
        ?.map((u) => u.node),
    ).toEqual(["i1", "l"]);
    expect(
      assetUses([{ id: "p", root }])
        .get("b")
        ?.map((u) => u.node),
    ).toEqual(["i2"]);
  });

  test("the plugin's used assets against plugin.assetBytes: a warning from 80 %, over it fails publish", () => {
    const mb = 1024 * 1024;
    const assets = [
      { id: "a", size: 15 * mb },
      { id: "b", size: 2 * mb },
      { id: "c", size: 9 * mb },
    ];
    expect(assetBudget(assets, new Set(["a"]))).toEqual({ total: 15 * mb, limit: 20 * mb, state: "within" });
    expect(assetBudget(assets, new Set(["a", "b"])).state).toBe("warning");
    expect(assetBudget(assets, new Set(["a", "c"])).state).toBe("over");
    expect(assetFileLimit()).toBe(10 * mb);
  });
});
