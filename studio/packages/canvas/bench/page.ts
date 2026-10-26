// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Benchmark page: drives the canvas worker through the host API and writes a
// JSON summary into <pre id="result"> for the runner. Pans and drags are timed
// on the main thread from one animation frame to the next — what the user
// sees, including the browser presenting the base and the interaction layer
// (M-12) — with the worker's own frame times beside them.

import { defaultScreenTheme } from "@plux/layout";
import { ArrowCanvas } from "../src/arrow-layer.ts";
import { ArrowRouter, deriveAppBundles } from "../src/arrows.ts";
import { CanvasHost } from "../src/host.ts";
import type {
  FrameStats,
  ItemSpec,
  RendererPreference,
  SceneOp,
  ScreenDocument,
  ScreenStyle,
} from "../src/protocol.ts";
import { measurePropertyChange, type PropertySummary } from "./property.ts";
import { boxPosition, type LinkRef, linkSpec, sceneA, sceneB, sceneC } from "./scenes.ts";

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

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Time until every tile the view shows is current at full resolution
 * (`visibleMs`, the settle target) and until the overscan and idle work are
 * done too (`fullMs`). The first frame is skipped: it may have been in flight
 * before the call.
 */
async function settleTimes(host: CanvasHost): Promise<{ visibleMs: number; fullMs: number }> {
  const start = performance.now();
  let visibleMs = -1;
  for (let i = 0; i < 400; i++) {
    host.patch([]);
    const stats = await host.nextFrame();
    if (i === 0) {
      continue;
    }
    if (visibleMs < 0 && stats.visibleJobs === 0 && stats.resolution === 1) {
      visibleMs = round(performance.now() - start);
    }
    if (stats.idleQueue === 0 && stats.baseJobs === 0) {
      break;
    }
    await sleep(2);
  }
  return { visibleMs, fullMs: round(performance.now() - start) };
}

async function settle(host: CanvasHost): Promise<number> {
  return (await settleTimes(host)).fullMs;
}

function add(items: ItemSpec[]): SceneOp[] {
  return items.map((item) => ({ op: "add", item }));
}

/** Frame-to-frame intervals on the main thread, sorted: percentiles, longest gap and frames dropped. */
function intervalSummary(intervals: number[]): Record<string, number> {
  intervals.sort((a, b) => a - b);
  return {
    frames: intervals.length,
    p50: round(percentile(intervals, 0.5)),
    p95: round(percentile(intervals, 0.95)),
    max: round(intervals.at(-1) ?? 0),
    droppedFrames: intervals.filter((ms) => ms > 1.5 * FRAME_MS).length,
  };
}

/** The worker's side of a drag: its frame time and what the interaction layer drew and presented. */
function workerDrag(worker: FrameStats[]): Record<string, number> {
  return {
    workerFrames: worker.length,
    workerFrameMsP95: round(
      percentile(
        worker.map((f) => f.ms).sort((a, b) => a - b),
        0.95,
      ),
    ),
    layerMsMean: round(mean(worker.map((f) => f.layerMs))),
    layerPixelsMean: Math.round(mean(worker.map((f) => f.layerPixels))),
    layerPixelsMax: Math.max(0, ...worker.map((f) => f.layerPixels)),
  };
}

/** Where a frame's time went: the base's patches and flush, and the interaction layer. */
function breakdown(frame: FrameStats | undefined): Record<string, number> {
  if (!frame) {
    return {};
  }
  const { ms, regionMs, flushMs, layerMs, layerPixels, regions } = frame;
  return {
    ms: round(ms),
    regionMs: round(regionMs),
    flushMs: round(flushMs),
    layerMs: round(layerMs),
    layerPixels,
    regions,
  };
}

/**
 * The drop at the end of a drag: the worker's first frame after
 * `endInteraction` (`dropFrameMs`) and the time until the tiles the dropped
 * items left and joined are refreshed (`dropSettleMs`, M-13).
 */
async function dropScenario(host: CanvasHost, ids: readonly string[]): Promise<Record<string, number>> {
  const frames: FrameStats[] = [];
  const stop = host.onFrame((stats) => frames.push(stats));
  host.endInteraction("drag", ids);
  const dropSettleMs = (await settleTimes(host)).fullMs;
  stop();
  return { dropFrameMs: round(frames[0]?.ms ?? 0), dropSettleMs };
}

/** Scene A: one box and its links dragged 2 × 1 world units per animation frame, timed rAF to rAF. */
async function dragScenario(host: CanvasHost, links: LinkRef[]): Promise<Record<string, unknown>> {
  const touching = links.filter((l) => l.a === DRAGGED || l.b === DRAGGED);
  const ids = [`box-${DRAGGED}`, `label-${DRAGGED}`, ...touching.map((l) => l.id)];
  const origin = boxPosition(DRAGGED);
  const worker: FrameStats[] = [];
  const stop = host.onFrame((stats) => worker.push(stats));
  host.startInteraction("drag", ids);
  const intervals: number[] = [];
  let previous = await animationFrame();
  let liftFrameMs = 0;
  let liftFrame: Record<string, number> = {};
  for (let i = 0; i < WARMUP + FRAMES; i++) {
    const now = await animationFrame();
    if (i === WARMUP) {
      // The lift's frame is the first after `startInteraction`; the warm-up hides its cost from the drag.
      liftFrameMs = round(worker[0]?.ms ?? 0);
      liftFrame = breakdown(worker[0]);
      worker.length = 0;
    }
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
    if (i >= WARMUP) {
      intervals.push(now - previous);
    }
    previous = now;
  }
  const layerBox = host.layerBox;
  stop();
  const drop = await dropScenario(host, ids);
  return {
    ...intervalSummary(intervals),
    ...workerDrag(worker),
    ...baseRenders(worker),
    layerBox,
    liftFrameMs,
    liftFrame,
    ...drop,
  };
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

/** The worst base frame's breakdown, to name the bottleneck. */
function worstBase(frames: FrameStats[]): Record<string, number> | null {
  const worst = frames.filter((f) => f.base).sort((a, b) => b.ms - a.ms)[0];
  if (!worst) {
    return null;
  }
  const { ms, jobMs, flushMs, tileMs, regionMs, regions, tilesDrawn, drawCalls, baseJobs } = worst;
  return { ms, jobMs, flushMs, tileMs, regionMs, regions, tilesDrawn, drawCalls, baseJobs };
}

/** A pan of 4 × 1 CSS pixels per animation frame, timed rAF to rAF on the main thread. */
async function panScenario(host: CanvasHost): Promise<Record<string, unknown>> {
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
  // Settle: from the gesture's end to the full-resolution base, its progressive jobs done (M-12).
  const settled = await settleTimes(host);
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
    settleVisibleMs: settled.visibleMs,
    settleFullMs: settled.fullMs,
    workerFrames: worker.length,
    ...baseRenders(worker),
    worstBase: worstBase(worker),
  };
}

/**
 * STU-045 with real routing: scene C's arrows derived, bundled and routed; a pan,
 * then a drag of one box paced by animation frames, re-routing on the main
 * thread every frame (`routeMs`) and patching the worker.
 */
async function arrowScenario(host: CanvasHost): Promise<Record<string, unknown>> {
  const c = sceneC();
  const t0 = performance.now();
  const bundles = deriveAppBundles(c.edges, c.pluginOf);
  const router = new ArrowRouter(c.boxes, bundles);
  const arrows = new ArrowCanvas(router, undefined, 0.6);
  const ops = arrows.render();
  const initialRouteMs = performance.now() - t0;
  host.patch([{ op: "clear" }, ...add(c.items), ...ops]);
  host.setViewport(0, 0, 0.6);
  const settleMs = await settle(host);
  const pan = await panScenario(host);
  host.setViewport(0, 0, 0.6);
  await settle(host);
  const worker: FrameStats[] = [];
  const stop = host.onFrame((stats) => worker.push(stats));
  const box = `box-${DRAGGED}`;
  const ids = [box, `label-${DRAGGED}`, ...arrows.beginDrag(box)];
  const origin = boxPosition(DRAGGED);
  host.startInteraction("drag", ids);
  // Lifting the arrows: its first frame (`liftFrameMs`), then the tiles under them refreshed in idle time.
  const dragStartMs = (await settleTimes(host)).fullMs;
  const lift = worker.filter((f) => f.tileRenders > 0);
  const liftFrameMs = round(worker[0]?.ms ?? 0);
  const liftFrame = breakdown(worker[0]);
  const liftTileRenders = (lift.at(-1)?.tileRenders ?? 0) - (worker[0]?.tileRenders ?? 0);
  worker.length = 0;
  const intervals: number[] = [];
  const route: number[] = [];
  const main: number[] = [];
  const rerouted: number[] = [];
  let box0 = host.layerBox;
  let layerBoxChanges = 0;
  let previous = await animationFrame();
  for (let i = 0; i < WARMUP + FRAMES; i++) {
    const now = await animationFrame();
    const begin = performance.now();
    // A sweep of 600 x 300 world units: the box crosses others and switches sides.
    const patch = arrows.moveBox(box, origin.x + (i + 1) * 2, origin.y + (i + 1));
    const ms = performance.now() - begin;
    host.patch([{ op: "move", id: box, dx: 2, dy: 1 }, { op: "move", id: `label-${DRAGGED}`, dx: 2, dy: 1 }, ...patch]);
    const mainMs = performance.now() - begin;
    if (host.layerBox !== box0) {
      box0 = host.layerBox;
      layerBoxChanges++;
    }
    if (i >= WARMUP) {
      intervals.push(now - previous);
      main.push(mainMs);
      route.push(ms);
      rerouted.push(patch.length);
    }
    previous = now;
  }
  const layerBox = host.layerBox;
  const drop = performance.now();
  const caughtUp = arrows.endDrag();
  const dropRouteMs = performance.now() - drop;
  host.patch(caughtUp);
  stop();
  const dropped = await dropScenario(host, ids);
  route.sort((a, b) => a - b);
  const clear = [...router.routes.values()].filter((r) => r.clear).length;
  return {
    links: c.edges.length,
    bundles: router.routes.size,
    clearRoutes: clear,
    items: c.items.length + ops.length,
    initialRouteMs: round(initialRouteMs),
    settleMs,
    pan,
    drag: {
      ...intervalSummary(intervals),
      routeMsP50: round(percentile(route, 0.5)),
      routeMsP95: round(percentile(route, 0.95)),
      routeMsMax: round(route.at(-1) ?? 0),
      mainThreadMsP95: round(
        percentile(
          main.sort((a, b) => a - b),
          0.95,
        ),
      ),
      ...workerDrag(worker),
      layerBox,
      layerBoxChanges,
      opsPerFrameMean: round(mean(rerouted)),
      dragStartMs,
      liftFrameMs,
      liftFrame,
      liftTileRenders,
      dropRouteMs: round(dropRouteMs),
      dropOps: caughtUp.length,
      ...dropped,
      ...baseRenders(worker),
    },
  };
}

const propertyStyle: ScreenStyle = {
  deviceHeight: 844,
  theme: { ...defaultScreenTheme, fontFamilies: ["Noto Sans"] },
  frame: 0xffffffff,
  frameStroke: 0x11,
  box: 0x22,
  boxStroke: 0x33,
  placeholder: 0x44,
  text: 0x55,
  overflowFill: 0x66,
  overflowBox: 0x77,
  overflowText: 0x88,
  overflowStroke: 0x99,
};

/** A plugin of eight screens with forty text rows each: the size of a large page set. */
function propertyScreens(): ScreenDocument[] {
  return Array.from({ length: 8 }, (_, s) => ({
    id: `s${s}`,
    title: `Screen ${s}`,
    x: s * 420,
    y: 0,
    root: {
      id: `s${s}-root`,
      slots: {
        body: {
          id: `s${s}-body`,
          children: Array.from({ length: 40 }, (_, r) => ({
            id: `s${s}-t${r}`,
            type: "Text",
            props: { data: `Row ${r}` },
          })),
        },
      },
    },
  }));
}

/** STU-121: edit one text property, relayout in the worker, draw; p95 against 50 ms. */
function propertyScenario(host: CanvasHost): Promise<PropertySummary> {
  return measurePropertyChange(
    host,
    propertyScreens(),
    propertyStyle,
    (screens, i) =>
      screens.map((s, n) =>
        n !== 0
          ? s
          : {
              ...s,
              root: {
                id: s.root?.id ?? "",
                slots: {
                  body: {
                    id: "s0-body",
                    // biome-ignore lint/complexity/useLiteralKeys: noPropertyAccessFromIndexSignature
                    children: (s.root?.slots?.["body"]?.children ?? []).map((c, r) =>
                      r === 3 ? { ...c, props: { data: `Row 3 edit ${i}` } } : c,
                    ),
                  },
                },
              },
            },
      ),
    { warmup: WARMUP, runs: FRAMES, now: () => performance.now() },
  );
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
  const aDrag = await dragScenario(host, a.links);
  host.patch([{ op: "clear" }, ...add(sceneB())]);
  host.setViewport(0, 0, 0.3);
  const settleB = await settle(host);
  const bPan = await panScenario(host);
  const arrowsC = await arrowScenario(host);
  host.patch([{ op: "clear" }]);
  const property = await propertyScenario(host);
  host.dispose();
  return {
    renderer,
    viewport: { width: WIDTH, height: HEIGHT, dpr: 1 },
    budgetMs: 16.7,
    sceneA: { items: a.items.length, zoom: 0.6, settleMs: settleA, pan: aPan, drag: aDrag },
    sceneB: { zoom: 0.3, settleMs: settleB, pan: bPan },
    sceneC: { zoom: 0.6, ...arrowsC },
    propertyToCanvas: property,
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
