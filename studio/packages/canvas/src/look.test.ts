// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import type { CanvasKit, Surface } from "canvaskit-wasm";
import { box, fontBytes, loadCanvasKit } from "../test-support/canvaskit.ts";
import { makeRect } from "./geometry.ts";
import { gridLevels, gridPoints, gridStep, MAX_GRID_DOTS } from "./grid.ts";
import type { GridStyle, RectSpec } from "./protocol.ts";
import { SceneRenderer } from "./renderer.ts";
import { boundsOf, Scene, SOFT_SHADOW_MARGIN } from "./scene.ts";
import { Viewport } from "./viewport.ts";

const grid: GridStyle = { spacing: 20, radius: 1, minGap: 12, color: 0x000000ff };

describe("[STU-032] the dot grid boxes align on", () => {
  test("the step doubles until dots are minGap CSS pixels apart, and never shifts a dot", () => {
    expect(gridStep(grid, 1)).toBe(20);
    expect(gridStep(grid, 0.6)).toBe(20);
    expect(gridStep(grid, 0.5)).toBe(40);
    expect(gridStep(grid, 0.1)).toBe(160);
    expect(gridStep(grid, 4)).toBe(20);
    for (const zoom of [0.1, 0.25, 0.5, 1, 2]) {
      expect(gridStep(grid, zoom) % grid.spacing).toBe(0);
    }
  });

  test("dots sit on multiples of the step inside the rectangle, including its edges", () => {
    const points = gridPoints(makeRect(-5, 0, 41, 20), 20);
    expect(Array.from(points ?? [])).toEqual([0, 0, 20, 0, 40, 0, 0, 20, 20, 20, 40, 20]);
    expect(gridPoints(makeRect(1, 1, 19, 19), 20)?.length).toBe(0);
    // The finer level leaves out the coarser level's dots, which are drawn on their own.
    expect(Array.from(gridPoints(makeRect(0, 0, 40, 0), 20, 40) ?? [])).toEqual([20, 0]);
  });

  test("a rectangle with too many dots draws none rather than stall a frame", () => {
    const side = Math.ceil(Math.sqrt(MAX_GRID_DOTS)) * 20 + 40;
    expect(gridPoints(makeRect(0, 0, side, side), 20)).toBeNull();
  });

  test("zooming thins the grid gradually: the finer level fades in and out, never pops", () => {
    // Just above a switch the finer dots are invisible; just below it they are fully drawn as the step.
    const above = gridLevels(grid, 12 / 40 + 1e-6);
    expect(above[0]?.step).toBe(40);
    expect(above[1]?.alpha ?? 0).toBeLessThan(0.01);
    const below = gridLevels(grid, 12 / 40 - 1e-6);
    expect(below[0]?.step).toBe(80);
    expect(below[1]).toMatchObject({ step: 40 });
    expect(below[1]?.alpha ?? 0).toBeGreaterThan(0.99);
    // Across a sweep of zooms, the opacity of every dot changes by at most a little between neighbouring zooms.
    const opacity = (zoom: number, x: number) => {
      let a = 0;
      for (const level of gridLevels(grid, zoom)) {
        if (x % level.step === 0) {
          a = Math.max(a, level.alpha);
        }
      }
      return a;
    };
    for (let zoom = 0.1; zoom < 2; zoom *= 1.01) {
      for (const x of [20, 40, 80, 160]) {
        expect(Math.abs(opacity(zoom, x) - opacity(zoom * 1.01, x))).toBeLessThan(0.1);
      }
    }
  });
});

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

function render(options: { grid?: GridStyle }, zoom: number, fill: (scene: Scene) => void): Uint8Array {
  const surface = ck.MakeSurface(100, 100) as Surface;
  const scene = new Scene();
  const viewport = new Viewport(100, 100);
  viewport.apply({ op: "set", x: 0, y: 0, zoom });
  const renderer = new SceneRenderer(
    ck,
    scene,
    viewport,
    surface,
    1,
    { tiles: false, tileSize: 256, background: 0xffffffff, fontFamily: "Noto Sans", measureFrames: true, ...options },
    () => performance.now(),
    ck.FontMgr.FromData(fontBytes(), boldBytes()),
  );
  fill(scene);
  renderer.frame();
  const image = surface.makeImageSnapshot();
  const pixels = image.readPixels(0, 0, {
    width: 100,
    height: 100,
    colorType: ck.ColorType.RGBA_8888,
    alphaType: ck.AlphaType.Unpremul,
    colorSpace: ck.ColorSpace.SRGB,
  }) as Uint8Array;
  image.delete();
  renderer.dispose();
  return pixels;
}

/** Noto Sans Bold beside the regular face of `fontBytes`, so bold text has its own face. */
function boldBytes(): ArrayBuffer {
  const bytes = readFileSync(join(import.meta.dir, "../../../../schema/goldens/fonts/NotoSans_700Bold.ttf"));
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

const red = (pixels: Uint8Array, x: number, y: number): number => pixels[(y * 100 + x) * 4] ?? 0;

describe("[STU-032] the renderer draws the grid under the scene", () => {
  test("dots on grid points, background between them, none without a grid", () => {
    const dotted = render({ grid }, 1, () => undefined);
    expect(red(dotted, 40, 40)).toBeLessThan(128);
    expect(red(dotted, 50, 50)).toBe(255);
    const plain = render({}, 1, () => undefined);
    expect(red(plain, 40, 40)).toBe(255);
  });

  test("a box covers the dots under it", () => {
    const covered = render({ grid }, 1, (scene) =>
      scene.apply([{ op: "add", item: { ...box("b", 30, 30, 0xffffffff), w: 30, h: 30 } }]),
    );
    expect(red(covered, 40, 40)).toBe(255);
    expect(red(covered, 80, 80)).toBeLessThan(128);
  });
});

describe("[STU-030] the soft shadow of a plugin box", () => {
  test("darkens the canvas just below the box, faintly, and widens its bounds", () => {
    const item = { ...box("b", 20, 20, 0xffffffff), w: 60, h: 40, shadow: true };
    const shadowed = render({}, 1, (scene) => scene.apply([{ op: "add", item }]));
    const below = red(shadowed, 50, 63);
    expect(below).toBeLessThan(255);
    expect(below).toBeGreaterThan(200);
    expect(red(shadowed, 50, 40)).toBe(255);
    const bounds = boundsOf(item, makeRect(0, 0, 0, 0));
    expect(bounds.b).toBe(60 + 0.5 + SOFT_SHADOW_MARGIN);
  });
});

describe("[STU-030] text on a plugin box", () => {
  const ink = (pixels: Uint8Array, top: number, bottom: number): number => {
    let count = 0;
    for (let y = top; y < bottom; y++) {
      for (let x = 0; x < 100; x++) {
        count += red(pixels, x, y) < 160 ? 1 : 0;
      }
    }
    return count;
  };
  const long = { kind: "text" as const, id: "t", x: 0, y: 0, w: 60, text: "Loans and savings accounts", size: 14 };

  test("a title kept to one line ends in an ellipsis instead of wrapping into the line below", () => {
    const wrapped = render({}, 1, (scene) => scene.apply([{ op: "add", item: { ...long, color: 0x000000ff, z: 1 } }]));
    const one = render({}, 1, (scene) =>
      scene.apply([{ op: "add", item: { ...long, color: 0x000000ff, maxLines: 1, z: 1 } }]),
    );
    expect(ink(wrapped, 22, 60)).toBeGreaterThan(0);
    expect(ink(one, 22, 60)).toBe(0);
    expect(ink(one, 0, 20)).toBeGreaterThan(0);
  });

  test("bold text inks more than regular, and centred text starts further in", () => {
    const word = { kind: "text" as const, id: "t", x: 0, y: 0, w: 100, text: "L", size: 14, color: 0x000000ff, z: 1 };
    const regular = render({}, 1, (scene) => scene.apply([{ op: "add", item: word }]));
    const bold = render({}, 1, (scene) => scene.apply([{ op: "add", item: { ...word, weight: "bold" } }]));
    expect(ink(bold, 0, 20)).toBeGreaterThan(ink(regular, 0, 20));
    const centred = render({}, 1, (scene) => scene.apply([{ op: "add", item: { ...word, align: "center" } }]));
    const firstInk = (pixels: Uint8Array): number => {
      for (let x = 0; x < 100; x++) {
        for (let y = 0; y < 20; y++) {
          if (red(pixels, x, y) < 160) {
            return x;
          }
        }
      }
      return -1;
    };
    expect(firstInk(centred)).toBeGreaterThan(40);
    expect(firstInk(regular)).toBeLessThan(10);
  });
});

describe("[STU-032] overlapping boxes keep their depth", () => {
  test("a box over another hides the lower box's text; batching stays within one depth", () => {
    const plain = (id: string, z: number): RectSpec => ({
      kind: "rect",
      id,
      x: 0,
      y: 0,
      w: 100,
      h: 100,
      radius: 0,
      fill: 0xffffffff,
      z,
    });
    const lower = plain("a", 1);
    const label = { kind: "text" as const, id: "a::t", x: 10, y: 10, w: 80, text: "MMMM", size: 20 };
    const upper = plain("b", 3);
    const pixels = render({}, 1, (scene) =>
      scene.apply([
        { op: "add", item: lower },
        { op: "add", item: { ...label, color: 0x000000ff, z: 2 } },
        { op: "add", item: upper },
      ]),
    );
    let ink = 0;
    for (let y = 0; y < 100; y++) {
      for (let x = 0; x < 100; x++) {
        ink += red(pixels, x, y) < 200 ? 1 : 0;
      }
    }
    expect(ink).toBe(0);
  });
});
