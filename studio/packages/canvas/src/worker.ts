// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The canvas worker (STU-003, studio-development §2.5): owns the scene, the
 * viewport and the renderer, applies messages from the main thread, draws on
 * animation frames and renders tiles when idle. `CanvasWorkerCore` holds no
 * global state and runs unchanged on the main thread as the fallback; the
 * environment (CanvasKit loading, frame and idle scheduling, surfaces, the
 * WebGL probe) is injected, so `serveCanvasWorker` is called by the worker
 * entry point rather than run at import.
 *
 * The base surface is overscanned and re-rendered only when the content
 * changes or the view nears its edge; the host moves it with a CSS transform
 * in between. Dragged items are drawn into the interaction layer (M-12, see
 * `compositor.ts`). With tiles, a recentred base is re-rendered progressively
 * in tile-sized jobs under a per-frame budget (`progressive.ts`), replanned
 * whenever the view moves, so no frame waits for the whole base.
 */

import { CanvasKitParagraphEngine, type ParagraphEngine } from "@plux/layout";
import type { CanvasKit, Surface } from "canvaskit-wasm";
import {
  type BaseOrigin,
  baseSize,
  centredOrigin,
  isCentred,
  type LayerBox,
  layerBox,
  needsRender,
  overscanMargin,
} from "./compositor.ts";
import { intersectInto, makeRect, unionInto } from "./geometry.ts";
import { BaseJobQueue } from "./progressive.ts";
import {
  type CanvasOptions,
  type HostMessage,
  type InteractionKind,
  type OffscreenCanvasLike,
  PROTOCOL_VERSION,
  type RendererInfo,
  readHostMessage,
  type WorkerMessage,
} from "./protocol.ts";
import { type LiveLayer, SceneRenderer } from "./renderer.ts";
import { Scene, type SceneItem } from "./scene.ts";
import { buildSceneModel } from "./scene-model.ts";
import { ScreenLayouts } from "./screens.ts";
import { chooseRenderer, type GlProbe } from "./select.ts";
import { Viewport } from "./viewport.ts";

/** Time per idle slice for tile rendering, in ms. */
export const IDLE_BUDGET_MS = 8;

/** The environment of the canvas worker. */
export interface WorkerDeps {
  loadCanvasKit(): Promise<CanvasKit>;
  now(): number;
  requestFrame(callback: () => void): void;
  requestIdle(callback: () => void): void;
  probeGl(): GlProbe;
  /** A surface on `canvas`; `layer` is the transparent CPU raster interaction layer. */
  makeSurface(ck: CanvasKit, canvas: OffscreenCanvasLike, kind: "webgl" | "raster" | "layer"): Surface | null;
  /**
   * Watches `canvas` for the loss and restoration of its context
   * (`webglcontextlost`/`webglcontextrestored`, `contextlost`/`contextrestored`);
   * optional, as a raster surface on the main thread cannot lose it.
   */
  watchContext?(canvas: OffscreenCanvasLike, handlers: SurfaceHandlers): void;
  /**
   * The paragraph engine screens are laid out with, its fonts registered;
   * optional, by default SkParagraph on `ck` (ADR-0013 Text).
   */
  paragraphEngine?(ck: CanvasKit, options: CanvasOptions): DisposableEngine;
}

/** A paragraph engine the worker releases on dispose. */
export type DisposableEngine = ParagraphEngine & { dispose(): void };

/** SkParagraph on `ck` with the canvas fonts registered under their families. */
export function canvasParagraphEngine(ck: CanvasKit, options: CanvasOptions): CanvasKitParagraphEngine {
  const engine = new CanvasKitParagraphEngine(ck);
  options.fonts.forEach((bytes, i) => {
    const family = options.fontFamilies?.[i] ?? options.fontFamily;
    engine.registerFont(new Uint8Array(bytes), family);
  });
  return engine;
}

/** The text families in fallback order: the primary, then every other loaded family. */
export function fallbackFamilies(options: CanvasOptions): string[] {
  return [...new Set(options.fontFamilies ?? [])].filter((family) => family !== options.fontFamily);
}

/** What the worker does when its surface is lost and when it may be rebuilt. */
export interface SurfaceHandlers {
  lost(): void;
  restored(): void;
}

interface Running {
  readonly ck: CanvasKit;
  readonly canvas: OffscreenCanvasLike;
  readonly info: RendererInfo;
  readonly scene: Scene;
  /** The view the host shows; authoritative for the scene model. */
  readonly viewport: Viewport;
  /** The overscanned view the base surface is rendered through. */
  readonly render: Viewport;
  renderer: SceneRenderer;
  readonly options: CanvasOptions;
  /** The paragraph engine screens are laid out with; made on the first `screens` message. */
  layoutEngine: DisposableEngine | null;
  /** The laid-out screens, kept so an update lays out only what changed (STU-121). */
  readonly screens: ScreenLayouts;
  /** The surface is lost: frames are skipped until it is rebuilt. */
  lost: boolean;
  readonly layerCanvas: OffscreenCanvasLike | null;
  margin: number;
  dpr: number;
  origin: BaseOrigin;
  baseDirty: boolean;
  gesture: InteractionKind | null;
  dragging: boolean;
  recentreWhenIdle: boolean;
  /** The view's movement since the previous viewport change, in world units; orders the base jobs. */
  readonly direction: { x: number; y: number };
  /** The base was realigned to `origin` by a job and the host has not been told yet. */
  originPending: boolean;
  layerSurface: Surface | null;
  layerBox: LayerBox | null;
  layerChanged: boolean;
}

type InitMessage = Extract<HostMessage, { type: "init" }>;
type InteractionMessage = Extract<HostMessage, { type: "interaction" }>;

function sizeCanvas(canvas: OffscreenCanvasLike, width: number, height: number, dpr: number): void {
  canvas.width = Math.max(1, Math.round(width * dpr));
  canvas.height = Math.max(1, Math.round(height * dpr));
}

/** The worker's message handling, independent of where it runs. */
export class CanvasWorkerCore {
  private running: Running | null = null;
  private readonly queued: HostMessage[] = [];
  private frameRequested = false;
  private idleRequested = false;
  /** An animation frame is awaited to resume patches whose frame budget is spent. */
  private pacing = false;
  private disposed = false;
  private readonly live: SceneItem[] = [];
  private readonly liveBounds = makeRect(0, 0, 0, 0);
  private readonly shown = makeRect(0, 0, 0, 0);
  private readonly focus = makeRect(0, 0, 0, 0);
  private readonly jobs: BaseJobQueue;

  constructor(
    private readonly post: (message: WorkerMessage) => void,
    private readonly deps: WorkerDeps,
  ) {
    this.jobs = new BaseJobQueue({ now: () => deps.now(), schedule: (callback) => deps.requestFrame(callback) });
  }

  /** Handles one message from the main thread. */
  async receive(data: unknown): Promise<void> {
    const message = readHostMessage(data);
    if (!message) {
      this.error("unsupported message");
      return;
    }
    if (message.type === "init") {
      await this.init(message);
    } else if (this.running) {
      this.dispatch(this.running, message);
    } else if (!this.disposed) {
      this.queued.push(message);
    }
  }

  private error(message: string): void {
    this.post({ v: PROTOCOL_VERSION, type: "error", message });
  }

  private async init(message: InitMessage): Promise<void> {
    try {
      const ck = await this.deps.loadCanvasKit();
      const { canvas, options } = message;
      const margin = overscanMargin(options.tileSize, message.dpr);
      const size = baseSize(message.width, message.height, margin);
      sizeCanvas(canvas, size.width, size.height, message.dpr);
      const { surface, info } = this.start(ck, canvas, options);
      const scene = new Scene();
      const viewport = new Viewport(message.width, message.height);
      const render = new Viewport(size.width, size.height);
      const renderer = this.makeRenderer(ck, scene, render, surface, message.dpr, info, options);
      const running: Running = {
        ck,
        canvas,
        info,
        scene,
        viewport,
        render,
        renderer,
        options,
        layoutEngine: null,
        screens: new ScreenLayouts(),
        lost: false,
        layerCanvas: message.layer ?? null,
        margin,
        dpr: message.dpr,
        origin: centredOrigin(viewport, margin, message.dpr),
        baseDirty: true,
        gesture: null,
        dragging: false,
        recentreWhenIdle: false,
        direction: { x: 0, y: 0 },
        originPending: false,
        layerSurface: null,
        layerBox: null,
        layerChanged: false,
      };
      this.running = running;
      this.deps.watchContext?.(canvas, { lost: () => this.surfaceLost(), restored: () => this.surfaceRestored() });
      this.recentre(running);
      this.post({ v: PROTOCOL_VERSION, type: "ready", renderer: info });
      for (const queued of this.queued.splice(0)) {
        this.dispatch(running, queued);
      }
      this.requestFrame();
    } catch (error) {
      this.error(error instanceof Error ? error.message : String(error));
    }
  }

  private makeRenderer(
    ck: CanvasKit,
    scene: Scene,
    render: Viewport,
    surface: Surface,
    dpr: number,
    info: RendererInfo,
    options: CanvasOptions,
  ): SceneRenderer {
    const fontMgr = options.fonts.length > 0 ? ck.FontMgr.FromData(...options.fonts) : null;
    return new SceneRenderer(
      ck,
      scene,
      render,
      surface,
      dpr,
      {
        tiles: info.tiles,
        tileSize: options.tileSize,
        background: options.background,
        fontFamily: options.fontFamily,
        fallbackFamilies: fallbackFamilies(options),
        measureFrames: options.measureFrames,
      },
      () => this.deps.now(),
      fontMgr,
    );
  }

  /**
   * The surface was lost (WebGL context loss, or the worker's surface gone):
   * stop drawing; the scene keeps taking patches, so nothing is lost with it.
   */
  surfaceLost(): void {
    const running = this.running;
    if (!running || running.lost) {
      return;
    }
    running.lost = true;
    this.jobs.cancel();
    this.post({ v: PROTOCOL_VERSION, type: "surface", state: "lost" });
  }

  /**
   * The context may be used again: rebuild the surface, the renderer and every
   * cache (tiles, paragraphs, the shadow, the interaction layer) from the
   * retained scene, and draw the whole base on the next frame.
   */
  surfaceRestored(): void {
    const running = this.running;
    if (!running?.lost) {
      return;
    }
    const surface = this.deps.makeSurface(running.ck, running.canvas, running.info.kind);
    if (!surface) {
      this.error("no CanvasKit surface after context loss");
      return;
    }
    try {
      running.renderer.dispose();
    } catch {
      // Objects of a lost context may fail to delete; they are gone with it.
    }
    running.renderer = this.makeRenderer(
      running.ck,
      running.scene,
      running.render,
      surface,
      running.dpr,
      running.info,
      running.options,
    );
    running.layerSurface = null;
    running.layerBox = null;
    running.layerChanged = true;
    running.lost = false;
    this.recentre(running);
    this.post({ v: PROTOCOL_VERSION, type: "surface", state: "restored" });
    this.requestFrame();
  }

  private start(
    ck: CanvasKit,
    canvas: OffscreenCanvasLike,
    options: CanvasOptions,
  ): { surface: Surface; info: RendererInfo } {
    const probe = this.deps.probeGl();
    let kind = chooseRenderer(options.renderer, probe);
    let surface = this.deps.makeSurface(ck, canvas, kind);
    if (!surface && kind === "webgl") {
      kind = "raster";
      surface = this.deps.makeSurface(ck, canvas, kind);
    }
    if (!surface) {
      throw new Error("no CanvasKit surface for the canvas");
    }
    const info: RendererInfo = {
      kind,
      glRenderer: probe.renderer,
      software: probe.software,
      majorPerformanceCaveat: probe.majorPerformanceCaveat,
      tiles: options.tiles ?? true,
    };
    return { surface, info };
  }

  private dispatch(running: Running, message: HostMessage): void {
    switch (message.type) {
      case "patch":
        running.scene.apply(message.ops);
        this.markBase(running);
        break;
      case "viewport": {
        const { x, y } = running.viewport;
        running.viewport.apply(message.change);
        running.direction.x = running.viewport.x - x;
        running.direction.y = running.viewport.y - y;
        this.place(running);
        if (this.jobs.active) {
          this.rebase(running);
        }
        break;
      }
      case "interaction":
        this.interaction(running, message);
        break;
      case "resize":
        this.resize(running, message.width, message.height, message.dpr);
        break;
      case "snapshot":
        this.post({
          v: PROTOCOL_VERSION,
          type: "snapshot",
          requestId: message.requestId,
          model: buildSceneModel(running.scene, running.viewport),
        });
        return;
      case "images":
        for (const image of message.images) {
          running.renderer.setImage(image.key, image.bytes);
        }
        this.markBase(running);
        break;
      case "thumbnail": {
        const item = running.scene.get(message.id);
        const png = item === undefined ? null : running.renderer.thumbnail(item.bounds, message.width);
        this.post({ v: PROTOCOL_VERSION, type: "thumbnail", requestId: message.requestId, png });
        return;
      }
      case "screens":
      case "screensUpdate": {
        running.layoutEngine ??= (this.deps.paragraphEngine ?? canvasParagraphEngine)(running.ck, running.options);
        const { ops, heights } =
          message.type === "screens"
            ? running.screens.replaceAll(message.screens, message.style, running.layoutEngine)
            : running.screens.update(message.screens, message.removed, message.style, running.layoutEngine);
        running.scene.apply(ops);
        this.markBase(running);
        this.post({ v: PROTOCOL_VERSION, type: "screens", requestId: message.requestId, ops, heights });
        break;
      }
      case "dispose":
        this.jobs.cancel();
        running.renderer.dispose();
        running.layoutEngine?.dispose();
        running.layoutEngine = null;
        this.running = null;
        this.disposed = true;
        return;
      case "init":
        return;
    }
    this.requestFrame();
  }

  private interaction(running: Running, message: InteractionMessage): void {
    const start = message.phase === "start";
    if (message.kind === "drag") {
      // Pans and zooms move the base in the compositor; only drags render tiles at reduced resolution.
      running.renderer.interacting = start;
      running.scene.setLive(message.ids, start);
      running.dragging = start && message.ids.length > 0;
      this.markBase(running);
    } else {
      running.gesture = start ? message.kind : null;
      this.place(running);
    }
  }

  /** Marks the base for re-rendering when the static content changed, or when drags draw on it. */
  private markBase(running: Running): void {
    if (running.dragging && !running.layerCanvas) {
      running.baseDirty = true;
    } else if (running.scene.hasDirty) {
      if (this.jobs.active) {
        this.rebase(running);
      } else {
        running.baseDirty = true;
      }
    }
  }

  /**
   * Keeps the base in step with the view: re-rendered now when the zoom
   * changed or the view left it outside a gesture, in idle time when a
   * gesture nears its edge or the view settles off-centre.
   */
  private place(running: Running): void {
    const zooming = running.gesture === "zoom";
    const gesturing = running.gesture !== null;
    if (needsRender(running.viewport, running.origin, running.margin, zooming)) {
      if (gesturing) {
        this.recentreLater(running);
      } else {
        this.recentre(running, true);
      }
    } else if (!gesturing && !isCentred(running.viewport, running.origin, running.margin, running.dpr)) {
      this.recentreLater(running);
    }
  }

  private recentreLater(running: Running): void {
    running.recentreWhenIdle = true;
    this.requestIdle();
  }

  /**
   * Centres the base on the view. The next frame re-renders it whole, or —
   * when `progressive` and tiled, as the old base still covers the view — it
   * is re-rendered in base jobs (`rebase`).
   */
  private recentre(running: Running, progressive = false): void {
    const origin = centredOrigin(running.viewport, running.margin, running.dpr);
    running.origin = origin;
    running.render.x = origin.x;
    running.render.y = origin.y;
    running.render.zoom = origin.zoom;
    running.recentreWhenIdle = false;
    if (progressive && running.renderer.tiled) {
      this.rebase(running);
    } else {
      this.jobs.cancel();
      running.baseDirty = true;
    }
  }

  /**
   * (Re)plans the progressive base re-render around what the view shows and
   * where it heads, replacing any queued jobs, and arms the job queue.
   */
  private rebase(running: Running): void {
    running.viewport.worldRect(this.focus);
    running.renderer.planBase(this.focus, running.direction);
    if (running.renderer.baseJobs === 0) {
      this.jobs.cancel();
      return;
    }
    this.jobs.start(
      (deadline) => {
        if (running.renderer.baseJob(deadline).shifted) {
          running.originPending = true;
        }
        return running.renderer.baseJobs > 0;
      },
      (ms, more) => this.jobDone(running, ms, more),
    );
  }

  /** Reports a slice of base jobs to the host, with the new origin once the base was realigned. */
  private jobDone(running: Running, ms: number, more: boolean): void {
    if (this.running !== running) {
      return;
    }
    const origin = running.originPending;
    running.originPending = false;
    this.post({
      v: PROTOCOL_VERSION,
      type: "frame",
      stats: running.renderer.jobStats(ms),
      ...(origin ? { origin: running.origin } : {}),
    });
    if (!more && running.renderer.hasIdleWork()) {
      this.requestIdle();
    }
  }

  private resize(running: Running, width: number, height: number, dpr: number): void {
    running.viewport.width = width;
    running.viewport.height = height;
    running.margin = overscanMargin(running.renderer.tileCache.tileSize, dpr);
    running.dpr = dpr;
    const size = baseSize(width, height, running.margin);
    running.render.width = size.width;
    running.render.height = size.height;
    sizeCanvas(running.canvas, size.width, size.height, dpr);
    const surface = this.deps.makeSurface(running.ck, running.canvas, running.info.kind);
    if (surface) {
      running.renderer.replaceSurface(surface, dpr);
    } else {
      this.error("no CanvasKit surface after resize");
    }
    this.recentre(running);
  }

  /**
   * The interaction layer for this frame while items are dragged: its box on
   * the base follows the bounds of the live items, re-allocated in steps.
   */
  private prepareLayer(running: Running): LiveLayer | null {
    const items = running.dragging && running.layerCanvas ? running.scene.liveItems(this.live) : [];
    const [first, ...rest] = items;
    if (!first || !running.layerCanvas) {
      return this.hideLayer(running);
    }
    const bounds = Object.assign(this.liveBounds, first.bounds);
    for (const item of rest) {
      unionInto(bounds, item.bounds);
    }
    // Long links reach far beyond the view; the layer covers only what is visible of them.
    if (!intersectInto(bounds, bounds, running.viewport.worldRect(this.shown))) {
      return this.hideLayer(running);
    }
    const { origin, dpr } = running;
    const box = layerBox(
      {
        x: (bounds.l - origin.x) * origin.zoom,
        y: (bounds.t - origin.y) * origin.zoom,
        w: (bounds.r - bounds.l) * origin.zoom,
        h: (bounds.b - bounds.t) * origin.zoom,
      },
      running.layerBox,
    );
    const previous = running.layerBox;
    if (box !== previous) {
      running.layerChanged = true;
      running.layerBox = box;
      if (!previous || previous.w !== box.w || previous.h !== box.h || !running.layerSurface) {
        this.allocateLayer(running, running.layerCanvas, box);
      }
    }
    const surface = running.layerSurface;
    if (!surface) {
      return null;
    }
    return { surface, x: origin.x + box.x / origin.zoom, y: origin.y + box.y / origin.zoom, scale: origin.zoom * dpr };
  }

  private hideLayer(running: Running): null {
    if (running.layerBox) {
      running.layerBox = null;
      running.layerChanged = true;
    }
    return null;
  }

  private allocateLayer(running: Running, canvas: OffscreenCanvasLike, box: LayerBox): void {
    const resources = running.renderer.resources;
    if (running.layerSurface) {
      resources.free(running.layerSurface);
      running.layerSurface = null;
    }
    sizeCanvas(canvas, box.w, box.h, running.dpr);
    // The layer is small and redrawn every frame: a raster surface avoids a second GL context.
    const surface = this.deps.makeSurface(running.ck, canvas, "layer");
    if (surface) {
      running.layerSurface = resources.own(surface);
    } else {
      this.error("no CanvasKit surface for the interaction layer");
    }
  }

  private requestFrame(): void {
    if (this.frameRequested || !this.running) {
      return;
    }
    this.frameRequested = true;
    this.deps.requestFrame(() => this.onFrame());
  }

  private onFrame(): void {
    this.frameRequested = false;
    const running = this.running;
    if (!running || running.lost) {
      return;
    }
    const layer = this.prepareLayer(running);
    const drawBase = running.baseDirty;
    running.baseDirty = false;
    if (drawBase) {
      // A whole base draw supersedes the queued jobs.
      this.jobs.cancel();
      running.originPending = false;
    }
    const stats = running.renderer.frame(drawBase, layer);
    const layerChanged = running.layerChanged;
    running.layerChanged = false;
    this.post({
      v: PROTOCOL_VERSION,
      type: "frame",
      stats,
      ...(drawBase ? { origin: running.origin } : {}),
      ...(layerChanged ? { layer: running.layerBox } : {}),
    });
    if (running.renderer.hasIdleWork()) {
      this.requestIdle();
    }
  }

  private idleAfterFrame(running: Running): void {
    if (this.pacing) {
      return;
    }
    this.pacing = true;
    this.deps.requestFrame(() => {
      this.pacing = false;
      if (this.running === running && !running.lost) {
        running.renderer.nextPatchFrame();
        this.requestIdle();
      }
    });
  }

  private requestIdle(): void {
    if (this.idleRequested) {
      return;
    }
    this.idleRequested = true;
    this.deps.requestIdle(() => this.onIdle());
  }

  private onIdle(): void {
    this.idleRequested = false;
    const running = this.running;
    if (!running) {
      return;
    }
    if (this.frameRequested) {
      // A message asked for a frame: idle work yields to it, and the frame asks for idle time again.
      return;
    }
    if (running.recentreWhenIdle) {
      this.recentre(running, true);
    }
    if (this.jobs.active) {
      // The base jobs render the tiles the base needs; `jobDone` asks for idle time again when they finish.
      return;
    }
    const rendered = running.renderer.idle(this.deps.now() + IDLE_BUDGET_MS, () => this.frameRequested);
    if (running.renderer.patchesWaitForFrame) {
      // Patches are paced to one frame's budget per animation frame: idle time resumes them after the next one.
      this.idleAfterFrame(running);
    }
    if (rendered === 0) {
      return;
    }
    // A frame follows when the view shows a new tile, or to report that the idle work is done (it draws nothing
    // then); tiles refreshed in place that the surface already shows need no frame.
    const more = running.renderer.hasIdleWork();
    if (running.renderer.idleChangedView || !more) {
      running.baseDirty = true;
      this.requestFrame();
    } else {
      this.requestIdle();
    }
  }
}

/** The parts of a worker's global scope the canvas worker uses. */
export interface WorkerScopeLike {
  onmessage: ((event: { readonly data: unknown }) => void) | null;
  postMessage(message: unknown): void;
}

/** Serves the canvas protocol on a worker scope; called by the worker entry point. */
export function serveCanvasWorker(scope: WorkerScopeLike, deps: WorkerDeps): CanvasWorkerCore {
  const core = new CanvasWorkerCore((message) => scope.postMessage(message), deps);
  scope.onmessage = (event) => {
    void core.receive(event.data);
  };
  return core;
}
