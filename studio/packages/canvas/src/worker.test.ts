// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import type { CanvasKit } from "canvaskit-wasm";
import { box, fontBytes, link, loadCanvasKit, TestEnvironment, text, tick } from "../test-support/canvaskit.ts";
import { CanvasHost, type WorkerLike } from "./host.ts";
import { type CanvasOptions, type HostMessage, PROTOCOL_VERSION as v, type WorkerMessage } from "./protocol.ts";
import { CanvasWorkerCore, serveCanvasWorker, type WorkerScopeLike } from "./worker.ts";

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

function options(extra: Partial<CanvasOptions> = {}): CanvasOptions {
  return {
    renderer: "auto",
    tileSize: 256,
    background: 0xffffffff,
    fonts: [fontBytes()],
    fontFamily: "Noto Sans",
    measureFrames: false,
    ...extra,
  };
}

function init(extra: Partial<CanvasOptions> = {}): HostMessage {
  return { v, type: "init", canvas: { width: 0, height: 0 }, width: 320, height: 200, dpr: 1, options: options(extra) };
}

function start(env: TestEnvironment): { core: CanvasWorkerCore; posted: WorkerMessage[] } {
  const posted: WorkerMessage[] = [];
  return { core: new CanvasWorkerCore((message) => posted.push(message), env.deps()), posted };
}

function ofType<T extends WorkerMessage["type"]>(
  posted: WorkerMessage[],
  type: T,
): Extract<WorkerMessage, { type: T }>[] {
  return posted.filter((m): m is Extract<WorkerMessage, { type: T }> => m.type === type);
}

describe("STU-003 canvas worker core", () => {
  test("renders a small scene headless and answers with its scene model", async () => {
    const env = new TestEnvironment(ck);
    const { core, posted } = start(env);
    await core.receive({ v, type: "patch", ops: [{ op: "add", item: box("a", 10, 10) }] });
    await core.receive(init({ tiles: true }));
    const [ready] = ofType(posted, "ready");
    expect(ready?.renderer.glRenderer).toContain("SwiftShader");
    expect(ready?.renderer).toMatchObject({ kind: "raster", software: true });
    await core.receive({
      v,
      type: "patch",
      ops: [
        { op: "add", item: link("l", 110, 35, 200, 100) },
        { op: "add", item: text("t", 10, 70, "Title") },
      ],
    });
    await core.receive({ v, type: "viewport", change: { op: "pan", dx: 10, dy: 0 } });
    expect(env.drain()).toBeGreaterThan(1);
    const frames = ofType(posted, "frame");
    expect(frames.length).toBeGreaterThan(0);
    expect(frames.at(-1)?.stats.idleQueue).toBe(0);
    await core.receive({ v, type: "snapshot", requestId: 3 });
    const [snapshot] = ofType(posted, "snapshot");
    expect(snapshot?.requestId).toBe(3);
    expect(snapshot?.model.nodes.map((n) => n.id)).toEqual(["l", "a", "t"]);
    expect(snapshot?.model.viewport.x).toBe(-10);
  });

  test("STU-052 renders a thumbnail of an item as a PNG and draws it back as an image", async () => {
    const env = new TestEnvironment(ck);
    const { core, posted } = start(env);
    await core.receive(init({ tiles: true }));
    await core.receive({ v, type: "patch", ops: [{ op: "add", item: box("a", 10, 10, 0xff0000ff) }] });
    await core.receive({ v, type: "thumbnail", requestId: 1, id: "a", width: 40 });
    await core.receive({ v, type: "thumbnail", requestId: 2, id: "missing", width: 40 });
    const [known, unknown] = ofType(posted, "thumbnail");
    expect(Array.from(known?.png?.slice(0, 4) ?? [])).toEqual([137, 80, 78, 71]);
    expect(unknown?.png).toBeNull();
    const png = known?.png ?? new Uint8Array();
    await core.receive({
      v,
      type: "images",
      images: [
        { key: "thumb", bytes: png },
        { key: "bad", bytes: new Uint8Array(3) },
      ],
    });
    await core.receive({
      v,
      type: "patch",
      ops: [
        {
          op: "add",
          item: { kind: "rect", id: "ghost", x: 200, y: 10, w: 100, h: 50, radius: 6, image: "thumb", z: 3 },
        },
      ],
    });
    expect(env.drain()).toBeGreaterThan(0);
    await core.receive({ v, type: "thumbnail", requestId: 3, id: "ghost", width: 40 });
    expect(ofType(posted, "thumbnail")[2]?.png?.length).toBeGreaterThan(0);
    expect(ofType(posted, "error")).toEqual([]);
  });

  test("STU-121 idle tile work yields to a pending message's frame", async () => {
    const env = new TestEnvironment(ck);
    const { core, posted } = start(env);
    await core.receive(init({ tiles: true }));
    await core.receive({ v, type: "patch", ops: [{ op: "add", item: box("a", 10, 10) }] });
    for (const callback of env.frames.splice(0)) {
      callback();
    }
    const queued = ofType(posted, "frame").at(-1)?.stats;
    expect(queued?.idleQueue).toBeGreaterThan(0);
    expect(env.idles.length).toBe(1);
    // A message arrives before the idle slice runs: the slice renders nothing and the frame goes first.
    await core.receive({ v, type: "patch", ops: [{ op: "add", item: box("b", 60, 60) }] });
    for (const callback of env.idles.splice(0)) {
      callback();
    }
    expect(env.frames.length).toBe(1);
    for (const callback of env.frames.splice(0)) {
      callback();
    }
    const next = ofType(posted, "frame").at(-1)?.stats;
    expect(next?.tileRenders).toBe(queued?.tileRenders);
    // The frame asks for idle time again, and the tiles are then rendered.
    expect(env.idles.length).toBe(1);
    env.drain();
    expect(ofType(posted, "frame").at(-1)?.stats.idleQueue).toBe(0);
  });

  test("drag, resize and dispose", async () => {
    const env = new TestEnvironment(ck);
    const { core, posted } = start(env);
    await core.receive(init({ renderer: "raster" }));
    expect(ofType(posted, "ready")[0]?.renderer).toMatchObject({ kind: "raster", tiles: true });
    await core.receive({ v, type: "patch", ops: [{ op: "add", item: box("a", 10, 10) }] });
    await core.receive({ v, type: "interaction", phase: "start", kind: "drag", ids: ["a"] });
    env.drain();
    expect(ofType(posted, "frame").at(-1)?.stats.resolution).toBe(0.5);
    await core.receive({ v, type: "interaction", phase: "end", kind: "drag", ids: ["a"] });
    await core.receive({ v, type: "interaction", phase: "start", kind: "pan", ids: [] });
    await core.receive({ v, type: "resize", width: 100, height: 80, dpr: 2 });
    const before = ofType(posted, "frame").length;
    env.drain();
    // The new surface is drawn; a closing idle frame over an unchanged surface draws nothing.
    expect(
      Math.max(
        ...ofType(posted, "frame")
          .slice(before)
          .map((f) => f.stats.drawCalls),
      ),
    ).toBeGreaterThan(0);
    env.surfaces = false;
    await core.receive({ v, type: "resize", width: 50, height: 50, dpr: 1 });
    expect(ofType(posted, "error")[0]?.message).toBe("no CanvasKit surface after resize");
    await core.receive({ v, type: "dispose" });
    await core.receive({ v, type: "snapshot", requestId: 1 });
    env.drain();
    expect(ofType(posted, "snapshot")).toHaveLength(0);
  });

  test("falls back from WebGL to raster, reports failures and unsupported messages", async () => {
    const env = new TestEnvironment(ck);
    env.webglSurfaces = false;
    env.probe = { available: true, software: false, renderer: "NVIDIA GeForce", majorPerformanceCaveat: false };
    const fallback = start(env);
    await fallback.core.receive(init());
    expect(ofType(fallback.posted, "ready")[0]?.renderer).toMatchObject({ kind: "raster", software: false });
    env.surfaces = false;
    const failing = start(env);
    await failing.core.receive(init({ renderer: "webgl" }));
    await failing.core.receive({ v: 99, type: "patch" });
    expect(ofType(failing.posted, "error").map((m) => m.message)).toEqual([
      "no CanvasKit surface for the canvas",
      "unsupported message",
    ]);
  });

  test("serves the protocol on a worker scope", async () => {
    const env = new TestEnvironment(ck);
    const posted: unknown[] = [];
    const scope: WorkerScopeLike = { onmessage: null, postMessage: (message) => posted.push(message) };
    serveCanvasWorker(scope, env.deps());
    scope.onmessage?.({ data: init() });
    await tick();
    expect(posted).toHaveLength(1);
  });
});

describe("STU-003 main-thread host", () => {
  test("transfers the canvas to a worker and drives it through the protocol", async () => {
    const env = new TestEnvironment(ck);
    let terminated = false;
    let transferred: unknown[] = [];
    const worker: WorkerLike = {
      onmessage: null,
      postMessage(message, transfer) {
        transferred = transfer.length > 0 ? transfer : transferred;
        void core.receive(message);
      },
      terminate() {
        terminated = true;
      },
    };
    const core = new CanvasWorkerCore((message) => worker.onmessage?.({ data: message }), env.deps());
    const canvas = { width: 0, height: 0, transferControlToOffscreen: () => ({ width: 0, height: 0 }) };
    const host = new CanvasHost(canvas, { width: 200, height: 100, dpr: 1 }, options(), {
      createWorker: () => worker,
      mainThread: () => env.deps(),
    });
    expect(host.mode).toBe("worker");
    expect((await host.ready).kind).toMatch(/webgl|raster/);
    expect(transferred).toHaveLength(2);
    host.patch([{ op: "add", item: box("a", 0, 0) }]);
    host.setViewport(0, 0, 1);
    host.zoomAbout(2, 0, 0);
    host.pan(5, 5);
    host.startInteraction("drag", ["a"]);
    host.endInteraction("drag", ["a"]);
    host.resize(220, 120, 1);
    const frame = host.nextFrame();
    env.drain();
    expect((await frame).frame).toBeGreaterThanOrEqual(0);
    const model = await host.snapshot();
    expect(model.viewport).toMatchObject({ zoom: 2, width: 220 });
    host.dispose();
    expect(terminated).toBe(true);
  });

  test("runs the same renderer on the main thread without a worker, and reports start-up errors", async () => {
    const env = new TestEnvironment(ck);
    const host = new CanvasHost({ width: 0, height: 0 }, { width: 100, height: 100, dpr: 1 }, options(), {
      mainThread: () => env.deps(),
    });
    expect(host.mode).toBe("main-thread");
    await host.ready;
    host.patch([{ op: "add", item: box("a", 0, 0) }]);
    const seen: number[] = [];
    const stop = host.onFrame((stats) => seen.push(stats.frame));
    env.drain();
    stop();
    expect(seen.length).toBeGreaterThan(0);
    expect((await host.snapshot()).nodes).toHaveLength(1);
    host.dispose();
    env.surfaces = false;
    const broken = new CanvasHost({ width: 0, height: 0 }, { width: 10, height: 10, dpr: 1 }, options(), {
      mainThread: () => env.deps(),
    });
    await expect(broken.ready).rejects.toThrow("no CanvasKit surface");
  });
});
