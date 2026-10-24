// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Benchmark page: drives the canvas worker through the host API and writes a
// JSON summary into <pre id="result"> for the runner.

import { CanvasHost } from "../src/host.ts";
import type { FrameStats, ItemSpec, RendererPreference, SceneOp } from "../src/protocol.ts";
import { boxPosition, type LinkRef, linkSpec, sceneA, sceneB } from "./scenes.ts";

const WIDTH = 1280;
const HEIGHT = 800;
const WARMUP = 30;
const FRAMES = 300;
const DRAGGED = 105;

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
    host.pan(0, 0);
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

async function dragScenario(host: CanvasHost, links: LinkRef[]): Promise<FrameStats[]> {
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
  host.endInteraction("drag", ids);
  return frames;
}

async function panScenario(host: CanvasHost): Promise<FrameStats[]> {
  host.startInteraction("pan");
  const frames = await measure(host, () => host.pan(-4, -1));
  host.endInteraction("pan");
  return frames;
}

function rendererPreference(): RendererPreference {
  const value = new URLSearchParams(location.search).get("renderer");
  return value === "webgl" || value === "raster" ? value : "auto";
}

async function run(): Promise<unknown> {
  const canvas = document.createElement("canvas");
  canvas.style.width = `${WIDTH}px`;
  canvas.style.height = `${HEIGHT}px`;
  document.body.append(canvas);
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
  );
  const renderer = await host.ready;
  const a = sceneA();
  host.patch(add(a.items));
  host.setViewport(0, 0, 0.6);
  const settleA = await settle(host);
  const aPan = summarise(await panScenario(host));
  host.setViewport(0, 0, 0.6);
  await settle(host);
  const aDrag = summarise(await dragScenario(host, a.links));
  host.patch([{ op: "clear" }, ...add(sceneB())]);
  host.setViewport(0, 0, 0.3);
  const settleB = await settle(host);
  const bPan = summarise(await panScenario(host));
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
