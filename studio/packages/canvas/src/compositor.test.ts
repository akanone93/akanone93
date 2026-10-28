// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import type { CanvasKit } from "canvaskit-wasm";
import { box, fontBytes, loadCanvasKit, TestEnvironment } from "../test-support/canvaskit.ts";
import {
  centredOrigin,
  elementTransform,
  isCentred,
  LAYER_STEP,
  layerBox,
  needsRender,
  overscanMargin,
  transformCss,
} from "./compositor.ts";
import { type CanvasElementLike, CanvasHost, type StyleLike } from "./host.ts";
import type { CanvasOptions, FrameStats } from "./protocol.ts";

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

const TILE = 256;
const MARGIN = overscanMargin(TILE, 1);

function options(): CanvasOptions {
  return {
    renderer: "raster",
    tileSize: TILE,
    background: 0xffffffff,
    fonts: [fontBytes()],
    fontFamily: "Noto Sans",
    measureFrames: false,
  };
}

function element(): CanvasElementLike & { style: StyleLike } {
  const style: StyleLike = {
    position: "",
    left: "",
    top: "",
    width: "",
    height: "",
    transform: "",
    transformOrigin: "",
    willChange: "",
    pointerEvents: "",
    display: "",
  };
  return { width: 0, height: 0, style };
}

async function start(env: TestEnvironment, layer: CanvasElementLike | null = null) {
  const base = element();
  const host = new CanvasHost(
    base,
    { width: 320, height: 200, dpr: 1 },
    options(),
    { mainThread: () => env.deps() },
    layer,
  );
  await host.ready;
  const frames: FrameStats[] = [];
  host.onFrame((stats) => frames.push(stats));
  host.patch([{ op: "add", item: box("a", 300, 40) }]);
  env.drain(200);
  frames.length = 0;
  return { host, base, frames };
}

describe("[STU-120] [STU-045] compositor arithmetic (M-12)", () => {
  test("a centred base sits one margin above and left of the view, snapped to device pixels", () => {
    const view = { x: 10.3, y: -4, zoom: 0.6, width: 1280, height: 800 };
    const origin = centredOrigin(view, MARGIN, 2);
    expect(Number.isInteger(origin.x * 0.6 * 2)).toBe(true);
    const transform = elementTransform(view, origin);
    expect(transform.tx).toBeCloseTo(-MARGIN, 0);
    expect(transform.scale).toBe(1);
    expect(isCentred(view, origin, MARGIN, 2)).toBe(true);
    expect(isCentred({ ...view, x: 50 }, origin, MARGIN, 2)).toBe(false);
  });

  test("the transform is CSS translate3d, with scale only while the zoom differs", () => {
    expect(transformCss({ tx: -256, ty: -10, scale: 1 })).toBe("translate3d(-256px, -10px, 0)");
    expect(transformCss({ tx: -10, ty: 0, scale: 2 }, 5, 1)).toBe("translate3d(0px, 2px, 0) scale(2)");
  });

  test("the overscan is re-rendered when the view comes within a quarter margin of its edge", () => {
    const view = { x: 0, y: 0, zoom: 1, width: 320, height: 200 };
    const origin = centredOrigin(view, MARGIN, 1);
    expect(needsRender(view, origin, MARGIN, false)).toBe(false);
    expect(needsRender({ ...view, x: 0.7 * MARGIN }, origin, MARGIN, false)).toBe(false);
    expect(needsRender({ ...view, x: 0.8 * MARGIN }, origin, MARGIN, false)).toBe(true);
    expect(needsRender({ ...view, y: -0.8 * MARGIN }, origin, MARGIN, false)).toBe(true);
    expect(needsRender({ ...view, zoom: 1.5 }, origin, MARGIN, false)).toBe(true);
    expect(needsRender({ ...view, zoom: 1.5 }, origin, MARGIN, true)).toBe(false);
    expect(needsRender({ ...view, zoom: 0.5 }, origin, MARGIN, true)).toBe(true);
  });

  test("the interaction layer's box follows the dragged bounds in steps", () => {
    const first = layerBox({ x: 300, y: 40, w: 100, h: 50 }, null);
    expect(first).toEqual({ x: 128, y: -128, w: 512, h: 384 });
    expect(layerBox({ x: 340, y: 60, w: 100, h: 50 }, first)).toBe(first);
    const moved = layerBox({ x: 600, y: 60, w: 100, h: 50 }, first);
    expect(moved).not.toBe(first);
    expect(moved.x % LAYER_STEP).toBe(0);
    expect(moved.x + moved.w).toBeGreaterThanOrEqual(700);
  });
});

describe("[STU-120] [STU-045] compositor-assisted pan and interaction layer (M-12)", () => {
  test("the host moves the base with a CSS transform and the worker re-renders only at the margin", async () => {
    const env = new TestEnvironment(ck);
    const { host, base, frames } = await start(env);
    expect(base.style.width).toBe(`${320 + 2 * MARGIN}px`);
    expect(base.style.transform).toBe(`translate3d(${-MARGIN}px, ${-MARGIN}px, 0)`);
    host.startInteraction("pan");
    host.pan(-150, -10);
    expect(base.style.transform).toBe(`translate3d(${-MARGIN - 150}px, ${-MARGIN - 10}px, 0)`);
    env.drain(200);
    expect(frames.some((f) => f.base)).toBe(false);
    host.pan(-60, 0);
    env.drain(200);
    // One re-render at the margin, run as progressive base jobs that end with an empty queue.
    const rebased = frames.filter((f) => f.base);
    expect(rebased.length).toBeGreaterThan(0);
    expect(rebased.at(-1)?.baseJobs).toBe(0);
    env.drain(200);
    expect(frames.filter((f) => f.base)).toHaveLength(rebased.length);
    expect(host.transform.tx).toBeCloseTo(-MARGIN, 6);
    host.endInteraction("pan");
    env.drain(200);
    expect(host.transform).toEqual({ tx: -MARGIN, ty: -MARGIN, scale: 1 });
    host.dispose();
  });

  test("zoom gestures scale the element and re-render crisply on settle", async () => {
    const env = new TestEnvironment(ck);
    const { host, base, frames } = await start(env);
    host.startInteraction("zoom");
    host.zoomAbout(2, 0, 0);
    env.drain(200);
    expect(frames.some((f) => f.base)).toBe(false);
    expect(host.transform.scale).toBe(2);
    expect(base.style.transform).toContain("scale(2)");
    host.endInteraction("zoom");
    env.drain(200);
    expect(frames.some((f) => f.base)).toBe(true);
    expect(host.transform.scale).toBe(1);
    expect(host.baseOrigin.zoom).toBe(2);
    host.dispose();
  });

  test("the scene-model viewport equals what the transform shows", async () => {
    const env = new TestEnvironment(ck);
    const { host } = await start(env);
    host.startInteraction("pan");
    host.pan(-37, 12);
    host.zoomAbout(1.25, 100, 50);
    host.pan(-90, 4);
    env.drain(200);
    const { viewport } = await host.snapshot();
    expect(viewport).toMatchObject({ x: host.viewport.x, y: host.viewport.y, zoom: host.viewport.zoom });
    const origin = host.baseOrigin;
    const { tx, ty, scale } = host.transform;
    for (const [wx, wy] of [
      [0, 0],
      [300, 40],
      [-120, 410],
    ] as const) {
      expect(tx + (wx - origin.x) * origin.zoom * scale).toBeCloseTo((wx - viewport.x) * viewport.zoom, 6);
      expect(ty + (wy - origin.y) * origin.zoom * scale).toBeCloseTo((wy - viewport.y) * viewport.zoom, 6);
    }
    host.dispose();
  });

  test("drags draw into the interaction layer, whose box follows the dragged item", async () => {
    const env = new TestEnvironment(ck);
    const layer = element();
    const { host, frames } = await start(env, layer);
    expect(layer.style.pointerEvents).toBe("none");
    host.startInteraction("drag", ["a"]);
    host.patch([{ op: "move", id: "a", dx: 4, dy: 2 }]);
    env.drain(200);
    const first = host.layerBox;
    const origin = host.baseOrigin;
    const itemX = (304 - origin.x) * origin.zoom;
    expect(first).not.toBeNull();
    expect(first?.x).toBeLessThanOrEqual(itemX);
    expect((first?.x ?? 0) + (first?.w ?? 0)).toBeGreaterThanOrEqual(itemX + 100);
    expect(layer.style.display).toBe("block");
    frames.length = 0;
    host.patch([{ op: "move", id: "a", dx: 2, dy: 0 }]);
    env.drain(200);
    expect(frames.at(-1)).toMatchObject({ base: false, liveItems: 1 });
    expect(host.layerBox).toBe(first);
    host.patch([{ op: "move", id: "a", dx: -200, dy: 100 }]);
    env.drain(200);
    expect(host.layerBox?.x).toBeLessThan(first?.x ?? 0);
    host.endInteraction("drag", ["a"]);
    env.drain(200);
    expect(host.layerBox).toBeNull();
    expect(layer.style.display).toBe("none");
    host.dispose();
  });

  test("drag end re-renders only the tiles the dragged item touched", async () => {
    const env = new TestEnvironment(ck);
    const { host, frames } = await start(env, element());
    host.patch([{ op: "add", item: box("far", 40, 300) }]);
    env.drain(200);
    host.startInteraction("drag", ["a"]);
    host.patch([{ op: "move", id: "a", dx: 20, dy: 10 }]);
    env.drain(200);
    const before = frames.at(-1)?.tileRenders ?? 0;
    host.endInteraction("drag", ["a"]);
    env.drain(200);
    // The box stays inside tile (1, 0): that tile alone is refined and redrawn.
    expect((frames.at(-1)?.tileRenders ?? 0) - before).toBe(1);
    host.dispose();
  });
});
