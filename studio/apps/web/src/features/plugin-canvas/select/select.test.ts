// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Node } from "../plugin-model.ts";
import { cloneWithFreshIds, duplicateRecipe, nudgeRecipe, reparentRecipe, unwrapRecipe, wrapRecipe } from "./edits.ts";
import { distances, nudgeDelta, resizeBox, snapBox } from "./geometry.ts";
import { handleKey } from "./keys.ts";
import { announce } from "./live-region.ts";
import { resolveDrop } from "./reparent.ts";
import * as sel from "./selection.ts";
import { createSelectionStore } from "./store.ts";
import { indexNodes } from "./tree.ts";

const root: Node = {
  id: "root",
  children: [
    { id: "a", children: [{ id: "a1" }, { id: "a2" }] },
    { id: "b", children: [{ id: "b1" }] },
  ],
};
const index = indexNodes([root]);
const empty = sel.emptySelection;

describe("selection rules (STU-067)", () => {
  test("STU-067 click selects the direct child of the scope, deep click the innermost", () => {
    expect(sel.click(index, empty, ["a1", "a", "root"]).ids).toEqual(["a"]);
    expect(sel.click(index, empty, ["a1", "a", "root"], { deep: true }).ids).toEqual(["a1"]);
  });
  test("STU-067 shift-click toggles", () => {
    const one = sel.click(index, empty, ["a"]);
    const two = sel.click(index, one, ["b"], { toggle: true });
    expect(two.ids).toEqual(["a", "b"]);
    expect(sel.click(index, two, ["a"], { toggle: true }).ids).toEqual(["b"]);
  });
  test("STU-067 click on nothing clears", () => {
    expect(sel.click(index, sel.click(index, empty, ["a"]), []).ids).toEqual([]);
  });
  test("STU-067 marquee picks direct children of the scope only", () => {
    expect(sel.marquee(index, empty, ["a1", "a", "b", "root"]).ids).toEqual(["a", "b"]);
    const inside = sel.enter(index, sel.click(index, empty, ["a"]));
    expect(sel.marquee(index, inside, ["a1", "a2", "b1"]).ids).toEqual(["a1", "a2"]);
  });
  test("STU-067 Esc goes to the parent, then clears; Enter goes to the first child", () => {
    const deep = sel.click(index, empty, ["a1"], { deep: true });
    const up = sel.toParent(index, deep);
    expect(up.ids).toEqual(["a"]);
    expect(sel.toParent(index, up).ids).toEqual([]);
    const into = sel.enter(index, up);
    expect(into.ids).toEqual(["a1"]);
    expect(into.scope).toBe("a");
  });
  test("STU-067 Tab walks siblings and deleted nodes drop out", () => {
    const a = sel.click(index, empty, ["a"]);
    expect(sel.sibling(index, a, 1).ids).toEqual(["b"]);
    expect(sel.sibling(index, a, -1).ids).toEqual(["b"]);
    expect(sel.prune(indexNodes([{ id: "root", children: [{ id: "b" }] }]), a).ids).toEqual([]);
  });
});

describe("live region and store (STU-067)", () => {
  const a = { none: () => "none", one: (n: string) => `${n} selected`, many: (c: number) => `${c} selected` };
  test("STU-067 announces none, one and many", () => {
    expect(announce(a, empty, (i) => i)).toBe("none");
    expect(announce(a, sel.click(index, empty, ["a"]), (i) => i.toUpperCase())).toBe("A selected");
    expect(announce(a, { ids: ["a", "b"], anchor: "b", scope: "root" }, (i) => i)).toBe("2 selected");
  });
  test("STU-067 each store is its own and notifies subscribers", () => {
    const s1 = createSelectionStore();
    const s2 = createSelectionStore();
    let n = 0;
    s1.subscribe(() => n++);
    s1.set(sel.click(index, empty, ["a"]));
    expect(n).toBe(1);
    expect(s2.get().ids).toEqual([]);
  });
});

describe("keyboard, nudge, edits (STU-067)", () => {
  test("STU-067 arrows nudge 1 px and Shift+arrows 10 px through the injected edit", () => {
    expect(nudgeDelta("ArrowLeft", false)).toEqual({ x: -1, y: 0 });
    expect(nudgeDelta("ArrowDown", true)).toEqual({ x: 0, y: 10 });
    const store = createSelectionStore(sel.click(index, empty, ["a"]));
    const calls: [string, string | undefined][] = [];
    const draft = { id: "root", children: [{ id: "a" }] };
    const ctx = {
      store,
      index,
      docOf: () => "doc",
      edit: (d: string, r: (x: unknown) => void, o?: { group?: string }) => {
        calls.push([d, o?.group]);
        r(draft);
      },
    };
    const key = { shiftKey: true, ctrlKey: false, metaKey: false };
    expect(handleKey(ctx, { key: "ArrowRight", ...key })).toBe(true);
    expect(calls).toEqual([["doc", "nudge"]]);
    expect(JSON.stringify(draft)).toContain('"offset":{"x":10,"y":0}');
    expect(handleKey(ctx, { key: "x", ...key })).toBe(false);
  });
  test("STU-067 wrap, unwrap and reparent edit the draft", () => {
    const d = { id: "r", children: [{ id: "a" }, { id: "g", children: [{ id: "c" }] }] };
    wrapRecipe("a", "w", "Row")(d);
    expect(d.children[0]).toEqual({ id: "w", type: "Row", children: [{ id: "a" }] });
    unwrapRecipe("w")(d);
    expect(d.children.map((c) => c.id)).toEqual(["a", "g"]);
    reparentRecipe(["a"], "g", 0)(d);
    expect(d.children.map((c) => c.id)).toEqual(["g"]);
    expect((d.children[0]?.children ?? []).map((c) => c.id)).toEqual(["a", "c"]);
    reparentRecipe(["g"], "c", 0)(d);
    expect(d.children.map((c) => c.id)).toEqual(["g"]);
  });
  test("STU-067 duplicate remaps every ID, including nested", () => {
    const d = { id: "r", children: [{ id: "a", children: [{ id: "a1" }] }] };
    let n = 0;
    duplicateRecipe("a", (old) => `${old}-${++n}`)(d);
    expect(d.children).toHaveLength(2);
    expect(JSON.stringify(d.children[1])).toBe('{"id":"a-1","children":[{"id":"a1-2"}]}');
    expect([...cloneWithFreshIds({ id: "x", k: [{ id: "y" }] }, (o) => `n${o}`).map]).toEqual([
      ["x", "nx"],
      ["y", "ny"],
    ]);
    const e = { id: "r", children: [{ id: "a" }] };
    nudgeRecipe(["a"], 1, 2)(e);
    nudgeRecipe(["a"], 1, 2)(e);
    expect(JSON.stringify(e)).toContain('"offset":{"x":2,"y":4}');
  });
});

describe("guides, resize, reparent target (STU-067)", () => {
  test("STU-067 snapping aligns to edges within the threshold and draws guides", () => {
    const other = { l: 100, t: 0, r: 200, b: 50 };
    const r = snapBox({ l: 103, t: 80, r: 153, b: 130 }, [other], 1);
    expect(r.delta.x).toBe(-3);
    expect(r.guides.some((g) => g.axis === "x" && g.at === 100)).toBe(true);
    expect(snapBox({ l: 120, t: 80, r: 135, b: 130 }, [other], 1).delta).toEqual({ x: 0, y: 0 });
    expect(snapBox({ l: 103, t: 80, r: 153, b: 130 }, [other], 0.25).delta.x).toBe(-3);
  });
  test("STU-067 resize keeps a minimum size and distances measure gaps", () => {
    expect(resizeBox({ l: 0, t: 0, r: 10, b: 10 }, "se", { x: -50, y: 5 }, 2)).toEqual({ l: 0, t: 0, r: 2, b: 15 });
    expect(resizeBox({ l: 0, t: 0, r: 10, b: 10 }, "nw", { x: 3, y: 4 })).toEqual({ l: 3, t: 4, r: 10, b: 10 });
    expect(distances({ l: 0, t: 0, r: 10, b: 10 }, { l: 25, t: 0, r: 40, b: 10 })).toEqual([
      { axis: "x", from: 10, to: 25, across: 5, value: 15 },
    ]);
  });
  test("STU-067 reparent resolves the smallest valid container and the insertion index", () => {
    const box = (l: number, t: number, r: number, b: number) => ({ l, t, r, b });
    const containers = [
      {
        id: "root",
        box: box(0, 0, 100, 100),
        axis: "column" as const,
        children: [
          { id: "a", box: box(0, 0, 100, 40) },
          { id: "b", box: box(0, 50, 100, 90) },
        ],
      },
      {
        id: "a",
        box: box(0, 0, 100, 40),
        axis: "row" as const,
        children: [
          { id: "a1", box: box(0, 0, 40, 40) },
          { id: "a2", box: box(50, 0, 90, 40) },
        ],
      },
    ];
    const t = resolveDrop(index, containers, ["b1"], 45, 10);
    expect(t?.parent).toBe("a");
    expect(t?.index).toBe(1);
    expect(t?.indicator).toEqual(box(50, 0, 50, 40));
    expect(resolveDrop(index, containers, ["a"], 10, 10)?.parent).toBe("root");
    expect(resolveDrop(index, containers, ["a"], 500, 500)).toBeNull();
  });
});
