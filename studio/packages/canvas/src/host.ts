// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The main-thread side of Plux Canvas (STU-003): transfers the canvas to the
 * canvas worker and talks to it through the versioned protocol. Where an
 * OffscreenCanvas or a worker is unavailable, the same worker core and
 * renderer run on the main thread behind the same API.
 */

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
import { CanvasWorkerCore, type WorkerDeps } from "./worker.ts";

/** The canvas element as the host uses it. */
export interface CanvasElementLike extends OffscreenCanvasLike {
  transferControlToOffscreen?: () => OffscreenCanvasLike;
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

  constructor(canvas: CanvasElementLike, size: CanvasSize, options: CanvasOptions, deps: HostDeps) {
    this.ready = new Promise((resolve, reject) => {
      this.settle = { resolve, reject };
    });
    const offscreen =
      deps.createWorker && canvas.transferControlToOffscreen ? canvas.transferControlToOffscreen() : null;
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
    const transfer = offscreen ? [offscreen, ...options.fonts] : [];
    this.send({ v: PROTOCOL_VERSION, type: "init", canvas: target, ...size, options }, transfer);
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
    this.send({ v: PROTOCOL_VERSION, type: "viewport", change: { op: "pan", dx, dy } });
  }

  /** Zooms by `factor` about CSS point (`x`, `y`), clamped to 10–400 %. */
  zoomAbout(factor: number, x: number, y: number): void {
    this.send({ v: PROTOCOL_VERSION, type: "viewport", change: { op: "zoom", factor, x, y } });
  }

  /** Sets the view: world top-left and zoom. */
  setViewport(x: number, y: number, zoom: number): void {
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
