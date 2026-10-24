// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import type { CanvasKit, Surface } from "canvaskit-wasm";
import { box, fontBytes, link, loadCanvasKit, text } from "../test-support/canvaskit.ts";
import type { FrameSpec } from "./protocol.ts";
import { MAX_REGIONS, type RendererOptions, SceneRenderer } from "./renderer.ts";
import { Scene } from "./scene.ts";
import { Viewport } from "./viewport.ts";

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

function surface(width: number, height: number): Surface {
  const made = ck.MakeSurface(width, height);
  if (!made) {
    throw new Error("no raster surface");
  }
  return made;
}

function setup(options: Partial<RendererOptions> = {}, fonts = true) {
  const scene = new Scene();
  const viewport = new Viewport(400, 300);
  const renderer = new SceneRenderer(
    ck,
    scene,
    viewport,
    surface(400, 300),
    1,
    { tiles: false, tileSize: 256, background: 0xf5f5f7ff, fontFamily: "Noto Sans", measureFrames: true, ...options },
    () => performance.now(),
    fonts ? ck.FontMgr.FromData(fontBytes()) : null,
  );
  return { scene, viewport, renderer };
}

const screen: FrameSpec = {
  kind: "frame",
  id: "screen",
  name: "Home",
  x: 20,
  y: 20,
  w: 200,
  h: 200,
  radius: 16,
  fill: 0xffffffff,
  shadow: true,
  z: 0,
};

describe("STU-120 batching (M-12)", () => {
  test("one path per stroke style for links and one per fill or stroke style for boxes", () => {
    const { scene, renderer } = setup();
    const colors = [0x888888ff, 0x3366ccff, 0xcc3333ff];
    scene.apply(
      Array.from({ length: 30 }, (_, i) => ({
        op: "add" as const,
        item: link(`l${i}`, i * 10, 0, i * 10 + 50, 200, colors[i % 3]),
      })),
    );
    expect(renderer.frame().drawCalls).toBe(1 + 3);
    scene.apply(
      Array.from({ length: 20 }, (_, i) => ({
        op: "add" as const,
        item: box(`b${i}`, (i % 5) * 70, Math.floor(i / 5) * 60, i % 2 ? 0x3366ccff : 0xffcc00ff),
      })),
    );
    expect(renderer.frame().drawCalls).toBe(1 + 3 + 2 + 1);
    renderer.dispose();
  });
});

describe("STU-120 level of detail in the renderer (M-12)", () => {
  test("text is drawn as paragraphs at full zoom and as one bar batch when small", () => {
    const { scene, viewport, renderer } = setup();
    scene.apply(Array.from({ length: 5 }, (_, i) => ({ op: "add" as const, item: text(`t${i}`, 10, i * 30) })));
    expect(renderer.frame().drawCalls).toBe(1 + 5);
    viewport.apply({ op: "set", x: 0, y: 0, zoom: 0.4 });
    expect(renderer.frame().drawCalls).toBe(1 + 1);
    renderer.dispose();
  });

  test("frame shadows are pre-rendered nine-patch images, dropped when small", () => {
    const { scene, viewport, renderer } = setup();
    scene.apply([{ op: "add", item: { ...screen, stroke: { color: 0xddddddff, width: 1 } } }]);
    expect(renderer.frame().drawCalls).toBe(1 + 1 + 2);
    viewport.apply({ op: "set", x: 0, y: 0, zoom: 0.2 });
    expect(renderer.frame().drawCalls).toBe(1 + 2);
    renderer.dispose();
  });

  test("without fonts text is skipped", () => {
    const { scene, renderer } = setup({}, false);
    scene.apply([{ op: "add", item: text("t", 0, 0) }]);
    expect(renderer.frame().drawCalls).toBe(1);
    renderer.dispose();
  });
});

describe("STU-120 tile cache, dirty regions and idle rendering (M-12)", () => {
  test("tiles render when idle, are blitted while panning, and changes redraw only dirty regions", () => {
    const { scene, viewport, renderer } = setup({ tiles: true });
    scene.apply([
      { op: "add", item: screen },
      { op: "add", item: box("a", 300, 40) },
      { op: "add", item: link("l", 120, 120, 300, 60) },
      { op: "add", item: text("t", 40, 40) },
    ]);
    const first = renderer.frame();
    expect([first.tilesDrawn, first.regions, first.idleQueue]).toEqual([0, 4, 16]);
    expect(renderer.hasIdleWork()).toBe(true);
    expect(renderer.idle(Number.POSITIVE_INFINITY)).toBe(16);
    expect(renderer.hasIdleWork()).toBe(false);
    const cached = renderer.frame();
    expect([cached.tilesDrawn, cached.regions, cached.drawCalls, cached.idleQueue]).toEqual([4, 0, 4, 0]);
    viewport.pan(-30, -10);
    expect(renderer.frame().regions).toBe(0);
    scene.apply([{ op: "move", id: "a", dx: 5, dy: 5 }]);
    const moved = renderer.frame();
    expect(moved.regions).toBe(1);
    expect(moved.tilesDrawn).toBe(4);
    renderer.idle(Number.POSITIVE_INFINITY);
    expect(renderer.frame().regions).toBe(0);
    renderer.dispose();
  });

  test("dragged items are live: excluded from tiles, drawn each frame; tiles render at half resolution meanwhile", () => {
    const { scene, renderer } = setup({ tiles: true });
    scene.apply([
      { op: "add", item: box("a", 40, 40) },
      { op: "add", item: box("b", 300, 200) },
    ]);
    renderer.frame();
    renderer.idle(Number.POSITIVE_INFINITY);
    renderer.interacting = true;
    scene.setLive(["a"], true);
    const start = renderer.frame();
    expect([start.liveItems, start.regions, start.resolution]).toEqual([1, 1, 0.5]);
    renderer.idle(Number.POSITIVE_INFINITY);
    scene.apply([{ op: "move", id: "a", dx: 3, dy: 0 }]);
    const drag = renderer.frame();
    expect([drag.liveItems, drag.regions]).toEqual([1, 0]);
    renderer.interacting = false;
    scene.setLive(["a"], false);
    expect(renderer.frame().regions).toBe(1);
    expect(renderer.hasIdleWork()).toBe(true);
    renderer.idle(Number.POSITIVE_INFINITY);
    const settled = renderer.frame();
    expect([settled.regions, settled.idleQueue, settled.resolution]).toEqual([0, 0, 1]);
    renderer.dispose();
  });

  test("a zoom change over 25 % shows the stale tiles until the new ones are rendered", () => {
    const { scene, viewport, renderer } = setup({ tiles: true });
    scene.apply([{ op: "add", item: box("a", 40, 40) }]);
    renderer.frame();
    renderer.idle(Number.POSITIVE_INFINITY);
    viewport.zoomAbout(1.2, 0, 0);
    expect(renderer.frame().tilesDrawn).toBe(2);
    viewport.zoomAbout(1.5, 0, 0);
    const zoomed = renderer.frame();
    expect([zoomed.tilesDrawn, zoomed.regions]).toEqual([0, 0]);
    expect(zoomed.staleTilesDrawn).toBeGreaterThan(0);
    renderer.idle(Number.POSITIVE_INFINITY);
    const fresh = renderer.frame();
    expect([fresh.staleTilesDrawn, fresh.tilesDrawn]).toEqual([0, 4]);
    renderer.dispose();
  });

  test("more dirty regions than the limit are merged", () => {
    const { scene, renderer } = setup({ tiles: true, tileSize: 64 });
    scene.apply(Array.from({ length: 8 }, (_, i) => ({ op: "add" as const, item: box(`b${i}`, i * 45, i * 30) })));
    renderer.frame();
    renderer.idle(Number.POSITIVE_INFINITY);
    scene.apply(Array.from({ length: 8 }, (_, i) => ({ op: "move" as const, id: `b${i}`, dx: 1, dy: 1 })));
    expect(renderer.frame().regions).toBe(MAX_REGIONS);
    expect(renderer.tiled).toBe(true);
    expect(renderer.tileCache.imageCount).toBeGreaterThan(0);
    renderer.dispose();
  });
});

describe("STU-121 CanvasKit objects are owned and never leaked", () => {
  test("steady frames allocate no lasting objects; dispose deletes every one", () => {
    const { scene, viewport, renderer } = setup({ tiles: true });
    scene.apply([
      { op: "add", item: screen },
      { op: "add", item: box("a", 300, 40) },
      { op: "add", item: link("l", 0, 0, 300, 60) },
      { op: "add", item: text("t", 40, 40) },
    ]);
    renderer.frame();
    renderer.idle(Number.POSITIVE_INFINITY);
    renderer.frame();
    const live = renderer.resources.live;
    for (let i = 0; i < 20; i++) {
      viewport.pan(1, 0);
      renderer.frame();
    }
    expect(renderer.resources.live).toBe(live);
    scene.apply([{ op: "replace", item: text("t", 40, 40, "Changed") }]);
    renderer.frame();
    expect(renderer.resources.live).toBe(live);
    scene.apply([{ op: "remove", id: "t" }]);
    renderer.frame();
    expect(renderer.resources.live).toBe(live - 1);
    renderer.replaceSurface(surface(200, 200), 2);
    renderer.frame();
    renderer.idle(Number.POSITIVE_INFINITY);
    expect(renderer.resources.created).toBe(renderer.resources.deleted);
    const owned = renderer.resources.list();
    renderer.dispose();
    expect(renderer.resources.live).toBe(0);
    expect(owned.every((object) => object.isDeleted())).toBe(true);
  });
});
