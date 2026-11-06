// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  clearOverridesRecipe,
  flipRecipe,
  layoutContextOf,
  overridesOf,
  padOf,
  pinRecipe,
  sizingOf,
  sizingRecipe,
} from "./layout-model.ts";

const text = (id: string) => ({ id, type: "Text", props: { data: id } });
type Tree = Record<string, unknown> & { children: Record<string, unknown>[] };
const row = (): Tree => ({
  id: "row",
  type: "Row",
  props: { mainAxisAlignment: "center" },
  children: [text("a"), { id: "x", type: "Expanded", props: { flex: 2 }, slots: { child: text("b") } }, text("c")],
});

describe("PC-22 layout editors [WGT-010]", () => {
  test("a node's layout: the box it is and how its row, column or stack lays it out", () => {
    const root = row();
    expect(layoutContextOf(root, "row").box).toMatchObject({ kind: "flex", axis: "horizontal" });
    expect(layoutContextOf(root, "a").item).toEqual({ kind: "flex", axis: "horizontal", parent: "row" });
    expect(layoutContextOf(root, "b").item).toMatchObject({ wrapper: { id: "x", type: "Expanded", flex: 2 } });
    const stack = {
      id: "s",
      type: "Stack",
      children: [{ id: "p", type: "Positioned", props: { top: 4 }, slots: { child: text("t") } }, text("u")],
    };
    expect(layoutContextOf(stack, "t").item).toMatchObject({
      kind: "stack",
      positioned: { id: "p", props: { top: 4 } },
    });
    expect(layoutContextOf(stack, "u").item).toEqual({ kind: "stack", parent: "s" });
    expect(layoutContextOf({ id: "w", type: "Wrap", props: { direction: "vertical" } }, "w").box).toMatchObject({
      kind: "wrap",
      axis: "vertical",
    });
    expect(padOf({ mainAxisAlignment: "end" })).toEqual({ main: "end", cross: "center" });
    expect(padOf({ crossAxisAlignment: "stretch" })).toEqual({ main: "start", cross: undefined });
  });

  test("hug, fill and fixed along a row: Expanded in and out, a width or a SizedBox", () => {
    const root = row();
    sizingRecipe("a", "horizontal", "fill", "e1", 0)(root);
    expect(root.children[0]).toMatchObject({ id: "e1", type: "Expanded", slots: { child: { id: "a" } } });
    expect(sizingOf(text("a"), layoutContextOf(root, "a").item, "horizontal")).toBe("fill");
    sizingRecipe("b", "horizontal", "hug", "unused", 0)(root);
    expect(root.children[1]).toEqual(text("b"));
    sizingRecipe("c", "horizontal", "fixed", "sb", 80)(root);
    expect(root.children[2]).toEqual({ id: "sb", type: "SizedBox", props: { width: 80 }, slots: { child: text("c") } });
    const box = { id: "r", type: "Row", children: [{ id: "k", type: "Container" }] };
    sizingRecipe("k", "vertical", "fixed", "unused", 40)(box);
    expect(box.children[0] as unknown).toEqual({ id: "k", type: "Container", props: { height: 40 } });
  });

  test("pins in a stack wrap the widget in a Positioned and the last pin taken off unwraps it", () => {
    const stack = { id: "s", type: "Stack", children: [text("u")] as Record<string, unknown>[] };
    pinRecipe("u", "left", 0, "pos")(stack);
    expect(stack.children[0]).toEqual({
      id: "pos",
      type: "Positioned",
      props: { left: 0 },
      slots: { child: text("u") },
    });
    pinRecipe("u", "bottom", 12, "unused")(stack);
    expect(stack.children[0]?.["props"]).toEqual({ left: 0, bottom: 12 });
    pinRecipe("u", "left", undefined, "unused")(stack);
    pinRecipe("u", "bottom", undefined, "unused")(stack);
    expect(stack.children[0]).toEqual(text("u"));
  });

  test("a row turns into a column keeping its props; overrides are listed and cleared per size class", () => {
    const root = { id: "pad", type: "Padding", slots: { child: row() } };
    const ctx = layoutContextOf(root, "row");
    if (ctx.box !== undefined) {
      flipRecipe("row", ctx.box)(root);
    }
    expect(root.slots.child).toMatchObject({ type: "Column", props: { mainAxisAlignment: "center" } });
    const node = {
      id: "n",
      type: "Row",
      responsive: { medium: { spacing: 8, mainAxisSize: "min" }, expanded: { spacing: 16 } },
    };
    expect(overridesOf(node)).toEqual({ medium: ["spacing", "mainAxisSize"], expanded: ["spacing"] });
    clearOverridesRecipe("n", "medium")(node);
    expect(node.responsive as unknown).toEqual({ expanded: { spacing: 16 } });
    clearOverridesRecipe("n", "expanded")(node);
    expect("responsive" in node).toBe(false);
  });
});
