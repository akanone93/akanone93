// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { layerRefusal, modelOf, moveLayer } from "../../panels/model.ts";
import { setPropRecipe } from "../../panels/properties-model.ts";
import type { Node } from "../plugin-model.ts";
import { type DropEnv, placement, planDrop } from "./drop-rules.ts";
import { findNode, moveToRecipe } from "./edits.ts";
import { finishMove, type GestureEnv } from "./gesture.ts";
import { indexNodes } from "./tree.ts";

type B = { l: number; t: number; r: number; b: number };
const box = (l: number, t: number, r: number, b: number): B => ({ l, t, r, b });

function envOf(root: Node, boxes: Record<string, B>, extra: Partial<DropEnv> = {}): DropEnv {
  const index = indexNodes([root]);
  return { index, boxOf: (id) => boxes[id], typeOf: (id) => index.type.get(id), ...extra };
}

describe("[STU-067] the placements Flutter refuses are never written (PC-20)", () => {
  test("Expanded, Flexible and Spacer go in a Row, Column or Flex only", () => {
    expect(placement("Column", null, "Expanded")).toEqual({});
    expect(placement("Row", null, "Spacer")).toEqual({});
    expect(placement("Wrap", null, "Flexible")).toBe("needsFlex");
    expect(placement("Padding", "child", "Expanded")).toBe("needsFlex");
  });

  test("Positioned goes in a Stack only", () => {
    expect(placement("Stack", null, "Positioned")).toEqual({});
    expect(placement("Column", null, "AnimatedPositioned")).toBe("needsStack");
  });

  test("a sliver goes among slivers; a box there is wrapped in a SliverToBoxAdapter", () => {
    expect(placement("CustomScrollView", "slivers", "SliverList")).toEqual({});
    expect(placement("Column", null, "SliverAppBar")).toBe("needsScrollView");
    expect(placement("CustomScrollView", "slivers", "Text")).toEqual({ wrap: "SliverToBoxAdapter" });
    expect(placement("SliverPadding", "sliver", "Card")).toEqual({ wrap: "SliverToBoxAdapter" });
    expect(placement("CustomScrollView", "slivers", "Expanded")).toBe("needsFlex");
  });

  test("a sliver on a screen's empty body comes in a CustomScrollView", () => {
    expect(placement("Scaffold", "body", "SliverAppBar")).toEqual({ wrap: "CustomScrollView" });
  });
});

const screen = (): Node =>
  ({
    id: "s",
    type: "Scaffold",
    slots: {
      body: {
        id: "col",
        type: "Column",
        children: [
          {
            id: "tile",
            type: "ListTile",
            slots: { title: { id: "tt", type: "Text", props: { data: "T" } } },
          },
          { id: "card", type: "Card" },
          { id: "wrap", type: "Wrap", children: [{ id: "w1", type: "Text", props: { data: "W" } }] },
        ],
      },
    },
  }) as unknown as Node;

const screenBoxes: Record<string, B> = {
  s: box(0, 0, 400, 800),
  col: box(0, 0, 400, 800),
  tile: box(0, 0, 400, 72),
  tt: box(72, 20, 300, 52),
  card: box(0, 100, 400, 300),
  wrap: box(0, 320, 400, 400),
  w1: box(0, 320, 60, 340),
};

describe("[STU-067] a drop lands in the widget under the pointer that can take it (PC-20)", () => {
  test("an app bar goes to its screen's app bar slot, wherever it is dropped on the screen", () => {
    const plan = planDrop(envOf(screen(), screenBoxes), { type: "AppBar", ids: [] }, 200, 200);
    expect(plan).toMatchObject({ ok: true, target: { parent: "s", slot: "appBar" } });
  });

  test("a tile's leading part is at its start, its trailing part at its end, its title between", () => {
    const e = envOf(screen(), screenBoxes);
    expect(planDrop(e, { type: "Icon", ids: [] }, 20, 36)).toMatchObject({
      ok: true,
      target: { parent: "tile", slot: "leading" },
    });
    expect(planDrop(e, { type: "Icon", ids: [] }, 380, 36)).toMatchObject({
      ok: true,
      target: { parent: "tile", slot: "trailing" },
    });
    // The title is taken: the subtitle is the next part between.
    expect(planDrop(e, { type: "Text", ids: [] }, 200, 36)).toMatchObject({
      ok: true,
      target: { parent: "tile", slot: "subtitle" },
    });
  });

  test("in a right-to-left preview a tile's leading part is at its right", () => {
    const e = envOf(screen(), screenBoxes, { rtl: true });
    expect(planDrop(e, { type: "Icon", ids: [] }, 380, 36)).toMatchObject({
      ok: true,
      target: { parent: "tile", slot: "leading" },
    });
  });

  test("near the edge of a widget in a list the drop goes beside it; in its middle, into it", () => {
    const e = envOf(screen(), screenBoxes);
    expect(planDrop(e, { type: "Text", ids: [] }, 200, 104)).toMatchObject({
      ok: true,
      target: { parent: "col", index: 1 },
    });
    expect(planDrop(e, { type: "Text", ids: [] }, 200, 200)).toMatchObject({
      ok: true,
      target: { parent: "card", slot: "child" },
    });
  });

  test("a widget no container under the pointer can take is refused, with the innermost reason", () => {
    const e = envOf(screen(), screenBoxes);
    // Over the Wrap, inside the Column: the Column takes the Expanded beside the Wrap.
    expect(planDrop(e, { type: "Expanded", ids: [] }, 200, 360)).toMatchObject({ ok: true, target: { parent: "col" } });
    const plan = planDrop(e, { type: "SliverList", ids: [] }, 200, 360);
    expect(plan).toEqual({ ok: false, reason: "needsScrollView", at: "wrap" });
    expect(planDrop(e, { type: "Text", ids: [] }, 900, 900)).toEqual({ ok: false, reason: "offScreen", at: null });
  });

  test("a locked layer takes nothing in", () => {
    const e = envOf(screen(), screenBoxes, { locked: (id) => id === "card" });
    expect(planDrop(e, { type: "Text", ids: [] }, 200, 200)).toMatchObject({ ok: true, target: { parent: "col" } });
  });

  test("a box dropped among slivers goes in at its place, wrapped", () => {
    const root = {
      id: "csv",
      type: "CustomScrollView",
      slots: {
        slivers: [
          { id: "bar", type: "SliverAppBar" },
          { id: "list", type: "SliverList" },
        ],
      },
    } as unknown as Node;
    const boxes = { csv: box(0, 0, 400, 800), bar: box(0, 0, 400, 200), list: box(0, 200, 400, 800) };
    // In the middle of the app bar, a text is its title; near its bottom edge, it goes after it.
    expect(planDrop(envOf(root, boxes), { type: "Text", ids: [] }, 200, 120)).toMatchObject({
      ok: true,
      target: { parent: "bar", slot: "title" },
    });
    const plan = planDrop(envOf(root, boxes), { type: "Text", ids: [] }, 200, 195);
    expect(plan).toMatchObject({
      ok: true,
      target: { parent: "csv", slot: "slivers", list: true, index: 1, wrap: "SliverToBoxAdapter" },
    });
  });
});

describe("[STU-067] edits reach the widgets of list slots", () => {
  const doc = () => ({
    id: "csv",
    type: "CustomScrollView",
    slots: {
      slivers: [
        {
          id: "bar",
          type: "SliverAppBar",
          slots: {
            actions: [
              { id: "a1", type: "IconButton" },
              { id: "a2", type: "IconButton" },
            ],
          },
        },
      ],
    },
  });

  test("a widget in a list slot is found, and its props are set", () => {
    const d = doc();
    expect(findNode(d, "a2")).toMatchObject({ slot: "actions", index: 1 });
    setPropRecipe("a2", "compact", "tooltip", "Inbox")(d);
    expect(JSON.stringify(d)).toContain('"tooltip":"Inbox"');
  });

  test("a move into a list slot keeps the order; a box moved among slivers is wrapped", () => {
    const d = doc();
    moveToRecipe("a2", { parent: "bar", slot: "actions", list: true, index: 0 })(d);
    expect((d.slots.slivers[0]?.slots.actions ?? []).map((x) => x.id)).toEqual(["a2", "a1"]);
    moveToRecipe("a1", { parent: "csv", slot: "slivers", list: true, index: 1 }, (n) => ({
      id: "w",
      type: "SliverToBoxAdapter",
      slots: { child: n },
    }))(d);
    expect(d.slots.slivers.map((x) => x.id)).toEqual(["bar", "w"]);
  });

  test("a move never fills a taken single slot or goes inside itself", () => {
    const d = { id: "p", type: "Padding", slots: { child: { id: "t", type: "Text" } }, children: [] };
    const before = JSON.stringify(d);
    moveToRecipe("p", { parent: "t", index: 0 })(d);
    expect(JSON.stringify(d)).toBe(before);
  });
});

describe("[STU-067] a refused canvas move writes nothing and says why", () => {
  test("an Expanded dragged into a Wrap's list stays where it is", () => {
    const root = {
      id: "col",
      type: "Column",
      children: [
        { id: "x", type: "Expanded", slots: { child: { id: "t", type: "Text" } } },
        { id: "wrap", type: "Wrap", children: [{ id: "w1", type: "Text" }] },
      ],
    } as unknown as Node;
    const boxes = {
      col: box(0, 0, 400, 400),
      x: box(0, 0, 400, 100),
      t: box(0, 0, 400, 100),
      wrap: box(0, 100, 400, 400),
      w1: box(0, 100, 50, 120),
    };
    const index = indexNodes([root]);
    const edits: string[] = [];
    const env: GestureEnv = {
      index,
      edit: (doc) => {
        edits.push(doc);
      },
      docOf: () => "p",
      boxOf: (id) => boxes[id as keyof typeof boxes],
      typeOf: (id) => index.type.get(id),
      // The Column is locked, so only the Wrap is under the pointer to take the drop.
      locked: (id) => id === "col",
    };
    const plan = finishMove(env, "x", { x: 0, y: 200 }, 200, 250);
    expect(plan).toEqual({ ok: false, reason: "needsFlex", at: "wrap" });
    expect(edits).toEqual([]);
  });
});

describe("[STU-067] the Layers tree keeps the same rules", () => {
  const root = {
    id: "col",
    type: "Column",
    children: [
      { id: "x", type: "Expanded", slots: { child: { id: "t", type: "Text" } } },
      { id: "wrap", type: "Wrap", children: [{ id: "w1", type: "Text" }] },
      {
        id: "bar",
        type: "AppBar",
        slots: {
          actions: [
            { id: "a1", type: "IconButton" },
            { id: "a2", type: "IconButton" },
          ],
        },
      },
    ],
  } as unknown as Node;

  test("an Expanded dropped beside a Wrap's child is refused with its reason", () => {
    const index = indexNodes([root]);
    const model = modelOf([root]);
    expect(layerRefusal(model, index, "x", "w1", "before", null)).toBe("needsFlex");
    expect(layerRefusal(model, index, "x", "wrap", "before", null)).toBeNull();
    expect(layerRefusal(model, index, "col", "w1", "before", null)).toBe("nowhere");
  });

  test("a layer reorders among an app bar's actions", () => {
    const draft = structuredClone(root) as unknown as { children: { slots?: { actions?: { id: string }[] } }[] };
    const index = indexNodes([root]);
    const moved = moveLayer(
      modelOf([root]),
      index,
      "a2",
      "a1",
      "before",
      (_doc, recipe) => recipe(draft),
      () => "p",
      true,
    );
    expect(moved).toBe(true);
    expect((draft.children[2]?.slots?.actions ?? []).map((a) => a.id)).toEqual(["a2", "a1"]);
  });
});
