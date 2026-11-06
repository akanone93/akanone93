// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { intoSlotRecipe, takeOutRecipe } from "./edits.ts";
import { containersOf, dropAt, finishMove, type GestureEnv } from "./gesture.ts";
import { indexNodes } from "./tree.ts";

type N = { id: string; type: string; props?: Record<string, unknown>; children?: N[]; slots?: Record<string, N> };

const pageA = (): N => ({
  id: "colA",
  type: "Column",
  children: [
    { id: "t1", type: "Text", props: { data: "One" } },
    { id: "pad", type: "Padding", props: { padding: { all: 8 } } },
  ],
});
const pageB = (): N => ({ id: "colB", type: "Column", children: [{ id: "t2", type: "Text", props: { data: "Two" } }] });

const boxes: Record<string, { l: number; t: number; r: number; b: number }> = {
  colA: { l: 0, t: 0, r: 390, b: 300 },
  t1: { l: 0, t: 0, r: 390, b: 20 },
  pad: { l: 0, t: 20, r: 390, b: 120 },
  colB: { l: 500, t: 0, r: 890, b: 300 },
  t2: { l: 500, t: 0, r: 890, b: 20 },
};

function env(roots: Record<string, N>, edits: { doc: string; group?: string }[]): GestureEnv {
  const index = indexNodes(Object.values(roots) as never);
  const docOf = (id: string) => (id === "colA" || index.parent.get(id) === "colA" ? "A" : id === "pad" ? "A" : "B");
  return {
    index,
    edit: (doc, recipe, options) => {
      recipe(roots[doc]);
      edits.push({ doc, ...(options?.group === undefined ? {} : { group: options.group }) });
    },
    docOf,
    boxOf: (id) => boxes[id],
    typeOf: (id) => index.type.get(id),
  };
}

describe("PC-20 drag and drop everywhere [STU-067]", () => {
  test("an empty single-child holder takes a drop into its slot; a full one does not", () => {
    const roots = { A: pageA(), B: pageB() };
    const e = env(roots, []);
    expect(containersOf(e).find((c) => c.id === "pad")).toMatchObject({ axis: "slot", slot: "child" });
    expect(dropAt(e, ["t1"], 100, 60)).toMatchObject({ parent: "pad", slot: "child", indicator: boxes["pad"] });
    finishMove(e, "t1", { x: 0, y: 40 }, 100, 60);
    expect(roots.A.children?.map((c) => c.id)).toEqual(["pad"]);
    expect(roots.A.children?.[0]?.slots?.["child"]?.id).toBe("t1");
  });

  test("a widget dragged onto another screen moves there, as one undo step across the two pages", () => {
    const roots = { A: pageA(), B: pageB() };
    const edits: { doc: string; group?: string }[] = [];
    finishMove(env(roots, edits), "t1", { x: 600, y: 100 }, 600, 200);
    expect(roots.A.children?.map((c) => c.id)).toEqual(["pad"]);
    expect(roots.B.children?.map((c) => c.id)).toEqual(["t2", "t1"]);
    expect(edits.map((x) => x.doc)).toEqual(["A", "B"]);
    expect(edits[0]?.group).toBe(edits[1]?.group);
  });

  test("taking out and filling a slot leave a node inside itself alone", () => {
    const root = pageA();
    intoSlotRecipe("colA", "pad", "child")(root);
    expect(root.children?.length).toBe(2);
    let taken: unknown;
    takeOutRecipe("t1", (n) => {
      taken = n;
    })(root);
    expect(taken).toMatchObject({ id: "t1" });
  });
});
