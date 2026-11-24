// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { pastePlace, pasteRecipe, removeRecipe, targetHas } from "./use-clipboard.tsx";

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

  test("a paste lands in a list, never on a widget that takes none (#75)", () => {
    const screen = () => ({
      id: "r",
      type: "Scaffold",
      slots: {
        appBar: { id: "bar", type: "AppBar", slots: { title: { id: "title", type: "Text" } } },
        body: {
          id: "col",
          type: "Column",
          children: [
            { id: "a", type: "Center", slots: { child: { id: "inner", type: "Text" } } },
            { id: "b", type: "Text" },
          ],
        },
      },
    });
    // The anchor in a single-child slot: after its nearest ancestor in a list.
    const s1 = screen();
    pasteRecipe("inner", [{ id: "n" }])(s1);
    expect(s1.slots.body.children.map((c) => c.id)).toEqual(["a", "n", "b"]);
    // No anchor, or one with no list above it: at the end of the screen's first list — not on the Scaffold.
    const s2 = screen();
    pasteRecipe(undefined, [{ id: "m" }])(s2);
    pasteRecipe("title", [{ id: "t" }])(s2);
    expect(s2.slots.body.children.map((c) => c.id)).toEqual(["a", "b", "m", "t"]);
    expect("children" in s2).toBe(false);
    // A screen with no list at all has no place: the paste is refused, and the recipe changes nothing.
    const bare = {
      id: "r",
      type: "Scaffold",
      slots: { body: { id: "c", type: "Center", slots: { child: { id: "x" } } } },
    };
    expect(pastePlace(bare, "x")).toBeUndefined();
    const before = JSON.stringify(bare);
    pasteRecipe("x", [{ id: "y" }])(bare);
    expect(JSON.stringify(bare)).toBe(before);
    // A list slot (an app bar's actions) is a list too.
    expect(pastePlace({ id: "r", slots: { actions: [{ id: "i" }] } }, "i")).toMatchObject({ index: 1 });
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
