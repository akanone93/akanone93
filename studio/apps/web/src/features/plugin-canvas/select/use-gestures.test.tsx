// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import type { Guides } from "@plux/canvas";
import { cleanup, renderHook } from "@testing-library/react";
import { createStore } from "zustand/vanilla";
import type { Node } from "../plugin-model.ts";
import type { CanvasSurface } from "../surface.ts";
import type { EditFn } from "./edits.ts";
import * as sel from "./selection.ts";
import { selectionStoreOf } from "./store.ts";
import { indexNodes, itemIdOf } from "./tree.ts";
import { useGestures } from "./use-gestures.ts";

afterEach(cleanup);

const tree: Node = {
  id: "r0",
  type: "Column",
  children: [
    {
      id: "st",
      type: "Stack",
      children: [{ id: "p", type: "Positioned", slots: { child: { id: "t", type: "Text" } } }],
    },
    { id: "c", type: "Column", children: [{ id: "a" }, { id: "b" }] },
    { id: "r", type: "Row", children: [{ id: "x" }] },
    { id: "z", type: "SizedBox" },
  ],
};
const boxes: Record<string, [number, number, number, number]> = {
  st: [0, 0, 100, 100],
  t: [10, 10, 50, 50],
  c: [0, 200, 100, 300],
  a: [0, 200, 100, 240],
  b: [0, 250, 100, 290],
  r: [200, 200, 300, 300],
  x: [200, 200, 240, 240],
  z: [400, 0, 440, 40],
};

function setup(selected: string) {
  const guides: (Guides | null)[] = [];
  const surface = {
    viewport: { x: 0, y: 0, zoom: 1 },
    boundsOf: (item: string) => {
      const b = boxes[item.split("/")[1] ?? ""];
      return b === undefined ? undefined : { l: b[0], t: b[1], r: b[2], b: b[3] };
    },
    setGuides: (g: Guides | null) => guides.push(g),
  } as unknown as CanvasSurface;
  const index = indexNodes([tree]);
  const api = createStore<{ selection: sel.Selection }>(() => ({ selection: sel.emptySelection }));
  const store = selectionStoreOf(api);
  store.set({ ids: [selected], anchor: selected, scope: null });
  const draft = JSON.parse(JSON.stringify(tree)) as Node & { children: Node[] };
  const edits: { doc: string; group: string | undefined }[] = [];
  const edit: EditFn = (doc, recipe, options) => {
    edits.push({ doc, group: options?.group });
    recipe(draft);
  };
  const screenOf = new Map([...index.parent.keys()].map((id) => [id, "s"]));
  const canvas = { getBoundingClientRect: () => ({ left: 0, top: 0 }) } as HTMLCanvasElement;
  const { result } = renderHook(() =>
    useGestures({ surface: () => surface, canvas: () => canvas, store, index, edit, screenOf }),
  );
  return { g: result.current, guides, edits, draft };
}

const at = (x: number, y: number) => ({ clientX: x, clientY: y });
void itemIdOf;

describe("STU-067 drag gestures", () => {
  test("STU-067 dragging a Stack child shows guides, then moves its Positioned left/top", () => {
    const { g, guides, edits, draft } = setup("t");
    expect(g.down(at(20, 20))).toBe(true);
    expect(g.move(at(30, 35))).toBe(true);
    expect(guides.at(-1)).not.toBeNull();
    expect(g.up(at(30, 35))).toBe(true);
    expect(guides.at(-1)).toBeNull();
    expect(edits).toEqual([{ doc: "s", group: "move" }]);
    const p = draft.children[0]?.children?.[0];
    expect(p?.props as unknown).toEqual({ left: 10, top: 15 });
  });

  test("STU-067 dragging a Column child into a Row reparents it at the insertion indicator", () => {
    const { g, guides, edits, draft } = setup("a");
    g.down(at(50, 220));
    g.move(at(250, 250));
    expect(guides.at(-1)?.indicator).toBeDefined();
    g.up(at(250, 250));
    expect(edits).toEqual([{ doc: "s", group: "move" }]);
    const row = draft.children[2];
    expect((row?.children ?? []).map((n) => n.id)).toEqual(["x", "a"]);
    expect((draft.children[1]?.children ?? []).map((n) => n.id)).toEqual(["b"]);
  });

  test("STU-067 dragging within a Column does not change the document", () => {
    const { g, edits, draft } = setup("b");
    g.down(at(50, 270));
    g.move(at(50, 215));
    g.up(at(50, 215));
    expect((draft.children[1]?.children ?? []).map((n) => n.id)).toEqual(["a", "b"]);
    expect(edits.length).toBeLessThanOrEqual(1);
  });

  test("STU-067 the south-east handle resizes only a node with size props", () => {
    const sized = setup("z");
    expect(sized.g.down(at(440, 40))).toBe(true);
    sized.g.up(at(460, 50));
    expect(sized.draft.children[3]?.props as unknown).toEqual({ width: 60, height: 50 });
    const text = setup("t");
    text.g.down(at(50, 50));
    text.g.up(at(70, 70));
    expect(text.draft.children[0]?.children?.[0]?.slots?.["child"]?.props).toBeUndefined();
  });

  test("STU-067 a press without movement is a click: nothing is edited", () => {
    const { g, edits } = setup("t");
    g.down(at(20, 20));
    expect(g.up(at(20, 20))).toBe(false);
    expect(edits).toEqual([]);
    expect(setup("t").g.down(at(300, 300))).toBe(false);
  });
});
