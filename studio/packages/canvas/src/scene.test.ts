// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { box, link, text } from "../test-support/canvaskit.ts";
import { makeRect, type Rect } from "./geometry.ts";
import type { FrameSpec } from "./protocol.ts";
import { boundsOf, damageOf, moveSpec, Scene, type SceneItem, SHADOW_MARGIN } from "./scene.ts";
import { buildSceneModel } from "./scene-model.ts";
import { Viewport } from "./viewport.ts";

function drain(scene: Scene): Rect[] {
  const out: Rect[] = [];
  scene.drainDirty((rect) => out.push({ ...rect }));
  return out;
}

/** The rectangles lifted from and dropped at since the last drain, and the dirty ones, which lifts never add to. */
function lifts(scene: Scene): { lifted: Rect[]; dropped: Rect[]; dirty: Rect[] } {
  const lifted: Rect[] = [];
  const dropped: Rect[] = [];
  scene.drainLifts(
    (rect) => lifted.push({ ...rect }),
    (rect) => dropped.push({ ...rect }),
  );
  return { lifted, dropped, dirty: drain(scene) };
}

const screen: FrameSpec = {
  kind: "frame",
  id: "screen",
  name: "Home",
  x: 0,
  y: 0,
  w: 360,
  h: 780,
  radius: 24,
  fill: 0xffffffff,
  shadow: true,
  z: 0,
};

describe("[STU-003] retained scene graph", () => {
  test("bounds include stroke, shadow and control points", () => {
    expect(boundsOf(box("a", 10, 20), makeRect(0, 0, 0, 0))).toEqual({ l: 9.5, t: 19.5, r: 110.5, b: 70.5 });
    expect(boundsOf(screen, makeRect(0, 0, 0, 0)).l).toBe(-SHADOW_MARGIN);
    const curve = boundsOf(link("l", 0, 0, 100, 100), makeRect(0, 0, 0, 0));
    expect(curve).toEqual({ l: -2, t: -2, r: 102, b: 102 });
    expect(boundsOf(text("t", 0, 0), makeRect(0, 0, 0, 0)).b).toBeCloseTo(19.6, 9);
  });

  test("items are kept in z-order, then insertion order", () => {
    const scene = new Scene();
    scene.apply([
      { op: "add", item: box("top", 0, 0, 1, 5) },
      { op: "add", item: box("low", 0, 0, 1, 1) },
    ]);
    scene.apply([{ op: "add", item: box("mid", 0, 0, 1, 1) }]);
    expect(scene.items().map((item) => item.spec.id)).toEqual(["low", "mid", "top"]);
    scene.apply([{ op: "replace", item: box("low", 0, 0, 1, 9) }]);
    expect(scene.items().map((item) => item.spec.id)).toEqual(["mid", "top", "low"]);
    expect(scene.version).toBe(3);
  });

  test("every change records the old and new bounds as dirty", () => {
    const scene = new Scene();
    scene.apply([{ op: "add", item: box("a", 0, 0) }]);
    // The bounds and the antialiasing allowance around them.
    expect(drain(scene)).toEqual([{ l: -1.5, t: -1.5, r: 101.5, b: 51.5 }]);
    scene.apply([{ op: "move", id: "a", dx: 200, dy: 0 }]);
    expect(drain(scene)).toEqual([
      { l: -0.5, t: -0.5, r: 100.5, b: 50.5 },
      { l: 199.5, t: -0.5, r: 300.5, b: 50.5 },
    ]);
    scene.apply([
      { op: "move", id: "missing", dx: 1, dy: 1 },
      { op: "remove", id: "missing" },
    ]);
    expect(drain(scene)).toEqual([]);
    scene.apply([{ op: "remove", id: "a" }]);
    expect(drain(scene)).toHaveLength(1);
    const removed: string[] = [];
    scene.drainRemoved((id) => removed.push(id));
    expect(removed).toEqual(["a"]);
    expect(scene.size).toBe(0);
  });

  test("live items are excluded from dirty tracking and static queries", () => {
    const scene = new Scene();
    scene.apply([
      { op: "add", item: box("a", 0, 0) },
      { op: "add", item: box("b", 500, 0) },
    ]);
    drain(scene);
    scene.setLive(["a", "missing"], true);
    expect(scene.hasDirty).toBe(true);
    const lifted = lifts(scene);
    expect([lifted.lifted.length, lifted.dropped.length, lifted.dirty.length]).toEqual([1, 0, 0]);
    expect(scene.hasLive).toBe(true);
    scene.apply([{ op: "move", id: "a", dx: 10, dy: 10 }]);
    expect(drain(scene)).toEqual([]);
    const out: SceneItem[] = [];
    expect(scene.query(makeRect(0, 0, 1000, 100), out, false).map((i) => i.spec.id)).toEqual(["b"]);
    expect(scene.query(makeRect(0, 0, 1000, 100), out, true)).toHaveLength(2);
    expect(scene.liveItems(out).map((i) => i.spec.id)).toEqual(["a"]);
    // An item added under a live id (a re-routed arrow's new piece) is live at once.
    scene.apply([{ op: "add", item: box("missing", 0, 0) }]);
    expect(drain(scene)).toEqual([]);
    expect(scene.liveItems(out).map((i) => i.spec.id)).toEqual(["a", "missing"]);
    scene.setLive(["missing"], false);
    lifts(scene);
    scene.setLive(["a"], false);
    expect(lifts(scene)).toEqual({ lifted: [], dropped: [{ l: 8.5, t: 8.5, r: 111.5, b: 61.5 }], dirty: [] });
    expect(scene.hasLive).toBe(false);
    scene.setLive(["a"], false);
    expect(lifts(scene)).toEqual({ lifted: [], dropped: [], dirty: [] });
  });

  test("moving a link moves its control points; clear removes everything", () => {
    const moved = moveSpec(link("l", 0, 0, 100, 0), 5, 5);
    expect(moved.kind === "link" && [moved.from, moved.c1, moved.c2, moved.to]).toEqual([
      { x: 5, y: 5 },
      { x: 45, y: 5 },
      { x: 65, y: 5 },
      { x: 105, y: 5 },
    ]);
    const straight = moveSpec({ ...link("s", 0, 0, 1, 1), c1: undefined, c2: undefined } as never, 1, 1);
    expect(straight.kind === "link" && straight.c1).toBeFalsy();
    const scene = new Scene();
    scene.apply([{ op: "add", item: link("l", 0, 0, 1, 1) }, { op: "add", item: text("t", 0, 0) }, { op: "clear" }]);
    expect(scene.size).toBe(0);
  });

  test("the scene model lists nodes, frames, bounds, z-order and the view", () => {
    const scene = new Scene();
    scene.apply([
      { op: "add", item: screen },
      { op: "add", item: { ...box("card", 20, 40), frame: "screen", label: "Card" } },
      { op: "add", item: { ...text("title", 20, 10, "Welcome"), frame: "screen" } },
      { op: "add", item: { ...box("plain", 500, 0), fill: undefined, stroke: undefined } as never },
      { op: "add", item: link("l", 0, 0, 10, 10) },
    ]);
    const viewport = new Viewport(800, 600);
    viewport.zoomAbout(2, 0, 0);
    const model = structuredClone(buildSceneModel(scene, viewport));
    expect(model.frames).toEqual(["screen"]);
    expect(model.viewport).toEqual({ x: 0, y: 0, zoom: 2, width: 800, height: 600 });
    expect(model.nodes.map((n) => [n.id, n.kind, n.frame, n.label, n.z])).toEqual([
      ["screen", "frame", null, "Home", 0],
      ["l", "link", null, "", 0],
      ["card", "rect", "screen", "Card", 1],
      ["plain", "rect", null, "", 1],
      ["title", "text", "screen", "Welcome", 2],
    ]);
    expect(model.nodes[2]?.bounds).toEqual({ x: 19.5, y: 39.5, w: 101, h: 51 });
  });

  test("[STU-121] a resized box damages only strips around its moved edges; other changes their old and new bounds", () => {
    const column = { ...box("c", 0, 0), w: 100, h: 400 };
    const out: Rect[] = [];
    const bounds = (spec: typeof column): Rect => boundsOf(spec, { l: 0, t: 0, r: 0, b: 0 });
    damageOf(column, bounds(column), column, bounds(column), out);
    expect(out).toEqual([]);
    const wider = { ...column, w: 130 };
    damageOf(column, bounds(column), wider, bounds(wider), out);
    // pad = stroke / 2 + radius + 1 = 7.5 around the right edge, over the full height.
    expect(out).toEqual([{ l: 92.5, t: -8, r: 137.5, b: 408 }]);
    out.length = 0;
    const recoloured = { ...column, fill: 0x000000ff };
    damageOf(column, bounds(column), recoloured, bounds(recoloured), out);
    expect(out).toEqual([bounds(column), bounds(column)]);
    out.length = 0;
    const scene = new Scene();
    scene.apply([{ op: "add", item: column }]);
    drain(scene);
    scene.apply([{ op: "replace", item: wider }]);
    expect(drain(scene)).toEqual([{ l: 92.5, t: -8, r: 137.5, b: 408 }]);
  });
});
