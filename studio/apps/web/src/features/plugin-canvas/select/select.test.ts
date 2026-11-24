// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { createStore } from "zustand/vanilla";
import type { Node } from "../plugin-model.ts";
import { planDrop } from "./drop-rules.ts";
import {
  cloneWithFreshIds,
  duplicateRecipe,
  moveRecipe,
  nudgeRecipe,
  reparentRecipe,
  resizeRecipe,
  unwrapRecipe,
  wrapRecipe,
} from "./edits.ts";
import { nudgeDelta, resizeBox } from "./geometry.ts";
import { handleKey } from "./keys.ts";
import { announce } from "./live-region.ts";
import * as sel from "./selection.ts";
import { selectionStoreOf } from "./store.ts";

const createSelectionStore = (initial: sel.Selection = sel.emptySelection) =>
  selectionStoreOf(createStore<{ selection: sel.Selection }>(() => ({ selection: initial })));

import { indexNodes, itemIdOf, nodeIdOfHit } from "./tree.ts";

interface T {
  id: string;
  type?: string;
  props?: Record<string, unknown>;
  slots?: Record<string, T>;
  children?: T[];
}

const root: Node = {
  id: "root",
  children: [
    { id: "a", children: [{ id: "a1" }, { id: "a2" }] },
    { id: "b", children: [{ id: "b1" }] },
  ],
};
const index = indexNodes([root]);
const empty = sel.emptySelection;

describe("selection rules [STU-067]", () => {
  test("[STU-067] click selects the direct child of the scope, deep click the innermost", () => {
    expect(sel.click(index, empty, ["a1", "a", "root"]).ids).toEqual(["a"]);
    expect(sel.click(index, empty, ["a1", "a", "root"], { deep: true }).ids).toEqual(["a1"]);
  });
  test("[STU-067] shift-click toggles", () => {
    const one = sel.click(index, empty, ["a"]);
    const two = sel.click(index, one, ["b"], { toggle: true });
    expect(two.ids).toEqual(["a", "b"]);
    expect(sel.click(index, two, ["a"], { toggle: true }).ids).toEqual(["b"]);
  });
  test("[STU-067] click on nothing clears", () => {
    expect(sel.click(index, sel.click(index, empty, ["a"]), []).ids).toEqual([]);
  });
  test("[STU-067] marquee picks direct children of the scope only", () => {
    expect(sel.marquee(index, empty, ["a1", "a", "b", "root"]).ids).toEqual(["a", "b"]);
    const inside = sel.enter(index, sel.click(index, empty, ["a"]));
    expect(sel.marquee(index, inside, ["a1", "a2", "b1"]).ids).toEqual(["a1", "a2"]);
  });
  test("[STU-067] Esc goes to the parent, then the screen, then clears; Enter goes to the first child", () => {
    const deep = sel.click(index, empty, ["a1"], { deep: true });
    const up = sel.toParent(index, deep);
    expect(up.ids).toEqual(["a"]);
    const screen = sel.toParent(index, up);
    expect(screen.ids).toEqual(["root"]);
    expect(sel.toParent(index, screen).ids).toEqual([]);
    const into = sel.enter(index, up);
    expect(into.ids).toEqual(["a1"]);
    expect(into.scope).toBe("a");
  });
  test("[STU-067] double-click drills one level into the selection at the pointer", () => {
    const a = sel.click(index, empty, ["a2", "a", "root"]);
    expect(a.ids).toEqual(["a"]);
    const into = sel.drill(index, a, ["a2", "a", "root"]);
    expect(into).toEqual({ ids: ["a2"], anchor: "a2", scope: "a" });
    // A leaf stays; a hit outside the selection is a plain click.
    expect(sel.drill(index, into, ["a2", "a", "root"])).toBe(into);
    expect(sel.drill(index, into, ["b1", "b", "root"]).ids).toEqual(["b"]);
    expect(sel.drill(index, a, [])).toBe(a);
  });
  test("[STU-067] Tab walks siblings and deleted nodes drop out", () => {
    const a = sel.click(index, empty, ["a"]);
    expect(sel.sibling(index, a, 1).ids).toEqual(["b"]);
    expect(sel.sibling(index, a, -1).ids).toEqual(["b"]);
    expect(sel.prune(indexNodes([{ id: "root", children: [{ id: "b" }] }]), a).ids).toEqual([]);
  });
});

describe("live region and store [STU-067]", () => {
  const a = { none: () => "none", one: (n: string) => `${n} selected`, many: (c: number) => `${c} selected` };
  test("[STU-067] announces none, one and many", () => {
    expect(announce(a, empty, (i) => i)).toBe("none");
    expect(announce(a, sel.click(index, empty, ["a"]), (i) => i.toUpperCase())).toBe("A selected");
    expect(announce(a, { ids: ["a", "b"], anchor: "b", scope: "root" }, (i) => i)).toBe("2 selected");
  });
  test("[STU-067] each area store holds its own selection", () => {
    const s1 = createSelectionStore();
    const s2 = createSelectionStore();
    s1.set(sel.click(index, empty, ["a"]));
    expect(s1.get().ids).toEqual(["a"]);
    expect(s2.get().ids).toEqual([]);
  });
});

describe("keyboard, nudge, edits [STU-067]", () => {
  test("[STU-067] arrows nudge a Stack child's Positioned left/top 1 px, Shift 10 px", () => {
    expect(nudgeDelta("ArrowLeft", false)).toEqual({ x: -1, y: 0 });
    const d: T = {
      id: "s",
      type: "Stack",
      children: [{ id: "p", type: "Positioned", props: { left: 5 }, slots: { child: { id: "t", type: "Text" } } }],
    };
    nudgeRecipe("t", "ArrowRight", true)(d);
    nudgeRecipe("p", "ArrowDown", false)(d);
    expect(d.children?.[0]?.props).toEqual({ left: 15, top: 1 });
    moveRecipe("t", 2.4, -3)(d);
    expect(d.children?.[0]?.props).toEqual({ left: 17, top: -2 });
  });
  test("[STU-067] arrows in a Column reorder one sibling; across the axis and elsewhere nothing changes", () => {
    const d: T = { id: "c", type: "Column", children: [{ id: "a" }, { id: "b" }, { id: "c2" }] };
    nudgeRecipe("a", "ArrowDown", true)(d);
    expect((d.children ?? []).map((c) => c.id)).toEqual(["b", "a", "c2"]);
    nudgeRecipe("a", "ArrowRight", false)(d);
    nudgeRecipe("b", "ArrowUp", false)(d);
    expect((d.children ?? []).map((c) => c.id)).toEqual(["b", "a", "c2"]);
    const pad: T = { id: "p", type: "Padding", slots: { child: { id: "x", type: "Text" } } };
    nudgeRecipe("x", "ArrowRight", false)(pad);
    expect(JSON.stringify(pad)).not.toContain("left");
  });
  test("[STU-067] resize writes width/height only where the descriptor has size props", () => {
    const d: T = {
      id: "r",
      children: [
        { id: "z", type: "SizedBox" },
        { id: "t", type: "Text" },
      ],
    };
    resizeRecipe("z", 40, 20)(d);
    resizeRecipe("t", 40, 20)(d);
    expect(d.children?.[0]).toEqual({ id: "z", type: "SizedBox", props: { width: 40, height: 20 } });
    expect(d.children?.[1]).toEqual({ id: "t", type: "Text" });
  });
  test("[STU-067] arrow keys go through the injected edit, grouped", () => {
    const store = createSelectionStore(sel.click(index, empty, ["a"]));
    const calls: [string, string | undefined][] = [];
    const ctx = {
      store,
      index,
      docOf: () => "doc",
      edit: (dd: string, _r: (x: unknown) => void, o?: { group?: string }) => {
        calls.push([dd, o?.group]);
      },
    };
    const k = { shiftKey: true, ctrlKey: false, metaKey: false };
    expect(handleKey(ctx, { key: "ArrowRight", ...k })).toBe(true);
    expect(calls).toEqual([["doc", "nudge"]]);
    expect(handleKey(ctx, { key: "x", ...k })).toBe(false);
  });
  test("[STU-067] hit IDs map to document node IDs", () => {
    expect(nodeIdOfHit("home/n-1")).toBe("n-1");
    expect(nodeIdOfHit(itemIdOf("home", "n-1"))).toBe("n-1");
    expect(sel.click(index, empty, ["a1", "a"].map((n) => itemIdOf("s", n)).map(nodeIdOfHit)).ids).toEqual(["a"]);
  });
  test("[STU-067] wrap, unwrap and reparent edit the draft", () => {
    interface D {
      id: string;
      type?: string;
      children?: D[];
    }
    const d: { id: string; children: D[] } = { id: "r", children: [{ id: "a" }, { id: "g", children: [{ id: "c" }] }] };
    wrapRecipe("a", "w", "Row")(d);
    expect(d.children?.[0]).toEqual({ id: "w", type: "Row", children: [{ id: "a" }] });
    unwrapRecipe("w")(d);
    expect(d.children.map((c) => c.id)).toEqual(["a", "g"]);
    reparentRecipe(["a"], "g", 0)(d);
    expect(d.children.map((c) => c.id)).toEqual(["g"]);
    expect((d.children[0]?.children ?? []).map((c) => c.id)).toEqual(["a", "c"]);
    reparentRecipe(["g"], "c", 0)(d);
    expect(d.children.map((c) => c.id)).toEqual(["g"]);
  });
  test("[STU-067] duplicate remaps every ID, including nested", () => {
    const d = { id: "r", children: [{ id: "a", children: [{ id: "a1" }] }] };
    let n = 0;
    duplicateRecipe("a", (old) => `${old}-${++n}`)(d);
    expect(d.children).toHaveLength(2);
    expect(JSON.stringify(d.children?.[1])).toBe('{"id":"a-1","children":[{"id":"a1-2"}]}');
    expect([...cloneWithFreshIds({ id: "x", k: [{ id: "y" }] }, (o) => `n${o}`).map]).toEqual([
      ["x", "nx"],
      ["y", "ny"],
    ]);
  });
});

describe("resize, reparent target [STU-067]", () => {
  test("[STU-067] resize keeps a minimum size", () => {
    expect(resizeBox({ l: 0, t: 0, r: 10, b: 10 }, "se", { x: -50, y: 5 }, 2)).toEqual({ l: 0, t: 0, r: 2, b: 15 });
    expect(resizeBox({ l: 0, t: 0, r: 10, b: 10 }, "nw", { x: 3, y: 4 })).toEqual({ l: 3, t: 4, r: 10, b: 10 });
  });
  test("[STU-067] reparent resolves the smallest valid container and the insertion index", () => {
    const box = (l: number, t: number, r: number, b: number) => ({ l, t, r, b });
    const boxes: Record<string, ReturnType<typeof box>> = {
      root: box(0, 0, 100, 100),
      a: box(0, 0, 100, 40),
      a1: box(0, 0, 40, 40),
      a2: box(50, 0, 90, 40),
      b: box(0, 50, 100, 90),
      b1: box(0, 50, 100, 90),
    };
    const typed = indexNodes([
      {
        id: "root",
        type: "Column",
        children: [
          {
            id: "a",
            type: "Row",
            children: [
              { id: "a1", type: "Text" },
              { id: "a2", type: "Text" },
            ],
          },
          { id: "b", type: "Column", children: [{ id: "b1", type: "Text" }] },
        ],
      } as Node,
    ]);
    const env = { index: typed, boxOf: (id: string) => boxes[id], typeOf: (id: string) => typed.type.get(id) };
    const t = planDrop(env, { type: "Text", ids: ["b1"] }, 45, 10);
    expect(t.ok && t.target.parent).toBe("a");
    expect(t.ok && t.target.index).toBe(1);
    expect(t.ok && t.target.indicator).toEqual(box(50, 0, 50, 40));
    const out = planDrop(env, { type: "Row", ids: ["a"] }, 10, 10);
    expect(out.ok && out.target.parent).toBe("root");
    expect(planDrop(env, { type: "Row", ids: ["a"] }, 500, 500)).toEqual({ ok: false, reason: "offScreen", at: null });
  });
});
