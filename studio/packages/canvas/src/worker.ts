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
 */

import type { CanvasKit, Surface } from "canvaskit-wasm";
import {
  type CanvasOptions,
  type HostMessage,
  type OffscreenCanvasLike,
  PROTOCOL_VERSION,
  type RendererInfo,
  readHostMessage,
  type WorkerMessage,
} from "./protocol.ts";
import { SceneRenderer } from "./renderer.ts";
import { Scene } from "./scene.ts";
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
  readonly viewport: Viewport;
  readonly renderer: SceneRenderer;
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
      sizeCanvas(canvas, message.width, message.height, message.dpr);
      const { surface, info } = this.start(ck, canvas, options);
      const fontMgr = options.fonts.length > 0 ? ck.FontMgr.FromData(...options.fonts) : null;
      const scene = new Scene();
      const viewport = new Viewport(message.width, message.height);
      const renderer = new SceneRenderer(
        ck,
        scene,
        viewport,
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
      const running: Running = { ck, canvas, info, scene, viewport, renderer };
      this.running = running;
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
        break;
      case "viewport":
        running.viewport.apply(message.change);
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
    }
  }

  private resize(running: Running, width: number, height: number, dpr: number): void {
    running.viewport.width = width;
    running.viewport.height = height;
    sizeCanvas(running.canvas, width, height, dpr);
    const surface = this.deps.makeSurface(running.ck, running.canvas, running.info.kind);
    if (surface) {
      running.renderer.replaceSurface(surface, dpr);
    } else {
      this.error("no CanvasKit surface after resize");
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
    this.post({ v: PROTOCOL_VERSION, type: "frame", stats: running.renderer.frame() });
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
    if (running.renderer.idle(this.deps.now() + IDLE_BUDGET_MS) > 0) {
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
