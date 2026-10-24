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
 * `compositor.ts`).
 */

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
import { makeRect, unionInto } from "./geometry.ts";
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
import { chooseRenderer, type GlProbe, measureSurface } from "./select.ts";
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
  makeSurface(ck: CanvasKit, canvas: OffscreenCanvasLike, kind: "webgl" | "raster"): Surface | null;
  makeMeasureSurface(ck: CanvasKit, kind: "webgl" | "raster"): Surface | null;
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
  readonly renderer: SceneRenderer;
  readonly layerCanvas: OffscreenCanvasLike | null;
  margin: number;
  dpr: number;
  origin: BaseOrigin;
  baseDirty: boolean;
  gesture: InteractionKind | null;
  dragging: boolean;
  recentreWhenIdle: boolean;
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
  private disposed = false;
  private readonly live: SceneItem[] = [];
  private readonly liveBounds = makeRect(0, 0, 0, 0);

  constructor(
    private readonly post: (message: WorkerMessage) => void,
    private readonly deps: WorkerDeps,
  ) {}

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
      const fontMgr = options.fonts.length > 0 ? ck.FontMgr.FromData(...options.fonts) : null;
      const scene = new Scene();
      const viewport = new Viewport(message.width, message.height);
      const render = new Viewport(size.width, size.height);
      const renderer = new SceneRenderer(
        ck,
        scene,
        render,
        surface,
        message.dpr,
        {
          tiles: info.tiles,
          tileSize: options.tileSize,
          background: options.background,
          fontFamily: options.fontFamily,
          measureFrames: options.measureFrames,
        },
        () => this.deps.now(),
        fontMgr,
      );
      const running: Running = {
        ck,
        canvas,
        info,
        scene,
        viewport,
        render,
        renderer,
        layerCanvas: message.layer ?? null,
        margin,
        dpr: message.dpr,
        origin: centredOrigin(viewport, margin, message.dpr),
        baseDirty: true,
        gesture: null,
        dragging: false,
        recentreWhenIdle: false,
        layerSurface: null,
        layerBox: null,
        layerChanged: false,
      };
      this.running = running;
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

  private start(
    ck: CanvasKit,
    canvas: OffscreenCanvasLike,
    options: CanvasOptions,
  ): { surface: Surface; info: RendererInfo } {
    const probe = this.deps.probeGl();
    const measured = options.renderer === "auto" && probe.available && probe.software ? this.measure(ck) : null;
    let kind = chooseRenderer(options.renderer, probe, measured);
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
      measuredMs: measured,
    };
    return { surface, info };
  }

  private measure(ck: CanvasKit): { webgl: number; raster: number } | null {
    const time = (kind: "webgl" | "raster"): number | null => {
      const surface = this.deps.makeMeasureSurface(ck, kind);
      if (!surface) {
        return null;
      }
      const ms = measureSurface(ck, surface, () => this.deps.now());
      surface.delete();
      return ms;
    };
    const webgl = time("webgl");
    const raster = time("raster");
    return webgl === null || raster === null ? null : { webgl, raster };
  }

  private dispatch(running: Running, message: HostMessage): void {
    switch (message.type) {
      case "patch":
        running.scene.apply(message.ops);
        this.markBase(running);
        break;
      case "viewport":
        running.viewport.apply(message.change);
        this.place(running);
        break;
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
      case "dispose":
        running.renderer.dispose();
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
    running.renderer.interacting = start;
    if (message.kind === "drag") {
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
    if (running.scene.hasDirty || (running.dragging && !running.layerCanvas)) {
      running.baseDirty = true;
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
        this.recentre(running);
      }
    } else if (!gesturing && !isCentred(running.viewport, running.origin, running.margin, running.dpr)) {
      this.recentreLater(running);
    }
  }

  private recentreLater(running: Running): void {
    running.recentreWhenIdle = true;
    this.requestIdle();
  }

  /** Centres the base on the view; the next frame re-renders it. */
  private recentre(running: Running): void {
    const origin = centredOrigin(running.viewport, running.margin, running.dpr);
    running.origin = origin;
    running.render.x = origin.x;
    running.render.y = origin.y;
    running.render.zoom = origin.zoom;
    running.recentreWhenIdle = false;
    running.baseDirty = true;
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
      if (running.layerBox) {
        running.layerBox = null;
        running.layerChanged = true;
      }
      return null;
    }
    const bounds = Object.assign(this.liveBounds, first.bounds);
    for (const item of rest) {
      unionInto(bounds, item.bounds);
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

  private allocateLayer(running: Running, canvas: OffscreenCanvasLike, box: LayerBox): void {
    const resources = running.renderer.resources;
    if (running.layerSurface) {
      resources.free(running.layerSurface);
      running.layerSurface = null;
    }
    sizeCanvas(canvas, box.w, box.h, running.dpr);
    const surface = this.deps.makeSurface(running.ck, canvas, running.info.kind);
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
    if (!running) {
      return;
    }
    const layer = this.prepareLayer(running);
    const drawBase = running.baseDirty;
    running.baseDirty = false;
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
    if (running.recentreWhenIdle) {
      this.recentre(running);
      this.requestFrame();
      return;
    }
    if (running.renderer.idle(this.deps.now() + IDLE_BUDGET_MS) > 0) {
      running.baseDirty = true;
      this.requestFrame();
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
