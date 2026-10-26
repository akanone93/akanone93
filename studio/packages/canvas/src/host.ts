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

import type { AccessibleTree } from "./a11y.ts";
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
  AUTHORITATIVE_NOTICE,
  type CanvasNotice,
  complexScriptOf,
  type FontLoadDeps,
  type FontRequest,
  loadCanvasFonts,
} from "./fonts.ts";
import { makeRect, type Rect } from "./geometry.ts";
import { HitIndex, type RectMode } from "./hit.ts";
import type { Guides, SelectionOverlay } from "./overlay.ts";
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
  type ScreenDocument,
  type ScreenHeight,
  type ScreenStyle,
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

/** The main-thread layers beside the canvas: the selection overlay and the accessible tree. */
export interface HostLayers {
  readonly overlay?: SelectionOverlay;
  readonly tree?: AccessibleTree;
}

/** An event of the canvas for the UI. */
export type HostEvent =
  | { readonly type: "notice"; readonly notice: CanvasNotice }
  | { readonly type: "surface"; readonly state: "lost" | "restored" };

/** Hit areas are widened by this many CSS pixels, at every zoom. */
export const HIT_TOLERANCE = 3;

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
  private readonly thumbnails = new Map<number, (png: Uint8Array | null) => void>();
  private readonly layouts = new Map<number, (heights: readonly ScreenHeight[]) => void>();
  private nextRequest = 0;
  /** The screens last handed to the worker and their style; an update sends only what differs (STU-121). */
  private sent: { readonly style: ScreenStyle; readonly screens: ReadonlyMap<string, ScreenDocument> } | null = null;
  /** Each laid-out screen's content height, by screen id. */
  private readonly contentHeights = new Map<string, number>();
  private settle: { resolve(info: RendererInfo): void; reject(error: Error): void } | null = null;
  private readonly view: Viewport;
  private readonly base: CanvasElementLike;
  private readonly layerElement: CanvasElementLike | null;
  private readonly tileSize: number;
  private margin: number;
  private dpr: number;
  private origin: BaseOrigin;
  private box: LayerBox | null = null;
  private readonly hits = new HitIndex();
  private readonly layers: HostLayers;
  private readonly eventListeners = new Set<(event: HostEvent) => void>();
  private readonly notices: CanvasNotice[] = [];
  private readonly noticed = new Set<string>();
  private selected: readonly string[] = [];
  private guides: Guides | null = null;

  /**
   * Loads the fonts of `request` (STU-006) and then starts the canvas, so no
   * text is laid out before its fonts; the loader's notices are raised as
   * host events. Fonts that fail to load fall back to the next family.
   */
  static async create(
    canvas: CanvasElementLike,
    size: CanvasSize,
    options: CanvasOptions,
    deps: HostDeps,
    fonts: { readonly request: FontRequest; readonly deps: FontLoadDeps },
    layer: CanvasElementLike | null = null,
    layers: HostLayers = {},
  ): Promise<CanvasHost> {
    const loaded = await loadCanvasFonts(fonts.request, fonts.deps, options.fontFamily);
    const host = new CanvasHost(
      canvas,
      size,
      {
        ...options,
        fonts: [...options.fonts, ...loaded.buffers],
        fontFamilies: [
          ...(options.fontFamilies ?? options.fonts.map(() => options.fontFamily)),
          ...loaded.bufferFamilies,
        ],
        fontFamily: loaded.primary,
      },
      deps,
      layer,
      layers,
    );
    for (const notice of loaded.notices) {
      host.raise(notice);
    }
    return host;
  }

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
    layers: HostLayers = {},
  ) {
    this.layers = layers;
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
    this.layers.overlay?.resize(size.width, size.height, size.dpr);
    this.layers.tree?.setView(this.view.x, this.view.y, this.view.zoom);
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

  /** Where the base surface shown now was rendered. */
  get baseOrigin(): BaseOrigin {
    return this.origin;
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
      case "thumbnail":
        this.thumbnails.get(message.requestId)?.(message.png);
        this.thumbnails.delete(message.requestId);
        break;
      case "screens":
        this.follow(message.ops);
        this.layouts.get(message.requestId)?.(message.heights);
        this.layouts.delete(message.requestId);
        break;
      case "surface":
        this.emit({ type: "surface", state: message.state });
        break;
      case "error":
        this.settle?.reject(new Error(message.message));
        break;
      default:
        break;
    }
  }

  /** Applies scene patches; the hit index and accessible tree follow at once. */
  patch(ops: readonly SceneOp[]): void {
    this.send({ v: PROTOCOL_VERSION, type: "patch", ops });
    this.follow(ops);
  }

  /**
   * Shows `screens`, laid out in the worker with SkParagraph (ADR-0013);
   * resolves with each screen's content height once the hit index and
   * accessible tree follow the laid-out items. The first call, and any call in
   * another style, replaces the scene; later calls send only the screens that
   * differ from the last call (another id, title, place or root object) and
   * the ids that are gone, so the worker lays out and redraws only those
   * (STU-121). Edits must therefore replace a changed root, never mutate it.
   */
  layoutScreens(screens: readonly ScreenDocument[], style: ScreenStyle): Promise<readonly ScreenHeight[]> {
    const requestId = this.nextRequest++;
    const previous = this.sent;
    const next = new Map(screens.map((s) => [s.id, s]));
    this.sent = { style, screens: next };
    const promise = new Promise<readonly ScreenHeight[]>((resolve) => {
      this.layouts.set(requestId, (heights) => {
        for (const h of heights) {
          this.contentHeights.set(h.id, h.contentHeight);
        }
        resolve(screens.map((s) => ({ id: s.id, contentHeight: this.contentHeights.get(s.id) ?? 0 })));
      });
    });
    if (previous === null || previous.style !== style) {
      this.contentHeights.clear();
      this.send({ v: PROTOCOL_VERSION, type: "screens", requestId, screens, style });
      return promise;
    }
    const changed = screens.filter((s) => !sameScreen(previous.screens.get(s.id), s));
    const removed = [...previous.screens.keys()].filter((id) => !next.has(id));
    for (const id of removed) {
      this.contentHeights.delete(id);
    }
    this.send({ v: PROTOCOL_VERSION, type: "screensUpdate", requestId, screens: changed, removed, style });
    return promise;
  }

  /** The hit index, accessible tree and notices follow `ops`, which the worker applied. */
  private follow(ops: readonly SceneOp[]): void {
    this.hits.apply(ops);
    this.layers.tree?.apply(ops);
    for (const op of ops) {
      if ((op.op === "add" || op.op === "replace") && op.item.kind === "text") {
        const script = complexScriptOf(op.item.text);
        if (script) {
          this.raise({ code: AUTHORITATIVE_NOTICE, reason: "complex-script", subject: script });
        }
      }
    }
    if (this.selected.length > 0) {
      this.selected = this.selected.filter((id) => this.hits.spec(id) !== undefined);
      this.drawOverlay();
    }
  }

  /** The world point under CSS point (`x`, `y`) of the canvas. */
  toWorld(x: number, y: number): { x: number; y: number } {
    return { x: this.view.x + x / this.view.zoom, y: this.view.y + y / this.view.zoom };
  }

  /**
   * The ids of the items under CSS point (`x`, `y`), topmost first, answered
   * on the main thread without a render round-trip.
   */
  hitTest(x: number, y: number, tolerance = HIT_TOLERANCE): string[] {
    const world = this.toWorld(x, y);
    return this.hits.hitPoint(world.x, world.y, tolerance / this.view.zoom);
  }

  /** The ids of the items meeting the CSS rectangle `rect` (e.g. a marquee), topmost first. */
  hitTestRect(rect: Rect, mode: RectMode = "intersect"): string[] {
    const a = this.toWorld(Math.min(rect.l, rect.r), Math.min(rect.t, rect.b));
    const b = this.toWorld(Math.max(rect.l, rect.r), Math.max(rect.t, rect.b));
    return this.hits.hitRect(makeRect(a.x, a.y, b.x, b.y), mode);
  }

  /** The world rectangle of item `id`, or undefined when there is no such item. */
  boundsOf(id: string): Rect | undefined {
    return this.hits.spec(id) === undefined ? undefined : (this.hits.shapeOf(id, makeRect(0, 0, 0, 0)) ?? undefined);
  }

  /** The selected item ids. */
  get selection(): readonly string[] {
    return this.selected;
  }

  /**
   * Selects `ids` (unknown ids are ignored). Only the overlay is redrawn: the
   * worker is not told, and the base is never re-rendered for a selection.
   */
  setSelection(ids: readonly string[]): void {
    this.selected = ids.filter((id) => this.hits.spec(id) !== undefined);
    this.drawOverlay();
  }

  /**
   * Shows smart guides, distance labels and an insertion indicator (world coordinates) on the overlay;
   * `null` hides them. Only the overlay is redrawn; the worker is not told.
   */
  setGuides(guides: Guides | null): void {
    this.guides = guides;
    this.drawOverlay();
  }

  private drawOverlay(): void {
    const overlay = this.layers.overlay;
    if (!overlay) {
      return;
    }
    const boxes: Rect[] = [];
    for (const id of this.selected) {
      const box = this.hits.shapeOf(id, makeRect(0, 0, 0, 0));
      if (box) {
        boxes.push(box);
      }
    }
    overlay.draw(this.view, this.dpr, boxes, this.guides ?? undefined);
  }

  /** The view changed: the overlay and the accessible tree follow it. */
  private followView(): void {
    this.layers.tree?.setView(this.view.x, this.view.y, this.view.zoom);
    if (this.selected.length > 0 || this.guides !== null) {
      this.drawOverlay();
    }
  }

  /** Calls `listener` with every host event, first replaying the notices raised so far. */
  onEvent(listener: (event: HostEvent) => void): () => void {
    for (const notice of this.notices) {
      listener({ type: "notice", notice });
    }
    this.eventListeners.add(listener);
    return () => this.eventListeners.delete(listener);
  }

  /** The notices raised so far: "device preview is authoritative", with the reasons. */
  get raisedNotices(): readonly CanvasNotice[] {
    return this.notices;
  }

  private raise(notice: CanvasNotice): void {
    const key = `${notice.reason}:${notice.subject}`;
    if (this.noticed.has(key)) {
      return;
    }
    this.noticed.add(key);
    this.notices.push(notice);
    this.emit({ type: "notice", notice });
  }

  private emit(event: HostEvent): void {
    for (const listener of [...this.eventListeners]) {
      listener(event);
    }
  }

  /** Pans by CSS pixels. */
  pan(dx: number, dy: number): void {
    this.view.pan(dx, dy);
    this.applyTransform();
    this.followView();
    this.send({ v: PROTOCOL_VERSION, type: "viewport", change: { op: "pan", dx, dy } });
  }

  /** Zooms by `factor` about CSS point (`x`, `y`), clamped to 10–400 %. */
  zoomAbout(factor: number, x: number, y: number): void {
    this.view.zoomAbout(factor, x, y);
    this.applyTransform();
    this.followView();
    this.send({ v: PROTOCOL_VERSION, type: "viewport", change: { op: "zoom", factor, x, y } });
  }

  /** Sets the view: world top-left and zoom. */
  setViewport(x: number, y: number, zoom: number): void {
    this.view.apply({ op: "set", x, y, zoom });
    this.applyTransform();
    this.followView();
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
    this.layers.overlay?.resize(width, height, dpr);
    this.followView();
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

  /** Makes encoded images (PNG) available to scene items under their keys (`RectSpec.image`). */
  setImages(images: readonly { key: string; bytes: Uint8Array }[]): void {
    this.send({ v: PROTOCOL_VERSION, type: "images", images });
  }

  /**
   * The PNG of the item `id` rendered in the worker, `width` pixels wide, from the scene the canvas
   * already holds (STU-052); null when the item is unknown. The main thread does no raster work.
   */
  thumbnail(id: string, width: number): Promise<Uint8Array | null> {
    const requestId = this.nextRequest++;
    return new Promise((resolve) => {
      this.thumbnails.set(requestId, resolve);
      this.send({ v: PROTOCOL_VERSION, type: "thumbnail", requestId, id, width });
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
    this.eventListeners.clear();
  }
}

/** `b` lays out as `a` did: same title and place, and the same root object. */
function sameScreen(a: ScreenDocument | undefined, b: ScreenDocument): boolean {
  return a !== undefined && (a === b || (a.title === b.title && a.x === b.x && a.y === b.y && a.root === b.root));
}
