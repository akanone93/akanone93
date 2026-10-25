// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import type { CanvasKit } from "canvaskit-wasm";
import { box, fontBytes, link, loadCanvasKit, TestEnvironment, text, tick } from "../test-support/canvaskit.ts";
import { watchCanvasContext } from "./browser.ts";
import { CanvasHost, type HostEvent } from "./host.ts";
import { type CanvasOptions, type FrameStats, PROTOCOL_VERSION as v, type WorkerMessage } from "./protocol.ts";
import { CanvasWorkerCore, type SurfaceHandlers, type WorkerDeps } from "./worker.ts";

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

const options: CanvasOptions = {
  renderer: "raster",
  tileSize: 256,
  background: 0xffffffff,
  fonts: [fontBytes()],
  fontFamily: "Noto Sans",
  measureFrames: false,
};

function watched(env: TestEnvironment): { deps: WorkerDeps; handlers: () => SurfaceHandlers; surfaces: () => number } {
  let handlers: SurfaceHandlers | null = null;
  let made = 0;
  const base = env.deps();
  return {
    deps: {
      ...base,
      makeSurface: (c, canvas, kind) => {
        made++;
        return base.makeSurface(c, canvas, kind);
      },
      watchContext: (_canvas, h) => {
        handlers = h;
      },
    },
    handlers: () => {
      if (!handlers) {
        throw new Error("not watched");
      }
      return handlers;
    },
    surfaces: () => made,
  };
}

describe("context loss (ADR-0013)", () => {
  test("a simulated loss stops drawing; restoring rebuilds surface and caches from the scene", async () => {
    const env = new TestEnvironment(ck);
    const { deps, handlers, surfaces } = watched(env);
    const posted: WorkerMessage[] = [];
    const core = new CanvasWorkerCore((message) => posted.push(message), deps);
    await core.receive({
      v,
      type: "init",
      canvas: { width: 0, height: 0 },
      width: 320,
      height: 200,
      dpr: 1,
      options,
    });
    await core.receive({
      v,
      type: "patch",
      ops: [
        { op: "add", item: box("a", 10, 10) },
        { op: "add", item: text("t", 10, 80) },
      ],
    });
    env.drain();
    const before = surfaces();

    handlers().lost();
    handlers().lost();
    expect(posted.filter((m) => m.type === "surface")).toEqual([{ v, type: "surface", state: "lost" }]);
    // Edits during the loss reach the scene; nothing is drawn.
    await core.receive({ v, type: "patch", ops: [{ op: "add", item: link("l", 0, 0, 200, 150) }] });
    const framesDuringLoss = posted.length;
    env.drain();
    expect(posted.slice(framesDuringLoss).filter((m) => m.type === "frame")).toHaveLength(0);

    handlers().restored();
    handlers().restored();
    expect(surfaces()).toBe(before + 1);
    expect(posted.filter((m) => m.type === "surface").map((m) => (m.type === "surface" ? m.state : ""))).toEqual([
      "lost",
      "restored",
    ]);
    const after = posted.length;
    env.drain();
    const frames = posted
      .slice(after)
      .flatMap((m) => (m.type === "frame" ? [m.stats] : []))
      .filter((stats: FrameStats) => stats.frame >= 0);
    expect(frames.some((stats) => stats.base)).toBe(true);
    expect(frames.reduce((n, stats) => n + stats.tileRenders, 0)).toBeGreaterThan(0);
    await core.receive({ v, type: "snapshot", requestId: 1 });
    const snapshot = posted.find((m) => m.type === "snapshot");
    expect(snapshot?.type === "snapshot" ? snapshot.model.nodes.map((n) => n.id) : []).toEqual(["l", "a", "t"]);
    await core.receive({ v, type: "dispose" });
  });

  test("a failed rebuild stays lost until a later restore; the host relays surface events", async () => {
    const env = new TestEnvironment(ck);
    const { deps, handlers } = watched(env);
    const host = new CanvasHost({ width: 0, height: 0 }, { width: 100, height: 100, dpr: 1 }, options, {
      mainThread: () => deps,
    });
    await host.ready;
    const events: HostEvent[] = [];
    host.onEvent((event) => events.push(event));
    handlers().lost();
    env.surfaces = false;
    handlers().restored();
    env.surfaces = true;
    handlers().restored();
    await tick();
    expect(events).toEqual([
      { type: "surface", state: "lost" },
      { type: "surface", state: "restored" },
    ]);
    host.dispose();
  });

  test("the browser watcher prevents the default of a loss so the context can be restored", () => {
    const listeners = new Map<string, (event: { preventDefault(): void }) => void>();
    const canvas = {
      width: 1,
      height: 1,
      addEventListener: (type: string, listener: (event: { preventDefault(): void }) => void) => {
        listeners.set(type, listener);
      },
    };
    const calls: string[] = [];
    watchCanvasContext(canvas, { lost: () => calls.push("lost"), restored: () => calls.push("restored") });
    let prevented = false;
    listeners.get("webglcontextlost")?.({ preventDefault: () => (prevented = true) });
    listeners.get("contextrestored")?.({ preventDefault: () => undefined });
    expect(prevented).toBe(true);
    expect(calls).toEqual(["lost", "restored"]);
    expect([...listeners.keys()].sort()).toEqual([
      "contextlost",
      "contextrestored",
      "webglcontextlost",
      "webglcontextrestored",
    ]);
    watchCanvasContext({ width: 1, height: 1 }, { lost: () => undefined, restored: () => undefined });
  });
});
