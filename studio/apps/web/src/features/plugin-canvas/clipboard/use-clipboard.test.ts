// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { pasteRecipe, removeRecipe, targetHas } from "./use-clipboard.tsx";

const tree = () => ({
  id: "r",
  children: [
    { id: "a", type: "Text" },
    { id: "b", type: "Text" },
  ],
});

describe("[STU-067] paste and cut edit the document", () => {
  test("paste inserts after the anchor, or at the end of the root", () => {
    const t = tree();
    pasteRecipe("a", [{ id: "n" }])(t);
    expect(t.children.map((c) => c.id)).toEqual(["a", "n", "b"]);
    pasteRecipe(undefined, [{ id: "m" }])(t);
    expect(t.children.map((c) => c.id)).toEqual(["a", "n", "b", "m"]);
  });

  test("cut removes the node", () => {
    const t = tree();
    removeRecipe("a")(t);
    expect(t.children.map((c) => c.id)).toEqual(["b"]);
  });

  test("the target defines the IDs, keys and message keys of its documents", () => {
    const doc = new TextEncoder().encode(JSON.stringify({ id: "i", key: "k", messages: { greet: "x" } }));
    const has = targetHas([{ content: doc }, { content: new TextEncoder().encode("nope") }]);
    expect([...has.functions].sort()).toEqual(["greet", "i", "k"]);
  });
});
