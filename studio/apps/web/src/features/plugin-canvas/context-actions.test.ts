// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  addAfterRecipe,
  convertKeepsChildren,
  convertRecipe,
  convertTargets,
  crossAlignmentOf,
  crossAlignRecipe,
  siblingOf,
  wrapInRecipe,
} from "./context-actions.ts";

const text = (id: string) => ({ id, type: "Text", props: { data: id } });
const page = () => ({
  id: "col",
  type: "Column",
  props: { mainAxisAlignment: "center", crossAxisAlignment: "start", spacing: 8 },
  children: [text("a"), { id: "pad", type: "Padding", props: { padding: { all: 8 } }, slots: { child: text("b") } }],
});

describe("PC-19 the context toolbar [STU-067]", () => {
  test("wrapping puts the widget in a list or in the wrapper's child slot, as its descriptor takes it", () => {
    const root = page();
    wrapInRecipe("a", "w1", "Padding")(root);
    expect(root.children[0]).toEqual({
      id: "w1",
      type: "Padding",
      props: { padding: { all: 16 } },
      slots: { child: text("a") },
    });
    wrapInRecipe("w1", "w2", "Row")(root);
    expect(root.children[0]).toMatchObject({ id: "w2", type: "Row", children: [{ id: "w1" }] });
  });

  test("converting keeps the props both widgets declare and the children in the form the new one takes", () => {
    expect(convertTargets("Column")).toEqual(expect.arrayContaining(["Row", "Wrap"]));
    expect(convertTargets("Text")).toEqual([]);
    const root = { id: "r", type: "Center", slots: { child: page() } };
    convertRecipe("col", "Row")(root);
    expect(root.slots.child).toMatchObject({
      id: "col",
      type: "Row",
      props: { mainAxisAlignment: "center", crossAxisAlignment: "start", spacing: 8 },
    });
    const two = page();
    convertRecipe("pad", "Center")(two);
    expect(two.children[1] as unknown).toEqual({ id: "pad", type: "Center", slots: { child: text("b") } });
    expect(convertKeepsChildren(2, "Padding")).toBe(false);
    expect(convertKeepsChildren(2, "Row")).toBe(true);
  });

  test("a sibling is added right after the widget; a row's or column's alignment is set", () => {
    const root = page();
    let n = 0;
    const node = siblingOf("FilledButton", "Go", () => `n${++n}`);
    addAfterRecipe("a", node)(root);
    expect(root.children.map((c) => c.id)).toEqual(["a", "n1", "pad"]);
    expect(node).toMatchObject({ slots: { child: { props: { data: "Go" } } }, events: { onPressed: {} } });
    crossAlignRecipe("col", "stretch")(root);
    expect(crossAlignmentOf(root)).toBe("stretch");
    expect(crossAlignmentOf({})).toBe("center");
  });
});
