// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The main-thread side of Plux Canvas (STU-003): transfers the canvas to the
 * canvas worker and talks to it through the versioned protocol. Where an
 * OffscreenCanvas or a worker is unavailable, the same worker core and
 * renderer run on the main thread behind the same API.
 *
 * The host owns the CSS transform of the canvas elements (M-12): it keeps a
 * mirror of the viewport, applies every pan and zoom to it at once and moves
 * the overscanned base element with `translate3d()`/`scale()`, so a pan costs
 * the main thread a style write and the compositor a move, and no GL work. The
 * worker reports where each re-rendered base lies; the host re-derives the
 * transform from it. The elements sit absolutely positioned at the top-left of
 * a container that clips them (`overflow: hidden`) and has the canvas size.
 */

import {
  type BaseOrigin,
  baseSize,
  centredOrigin,
  type ElementTransform,
  type LayerBox,
  overscanMargin,
  transformCss,
  type ViewLike,
} from "./compositor.ts";
import {
  type CanvasOptions,
  type FrameStats,
  type HostMessage,
  type InteractionKind,
  type OffscreenCanvasLike,
  PROTOCOL_VERSION,
  type RendererInfo,
  readWorkerMessage,
  type SceneModel,
  type SceneOp,
} from "./protocol.ts";
import { Viewport } from "./viewport.ts";
import { CanvasWorkerCore, type WorkerDeps } from "./worker.ts";

/** The inline style properties the host sets; a `CSSStyleDeclaration` in the browser. */
export interface StyleLike {
  position: string;
  left: string;
  top: string;
  width: string;
  height: string;
  transform: string;
  transformOrigin: string;
  willChange: string;
  pointerEvents: string;
  display: string;
}

/** The canvas element as the host uses it. */
export interface CanvasElementLike extends OffscreenCanvasLike {
  transferControlToOffscreen?: () => OffscreenCanvasLike;
  readonly style?: StyleLike;
}

/** A dedicated worker as the host uses it. */
export interface WorkerLike {
  onmessage: ((event: { readonly data: unknown }) => void) | null;
  postMessage(message: unknown, transfer: unknown[]): void;
  terminate(): void;
}

/** How the host starts the worker, or the main-thread fallback. */
export interface HostDeps {
  readonly createWorker?: () => WorkerLike;
  readonly mainThread: () => WorkerDeps;
}

/** The canvas size in CSS pixels and the device-pixel ratio. */
export interface CanvasSize {
  readonly width: number;
  readonly height: number;
  readonly dpr: number;
}

type Post = (message: HostMessage, transfer: unknown[]) => void;

/** The main-thread handle of one canvas. */
export class CanvasHost {
  /** Resolves with the chosen renderer once the worker is ready. */
  readonly ready: Promise<RendererInfo>;
  /** Where the renderer runs. */
  readonly mode: "worker" | "main-thread";
  private readonly post: Post;
  private readonly worker: WorkerLike | null;
  private readonly frameListeners = new Set<(stats: FrameStats) => void>();
  private readonly snapshots = new Map<number, (model: SceneModel) => void>();
  private nextRequest = 0;
  private settle: { resolve(info: RendererInfo): void; reject(error: Error): void } | null = null;
  private readonly view: Viewport;
  private readonly base: CanvasElementLike;
  private readonly layerElement: CanvasElementLike | null;
  private readonly tileSize: number;
  private margin: number;
  private dpr: number;
  private origin: BaseOrigin;
  private box: LayerBox | null = null;

  /**
   * Starts the canvas on `canvas`, with `layer` — a second canvas element
   * stacked above it — as the interaction layer for drags.
   */
  constructor(
    canvas: CanvasElementLike,
    size: CanvasSize,
    options: CanvasOptions,
    deps: HostDeps,
    layer: CanvasElementLike | null = null,
  ) {
    this.ready = new Promise((resolve, reject) => {
      this.settle = { resolve, reject };
    });
    this.view = new Viewport(size.width, size.height);
    this.base = canvas;
    this.layerElement = layer;
    this.dpr = size.dpr;
    this.tileSize = options.tileSize;
    this.margin = overscanMargin(options.tileSize, size.dpr);
    this.origin = centredOrigin(this.view, this.margin, this.dpr);
    this.styleElements();
    const offscreen =
      deps.createWorker && canvas.transferControlToOffscreen ? canvas.transferControlToOffscreen() : null;
    const layerTarget = offscreen && layer?.transferControlToOffscreen ? layer.transferControlToOffscreen() : layer;
    let target: OffscreenCanvasLike = canvas;
    if (offscreen && deps.createWorker) {
      const worker = deps.createWorker();
      worker.onmessage = (event) => this.receive(event.data);
      this.worker = worker;
      this.post = (message, transfer) => worker.postMessage(message, transfer);
      this.mode = "worker";
      target = offscreen;
    } else {
      const core = new CanvasWorkerCore((message) => this.receive(message), deps.mainThread());
      this.worker = null;
      this.post = (message) => {
        void core.receive(message);
      };
      this.mode = "main-thread";
    }
    const transfer = offscreen ? [offscreen, ...(layerTarget ? [layerTarget] : []), ...options.fonts] : [];
    this.send(
      {
        v: PROTOCOL_VERSION,
        type: "init",
        canvas: target,
        ...(layerTarget ? { layer: layerTarget } : {}),
        ...size,
        options,
      },
      transfer,
    );
  }

  /** The view as the host shows it; equal to the scene model's viewport. */
  get viewport(): ViewLike {
    return this.view;
  }

  /** The current transform of the base element. */
  get transform(): ElementTransform {
    const { x, y, zoom } = this.origin;
    return {
      tx: (x - this.view.x) * this.view.zoom,
      ty: (y - this.view.y) * this.view.zoom,
      scale: this.view.zoom / zoom,
    };
  }

  /** The interaction layer's box on the base, or null while it is hidden. */
  get layerBox(): LayerBox | null {
    return this.box;
  }

  private styleElements(): void {
    const size = baseSize(this.view.width, this.view.height, this.margin);
    for (const element of [this.base, this.layerElement]) {
      const style = element?.style;
      if (!style) {
        continue;
      }
      style.position = "absolute";
      style.left = "0";
      style.top = "0";
      style.transformOrigin = "0 0";
      style.willChange = "transform";
      style.pointerEvents = element === this.base ? style.pointerEvents : "none";
    }
    const style = this.base.style;
    if (style) {
      style.width = `${size.width}px`;
      style.height = `${size.height}px`;
    }
    this.applyTransform();
  }

  /** Moves the elements so the base shows the mirrored view. */
  private applyTransform(): void {
    const transform = this.transform;
    const base = this.base.style;
    if (base) {
      base.transform = transformCss(transform);
    }
    const layer = this.layerElement?.style;
    if (!layer) {
      return;
    }
    const box = this.box;
    layer.display = box ? "block" : "none";
    if (box) {
      layer.width = `${box.w}px`;
      layer.height = `${box.h}px`;
      layer.transform = transformCss(transform, box.x, box.y);
    }
  }

  private send(message: HostMessage, transfer: unknown[] = []): void {
    this.post(message, transfer);
  }

  private receive(data: unknown): void {
    const message = readWorkerMessage(data);
    switch (message?.type) {
      case "ready":
        this.settle?.resolve(message.renderer);
        break;
      case "frame":
        if (message.origin) {
          this.origin = message.origin;
        }
        if (message.layer !== undefined) {
          this.box = message.layer;
        }
        if (message.origin || message.layer !== undefined) {
          this.applyTransform();
        }
        for (const listener of [...this.frameListeners]) {
          listener(message.stats);
        }
        break;
      case "snapshot":
        this.snapshots.get(message.requestId)?.(message.model);
        this.snapshots.delete(message.requestId);
        break;
      case "error":
        this.settle?.reject(new Error(message.message));
        break;
      default:
        break;
    }
  }

  /** Applies scene patches. */
  patch(ops: readonly SceneOp[]): void {
    this.send({ v: PROTOCOL_VERSION, type: "patch", ops });
  }

  /** Pans by CSS pixels. */
  pan(dx: number, dy: number): void {
    this.view.pan(dx, dy);
    this.applyTransform();
    this.send({ v: PROTOCOL_VERSION, type: "viewport", change: { op: "pan", dx, dy } });
  }

  /** Zooms by `factor` about CSS point (`x`, `y`), clamped to 10–400 %. */
  zoomAbout(factor: number, x: number, y: number): void {
    this.view.zoomAbout(factor, x, y);
    this.applyTransform();
    this.send({ v: PROTOCOL_VERSION, type: "viewport", change: { op: "zoom", factor, x, y } });
  }

  /** Sets the view: world top-left and zoom. */
  setViewport(x: number, y: number, zoom: number): void {
    this.view.apply({ op: "set", x, y, zoom });
    this.applyTransform();
    this.send({ v: PROTOCOL_VERSION, type: "viewport", change: { op: "set", x, y, zoom } });
  }

  /** Starts an interaction; for a drag, `ids` are the items that move. */
  startInteraction(kind: InteractionKind, ids: readonly string[] = []): void {
    this.send({ v: PROTOCOL_VERSION, type: "interaction", phase: "start", kind, ids });
  }

  /** Ends an interaction. */
  endInteraction(kind: InteractionKind, ids: readonly string[] = []): void {
    this.send({ v: PROTOCOL_VERSION, type: "interaction", phase: "end", kind, ids });
  }

  /** Resizes the canvas. */
  resize(width: number, height: number, dpr: number): void {
    this.view.width = width;
    this.view.height = height;
    this.margin = overscanMargin(this.tileSize, dpr);
    this.dpr = dpr;
    this.styleElements();
    this.send({ v: PROTOCOL_VERSION, type: "resize", width, height, dpr });
  }

  /** A snapshot of the scene model. */
  snapshot(): Promise<SceneModel> {
    const requestId = this.nextRequest++;
    return new Promise((resolve) => {
      this.snapshots.set(requestId, resolve);
      this.send({ v: PROTOCOL_VERSION, type: "snapshot", requestId });
    });
  }

  /** Calls `listener` with every frame's statistics; returns the unsubscribe function. */
  onFrame(listener: (stats: FrameStats) => void): () => void {
    this.frameListeners.add(listener);
    return () => this.frameListeners.delete(listener);
  }

  /** The statistics of the next frame. */
  nextFrame(): Promise<FrameStats> {
    return new Promise((resolve) => {
      const stop = this.onFrame((stats) => {
        stop();
        resolve(stats);
      });
    });
  }

  /** Disposes the renderer and stops the worker. */
  dispose(): void {
    this.send({ v: PROTOCOL_VERSION, type: "dispose" });
    this.worker?.terminate();
    this.frameListeners.clear();
  }
}
