// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Guides } from "@plux/canvas";
import { createStore } from "zustand/vanilla";
import type { Node } from "../plugin-model.ts";
import type { Point } from "../positions.ts";
import type { CanvasSurface } from "../surface.ts";
import type { EditFn } from "./edits.ts";
import type { Box } from "./geometry.ts";
import { createOverlayStore } from "./overlay-store.ts";
import * as sel from "./selection.ts";
import { selectionStoreOf } from "./store.ts";
import { indexNodes } from "./tree.ts";
import { type MenuTarget, type PointerInput, type PointerTool, pointerHandlers } from "./use-canvas-pointer.ts";

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
const other: Node = { id: "q0", type: "Column", children: [{ id: "q1", type: "Text" }] };

/** World boxes: two screens side by side, `s` holding `tree`, `s2` holding `other`. */
const boxes: Record<string, Box> = {
  "screen:s": { l: -10, t: -10, r: 600, b: 900 },
  "screen:s2": { l: 1000, t: 0, r: 1390, b: 844 },
  "s/st": { l: 0, t: 0, r: 100, b: 100 },
  "s/t#paint0": { l: 10, t: 10, r: 50, b: 50 },
  "s/c": { l: 0, t: 200, r: 100, b: 300 },
  "s/a": { l: 0, t: 200, r: 100, b: 240 },
  "s/b": { l: 0, t: 250, r: 100, b: 290 },
  "s/r": { l: 200, t: 200, r: 300, b: 300 },
  "s/x": { l: 200, t: 200, r: 240, b: 240 },
  "s/z": { l: 400, t: 0, r: 440, b: 40 },
  "s2/q1": { l: 1100, t: 100, r: 1200, b: 140 },
  "title:s2": { l: 1004, t: -30, r: 1386, b: -10 },
};
const area = (b: Box) => (b.r - b.l) * (b.b - b.t);
const inside = (b: Box, x: number, y: number) => x >= b.l && x <= b.r && y >= b.t && y <= b.b;

interface Rig {
  readonly h: ReturnType<typeof pointerHandlers>;
  readonly ids: () => readonly string[];
  readonly draft: Node & { children: Node[] };
  readonly edits: { doc: string; group: string | undefined }[];
  readonly guides: (Guides | null)[];
  readonly previews: ReadonlyMap<string, Point>[];
  readonly commits: ReadonlyMap<string, Point>[];
  readonly picks: (readonly string[])[];
  readonly menus: MenuTarget[];
  readonly renames: string[];
  readonly pans: [number, number][];
  readonly overlay: ReturnType<typeof createOverlayStore>;
  readonly select: (id: string) => void;
}

function rig(options: { tool?: PointerTool; canEdit?: boolean; locked?: string[] } = {}): Rig {
  const index = indexNodes([tree, other]);
  const store = selectionStoreOf(createStore<{ selection: sel.Selection }>(() => ({ selection: sel.emptySelection })));
  const draft = JSON.parse(JSON.stringify(tree)) as Node & { children: Node[] };
  const edits: Rig["edits"] = [];
  const guides: Rig["guides"] = [];
  const previews: Rig["previews"] = [];
  const commits: Rig["commits"] = [];
  const picks: Rig["picks"] = [];
  const menus: MenuTarget[] = [];
  const renames: string[] = [];
  const pans: Rig["pans"] = [];
  const overlay = createOverlayStore();
  overlay.subscribe(() => guides.push(overlay.get().guides));
  const edit: EditFn = (doc, recipe, o) => {
    edits.push({ doc, group: o?.group });
    recipe(draft);
  };
  const screenOf = new Map<string, string>([
    ...[...indexNodes([tree]).parent.keys()].map((id) => [id, "s"] as const),
    ...[...indexNodes([other]).parent.keys()].map((id) => [id, "s2"] as const),
  ]);
  const surface = {
    viewport: { x: 0, y: 0, zoom: 1 },
    hitTest: (x: number, y: number) =>
      Object.entries(boxes)
        .filter(([, b]) => inside(b, x, y))
        .sort(([, a], [, b]) => area(a) - area(b))
        .map(([id]) => id),
    hitTestRect: (r: Box) =>
      Object.entries(boxes)
        .filter(([, b]) => b.l <= r.r && r.l <= b.r && b.t <= r.b && r.t <= b.b)
        .map(([id]) => id),
    pan: (dx: number, dy: number) => pans.push([dx, dy]),
  } as unknown as CanvasSurface;
  const attributes = new Map<string, string>();
  const canvas = {
    getBoundingClientRect: () => ({ left: 0, top: 0 }),
    setAttribute: (k: string, v: string) => attributes.set(k, v),
    removeAttribute: (k: string) => attributes.delete(k),
    style: { setProperty: () => undefined },
  } as unknown as HTMLCanvasElement;
  const roots: Record<string, string> = { s: "r0", s2: "q0" };
  const boxOf = (id: string): Box | undefined => {
    const screen = Object.entries(roots).find(([, r]) => r === id)?.[0];
    return screen !== undefined
      ? boxes[`screen:${screen}`]
      : (boxes[`${screenOf.get(id)}/${id}`] ?? boxes[`s/${id}#paint0`]);
  };
  const places = new Map<string, Point>([
    ["s", { x: -10, y: -10 }],
    ["s2", { x: 1000, y: 0 }],
  ]);
  const input: PointerInput = {
    surface: () => surface,
    canvas: () => canvas,
    store,
    index,
    edit,
    screenOf,
    rootOf: (s) => roots[s],
    boxOf,
    locked: new Set(options.locked ?? []),
    canEdit: options.canEdit ?? true,
    resizable: (id) => id === "z",
    overlay,
    tool: options.tool ?? "select",
    spaceHeld: () => false,
    screens: { all: () => places, preview: (m) => previews.push(m), commit: (m) => commits.push(m) },
    onPick: (hits) => {
      picks.push(hits);
      return false;
    },
    onView: () => undefined,
    onContextMenu: (_at, target) => menus.push(target),
    onRenameScreen: (s) => renames.push(s),
  };
  const h = pointerHandlers(() => input, { current: null });
  return {
    h,
    ids: () => store.get().ids,
    draft,
    edits,
    guides,
    previews,
    commits,
    picks,
    menus,
    renames,
    pans,
    overlay,
    select: (id) => store.set({ ids: [id], anchor: id, scope: index.parent.get(id) ?? null }),
  };
}

const ev = (
  x: number,
  y: number,
  mods: Partial<Record<"shiftKey" | "altKey" | "ctrlKey" | "metaKey", boolean>> = {},
) => ({
  clientX: x,
  clientY: y,
  button: 0,
  shiftKey: false,
  altKey: false,
  ctrlKey: false,
  metaKey: false,
  ...mods,
});
const drag = (r: Rig, from: [number, number], to: [number, number], mods = {}) => {
  r.h.onPointerDown(ev(...from, mods));
  r.h.onPointerMove(ev((from[0] + to[0]) / 2, (from[1] + to[1]) / 2, mods));
  r.h.onPointerMove(ev(...to, mods));
  r.h.onPointerUp(ev(...to, mods));
};
const click = (r: Rig, x: number, y: number, mods = {}) => {
  r.h.onPointerDown(ev(x, y, mods));
  r.h.onPointerUp(ev(x, y, mods));
};

describe("[STU-067] picking on the canvas", () => {
  test("[STU-067] a click on a widget's own item (`<screen>/<node>#part`) selects the node, not nothing", () => {
    const r = rig();
    click(r, 20, 20);
    expect(r.ids()).toEqual(["st"]);
    click(r, 20, 20, { ctrlKey: true });
    expect(r.ids()).toEqual(["t"]);
  });

  test("[STU-067] Shift adds and removes; a click on the empty canvas clears; a locked layer is not picked", () => {
    // A locked layer and everything inside it (`lockedIds`).
    const r = rig({ locked: ["c", "a", "b"] });
    click(r, 20, 20);
    click(r, 420, 20, { shiftKey: true });
    expect(r.ids()).toEqual(["st", "z"]);
    click(r, 420, 20, { shiftKey: true });
    expect(r.ids()).toEqual(["st"]);
    // `c` is locked: the press reaches its screen.
    click(r, 50, 280);
    expect(r.ids()).toEqual(["r0"]);
    click(r, 800, 500);
    expect(r.ids()).toEqual([]);
    // Only a press with no node under it asks the ghosts and arrows; the empty canvas has nothing to ask.
    expect(r.picks).toEqual([]);
  });

  test("[STU-050] a press on a screen's frame or title selects the screen; a double-click on the title renames it", () => {
    const r = rig();
    click(r, 1300, 700);
    expect(r.ids()).toEqual(["q0"]);
    click(r, 1100, -20);
    expect(r.ids()).toEqual(["q0"]);
    r.h.onDoubleClick(ev(1100, -20));
    expect(r.renames).toEqual(["s2"]);
  });

  test("[STU-067] a double-click drills one level in at the pointer", () => {
    const r = rig();
    click(r, 20, 220);
    expect(r.ids()).toEqual(["c"]);
    r.h.onDoubleClick(ev(20, 220));
    expect(r.ids()).toEqual(["a"]);
  });

  test("[STU-067] hovering marks what a click would select; leaving clears it", () => {
    const r = rig();
    r.h.onPointerMove(ev(20, 220));
    expect(r.overlay.get().hover).toBe("c");
    r.h.onPointerMove(ev(20, 220, { ctrlKey: true }));
    expect(r.overlay.get().hover).toBe("a");
    r.h.onPointerLeave();
    expect(r.overlay.get().hover).toBeNull();
  });

  test("[STU-067] a secondary click selects what it is on and opens the menu for it", () => {
    const r = rig();
    r.h.onContextMenu(ev(20, 220));
    expect(r.ids()).toEqual(["c"]);
    r.h.onContextMenu(ev(1300, 700));
    r.h.onContextMenu(ev(800, 500));
    expect(r.menus).toEqual(["node", "screen", "canvas"]);
  });
});

describe("[STU-067] dragging widgets", () => {
  test("[STU-067] dragging a Stack child shows guides, then moves its Positioned left/top", () => {
    const r = rig();
    r.select("t");
    drag(r, [20, 20], [30, 35]);
    expect(r.guides.some((g) => g !== null)).toBe(true);
    expect(r.overlay.get().guides).toBeNull();
    expect(r.edits).toEqual([{ doc: "s", group: "move" }]);
    expect(r.draft.children[0]?.children?.[0]?.props as unknown).toEqual({ left: 10, top: 15 });
  });

  test("[STU-067] dragging a Column child into a Row reparents it at the insertion indicator", () => {
    const r = rig();
    r.select("a");
    r.h.onPointerDown(ev(50, 220));
    r.h.onPointerMove(ev(250, 250));
    expect(r.overlay.get().guides?.indicator).toBeDefined();
    expect(r.overlay.get().drop).toBe("r");
    expect(r.overlay.get().ghost).not.toBeNull();
    r.h.onPointerUp(ev(250, 250));
    expect(r.edits).toEqual([{ doc: "s", group: "move" }]);
    expect((r.draft.children[2]?.children ?? []).map((n) => n.id)).toEqual(["x", "a"]);
    expect((r.draft.children[1]?.children ?? []).map((n) => n.id)).toEqual(["b"]);
    expect(r.overlay.get().ghost).toBeNull();
  });

  test("[STU-067] dragging within a Column reorders at the insertion indicator", () => {
    const r = rig();
    r.select("b");
    drag(r, [50, 270], [50, 215]);
    expect((r.draft.children[1]?.children ?? []).map((n) => n.id)).toEqual(["b", "a"]);
    expect(r.edits).toEqual([{ doc: "s", group: "move" }]);
  });

  test("[STU-067] a handle resizes only a node with size props; the release point counts", () => {
    const sized = rig();
    sized.select("z");
    sized.h.onPointerDown(ev(440, 40));
    sized.h.onPointerUp(ev(460, 50));
    expect(sized.draft.children[3]?.props as unknown).toEqual({ width: 60, height: 50 });
    const west = rig();
    west.select("z");
    drag(west, [400, 20], [390, 20]);
    expect(west.draft.children[3]?.props as unknown).toEqual({ width: 50, height: 40 });
    const text = rig();
    text.select("t");
    drag(text, [50, 50], [70, 70]);
    expect(text.draft.children[0]?.children?.[0]?.slots?.["child"]?.props).toBeUndefined();
  });

  test("[STU-067] a press without movement is a click: nothing is edited; read-only drags nothing", () => {
    const r = rig();
    r.select("t");
    click(r, 20, 20);
    expect(r.edits).toEqual([]);
    const ro = rig({ canEdit: false });
    ro.select("b");
    drag(ro, [50, 270], [50, 215]);
    expect(ro.edits).toEqual([]);
    expect(ro.ids()).toEqual(["b"]);
  });
});

describe("[STU-050] moving screens", () => {
  test("[STU-050] dragging a screen by its title previews it live, snaps to the others and saves every place", () => {
    const r = rig();
    drag(r, [1100, -20], [900, -27]);
    expect(r.ids()).toEqual(["q0"]);
    expect(r.previews.length).toBeGreaterThan(0);
    // Its top snaps to the other screen's top (-10) within 6 px; x moves freely.
    expect(r.commits).toEqual([
      new Map([
        ["s", { x: -10, y: -10 }],
        ["s2", { x: 800, y: -10 }],
      ]),
    ]);
    expect(r.overlay.get().guides).toBeNull();
    expect(r.edits).toEqual([]);
  });

  test("[STU-050] the body of an unselected screen draws a marquee; of a selected one, moves it", () => {
    const r = rig();
    drag(r, [1300, 700], [1350, 750]);
    expect(r.commits).toEqual([]);
    expect(r.ids()).toEqual([]);
    click(r, 1300, 700);
    drag(r, [1300, 700], [1350, 740]);
    expect(r.commits).toEqual([
      new Map([
        ["s", { x: -10, y: -10 }],
        ["s2", { x: 1050, y: 40 }],
      ]),
    ]);
  });

  test("[STU-050] Shift-selected screens move together; read-only screens stay", () => {
    const r = rig();
    click(r, 500, 800);
    click(r, 1300, 700, { shiftKey: true });
    expect(r.ids()).toEqual(["r0", "q0"]);
    drag(r, [1300, 700], [1300, 1000]);
    expect(r.commits.at(-1)).toEqual(
      new Map([
        ["s", { x: -10, y: 290 }],
        ["s2", { x: 1000, y: 300 }],
      ]),
    );
    const ro = rig({ canEdit: false });
    drag(ro, [1100, -20], [900, -20]);
    expect(ro.commits).toEqual([]);
  });
});

describe("[STU-067] marquee and panning", () => {
  test("[STU-067] a marquee across screens selects the screens; inside one screen, the nodes of its top level", () => {
    const r = rig();
    drag(r, [-100, -100], [1100, 50]);
    expect(r.ids()).toEqual(["r0", "q0"]);
    click(r, 800, 500);
    drag(r, [-5, -5], [150, 320]);
    expect(r.ids()).toEqual(["st", "c"]);
    expect(r.overlay.get().marquee).toBeNull();
  });

  test("[STU-067] the hand tool and the middle button pan; nothing is selected", () => {
    const hand = rig({ tool: "hand" });
    drag(hand, [20, 20], [60, 80]);
    expect(hand.pans.reduce((s, [x, y]) => [s[0] + x, s[1] + y], [0, 0])).toEqual([40, 60]);
    expect(hand.ids()).toEqual([]);
    const middle = rig();
    middle.h.onPointerDown({ ...ev(20, 20), button: 1 });
    middle.h.onPointerMove(ev(25, 20));
    middle.h.onPointerUp(ev(25, 20));
    expect(middle.pans).toEqual([[5, 0]]);
  });
});
