// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import type { CanvasKit } from "canvaskit-wasm";
import { box, loadCanvasKit } from "../test-support/canvaskit.ts";
import { makeRect, type Rect } from "./geometry.ts";
import { BaseJobQueue, JOB_BUDGET_MS, jobBand, orderJobs } from "./progressive.ts";
import { SceneRenderer } from "./renderer.ts";
import { Scene } from "./scene.ts";
import { Viewport } from "./viewport.ts";

/** A fake clock and animation-frame scheduler. */
class Frames {
  time = 0;
  private callbacks: (() => void)[] = [];

  readonly deps = {
    now: () => this.time,
    schedule: (callback: () => void) => {
      this.callbacks.push(callback);
    },
  };

  get pending(): number {
    return this.callbacks.length;
  }

  /** Runs one animation frame. */
  frame(): void {
    const callbacks = this.callbacks;
    this.callbacks = [];
    for (const callback of callbacks) {
      callback();
    }
  }
}

/** A job queue of `count` jobs costing `cost` ms each; records the jobs run. */
function jobs(frames: Frames, count: number, cost: number) {
  const ran: number[] = [];
  let next = 0;
  const job = () => {
    ran.push(next++);
    frames.time += cost;
    return next < count;
  };
  return { ran, job };
}

const view = makeRect(0, 0, 100, 100);
const at = (x: number, y: number): Rect => makeRect(x, y, x + 50, y + 50);

describe("STU-120 STU-045 progressive base jobs (M-12)", () => {
  test("jobs in the view come first, then the margin the pan heads into, then the rest, nearest first", () => {
    const rects = [at(-200, 0), at(150, 0), at(25, 25), at(110, 0), at(-60, 0), at(60, 60)];
    expect(rects.map((r) => jobBand(r, view, { x: 1, y: 0 }))).toEqual([2, 1, 0, 1, 2, 0]);
    const ordered = orderJobs(rects, (r) => r, view, { x: 1, y: 0 });
    expect(ordered).toEqual([at(25, 25), at(60, 60), at(110, 0), at(150, 0), at(-60, 0), at(-200, 0)]);
    const back = orderJobs(rects, (r) => r, view, { x: -1, y: 0 });
    expect(back.slice(2, 4)).toEqual([at(-60, 0), at(-200, 0)]);
  });

  test("equal keys keep their order", () => {
    const rects = [at(200, 0), at(0, 200), at(-200, 0)];
    expect(orderJobs(["a", "b", "c"], (_, i = 0) => rects[i] ?? view, view, { x: 0, y: 0 })).toEqual(["a", "b", "c"]);
  });

  test("a slice runs jobs until the per-frame budget is spent, then yields to the next frame", () => {
    const frames = new Frames();
    const queue = new BaseJobQueue(frames.deps, 8);
    const { ran, job } = jobs(frames, 10, 3);
    const slices: [number, boolean][] = [];
    queue.start(job, (ms, more) => slices.push([ms, more]));
    expect(ran).toEqual([]);
    frames.frame();
    expect(ran).toEqual([0, 1, 2]);
    expect(slices).toEqual([[9, true]]);
    while (frames.pending > 0) {
      frames.frame();
    }
    expect(ran).toEqual([0, 1, 2, 3, 4, 5, 6, 7, 8, 9]);
    expect(slices.map(([, more]) => more)).toEqual([true, true, true, false]);
    expect(queue.active).toBe(false);
  });

  test("a job over budget runs alone in its frame", () => {
    const frames = new Frames();
    const queue = new BaseJobQueue(frames.deps);
    const { ran, job } = jobs(frames, 3, 2 * JOB_BUDGET_MS);
    queue.start(job, () => undefined);
    frames.frame();
    expect(ran).toEqual([0]);
    frames.frame();
    expect(ran).toEqual([0, 1]);
  });

  test("cancel drops the run; the slice already scheduled runs nothing and schedules nothing", () => {
    const frames = new Frames();
    const queue = new BaseJobQueue(frames.deps);
    const { ran, job } = jobs(frames, 10, 1);
    const slices: number[] = [];
    queue.start(job, (ms) => slices.push(ms));
    queue.cancel();
    expect(queue.active).toBe(false);
    frames.frame();
    expect([ran, slices, frames.pending]).toEqual([[], [], 0]);
  });

  test("a new start preempts the old run without a second slice in flight", () => {
    const frames = new Frames();
    const queue = new BaseJobQueue(frames.deps, 4);
    const old = jobs(frames, 100, 1);
    queue.start(old.job, () => undefined);
    frames.frame();
    expect(old.ran).toHaveLength(4);
    const replanned = jobs(frames, 2, 1);
    queue.start(replanned.job, () => undefined);
    queue.start(replanned.job, () => undefined);
    expect(frames.pending).toBe(1);
    frames.frame();
    expect(old.ran).toHaveLength(4);
    expect(replanned.ran).toEqual([0, 1]);
    expect([queue.active, frames.pending]).toEqual([false, 0]);
  });

  test("a job that replans from inside a slice ends the old slice at once", () => {
    const frames = new Frames();
    const queue = new BaseJobQueue(frames.deps);
    const after = jobs(frames, 1, 1);
    let calls = 0;
    const done: boolean[] = [];
    queue.start(
      () => {
        calls++;
        queue.start(after.job, (_, more) => done.push(more));
        return true;
      },
      () => done.push(true),
    );
    frames.frame();
    expect([calls, done, frames.pending]).toEqual([1, [], 1]);
    frames.frame();
    expect([after.ran, done]).toEqual([[0], [false]]);
  });
});

describe("STU-120 STU-045 progressive base re-render in the renderer (M-12)", () => {
  let ck: CanvasKit;
  beforeAll(async () => {
    ck = await loadCanvasKit();
  });

  function setup() {
    const scene = new Scene();
    const viewport = new Viewport(512, 512);
    const surface = ck.MakeSurface(512, 512);
    if (!surface) {
      throw new Error("no raster surface");
    }
    const renderer = new SceneRenderer(
      ck,
      scene,
      viewport,
      surface,
      1,
      { tiles: true, tileSize: 128, background: 0xf5f5f7ff, fontFamily: "Noto Sans", measureFrames: false },
      () => performance.now(),
      null,
    );
    scene.apply(Array.from({ length: 16 }, (_, i) => ({ op: "add", item: box(`b${i}`, (i % 4) * 128, Math.floor(i / 4) * 128) })));
    return { viewport, renderer };
  }

  test("the base is realigned first, then its tiles are queued around the focus and replaced one per job", () => {
    const { viewport, renderer } = setup();
    renderer.frame();
    viewport.x = 64;
    renderer.planBase(makeRect(400, 400, 440, 440), { x: 1, y: 0 });
    const queued = renderer.baseJobs;
    expect(queued).toBeGreaterThan(1);
    const first = renderer.queuedTiles()[0];
    expect(first ? renderer.tileCache.tileRect(first, makeRect(0, 0, 0, 0)) : null).toEqual(makeRect(384, 384, 512, 512));
    expect(renderer.baseJob(Number.POSITIVE_INFINITY)).toMatchObject({ shifted: true });
    expect(renderer.baseJobs).toBe(queued - 1);
    let runs = 0;
    while (renderer.baseJobs > 0) {
      expect(renderer.baseJob(Number.POSITIVE_INFINITY).shifted).toBe(false);
      runs++;
    }
    expect(runs).toBe(queued - 1);
    expect(renderer.baseJob(Number.POSITIVE_INFINITY)).toEqual({ ms: 0, shifted: false });
    renderer.planBase(makeRect(0, 0, 512, 512), { x: 0, y: 0 });
    expect(renderer.baseJobs).toBe(0);
  });

  test("cancel drops the queued tiles but keeps the realignment owed", () => {
    const { viewport, renderer } = setup();
    renderer.frame();
    viewport.x = 64;
    renderer.planBase(makeRect(0, 0, 512, 512), { x: 0, y: 0 });
    renderer.cancelBase();
    expect(renderer.queuedTiles()).toHaveLength(0);
    expect(renderer.baseJobs).toBe(1);
  });
});
