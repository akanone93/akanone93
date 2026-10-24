// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The Plux Canvas renderer on CanvasKit, designed for machines without a GPU
 * (ADR-0013 M-12, STU-045, STU-120, STU-121):
 *
 * - batching: one path per layer and style (`batch.ts`);
 * - a tile cache of the static scene, blitted while panning and rendered
 *   from an idle-time queue (`idle`), never inside a frame unless nothing
 *   else can be shown;
 * - dirty-region redraw: a changed area of a cached tile is redrawn clipped
 *   on top of the tile until the tile is re-rendered; dragged ("live") items
 *   are excluded from tiles and drawn on top each frame;
 * - level of detail by zoom (`lod.ts`), with pre-rendered nine-patch shadow
 *   images instead of blur;
 * - tiles rendered at half resolution while the user interacts, refined
 *   when idle.
 *
 * Without tiles (CPU raster, where image blits are slow) each frame draws the
 * visible items directly, batched and with level of detail.
 */

import type { Canvas, CanvasKit, FontMgr, Image, Paint, Paragraph, Surface } from "canvaskit-wasm";
import { Batches, BatchLayer, color4f } from "./batch.ts";
import { intersectInto, makeRect, type Rect, setRect, unionInto } from "./geometry.ts";
import { type Lod, lodForZoom, resolutionFor } from "./lod.ts";
import { type Direction, orderJobs } from "./progressive.ts";
import type { FrameSpec, FrameStats, ItemSpec, TextSpec } from "./protocol.ts";
import { Resources } from "./resources.ts";
import type { Scene, SceneItem } from "./scene.ts";
import { type Tile, TileCache } from "./tiles.ts";
import type { Viewport } from "./viewport.ts";

/** Options of a renderer. */
export interface RendererOptions {
  readonly tiles: boolean;
  readonly tileSize: number;
  readonly background: number;
  readonly fontFamily: string;
  readonly measureFrames: boolean;
}

/** Where the base surface's pixels were drawn: the render viewport's origin and zoom. */
interface Placement {
  readonly x: number;
  readonly y: number;
  readonly zoom: number;
}

/** The outcome of one progressive base job. */
export interface BaseJobResult {
  /** The job's duration, in ms; 0 when nothing was left to do. */
  readonly ms: number;
  /** The old base was realigned to the current placement, which the host now shows. */
  readonly shifted: boolean;
}

/** Above this many dirty regions in a frame they are merged into one. */
export const MAX_REGIONS = 4;

/** Where the interaction layer draws: its surface, the world point at its top-left, and its scale. */
export interface LiveLayer {
  readonly surface: Surface;
  readonly x: number;
  readonly y: number;
  readonly scale: number;
}
const SHADOW_SIZE = 64;
const SHADOW_INSET = 16;

interface CachedParagraph {
  readonly spec: TextSpec;
  readonly paragraph: Paragraph;
}

interface FrameCounters {
  tilesDrawn: number;
  staleTilesDrawn: number;
  tileMs: number;
  regionMs: number;
  liveMs: number;
  liveItems: number;
  layerMs: number;
}

/** Renders a `Scene` through a `Viewport` onto a CanvasKit surface. */
export class SceneRenderer {
  /** Every long-lived CanvasKit object the renderer owns. */
  readonly resources = new Resources();
  /** The user is panning, zooming or dragging: tiles render at reduced resolution. */
  interacting = false;
  private surface: Surface;
  private tileSurface: Surface | null = null;
  private dpr: number;
  private readonly tiles: TileCache<Image>;
  private readonly batches: Batches;
  private readonly paragraphs = new Map<string, CachedParagraph>();
  private readonly fontMgr: FontMgr | null;
  private readonly background: Float32Array;
  private readonly backgroundPaint: Paint;
  private readonly shadow: Image | null;
  private readonly items: SceneItem[] = [];
  private readonly texts: SceneItem[] = [];
  private readonly visible: Tile<Image>[] = [];
  private readonly scratchTiles: Tile<Image>[] = [];
  private readonly pendingTiles: Tile<Image>[] = [];
  private readonly regions: Rect[] = Array.from({ length: MAX_REGIONS }, () => makeRect(0, 0, 0, 0));
  private regionCount = 0;
  private readonly view = makeRect(0, 0, 0, 0);
  private readonly tileWorld = makeRect(0, 0, 0, 0);
  private readonly keep = makeRect(0, 0, 0, 0);
  private readonly merge = makeRect(0, 0, 0, 0);
  private readonly tileTarget = makeRect(0, 0, 0, 0);
  private readonly src = new Float32Array(4);
  private readonly dst = new Float32Array(4);
  private readonly irect = new Int32Array(4);
  private readonly shadowCenter = Int32Array.of(
    SHADOW_INSET + 4,
    SHADOW_INSET + 4,
    SHADOW_SIZE - SHADOW_INSET - 4,
    SHADOW_SIZE - SHADOW_INSET - 4,
  );
  private readonly counters: FrameCounters = {
    tilesDrawn: 0,
    staleTilesDrawn: 0,
    tileMs: 0,
    regionMs: 0,
    liveMs: 0,
    layerMs: 0,
    liveItems: 0,
  };
  private lod: Lod = lodForZoom(1);
  private calls = 0;
  private frameNo = 0;
  private tileRenders = 0;
  private tileRenderMs = 0;
  /** The placement the base pixels were drawn at; null before the first base job. */
  private shown: Placement | null = null;
  /** The tiles whose current image is on the base surface. */
  private readonly shownTiles = new Set<Tile<Image>>();
  private readonly baseQueue: Tile<Image>[] = [];
  private readonly oldBase = makeRect(0, 0, 0, 0);
  private readonly invalidate = (rect: Rect): void => {
    this.tiles.invalidate(rect);
  };
  private readonly forget = (id: string): void => {
    const cached = this.paragraphs.get(id);
    if (cached) {
      this.resources.free(cached.paragraph);
      this.paragraphs.delete(id);
    }
  };

  constructor(
    private readonly ck: CanvasKit,
    private readonly scene: Scene,
    private readonly viewport: Viewport,
    surface: Surface,
    dpr: number,
    private readonly options: RendererOptions,
    private readonly now: () => number,
    fontMgr: FontMgr | null,
  ) {
    this.fontMgr = fontMgr ? this.resources.own(fontMgr) : null;
    this.tiles = new TileCache<Image>(options.tileSize, (image) => this.resources.free(image));
    this.batches = new Batches(ck, this.resources);
    this.background = color4f(ck, options.background);
    this.backgroundPaint = this.resources.own(new ck.Paint());
    this.backgroundPaint.setColor(this.background);
    this.shadow = this.makeShadow();
    this.surface = this.resources.own(surface);
    this.dpr = dpr;
    this.tileSurface = this.makeTileSurface();
  }

  /** Whether tiles are in use. */
  get tiled(): boolean {
    return this.options.tiles;
  }

  /** The tile cache, for tests and statistics. */
  get tileCache(): TileCache<Image> {
    return this.tiles;
  }

  /** Replaces the surface (resize, context loss); tiles of the old surface are dropped. */
  replaceSurface(surface: Surface, dpr: number): void {
    this.tiles.clear();
    if (this.tileSurface) {
      this.resources.free(this.tileSurface);
    }
    this.resources.free(this.surface);
    this.surface = this.resources.own(surface);
    this.shown = null;
    this.shownTiles.clear();
    this.baseQueue.length = 0;
    this.dpr = dpr;
    this.tileSurface = this.makeTileSurface();
  }

  private makeTileSurface(): Surface | null {
    if (!this.options.tiles) {
      return null;
    }
    const ck = this.ck;
    const size = this.options.tileSize;
    return this.resources.own(
      this.surface.makeSurface({
        width: size,
        height: size,
        colorType: ck.ColorType.RGBA_8888,
        alphaType: ck.AlphaType.Premul,
        colorSpace: ck.ColorSpace.SRGB,
      }),
    );
  }

  private makeShadow(): Image | null {
    const ck = this.ck;
    const surface = ck.MakeSurface(SHADOW_SIZE, SHADOW_SIZE);
    if (!surface) {
      return null;
    }
    const paint = new ck.Paint();
    const blur = ck.MaskFilter.MakeBlur(ck.BlurStyle.Normal, 6, false);
    paint.setAntiAlias(true);
    paint.setColor(ck.Color4f(0, 0, 0, 0.22));
    paint.setMaskFilter(blur);
    const edge = SHADOW_SIZE - SHADOW_INSET;
    surface.getCanvas().drawRRect(ck.RRectXY(ck.LTRBRect(SHADOW_INSET, SHADOW_INSET, edge, edge), 8, 8), paint);
    const image = surface.makeImageSnapshot();
    paint.delete();
    blur.delete();
    surface.delete();
    return this.resources.own(image);
  }

  /**
   * Draws one frame and returns its statistics: the base surface when
   * `drawBase`, and the live (dragged) items into `layer` when given — then
   * the base leaves them out (M-12).
   */
  frame(drawBase = true, layer: LiveLayer | null = null): FrameStats {
    const start = this.now();
    this.calls = 0;
    this.resetCounters();
    this.scene.drainDirty(this.invalidate);
    this.scene.drainRemoved(this.forget);
    this.lod = lodForZoom(this.viewport.zoom);
    this.viewport.worldRect(this.view);
    let flushMs = 0;
    if (drawBase) {
      flushMs = this.drawBase(layer === null);
    }
    if (layer) {
      this.drawLayer(layer);
    }
    const end = this.now();
    return this.stats(end - start, flushMs, drawBase);
  }

  /** Draws the base surface; returns the flush time. */
  private drawBase(includeLive: boolean): number {
    const start = this.now();
    const canvas = this.surface.getCanvas();
    const scale = this.viewport.zoom * this.dpr;
    if (!this.options.tiles) {
      canvas.clear(this.background);
      this.calls++;
    }
    canvas.save();
    canvas.scale(scale, scale);
    canvas.translate(-Math.round(this.viewport.x * scale) / scale, -Math.round(this.viewport.y * scale) / scale);
    if (this.options.tiles) {
      this.drawTiled(canvas, scale, includeLive);
    } else {
      this.drawScene(canvas, this.view, includeLive);
      this.counters.tileMs = this.now() - start;
    }
    canvas.restore();
    return this.flushBase(canvas);
  }

  /** Flushes the base surface (and waits for the GPU when measuring); returns the time taken. */
  private flushBase(canvas: Canvas): number {
    const flushStart = this.now();
    this.surface.flush();
    if (this.options.measureFrames) {
      this.syncGpu(canvas);
    }
    return this.now() - flushStart;
  }

  /** Draws the live items into the interaction layer, on a transparent background. */
  private drawLayer(layer: LiveLayer): void {
    const start = this.now();
    const canvas = layer.surface.getCanvas();
    canvas.clear(this.ck.TRANSPARENT);
    canvas.save();
    canvas.scale(layer.scale, layer.scale);
    canvas.translate(-layer.x, -layer.y);
    this.drawScene(canvas, this.view, false, this.scene.liveItems(this.items));
    this.counters.liveItems = this.items.length;
    canvas.restore();
    layer.surface.flush();
    if (this.options.measureFrames) {
      this.syncGpu(canvas);
    }
    this.counters.layerMs = this.now() - start;
  }

  private resetCounters(): void {
    const c = this.counters;
    c.tilesDrawn = 0;
    c.staleTilesDrawn = 0;
    c.tileMs = 0;
    c.regionMs = 0;
    c.liveMs = 0;
    c.liveItems = 0;
    c.layerMs = 0;
  }

  private syncGpu(canvas: Canvas): void {
    const ck = this.ck;
    canvas.readPixels(0, 0, {
      width: 1,
      height: 1,
      colorType: ck.ColorType.RGBA_8888,
      alphaType: ck.AlphaType.Premul,
      colorSpace: ck.ColorSpace.SRGB,
    });
  }

  private stats(ms: number, flushMs: number, base: boolean, jobMs = 0): FrameStats {
    const c = this.counters;
    return {
      frame: this.frameNo++,
      ms,
      drawCalls: this.calls,
      tilesDrawn: c.tilesDrawn,
      staleTilesDrawn: c.staleTilesDrawn,
      regions: this.regionCount,
      liveItems: c.liveItems,
      idleQueue: this.idleQueue(),
      resolution: resolutionFor(this.interacting),
      tileMs: c.tileMs,
      regionMs: c.regionMs,
      liveMs: c.liveMs,
      flushMs,
      tileRenders: this.tileRenders,
      tileRenderMs: this.tileRenderMs,
      base,
      layerMs: c.layerMs,
      jobMs,
      baseJobs: this.baseJobs,
    };
  }

  private drawTiled(canvas: Canvas, scale: number, includeLive: boolean): void {
    const t0 = this.now();
    this.tiles.setScale(scale);
    if (!this.covered()) {
      canvas.clear(this.background);
      this.calls++;
    }
    for (const tile of this.tiles.stale(this.view, this.scratchTiles)) {
      this.blit(canvas, tile, true);
      this.counters.staleTilesDrawn++;
    }
    this.regionCount = 0;
    for (const tile of this.tiles.collect(this.view, this.visible)) {
      this.tiles.tileRect(tile, this.tileWorld);
      if (tile.image !== null) {
        this.blit(canvas, tile, false);
        this.counters.tilesDrawn++;
        if (tile.dirty) {
          this.addRegion(tile.dirty, this.tileWorld);
        }
      } else if (!this.tiles.hasStale) {
        this.addRegion(this.tileWorld, this.view);
      }
    }
    const t1 = this.now();
    this.drawRegions(canvas);
    const t2 = this.now();
    if (includeLive) {
      this.drawScene(canvas, this.view, false, this.scene.liveItems(this.items));
      this.counters.liveItems = this.items.length;
    }
    const t3 = this.now();
    this.counters.tileMs = t1 - t0;
    this.counters.regionMs = t2 - t1;
    this.counters.liveMs = t3 - t2;
  }

  /** Whether clean or dirty tiles cover the view, so no clear is needed under them. */
  private covered(): boolean {
    if (this.tiles.hasStale) {
      return false;
    }
    for (const tile of this.tiles.collect(this.view, this.visible)) {
      if (tile.image === null) {
        return false;
      }
    }
    return true;
  }

  private addRegion(a: Rect, b: Rect): void {
    const target = this.regions[Math.min(this.regionCount, MAX_REGIONS - 1)];
    if (!target) {
      return;
    }
    if (this.regionCount < MAX_REGIONS) {
      if (intersectInto(target, a, b)) {
        this.regionCount++;
      }
      return;
    }
    if (intersectInto(this.merge, a, b)) {
      unionInto(target, this.merge);
    }
  }

  private drawRegions(canvas: Canvas): void {
    for (let i = 0; i < this.regionCount; i++) {
      const region = this.regions[i];
      if (!region) {
        continue;
      }
      const r = this.toArray(region, this.dst);
      canvas.save();
      canvas.clipRect(r, this.ck.ClipOp.Intersect, false);
      canvas.drawRect(r, this.backgroundPaint);
      this.calls++;
      this.drawScene(canvas, region, false);
      canvas.restore();
    }
  }

  private blit(canvas: Canvas, tile: Tile<Image>, stale: boolean): void {
    const image = tile.image;
    if (image === null) {
      return;
    }
    this.src.set([0, 0, image.width(), image.height()]);
    const dst = this.toArray(this.tiles.tileRect(tile, this.tileWorld, stale), this.dst);
    // A full-resolution tile at the current scale maps pixel to pixel: nearest sampling is exact and cheapest.
    const exact = !stale && !tile.lowRes && this.tiles.scale === this.viewport.zoom * this.dpr;
    const filter = exact ? this.ck.FilterMode.Nearest : this.ck.FilterMode.Linear;
    canvas.drawImageRectOptions(image, this.src, dst, filter, this.ck.MipmapMode.None);
    this.calls++;
  }

  private toArray(rect: Rect, out: Float32Array): Float32Array {
    out[0] = rect.l;
    out[1] = rect.t;
    out[2] = rect.r;
    out[3] = rect.b;
    return out;
  }

  private drawScene(canvas: Canvas, rect: Rect, includeLive: boolean, items?: readonly SceneItem[]): void {
    const list = items ?? this.scene.query(rect, this.items, includeLive);
    this.texts.length = 0;
    for (const item of list) {
      this.addItem(canvas, item);
    }
    this.calls += this.batches.flush(canvas);
    for (const item of this.texts) {
      this.drawText(canvas, item.spec);
    }
  }

  private addItem(canvas: Canvas, item: SceneItem): void {
    const spec = item.spec;
    switch (spec.kind) {
      case "frame":
        if (spec.shadow && this.lod.shadows) {
          this.drawShadow(canvas, spec);
        }
        this.addBox(item, BatchLayer.frameFill, BatchLayer.frameStroke);
        break;
      case "rect":
        this.addBox(item, BatchLayer.rectFill, BatchLayer.rectStroke);
        break;
      case "link":
        this.batches.link(item.strokeKey ?? "", spec, this.lod.straightLinks);
        break;
      case "text":
        if (this.lod.textBars) {
          const width = Math.min(spec.w, spec.text.length * spec.size * 0.5);
          this.batches.rect(
            BatchLayer.bar,
            item.fillKey ?? "",
            spec.color,
            0,
            spec.x,
            spec.y + spec.size * 0.3,
            width,
            spec.size * 0.6,
            0,
          );
        } else {
          this.texts.push(item);
        }
        break;
    }
  }

  private addBox(item: SceneItem, fillLayer: BatchLayer, strokeLayer: BatchLayer): void {
    const spec = item.spec;
    if (spec.kind !== "frame" && spec.kind !== "rect") {
      return;
    }
    if (spec.fill !== undefined && item.fillKey) {
      this.batches.rect(fillLayer, item.fillKey, spec.fill, 0, spec.x, spec.y, spec.w, spec.h, spec.radius);
    }
    if (spec.stroke && item.strokeKey) {
      const { color, width } = spec.stroke;
      this.batches.rect(strokeLayer, item.strokeKey, color, width, spec.x, spec.y, spec.w, spec.h, spec.radius);
    }
  }

  private drawShadow(canvas: Canvas, spec: FrameSpec): void {
    if (!this.shadow) {
      return;
    }
    const dst = this.toArray(
      setRect(this.tileWorld, spec.x - 16, spec.y - 12, spec.x + spec.w + 16, spec.y + spec.h + 20),
      this.dst,
    );
    canvas.drawImageNine(this.shadow, this.shadowCenter, dst, this.ck.FilterMode.Linear);
    this.calls++;
  }

  private drawText(canvas: Canvas, spec: ItemSpec): void {
    if (spec.kind !== "text") {
      return;
    }
    const paragraph = this.paragraph(spec);
    if (paragraph) {
      canvas.drawParagraph(paragraph, spec.x, spec.y);
      this.calls++;
    }
  }

  private paragraph(spec: TextSpec): Paragraph | null {
    const cached = this.paragraphs.get(spec.id);
    if (cached?.spec === spec) {
      return cached.paragraph;
    }
    this.forget(spec.id);
    if (!this.fontMgr) {
      return null;
    }
    const ck = this.ck;
    const style = new ck.ParagraphStyle({
      textStyle: { color: color4f(ck, spec.color), fontSize: spec.size, fontFamilies: [this.options.fontFamily] },
    });
    const builder = ck.ParagraphBuilder.Make(style, this.fontMgr);
    builder.addText(spec.text);
    const paragraph = this.resources.own(builder.build());
    builder.delete();
    paragraph.layout(spec.w);
    this.paragraphs.set(spec.id, { spec, paragraph });
    return paragraph;
  }

  private idleQueue(): number {
    if (!this.options.tiles || this.tiles.scale === 0) {
      return 0;
    }
    return this.tiles.pending(this.view, this.interacting, this.pendingTiles, this.scratchTiles).length;
  }

  /** Whether tiles wait to be rendered or refined. */
  hasIdleWork(): boolean {
    this.viewport.worldRect(this.view);
    return this.idleQueue() > 0;
  }

  /**
   * Renders queued tiles until `deadline` (at least one), visible tiles
   * first; returns how many were rendered.
   */
  idle(deadline: number): number {
    if (!this.options.tiles || this.tiles.scale === 0) {
      return 0;
    }
    this.viewport.worldRect(this.view);
    let rendered = 0;
    for (const tile of this.tiles.pending(this.view, this.interacting, this.pendingTiles, this.scratchTiles)) {
      if (rendered > 0 && this.now() >= deadline) {
        break;
      }
      this.renderTile(tile);
      rendered++;
    }
    this.tiles.dropStaleIfCovered(this.view, this.scratchTiles);
    const margin = 2 * this.tiles.worldSize;
    this.tiles.evict(
      setRect(this.keep, this.view.l - margin, this.view.t - margin, this.view.r + margin, this.view.b + margin),
    );
    return rendered;
  }

  /** Jobs left in the progressive base re-render, the realignment of the old base included. */
  get baseJobs(): number {
    return this.baseQueue.length + (this.options.tiles && this.needsShift() ? 1 : 0);
  }

  /** The base tiles queued for the progressive re-render, in order; for tests. */
  queuedTiles(): readonly Tile<Image>[] {
    return this.baseQueue;
  }

  private needsShift(): boolean {
    const shown = this.shown;
    const view = this.viewport;
    return shown === null || shown.x !== view.x || shown.y !== view.y || shown.zoom !== view.zoom;
  }

  /**
   * Plans a progressive re-render of the base (M-12): every base tile whose
   * current image is not yet on the surface, ordered around `focus` (the
   * world rectangle the view shows) and `direction` by `orderJobs`.
   */
  planBase(focus: Rect, direction: Direction): void {
    this.baseQueue.length = 0;
    if (!this.options.tiles) {
      return;
    }
    this.scene.drainDirty(this.invalidate);
    this.scene.drainRemoved(this.forget);
    this.lod = lodForZoom(this.viewport.zoom);
    this.viewport.worldRect(this.view);
    if (this.tiles.setScale(this.viewport.zoom * this.dpr)) {
      this.shownTiles.clear();
    }
    const tiles = this.tiles.collect(this.view, this.visible);
    for (const tile of this.shownTiles) {
      if (!tiles.includes(tile)) {
        this.shownTiles.delete(tile);
      }
    }
    const todo = tiles.filter((tile) => !this.shownTiles.has(tile) || this.tiles.needsWork(tile, this.interacting));
    const rect = (tile: Tile<Image>): Rect => this.tiles.tileRect(tile, makeRect(0, 0, 0, 0));
    this.baseQueue.push(...orderJobs(todo, rect, focus, direction));
  }

  /** Drops the queued base jobs; a realignment still owed runs with the next job. */
  cancelBase(): void {
    this.baseQueue.length = 0;
  }

  /**
   * Runs one base job and flushes it on its own: the realignment of the old
   * base to the current placement first, then one tile per job — rendered
   * when needed and blitted over what the base shows, never cleared first. A
   * render that reaches `deadline` leaves the blit to the next job.
   */
  baseJob(deadline: number): BaseJobResult {
    const start = this.now();
    this.calls = 0;
    this.resetCounters();
    if (this.needsShift()) {
      this.shiftBase();
      return { ms: this.now() - start, shifted: true };
    }
    const tile = this.baseQueue[0];
    if (!tile) {
      return { ms: 0, shifted: false };
    }
    if (this.tiles.needsWork(tile, this.interacting)) {
      this.renderTile(tile);
      if (this.now() >= deadline) {
        return { ms: this.now() - start, shifted: false };
      }
    }
    this.baseQueue.shift();
    const canvas = this.surface.getCanvas();
    const scale = this.viewport.zoom * this.dpr;
    canvas.save();
    canvas.scale(scale, scale);
    canvas.translate(-Math.round(this.viewport.x * scale) / scale, -Math.round(this.viewport.y * scale) / scale);
    this.blit(canvas, tile, false);
    canvas.restore();
    this.counters.tilesDrawn = 1;
    this.flushBase(canvas);
    this.shownTiles.add(tile);
    return { ms: this.now() - start, shifted: false };
  }

  /** The statistics of a base job of `ms` milliseconds. */
  jobStats(ms: number): FrameStats {
    return this.stats(ms, 0, true, ms);
  }

  /**
   * Moves the old base pixels to the current placement in one draw, so the
   * base stays covered while its tiles are replaced; tiles that land exactly
   * where they were (a whole-pixel pan) stay shown.
   */
  private shiftBase(): void {
    const canvas = this.surface.getCanvas();
    const view = this.viewport;
    const scale = view.zoom * this.dpr;
    const shown = this.shown;
    const image = shown === null ? null : this.surface.makeImageSnapshot();
    canvas.clear(this.background);
    if (shown !== null && image !== null) {
      const old = shown.zoom * this.dpr;
      const k = scale / old;
      const tx = Math.round(shown.x * old) * k - Math.round(view.x * scale);
      const ty = Math.round(shown.y * old) * k - Math.round(view.y * scale);
      canvas.save();
      canvas.translate(tx, ty);
      canvas.scale(k, k);
      const filter = k === 1 ? this.ck.FilterMode.Nearest : this.ck.FilterMode.Linear;
      canvas.drawImageOptions(image, 0, 0, filter, this.ck.MipmapMode.None, null);
      canvas.restore();
      image.delete();
      this.keepShiftedTiles(shown, k === 1 && Number.isInteger(tx) && Number.isInteger(ty));
    } else {
      this.shownTiles.clear();
    }
    this.calls++;
    this.shown = { x: view.x, y: view.y, zoom: view.zoom };
    this.flushBase(canvas);
  }

  private keepShiftedTiles(shown: Placement, exact: boolean): void {
    if (!exact) {
      this.shownTiles.clear();
      return;
    }
    const view = this.viewport;
    const old = setRect(
      this.oldBase,
      shown.x,
      shown.y,
      shown.x + view.width / shown.zoom,
      shown.y + view.height / shown.zoom,
    );
    for (const tile of this.shownTiles) {
      const rect = this.tiles.tileRect(tile, this.tileTarget);
      if (rect.l < old.l || rect.t < old.t || rect.r > old.r || rect.b > old.b) {
        this.shownTiles.delete(tile);
      }
    }
  }

  private renderTile(tile: Tile<Image>): void {
    this.shownTiles.delete(tile);
    const surface = this.tileSurface;
    if (!surface) {
      return;
    }
    const start = this.now();
    const lowRes = this.interacting;
    const resolution = resolutionFor(lowRes);
    const px = Math.max(1, Math.round(this.options.tileSize * resolution));
    const scale = this.tiles.scale * resolution;
    const world = this.tiles.tileRect(tile, this.tileTarget);
    const canvas = surface.getCanvas();
    this.irect.set([0, 0, px, px]);
    canvas.save();
    canvas.clipRect(this.src.fill(0).fill(px, 2), this.ck.ClipOp.Intersect, false);
    canvas.clear(this.background);
    canvas.scale(scale, scale);
    canvas.translate(-world.l, -world.t);
    this.drawScene(canvas, world, false);
    canvas.restore();
    this.tiles.store(tile, this.resources.own(surface.makeImageSnapshot(this.irect)), lowRes);
    // Finish the tile's GPU work here, in idle time, rather than in the next frame's flush.
    surface.flush();
    this.syncGpu(canvas);
    this.tileRenders++;
    this.tileRenderMs += this.now() - start;
  }

  /** Deletes every CanvasKit object the renderer owns. */
  dispose(): void {
    this.tiles.clear();
    this.paragraphs.clear();
    this.resources.dispose();
  }
}
