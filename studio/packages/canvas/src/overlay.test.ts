// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import type { CanvasKit } from "canvaskit-wasm";
import { box, fontBytes, loadCanvasKit, TestEnvironment } from "../test-support/canvaskit.ts";
import { makeRect } from "./geometry.ts";
import { CanvasHost, type WorkerLike } from "./host.ts";
import {
  guideBars,
  HANDLE_SIZE,
  type OverlayCanvasLike,
  type SelectionMarks,
  SelectionOverlay,
  selectionMarks,
} from "./overlay.ts";
import type { CanvasOptions, FrameStats } from "./protocol.ts";
import { MAX_ZOOM, MIN_ZOOM } from "./viewport.ts";

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

const options: CanvasOptions = {
  renderer: "raster",
  tileSize: 256,
  background: 0xffffffff,
  fonts: [],
  fontFamily: "Noto Sans",
  measureFrames: false,
};

const colors = { outline: 0x0a84ffff, handleFill: 0xffffffff, handleStroke: 0x0a84ffff };

function overlayCanvas(): { canvas: OverlayCanvasLike; rects: number[][] } {
  const rects: number[][] = [];
  const context = {
    fillStyle: "" as unknown,
    setTransform: () => undefined,
    clearRect: () => {
      rects.length = 0;
    },
    fillRect: (x: number, y: number, w: number, h: number) => {
      rects.push([x, y, w, h]);
    },
  };
  return { canvas: { width: 0, height: 0, getContext: () => context }, rects };
}

function checkMarks(marks: SelectionMarks, dpr: number): void {
  const values = [...marks.outline, ...marks.handles].flatMap((rect) => [rect.x, rect.y, rect.w, rect.h]);
  expect(values.every(Number.isInteger)).toBe(true);
  expect(marks.handles).toHaveLength(8);
  expect(marks.handles.every((handle) => handle.w === Math.round(HANDLE_SIZE * dpr))).toBe(true);
  expect(marks.outline[0]?.h).toBe(Math.max(1, Math.round(dpr)));
}

describe("selection overlay (ADR-0013 M-12)", () => {
  test("snaps every mark to whole device pixels with a fixed size at every zoom from 10 to 400 %", () => {
    const bounds = makeRect(10.3, 20.7, 133.1, 87.9);
    for (const dpr of [1, 1.25, 2]) {
      for (let zoom = MIN_ZOOM; zoom <= MAX_ZOOM; zoom += 0.07) {
        checkMarks(selectionMarks({ x: -3.3, y: 7.1, zoom }, dpr, bounds), dpr);
      }
    }
  });

  test("draws outline and handles for each box and clears between draws", () => {
    const { canvas, rects } = overlayCanvas();
    const overlay = new SelectionOverlay(canvas, colors);
    overlay.resize(200, 100, 2);
    expect(canvas.width).toBe(400);
    overlay.draw({ x: 0, y: 0, zoom: 1 }, 2, [makeRect(0, 0, 50, 50)]);
    expect(rects).toHaveLength(4 + 16);
    overlay.draw({ x: 0, y: 0, zoom: 1 }, 2, []);
    expect(rects).toHaveLength(0);
    expect(overlay.draws).toBe(2);
  });

  test("changing the selection never re-renders the base nor messages the worker", async () => {
    const env = new TestEnvironment(ck);
    const { canvas, rects } = overlayCanvas();
    const overlay = new SelectionOverlay(canvas, colors);
    const host = new CanvasHost(
      { width: 0, height: 0 },
      { width: 300, height: 200, dpr: 1 },
      { ...options, fonts: [fontBytes()] },
      { mainThread: () => env.deps() },
      null,
      { overlay },
    );
    await host.ready;
    host.patch([
      { op: "add", item: box("a", 10, 10) },
      { op: "add", item: box("b", 150, 10) },
    ]);
    env.drain();
    const frames: FrameStats[] = [];
    host.onFrame((stats) => frames.push(stats));
    host.setSelection(["a", "missing"]);
    expect(host.selection).toEqual(["a"]);
    expect(rects.length).toBe(4 + 16);
    host.setSelection(["a", "b"]);
    host.setSelection(host.hitTest(20, 20));
    host.setSelection([]);
    env.drain();
    expect(frames.filter((stats) => stats.base)).toHaveLength(0);
    expect(env.frames).toHaveLength(0);
    // A pan redraws the overlay at the new place, still without a base render for the selection.
    host.setSelection(["b"]);
    const before = overlay.draws;
    host.pan(-10, 0);
    expect(overlay.draws).toBe(before + 1);
    expect(rects[0]?.[0]).toBe(140);
    host.patch([{ op: "remove", id: "b" }]);
    expect(host.selection).toEqual([]);
    host.dispose();
  });

  test("a worker host answers hit tests and selects with no message to the worker", () => {
    const sent: unknown[] = [];
    const worker: WorkerLike = {
      onmessage: null,
      postMessage: (message) => sent.push(message),
      terminate: () => undefined,
    };
    const element = { width: 0, height: 0, transferControlToOffscreen: () => ({ width: 0, height: 0 }) };
    const host = new CanvasHost(element, { width: 300, height: 200, dpr: 1 }, options, {
      createWorker: () => worker,
      mainThread: () => {
        throw new Error("unused");
      },
    });
    host.patch([{ op: "add", item: box("a", 10, 10) }]);
    host.zoomAbout(2, 0, 0);
    const count = sent.length;
    expect(host.hitTest(30, 30)).toEqual(["a"]);
    expect(host.hitTest(5, 5)).toEqual([]);
    expect(host.hitTestRect(makeRect(0, 0, 400, 400), "contain")).toEqual(["a"]);
    host.setSelection(["a"]);
    host.setGuides({ lines: [{ axis: "x", at: 10, from: 0, to: 50 }], labels: [], indicator: makeRect(0, 0, 5, 5) });
    host.setGuides(null);
    expect(sent.length).toBe(count);
  });

  test("guides are one device pixel wide at any zoom and the indicator keeps a minimum thickness", () => {
    for (const zoom of [0.25, 1, 4]) {
      const bars = guideBars({ x: 0, y: 0, zoom }, 2, {
        lines: [
          { axis: "x", at: 10, from: 0, to: 100 },
          { axis: "y", at: 5, from: 0, to: 40 },
        ],
        labels: [],
        indicator: makeRect(0, 0, 0, 20),
      });
      expect(bars[0]?.w).toBe(1);
      expect(bars[1]?.h).toBe(1);
      expect(bars[2]?.w).toBeGreaterThanOrEqual(2);
      expect(bars.every((b) => Number.isInteger(b.x) && Number.isInteger(b.y))).toBe(true);
    }
  });
});
