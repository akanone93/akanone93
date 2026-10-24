// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Benchmark page: drives the canvas worker through the host API and writes a
// JSON summary into <pre id="result"> for the runner. Pans are timed on the
// main thread from one animation frame to the next, since the compositor and
// not GL moves the base while panning (M-12); drags are timed in the worker.

import type { LayerBox } from "../src/compositor.ts";
import { CanvasHost } from "../src/host.ts";
import type { FrameStats, ItemSpec, RendererPreference, SceneOp } from "../src/protocol.ts";
import { boxPosition, type LinkRef, linkSpec, sceneA, sceneB } from "./scenes.ts";

const WIDTH = 1280;
const HEIGHT = 800;
const WARMUP = 30;
const FRAMES = 300;
const DRAGGED = 105;
const FRAME_MS = 1000 / 60;

function percentile(sorted: number[], p: number): number {
  return sorted[Math.min(sorted.length - 1, Math.floor((sorted.length - 1) * p))] ?? 0;
}

function mean(values: number[]): number {
  return values.length === 0 ? 0 : values.reduce((a, b) => a + b, 0) / values.length;
}

function round(value: number): number {
  return Math.round(value * 100) / 100;
}

function summarise(frames: FrameStats[]): Record<string, number> {
  const ms = frames.map((f) => f.ms).sort((a, b) => a - b);
  const first = frames[0];
  const last = frames.at(-1);
  const renders = first && last ? last.tileRenders - first.tileRenders : 0;
  const renderMs = first && last ? last.tileRenderMs - first.tileRenderMs : 0;
  return {
    frames: frames.length,
    p50: round(percentile(ms, 0.5)),
    p95: round(percentile(ms, 0.95)),
    max: round(ms.at(-1) ?? 0),
    drawCallsMean: round(mean(frames.map((f) => f.drawCalls))),
    drawCallsMax: Math.max(...frames.map((f) => f.drawCalls)),
    tileBlitMsMean: round(mean(frames.map((f) => f.tileMs))),
    regionMsMean: round(mean(frames.map((f) => f.regionMs))),
    liveMsMean: round(mean(frames.map((f) => f.liveMs))),
    flushMsMean: round(mean(frames.map((f) => f.flushMs))),
    regionsMean: round(mean(frames.map((f) => f.regions))),
    tilesDrawnMean: round(mean(frames.map((f) => f.tilesDrawn))),
    idleTileRenders: renders,
    idleMsPerTile: renders > 0 ? round(renderMs / renders) : 0,
  };
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function settle(host: CanvasHost): Promise<number> {
  const start = performance.now();
  for (let i = 0; i < 400; i++) {
    host.patch([]);
    if ((await host.nextFrame()).idleQueue === 0) {
      break;
    }
    await sleep(2);
  }
  return round(performance.now() - start);
}

async function measure(host: CanvasHost, step: (i: number) => void): Promise<FrameStats[]> {
  const out: FrameStats[] = [];
  for (let i = 0; i < WARMUP + FRAMES; i++) {
    step(i);
    const stats = await host.nextFrame();
    if (i >= WARMUP) {
      out.push(stats);
    }
  }
  return out;
}

function add(items: ItemSpec[]): SceneOp[] {
  return items.map((item) => ({ op: "add", item }));
}

async function dragScenario(
  host: CanvasHost,
  links: LinkRef[],
): Promise<{ frames: FrameStats[]; layerBox: LayerBox | null }> {
  const touching = links.filter((l) => l.a === DRAGGED || l.b === DRAGGED);
  const ids = [`box-${DRAGGED}`, `label-${DRAGGED}`, ...touching.map((l) => l.id)];
  const origin = boxPosition(DRAGGED);
  host.startInteraction("drag", ids);
  const frames = await measure(host, (i) => {
    const at = { x: origin.x + (i + 1) * 2, y: origin.y + (i + 1) };
    const ops: SceneOp[] = [
      { op: "move", id: `box-${DRAGGED}`, dx: 2, dy: 1 },
      { op: "move", id: `label-${DRAGGED}`, dx: 2, dy: 1 },
    ];
    for (const l of touching) {
      const a = l.a === DRAGGED ? at : boxPosition(l.a);
      const b = l.b === DRAGGED ? at : boxPosition(l.b);
      ops.push({ op: "replace", item: linkSpec(l.id, a, b, l.style) });
    }
    host.patch(ops);
  });
  const layerBox = host.layerBox;
  host.endInteraction("drag", ids);
  return { frames, layerBox };
}

function animationFrame(): Promise<number> {
  return new Promise((resolve) => requestAnimationFrame(resolve));
}

function baseRenders(frames: FrameStats[]): Record<string, number> {
  const ms = frames
    .filter((f) => f.base)
    .map((f) => f.ms)
    .sort((a, b) => a - b);
  return {
    baseRenders: ms.length,
    baseRenderMsP50: round(percentile(ms, 0.5)),
    baseRenderMsMax: round(ms.at(-1) ?? 0),
  };
}

/** A pan of 4 × 1 CSS pixels per animation frame, timed rAF to rAF on the main thread. */
async function panScenario(host: CanvasHost): Promise<Record<string, number>> {
  const worker: FrameStats[] = [];
  const stop = host.onFrame((stats) => worker.push(stats));
  host.startInteraction("pan");
  const intervals: number[] = [];
  const work: number[] = [];
  let previous = await animationFrame();
  for (let i = 0; i < WARMUP + FRAMES; i++) {
    const now = await animationFrame();
    const begin = performance.now();
    host.pan(-4, -1);
    if (i >= WARMUP) {
      intervals.push(now - previous);
      work.push(performance.now() - begin);
    }
    previous = now;
  }
  host.endInteraction("pan");
  stop();
  intervals.sort((a, b) => a - b);
  work.sort((a, b) => a - b);
  return {
    frames: intervals.length,
    p50: round(percentile(intervals, 0.5)),
    p95: round(percentile(intervals, 0.95)),
    max: round(intervals.at(-1) ?? 0),
    droppedFrames: intervals.filter((ms) => ms > 1.5 * FRAME_MS).length,
    mainThreadMsP95: round(percentile(work, 0.95)),
    workerFrames: worker.length,
    ...baseRenders(worker),
  };
}

function rendererPreference(): RendererPreference {
  const value = new URLSearchParams(location.search).get("renderer");
  return value === "webgl" || value === "raster" ? value : "auto";
}

async function run(): Promise<unknown> {
  const container = document.createElement("div");
  container.style.cssText = `position:relative;overflow:hidden;width:${WIDTH}px;height:${HEIGHT}px`;
  const canvas = document.createElement("canvas");
  const layer = document.createElement("canvas");
  container.append(canvas, layer);
  document.body.append(container);
  const font = await fetch("/font.ttf").then((r) => r.arrayBuffer());
  const host = new CanvasHost(
    canvas,
    { width: WIDTH, height: HEIGHT, dpr: 1 },
    {
      renderer: rendererPreference(),
      tileSize: 512,
      background: 0xf8f9faff,
      fonts: [font],
      fontFamily: "Noto Sans",
      measureFrames: true,
    },
    {
      createWorker: () => new Worker("/worker.js", { type: "module" }),
      mainThread: () => {
        throw new Error("the benchmark needs a worker");
      },
    },
    layer,
  );
  const renderer = await host.ready;
  const a = sceneA();
  host.patch(add(a.items));
  host.setViewport(0, 0, 0.6);
  const settleA = await settle(host);
  const aPan = await panScenario(host);
  host.setViewport(0, 0, 0.6);
  await settle(host);
  const { frames: dragFrames, layerBox } = await dragScenario(host, a.links);
  const aDrag = {
    ...summarise(dragFrames),
    layerMsMean: round(mean(dragFrames.map((f) => f.layerMs))),
    ...baseRenders(dragFrames),
    layerBox,
  };
  host.patch([{ op: "clear" }, ...add(sceneB())]);
  host.setViewport(0, 0, 0.3);
  const settleB = await settle(host);
  const bPan = await panScenario(host);
  host.dispose();
  return {
    renderer,
    viewport: { width: WIDTH, height: HEIGHT, dpr: 1 },
    budgetMs: 16.7,
    sceneA: { items: a.items.length, zoom: 0.6, settleMs: settleA, pan: aPan, drag: aDrag },
    sceneB: { zoom: 0.3, settleMs: settleB, pan: bPan },
  };
}

const output = document.createElement("pre");
output.id = "result";
run()
  .then((result) => {
    output.textContent = JSON.stringify(result, null, 2);
  })
  .catch((error: unknown) => {
    output.textContent = JSON.stringify({ error: String(error) });
  })
  .finally(() => document.body.append(output));
