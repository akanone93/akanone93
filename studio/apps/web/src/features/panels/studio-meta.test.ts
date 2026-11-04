// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Node } from "../plugin-canvas/plugin-model.ts";
import { lockedIds, metaOf, metaRecipe, withoutHidden } from "./studio-meta.ts";

const tree = (): Node =>
  ({
    id: "r",
    type: "Column",
    children: [
      { id: "a", type: "Text", "x-studio": { hidden: true } },
      { id: "b", type: "Row", "x-studio": { locked: true }, children: [{ id: "b1", type: "Text" }] },
    ],
    slots: { header: { id: "h", type: "Text" } },
  }) as unknown as Node;

describe("[STU-061] [SCH-004] a layer's name, hidden and locked live in x-studio", () => {
  test("a recipe sets and clears them, and drops an empty x-studio", () => {
    const root = tree() as unknown as Record<string, unknown>;
    metaRecipe("h", { name: "Header title", locked: true })(root);
    const h = (root["slots"] as Record<string, Record<string, unknown>>)["header"];
    expect(h?.["x-studio"]).toEqual({ name: "Header title", locked: true });
    expect(metaOf(h)).toEqual({ name: "Header title", locked: true });
    metaRecipe("h", { name: null, locked: false })(root);
    expect(h !== undefined && "x-studio" in h).toBe(false);
    metaRecipe("missing", { hidden: true })(root);
  });

  test("a lock covers everything inside the locked layer", () => {
    expect([...lockedIds([tree(), undefined])].sort()).toEqual(["b", "b1"]);
  });

  test("the canvas draws the tree without hidden layers, and keeps an untouched tree as it is", () => {
    const drawn = withoutHidden(tree());
    expect(drawn?.children?.map((c) => c.id)).toEqual(["b"]);
    expect(drawn?.slots?.["header"]?.id).toBe("h");
    const plain = { id: "p", type: "Text" } as Node;
    expect(withoutHidden(plain)).toBe(plain);
    expect(withoutHidden({ id: "x", "x-studio": { hidden: true } } as unknown as Node)).toBeUndefined();
  });
});
