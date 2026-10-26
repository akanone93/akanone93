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
 *   are excluded from tiles and drawn on top each frame — into the
 *   interaction layer only where they changed, so a frame presents the
 *   layer's damage and not the whole layer, whose upload through software
 *   GL is what stalls the browser's frames;
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
  /** Families after `fontFamily` that glyphs missing from it fall back to, in order. */
  readonly fallbackFamilies?: readonly string[];
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

/** Strips a tile is rendered in by progressive base jobs, one job each (M-12). */
export const TILE_PARTS = 4;

/** Above this many dirty regions in a frame they are merged into one. */
export const MAX_REGIONS = 8;

/** Above this many undrawn damage rectangles the next base draw is a whole one. */
export const MAX_DAMAGE = 32;

/** Where the interaction layer draws: its surface, the world point at its top-left, and its scale. */
export interface LiveLayer {
  readonly surface: Surface;
  readonly x: number;
  readonly y: number;
  readonly scale: number;
}
/** Device pixels added around the layer's damage for antialiasing. */
const LAYER_PAD = 2;

/** The most regions the interaction layer's damage is merged into per frame, each redrawn and presented alone. */
export const LAYER_REGIONS = 48;

/** Two damage rectangles merge when their union covers at most this factor of their summed areas. */
const LAYER_MERGE = 1.25;

/** What the interaction layer shows: where it was drawn, so a frame redraws only its damage. */
interface LayerShown {
  readonly surface: Surface;
  readonly x: number;
  readonly y: number;
  readonly scale: number;
  readonly zoom: number;
}

function area(r: Rect): number {
  return (r.r - r.l) * (r.b - r.t);
}

/**
 * Merges `rect` into the first `count` of `regions`: into the first whose
 * union with it is hardly larger than both (`LAYER_MERGE`), else into a new
 * one, else into the one it grows least; returns the new count. Pieces along
 * a link only touch at their corners, so they stay apart.
 */
function mergeRegion(regions: readonly Rect[], count: number, rect: Rect, scratch: Rect): number {
  let best: Rect | null = null;
  let growth = Number.POSITIVE_INFINITY;
  const size = area(rect);
  for (let i = 0; i < count; i++) {
    const region = regions[i];
    if (!region) {
      continue;
    }
    const union = area(unionInto(setRect(scratch, region.l, region.t, region.r, region.b), rect));
    if (union <= (area(region) + size) * LAYER_MERGE) {
      unionInto(region, rect);
      return count;
    }
    if (union - area(region) < growth) {
      growth = union - area(region);
      best = region;
    }
  }
  const next = regions[count];
  if (next) {
    setRect(next, rect.l, rect.t, rect.r, rect.b);
    return count + 1;
  }
  if (best) {
    unionInto(best, rect);
  }
  return count;
}

/**
 * Adds `a` clipped to `b` to the first `count` of `regions`, merging into the
 * last one beyond `regions.length`; returns the new count.
 */
function addClipped(regions: readonly Rect[], count: number, a: Rect, b: Rect, merge: Rect): number {
  const target = regions[Math.min(count, regions.length - 1)];
  if (!target) {
    return count;
  }
  if (count < regions.length) {
    return intersectInto(target, a, b) ? count + 1 : count;
  }
  if (intersectInto(merge, a, b)) {
    unionInto(target, merge);
  }
  return count;
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
  layerPixels: number;
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
  private readonly dirty = new Int32Array(4);
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
    layerPixels: 0,
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
  private readonly stripWorld = makeRect(0, 0, 0, 0);
  /** The tile whose strips the base jobs are rendering, and the next strip. */
  private partial: { readonly tile: Tile<Image>; readonly next: number } | null = null;
  /** The world rectangle the host shows, as of the last plan. */
  private readonly focus = makeRect(0, 0, 0, 0);
  /** World rectangles changed since the surface was last drawn; the next base draw redraws only them when it can. */
  private readonly damage: Rect[] = [];
  private readonly damageUnion = makeRect(0, 0, 0, 0);
  /** The interaction layer's damage this frame: where live items changed, clipped to the layer and merged. */
  private readonly layerRegions: Rect[] = Array.from({ length: LAYER_REGIONS }, () => makeRect(0, 0, 0, 0));
  private layerRegionCount = 0;
  private readonly layerWorld = makeRect(0, 0, 0, 0);
  private readonly layerClip = makeRect(0, 0, 0, 0);
  private readonly layerDirty = new Int32Array(4);
  private layerShown: LayerShown | null = null;
  private partialRenders = 0;
  private viewChanged = false;
  private readonly invalidate = (rect: Rect): void => {
    this.tiles.invalidate(rect);
    if (this.damage.length <= MAX_DAMAGE) {
      this.damage.push(makeRect(rect.l, rect.t, rect.r, rect.b));
    }
  };
  private readonly damageLayer = (rect: Rect): void => {
    if (intersectInto(this.layerClip, rect, this.layerWorld)) {
      this.layerRegionCount = mergeRegion(this.layerRegions, this.layerRegionCount, this.layerClip, this.merge);
    }
  };
  private readonly ignore = (): void => undefined;
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
    } else {
      this.layerShown = null;
      this.scene.drainLiveDirty(this.ignore);
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
    if (this.options.tiles && this.canDrawDamage(scale, includeLive)) {
      return this.drawDamage(canvas, scale);
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

  /**
   * Whether the surface already shows every visible tile at this placement,
   * so a base draw only has to redraw the damaged rectangles over it.
   */
  private canDrawDamage(scale: number, includeLive: boolean): boolean {
    if (
      this.damage.length > MAX_DAMAGE ||
      this.needsShift() ||
      this.tiles.scale !== scale ||
      this.tiles.hasStale ||
      (includeLive && this.scene.liveItems(this.items).length > 0)
    ) {
      return false;
    }
    for (const tile of this.tiles.collect(this.view, this.visible)) {
      if (tile.image === null || !this.shownTiles.has(tile)) {
        return false;
      }
    }
    return true;
  }

  /**
   * Redraws only the damaged rectangles (clipped, over the pixels the
   * surface keeps) and presents only their union; returns the flush time.
   */
  private drawDamage(canvas: Canvas, scale: number): number {
    const start = this.now();
    this.regionCount = 0;
    for (const rect of this.damage) {
      this.addRegion(rect, this.view);
    }
    this.damage.length = 0;
    if (this.regionCount === 0) {
      return 0;
    }
    const u = this.damageUnion;
    setRect(u, Infinity, Infinity, -Infinity, -Infinity);
    for (let i = 0; i < this.regionCount; i++) {
      const region = this.regions[i];
      if (region) {
        unionInto(u, region);
      }
    }
    canvas.save();
    canvas.scale(scale, scale);
    canvas.translate(-Math.round(this.viewport.x * scale) / scale, -Math.round(this.viewport.y * scale) / scale);
    this.drawRegions(canvas);
    canvas.restore();
    this.counters.regionMs = this.now() - start;
    return this.flushBase(canvas, this.deviceRect(this.toArray(u, this.dst), scale));
  }

  /** Whether the last `idle` rendered a visible tile the surface does not show yet, so the base needs drawing. */
  get idleChangedView(): boolean {
    return this.viewChanged;
  }

  /** Tiles refreshed in place (only their dirty rectangle redrawn) since creation; for tests and the benchmark. */
  get partialTileRenders(): number {
    return this.partialRenders;
  }

  /**
   * Flushes the base surface (and waits for the GPU when measuring); returns
   * the time taken. `dirty` is the changed device rectangle [l, t, r, b]: the
   * CPU raster surface then copies only it to the canvas, not every pixel,
   * and a WebGL surface ignores it.
   */
  private flushBase(canvas: Canvas, dirty?: Int32Array): number {
    const flushStart = this.now();
    (this.surface.flush as (dirtyRect?: Int32Array) => void).call(this.surface, dirty);
    if (this.options.measureFrames) {
      this.syncGpu(canvas);
    }
    return this.now() - flushStart;
  }

  /**
   * Draws the live items into the interaction layer, on a transparent
   * background, and presents the whole layer when it is new, moved or
   * rescaled, otherwise only the regions live items changed in (each old and
   * new bounds, merged). The draw itself stays whole and unclipped: it is
   * cheap, and pixels outside the regions then equal those presented before.
   */
  private drawLayer(layer: LiveLayer): void {
    const start = this.now();
    const canvas = layer.surface.getCanvas();
    const shown = this.layerShown;
    const zoom = this.viewport.zoom;
    const { scale } = layer;
    const width = layer.surface.width();
    const height = layer.surface.height();
    setRect(this.layerWorld, layer.x, layer.y, layer.x + width / scale, layer.y + height / scale);
    this.layerRegionCount = 0;
    this.scene.drainLiveDirty(this.damageLayer);
    const live = this.scene.liveItems(this.items);
    this.counters.liveItems = live.length;
    canvas.clear(this.ck.TRANSPARENT);
    canvas.save();
    canvas.scale(scale, scale);
    canvas.translate(-layer.x, -layer.y);
    this.drawScene(canvas, this.view, false, live);
    canvas.restore();
    if (
      shown === null ||
      shown.surface !== layer.surface ||
      shown.x !== layer.x ||
      shown.y !== layer.y ||
      shown.scale !== scale ||
      shown.zoom !== zoom
    ) {
      layer.surface.flush();
      this.counters.layerPixels = width * height;
    } else {
      this.presentLayerDamage(layer, width, height);
    }
    this.layerShown = { surface: layer.surface, x: layer.x, y: layer.y, scale, zoom };
    if (this.options.measureFrames) {
      this.syncGpu(canvas);
    }
    this.counters.layerMs = this.now() - start;
  }

  /** Presents the layer's damaged regions, each in whole device pixels padded for antialiasing. */
  private presentLayerDamage(layer: LiveLayer, width: number, height: number): void {
    const { scale } = layer;
    const pixels = this.layerDirty;
    for (let i = 0; i < this.layerRegionCount; i++) {
      const region = this.layerRegions[i];
      if (!region) {
        continue;
      }
      const l = Math.max(0, Math.floor((region.l - layer.x) * scale) - LAYER_PAD);
      const t = Math.max(0, Math.floor((region.t - layer.y) * scale) - LAYER_PAD);
      const r = Math.min(width, Math.ceil((region.r - layer.x) * scale) + LAYER_PAD);
      const b = Math.min(height, Math.ceil((region.b - layer.y) * scale) + LAYER_PAD);
      if (r <= l || b <= t) {
        continue;
      }
      pixels.set([l, t, r, b]);
      // The CPU raster layer copies only this rectangle [l, t, r, b] to its canvas, as the base does.
      (layer.surface.flush as (dirtyRect?: Int32Array) => void).call(layer.surface, pixels);
      this.counters.layerPixels += (r - l) * (b - t);
    }
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
    c.layerPixels = 0;
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
      layerPixels: c.layerPixels,
      jobMs,
      baseJobs: this.baseJobs,
      visibleJobs: this.visibleJobs,
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
    this.damage.length = 0;
    // The whole base is drawn here, so it is now placed at this viewport and holds exactly these tiles.
    this.shown = { x: this.viewport.x, y: this.viewport.y, zoom: this.viewport.zoom };
    this.shownTiles.clear();
    for (const tile of this.tiles.collect(this.view, this.visible)) {
      this.tiles.tileRect(tile, this.tileWorld);
      if (tile.image !== null) {
        this.blit(canvas, tile, false);
        this.shownTiles.add(tile);
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
    this.regionCount = addClipped(this.regions, this.regionCount, a, b, this.merge);
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
      textStyle: {
        color: color4f(ck, spec.color),
        fontSize: spec.size,
        fontFamilies: [this.options.fontFamily, ...(this.options.fallbackFamilies ?? [])],
      },
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
   * first, stopping early between tiles when `yieldNow` says a message
   * waits; returns how many were rendered.
   */
  idle(deadline: number, yieldNow: () => boolean = () => false): number {
    if (!this.options.tiles || this.tiles.scale === 0) {
      return 0;
    }
    this.viewport.worldRect(this.view);
    let rendered = 0;
    this.viewChanged = false;
    const visible = this.tiles.collect(this.view, this.visible);
    for (const tile of this.tiles.pending(this.view, this.interacting, this.pendingTiles, this.scratchTiles)) {
      if (rendered > 0 && (this.now() >= deadline || yieldNow())) {
        break;
      }
      this.renderTile(tile);
      rendered++;
      if (!this.shownTiles.has(tile) && visible.includes(tile)) {
        this.viewChanged = true;
      }
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

  /** Jobs left before every tile the host shows is current: the realignment and the queued tiles in the focus. */
  get visibleJobs(): number {
    if (!this.options.tiles) {
      return 0;
    }
    const f = this.focus;
    let count = this.needsShift() ? 1 : 0;
    for (const tile of this.baseQueue) {
      const r = this.tiles.tileRect(tile, this.tileWorld);
      if (r.l < f.r && r.r > f.l && r.t < f.b && r.b > f.t) {
        count++;
      }
    }
    return count;
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
    setRect(this.focus, focus.l, focus.t, focus.r, focus.b);
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
      if (!this.renderPart(tile) || this.now() >= deadline) {
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
    this.flushBase(canvas, this.deviceRect(this.dst, scale));
    this.shownTiles.add(tile);
    return { ms: this.now() - start, shifted: false };
  }

  /** The whole device pixels the world rectangle `world` ([l, t, r, b]) covers on the base surface, in `this.dirty`. */
  private deviceRect(world: Float32Array, scale: number): Int32Array {
    const ox = Math.round(this.viewport.x * scale);
    const oy = Math.round(this.viewport.y * scale);
    const l = Math.max(0, Math.floor((world[0] ?? 0) * scale - ox));
    const t = Math.max(0, Math.floor((world[1] ?? 0) * scale - oy));
    const r = Math.min(this.surface.width(), Math.max(l, Math.ceil((world[2] ?? 0) * scale - ox)));
    const b = Math.min(this.surface.height(), Math.max(t, Math.ceil((world[3] ?? 0) * scale - oy)));
    this.dirty.set([l, t, r, b]);
    return this.dirty;
  }

  /** The statistics of a base job of `ms` milliseconds. */
  jobStats(ms: number): FrameStats {
    return this.stats(ms, 0, true, ms);
  }

  /**
   * Realigns the base to the current placement without reading it back (a
   * snapshot of a GL surface stalls software GL for most of a second): the
   * cached tiles are blitted where they now belong — older-scale tiles under
   * them, scaled, as the interim — and those already current are taken off
   * the queue. Only tiles nothing covers yet show the background.
   */
  private shiftBase(): void {
    const canvas = this.surface.getCanvas();
    const view = this.viewport;
    const scale = view.zoom * this.dpr;
    view.worldRect(this.view);
    this.tiles.setScale(scale);
    canvas.clear(this.background);
    this.calls++;
    canvas.save();
    canvas.scale(scale, scale);
    canvas.translate(-Math.round(view.x * scale) / scale, -Math.round(view.y * scale) / scale);
    for (const tile of this.tiles.stale(this.view, this.scratchTiles)) {
      this.blit(canvas, tile, true);
      this.counters.staleTilesDrawn++;
    }
    this.shownTiles.clear();
    for (const tile of this.tiles.collect(this.view, this.visible)) {
      if (tile.image !== null) {
        this.blit(canvas, tile, false);
        this.counters.tilesDrawn++;
        if (!this.tiles.needsWork(tile, this.interacting)) {
          this.shownTiles.add(tile);
        }
      }
    }
    canvas.restore();
    this.shown = { x: view.x, y: view.y, zoom: view.zoom };
    this.flushBase(canvas);
    const queue = this.baseQueue.filter((tile) => !this.shownTiles.has(tile));
    this.baseQueue.length = 0;
    this.baseQueue.push(...queue);
  }

  /**
   * Renders the next of the `TILE_PARTS` strips of `tile` into the tile
   * surface and flushes it, so no job hands the GPU more than a strip; the
   * last strip stores the tile. Returns whether the tile is complete.
   */
  private renderPart(tile: Tile<Image>): boolean {
    const surface = this.tileSurface;
    if (!surface) {
      return true;
    }
    const part = this.partial?.tile === tile ? this.partial.next : 0;
    const start = this.now();
    const lowRes = this.interacting;
    const px = Math.max(1, Math.round(this.options.tileSize * resolutionFor(lowRes)));
    const scale = this.tiles.scale * resolutionFor(lowRes);
    const world = this.tiles.tileRect(tile, this.tileTarget);
    const y0 = Math.round((px * part) / TILE_PARTS);
    const y1 = Math.round((px * (part + 1)) / TILE_PARTS);
    const strip = setRect(this.stripWorld, world.l, world.t + y0 / scale, world.r, world.t + y1 / scale);
    const canvas = surface.getCanvas();
    canvas.save();
    this.src.set([0, y0, px, y1]);
    canvas.clipRect(this.src, this.ck.ClipOp.Intersect, false);
    canvas.clear(this.background);
    canvas.scale(scale, scale);
    canvas.translate(-world.l, -world.t);
    this.drawScene(canvas, strip, false);
    canvas.restore();
    const done = part + 1 >= TILE_PARTS;
    this.partial = done ? null : { tile, next: part + 1 };
    if (done) {
      this.shownTiles.delete(tile);
      this.irect.set([0, 0, px, px]);
      this.tiles.store(tile, this.resources.own(surface.makeImageSnapshot(this.irect)), lowRes);
    }
    surface.flush();
    this.syncGpu(canvas);
    if (done) {
      this.tileRenders++;
    }
    this.tileRenderMs += this.now() - start;
    return done;
  }

  private renderTile(tile: Tile<Image>): void {
    this.partial = null;
    const surface = this.tileSurface;
    if (!surface) {
      this.shownTiles.delete(tile);
      return;
    }
    if (this.refreshTile(tile, surface)) {
      return;
    }
    this.shownTiles.delete(tile);
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

  /**
   * Re-renders only the dirty rectangle of a full-resolution tile over its
   * cached pixels; returns false when the tile needs a whole render. A tile
   * the surface shows with all damage drawn stays shown: its pixels are
   * already on the surface.
   */
  private refreshTile(tile: Tile<Image>, surface: Surface): boolean {
    const image = tile.image;
    const dirty = tile.dirty;
    if (image === null || dirty === null || tile.lowRes || this.interacting) {
      return false;
    }
    const start = this.now();
    const px = Math.max(1, Math.round(this.options.tileSize));
    const scale = this.tiles.scale;
    const world = this.tiles.tileRect(tile, this.tileTarget);
    if (!intersectInto(this.stripWorld, dirty, world)) {
      return false;
    }
    const d = this.stripWorld;
    const l = Math.max(0, Math.floor((d.l - world.l) * scale));
    const t = Math.max(0, Math.floor((d.t - world.t) * scale));
    const r = Math.min(px, Math.ceil((d.r - world.l) * scale));
    const b = Math.min(px, Math.ceil((d.b - world.t) * scale));
    const current = this.shownTiles.has(tile) && this.damage.length === 0;
    const canvas = surface.getCanvas();
    canvas.save();
    canvas.clipRect(this.src.fill(0).fill(px, 2), this.ck.ClipOp.Intersect, false);
    canvas.drawImage(image, 0, 0);
    this.src.set([l, t, r, b]);
    canvas.clipRect(this.src, this.ck.ClipOp.Intersect, false);
    canvas.clear(this.background);
    canvas.scale(scale, scale);
    canvas.translate(-world.l, -world.t);
    setRect(d, world.l + l / scale, world.t + t / scale, world.l + r / scale, world.t + b / scale);
    this.drawScene(canvas, d, false);
    canvas.restore();
    this.irect.set([0, 0, px, px]);
    this.tiles.store(tile, this.resources.own(surface.makeImageSnapshot(this.irect)), false);
    surface.flush();
    this.syncGpu(canvas);
    if (!current) {
      this.shownTiles.delete(tile);
    }
    this.partialRenders++;
    this.tileRenders++;
    this.tileRenderMs += this.now() - start;
    return true;
  }

  /** Deletes every CanvasKit object the renderer owns. */
  dispose(): void {
    this.tiles.clear();
    this.paragraphs.clear();
    this.resources.dispose();
  }
}
