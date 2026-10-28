// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import type { CanvasKit, Surface } from "canvaskit-wasm";
import { box, fontBytes, link, loadCanvasKit, text } from "../test-support/canvaskit.ts";
import type { FrameSpec, ItemSpec } from "./protocol.ts";
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

describe("[STU-120] batching (M-12)", () => {
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

describe("[STU-120] level of detail in the renderer (M-12)", () => {
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

describe("[STU-120] tile cache, dirty regions and idle rendering (M-12)", () => {
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
    // The surface already shows every tile: only the damage (a strip around each moved edge) is redrawn, no tile blitted.
    expect(moved.regions).toBe(4);
    expect(moved.tilesDrawn).toBe(0);
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

describe("[STU-121] CanvasKit objects are owned and never leaked", () => {
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

describe("[STU-121] damage-rect redraw", () => {
  function pixels(s: Surface): Uint8Array {
    const image = s.makeImageSnapshot();
    const read = image.readPixels(0, 0, {
      width: s.width(),
      height: s.height(),
      colorType: ck.ColorType.RGBA_8888,
      alphaType: ck.AlphaType.Unpremul,
      colorSpace: ck.ColorSpace.SRGB,
    });
    image.delete();
    if (!(read instanceof Uint8Array)) {
      throw new Error("no pixels");
    }
    return read;
  }

  function differing(a: Uint8Array, b: Uint8Array): number {
    let count = 0;
    for (let i = 0; i < a.length; i++) {
      if (Math.abs((a[i] ?? 0) - (b[i] ?? 0)) > 8) {
        count++;
      }
    }
    return count;
  }

  function render(tiles: boolean, items: Parameters<Scene["apply"]>[0]) {
    const scene = new Scene();
    const viewport = new Viewport(400, 300);
    const target = surface(400, 300);
    const renderer = new SceneRenderer(
      ck,
      scene,
      viewport,
      target,
      1,
      { tiles, tileSize: 128, background: 0xf5f5f7ff, fontFamily: "Noto Sans", measureFrames: true },
      () => performance.now(),
      ck.FontMgr.FromData(fontBytes()),
    );
    scene.apply(items);
    return { scene, viewport, renderer, target };
  }

  const column = { ...box("col", 40, 40), w: 120, h: 220, fill: 0xffffffff };
  const rows = [text("r0", 50, 50, "Row 0"), text("r1", 50, 80, "Row 1")];

  test("a widened column and an edited row redraw only their damage, present only it, and match a whole redraw", () => {
    const { scene, renderer, target } = render(true, [
      { op: "add", item: column },
      ...rows.map((item) => ({ op: "add" as const, item })),
    ]);
    renderer.frame();
    renderer.idle(Number.POSITIVE_INFINITY);
    expect(renderer.frame().tilesDrawn).toBeGreaterThan(0);
    const edit = [
      { op: "replace" as const, item: { ...column, w: 150 } },
      { op: "replace" as const, item: text("r1", 50, 80, "Row 1 edited") },
    ];
    scene.apply(edit);
    const damaged = renderer.frame();
    // The right-edge strip of the column and the old and new row: nothing blitted, no tile rendered in the frame.
    expect([damaged.tilesDrawn, damaged.regions]).toEqual([0, 3]);
    const whole = render(false, [
      { op: "add", item: { ...column, w: 150 } },
      { op: "add", item: rows[0] ?? column },
      { op: "add", item: text("r1", 50, 80, "Row 1 edited") },
    ]);
    whole.renderer.frame();
    expect(differing(pixels(target), pixels(whole.target))).toBe(0);
    // Idle time refreshes only the dirty rectangles of the cached tiles; the surface already shows them.
    const before = renderer.partialTileRenders;
    expect(renderer.idle(Number.POSITIVE_INFINITY)).toBeGreaterThan(0);
    expect(renderer.partialTileRenders - before).toBeGreaterThan(0);
    expect(renderer.idleChangedView).toBe(false);
    expect(renderer.hasIdleWork()).toBe(false);
    renderer.dispose();
    whole.renderer.dispose();
  });

  test("tiles refreshed in place hold the same pixels as a whole redraw", () => {
    const items = [{ op: "add" as const, item: column }, ...rows.map((item) => ({ op: "add" as const, item }))];
    const { scene, viewport, renderer, target } = render(true, items);
    renderer.frame();
    renderer.idle(Number.POSITIVE_INFINITY);
    renderer.frame();
    scene.apply([{ op: "replace", item: { ...column, w: 90 } }]);
    renderer.frame();
    renderer.idle(Number.POSITIVE_INFINITY);
    expect(renderer.partialTileRenders).toBeGreaterThan(0);
    // A pan away and back draws the whole base from the cached tiles, the refreshed ones included.
    viewport.pan(-7, 0);
    renderer.frame();
    viewport.pan(7, 0);
    const blitted = renderer.frame();
    expect(blitted.tilesDrawn).toBeGreaterThan(0);
    expect(blitted.regions).toBe(0);
    const whole = render(false, [
      { op: "add", item: { ...column, w: 90 } },
      ...rows.map((item) => ({ op: "add" as const, item })),
    ]);
    whole.renderer.frame();
    expect(differing(pixels(target), pixels(whole.target))).toBe(0);
    renderer.dispose();
    whole.renderer.dispose();
  });
});

describe("[STU-045] interaction layer damage (M-12)", () => {
  function read(s: Surface): Uint8Array<ArrayBuffer> {
    const image = s.makeImageSnapshot();
    const out = image.readPixels(0, 0, {
      width: s.width(),
      height: s.height(),
      colorType: ck.ColorType.RGBA_8888,
      alphaType: ck.AlphaType.Premul,
      colorSpace: ck.ColorSpace.SRGB,
    });
    image.delete();
    if (!(out instanceof Uint8Array)) {
      throw new Error("no pixels");
    }
    return Uint8Array.from(out);
  }

  function scene(): Parameters<Scene["apply"]>[0] {
    return [
      { op: "add", item: box("a", 40, 40) },
      { op: "add", item: text("label", 44, 50) },
      { op: "add", item: box("b", 300, 220) },
      { op: "add", item: box("c", 260, 20) },
      { op: "add", item: link("ab", 140, 65, 300, 245) },
      { op: "add", item: link("ac", 140, 50, 260, 45) },
      { op: "add", item: link("bc", 350, 220, 310, 70) },
    ];
  }

  /** The base surface of `renderer`. */
  function base(renderer: SceneRenderer): Uint8Array<ArrayBuffer> {
    return read((renderer as unknown as { surface: Surface }).surface);
  }

  /**
   * Pixels over 32 levels apart between `renderer`'s base and a full
   * re-render of `specs` at the same view: a ghost of a lifted item or a
   * missing edge (both differ by over 100 levels). Skia picks a path's
   * antialiasing by the complexity of the whole batched path, so a tile
   * refreshed in a dirty rectangle may shade an edge pixel a few levels
   * differently.
   */
  function ghosts(
    renderer: SceneRenderer,
    viewport: Viewport,
    specs: readonly ItemSpec[],
    options: Partial<RendererOptions> = {},
  ): number {
    // The reference uses the same tile size: tile seams may shade an edge pixel differently.
    const fresh = setup({ tiles: true, ...options });
    fresh.scene.apply(specs.map((item) => ({ op: "add" as const, item })));
    fresh.viewport.x = viewport.x;
    fresh.viewport.y = viewport.y;
    fresh.renderer.frame();
    fresh.renderer.idle(Number.POSITIVE_INFINITY);
    fresh.renderer.frame();
    const after = base(renderer);
    const expected = base(fresh.renderer);
    expect(after.length).toBe(400 * 300 * 4);
    let count = 0;
    for (let k = 0; k < after.length; k++) {
      if (Math.abs((after[k] ?? 0) - (expected[k] ?? 0)) > 32) {
        count++;
      }
    }
    fresh.renderer.dispose();
    return count;
  }

  test("a drag redraws only the layer's damage, and every frame and the drop equal a whole render", () => {
    const { scene: live, viewport, renderer } = setup({ tiles: true });
    live.apply(scene());
    renderer.frame();
    renderer.idle(Number.POSITIVE_INFINITY);
    // The base shows every tile, as the worker draws it after idle work.
    const renders = renderer.frame().tileRenders;
    const lifted = ["a", "label", "ab", "ac"];
    const staticSpecs = () => live.items().flatMap((item) => (lifted.includes(item.spec.id) ? [] : [item.spec]));
    renderer.interacting = true;
    live.setLive(lifted, true);
    const layer = { surface: surface(400, 300), x: 0, y: 0, scale: 1 };
    // The canvas the layer presents to: a flush copies its dirty rectangle [l, t, r, b], or everything.
    const presented = new Uint8Array(400 * 300 * 4);
    const flush = layer.surface.flush.bind(layer.surface);
    layer.surface.flush = ((dirty?: Int32Array) => {
      flush();
      const pixels = read(layer.surface);
      const [l, t, r, b] = dirty ?? [0, 0, 400, 300];
      for (let y = t ?? 0; y < (b ?? 0); y++) {
        presented.set(pixels.subarray((y * 400 + (l ?? 0)) * 4, (y * 400 + (r ?? 0)) * 4), (y * 400 + (l ?? 0)) * 4);
      }
    }) as Surface["flush"];
    const start = renderer.frame(true, layer);
    expect(start.layerPixels).toBe(400 * 300);
    // The lift re-renders no tile (M-13): idle time masks the lifted items out of the base, in patches.
    expect(start.tileRenders).toBe(renders);
    expect(renderer.hasIdleWork()).toBe(true);
    expect(renderer.idle(Number.POSITIVE_INFINITY)).toBeGreaterThan(0);
    expect(renderer.hasIdleWork()).toBe(false);
    expect(renderer.frame(false, layer).tileRenders).toBe(renders);
    expect(ghosts(renderer, viewport, staticSpecs())).toBe(0);
    for (let i = 1; i <= 12; i++) {
      live.apply([
        { op: "move", id: "a", dx: 7, dy: 3 },
        { op: "move", id: "label", dx: 7, dy: 3 },
        { op: "replace", item: link("ab", 140 + 7 * i, 65 + 3 * i, 300, 245) },
        { op: "replace", item: link("ac", 140 + 7 * i, 50 + 3 * i, 260, 45) },
      ]);
      const stats = renderer.frame(false, layer);
      // Only the moved items' pieces are presented, not the whole layer.
      expect(stats.layerPixels).toBeGreaterThan(0);
      expect(stats.layerPixels).toBeLessThan(400 * 300 * 0.6);
      // A whole draw of the same live items on a fresh layer is identical, pixel for pixel.
      const whole = { surface: surface(400, 300), x: 0, y: 0, scale: 1 };
      const check = setup({ tiles: true });
      check.scene.apply(scene());
      check.scene.setLive(lifted, true);
      check.scene.apply(
        live
          .items()
          .filter((item) => lifted.includes(item.spec.id))
          .map((item) => ({ op: "replace" as const, item: item.spec })),
      );
      check.renderer.frame(false, whole);
      const want = read(whole.surface);
      expect(read(layer.surface)).toEqual(want);
      // What the layer's canvas shows — the whole start, then each frame's presented regions — equals it too.
      expect(presented).toEqual(want);
      check.renderer.dispose();
      whole.surface.delete();
    }
    // A whole base draw mid-drag (a pan) blits the tiles, which still hold the lifted items, and masks them again.
    viewport.pan(-30, -20);
    const panned = renderer.frame(true, layer);
    expect(panned.tilesDrawn).toBeGreaterThan(0);
    renderer.idle(Number.POSITIVE_INFINITY);
    expect(ghosts(renderer, viewport, staticSpecs())).toBe(0);
    viewport.pan(30, 20);
    renderer.frame(true, layer);
    renderer.idle(Number.POSITIVE_INFINITY);
    expect(ghosts(renderer, viewport, staticSpecs())).toBe(0);
    renderer.interacting = false;
    live.setLive(lifted, false);
    const dropped = renderer.frame(true, null);
    // The drop's frame renders no tile; it draws the dropped items at once and leaves patches and tiles to idle time.
    expect(dropped.tileRenders).toBe(start.tileRenders);
    expect(dropped.regions).toBe(lifted.length);
    expect(renderer.hasIdleWork()).toBe(true);
    const all = () => live.items().map((item) => item.spec);
    renderer.idle(Number.POSITIVE_INFINITY);
    renderer.frame(true, null);
    // After the drop settles the base equals a full re-render of the same scene.
    expect(renderer.hasIdleWork()).toBe(false);
    expect(ghosts(renderer, viewport, all())).toBe(0);
    // And so does a whole draw from the refreshed tiles alone.
    viewport.pan(-7, 0);
    renderer.frame(true, null);
    viewport.pan(7, 0);
    renderer.idle(Number.POSITIVE_INFINITY);
    renderer.frame(true, null);
    expect(ghosts(renderer, viewport, all())).toBe(0);
    renderer.dispose();
    layer.surface.delete();
  });

  test("M-13 lift patches are paced per frame and a drop refreshes only tile pieces, slice by slice, to a whole render", () => {
    // Small tiles make the frame's patch budget (one tile's pixels) smaller than the lift's patches.
    const small = { tileSize: 64 };
    const { scene: live, viewport, renderer } = setup({ tiles: true, ...small });
    live.apply(scene());
    renderer.frame();
    renderer.idle(Number.POSITIVE_INFINITY);
    renderer.frame();
    const lifted = ["a", "label", "ab", "ac"];
    const staticSpecs = () => live.items().flatMap((item) => (lifted.includes(item.spec.id) ? [] : [item.spec]));
    renderer.interacting = true;
    live.setLive(lifted, true);
    const layer = { surface: surface(400, 300), x: 0, y: 0, scale: 1 };
    renderer.frame(true, layer);
    let frames = 0;
    while (renderer.hasIdleWork() && frames < 1000) {
      renderer.idle(Number.POSITIVE_INFINITY);
      if (renderer.patchesWaitForFrame) {
        // The worker waits for the next animation frame here.
        expect(renderer.idle(Number.POSITIVE_INFINITY)).toBe(0);
        renderer.nextPatchFrame();
        frames++;
      }
    }
    expect(frames).toBeGreaterThan(0);
    expect(ghosts(renderer, viewport, staticSpecs(), small)).toBe(0);
    live.apply([
      { op: "move", id: "a", dx: 84, dy: 36 },
      { op: "move", id: "label", dx: 84, dy: 36 },
      { op: "replace", item: link("ab", 224, 101, 300, 245) },
      { op: "replace", item: link("ac", 224, 86, 260, 45) },
    ]);
    renderer.frame(false, layer);
    renderer.interacting = false;
    live.setLive(lifted, false);
    renderer.frame(true, null);
    // Idle slices whose deadline has passed: each draws one chunk of patches or of a tile's pieces.
    const refreshed = renderer.partialTileRenders;
    let slices = 0;
    while (renderer.hasIdleWork() && slices < 10_000) {
      renderer.idle(0);
      slices++;
      if (renderer.idleChangedView) {
        renderer.frame(true, null);
      } else {
        renderer.nextPatchFrame();
      }
    }
    expect(renderer.hasIdleWork()).toBe(false);
    expect(slices).toBeGreaterThan(1);
    expect(renderer.partialTileRenders).toBeGreaterThan(refreshed);
    // Pieces the settled base shows are copied from it rather than redrawn.
    expect(renderer.copiedTileRefreshes).toBeGreaterThan(0);
    renderer.frame(true, null);
    const all = () => live.items().map((item) => item.spec);
    expect(ghosts(renderer, viewport, all(), small)).toBe(0);
    // And so does a whole draw from the refreshed tiles alone.
    viewport.pan(-7, 0);
    renderer.frame(true, null);
    viewport.pan(7, 0);
    renderer.idle(Number.POSITIVE_INFINITY);
    renderer.frame(true, null);
    expect(ghosts(renderer, viewport, all(), small)).toBe(0);
    renderer.dispose();
    layer.surface.delete();
  });
});
