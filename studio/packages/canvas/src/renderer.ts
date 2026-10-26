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
 * - lift and drop without re-rendering tiles: a lifted item stays in the
 *   cached tiles and is masked out of the base by patches — its pieces
 *   redrawn without it, in clipped chunks in preemptible idle time, paced
 *   to one tile's pixels per animation frame, again after every later blit
 *   of the tiles — and a drop draws the items over the base at once, then
 *   patches their pieces in z-order and, in idle time after the frame is
 *   shown, redraws only those pieces of the tiles they left and joined (M-13);
 * - level of detail by zoom (`lod.ts`), with pre-rendered nine-patch shadow
 *   images instead of blur;
 * - tiles rendered at half resolution while the user interacts, refined
 *   when idle.
 *
 * Without tiles (CPU raster, where image blits are slow) each frame draws the
 * visible items directly, batched and with level of detail.
 */

import type { Canvas, CanvasKit, FontMgr, Image, Paint, Paragraph, PathBuilder, Surface } from "canvaskit-wasm";
import { Batches, BatchLayer, color4f } from "./batch.ts";
import { intersectInto, intersects, makeRect, type Rect, setRect, unionInto } from "./geometry.ts";
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

/** The most patch rectangles drawn together in one idle-time chunk. */
export const PATCH_CHUNK = 8;

/** The tile-sized cell of the world a patch rectangle's centre falls in, as a sortable number. */
function cellOf(rect: Rect, size: number): number {
  const row = Math.floor((rect.t + rect.b) / 2 / size);
  const col = Math.floor((rect.l + rect.r) / 2 / size);
  return row * 1_000_003 + col;
}

/**
 * The most dirty pieces of a tile redrawn together in one refresh pass: one
 * clip and one draw of the items meeting any of them, since the long links
 * a drop leaves pieces along meet most pieces of a tile and would otherwise
 * be drawn once per pass.
 */
export const REFRESH_CHUNK = 32;

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
  patches: number;
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
  private readonly images = new Map<string, Image>();
  private readonly picture: Paint;
  private readonly fontMgr: FontMgr | null;
  private readonly background: Float32Array;
  private readonly backgroundPaint: Paint;
  private readonly shadow: Image | null;
  private readonly items: SceneItem[] = [];
  private readonly texts: SceneItem[] = [];
  private readonly pictures: SceneItem[] = [];
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
    patches: 0,
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
  /** World rectangles of lifted items that cached tiles still show: every blit of tiles masks them out again. */
  private readonly masks: Rect[] = [];
  /** World rectangles idle time redraws over the base's pixels, in chunks, without touching tiles: lifts and drops. */
  private readonly patches: Rect[] = [];
  /** Items dropped since the last base draw, which draws them over the base until their patches are drawn. */
  private readonly dropItems: SceneItem[] = [];
  private readonly patchClip: Rect[] = [];
  private readonly patchUnion = makeRect(0, 0, 0, 0);
  /** The patches drawn since the last flush of the base. */
  private readonly patchGroup = makeRect(0, 0, 0, 0);
  /** Device pixels of patches flushed since the last animation frame (`patchBudget`). */
  private patchPixels = 0;
  private readonly dropUnion = makeRect(0, 0, 0, 0);
  private readonly patchRect = new Float32Array(4);
  /** The tile pixels [l, t, r, b] a refresh chunk's pieces cover. */
  private readonly pieceBounds = new Float32Array(4);
  private copiedRefreshes = 0;
  private readonly patchItems: SceneItem[] = [];
  private readonly patchPath: PathBuilder;
  /** The last frame drew live items into an interaction layer: a lift masks the base instead of re-rendering tiles. */
  private layered = false;
  private readonly lift = (rect: Rect): void => {
    if (!this.options.tiles) {
      return;
    }
    if (!this.layered) {
      // Without a layer the base is drawn whole while dragging: the tiles leave the item out instead.
      this.invalidate(rect);
      return;
    }
    const copy = makeRect(rect.l, rect.t, rect.r, rect.b);
    this.masks.push(copy);
    this.patches.push(copy);
  };
  private readonly drop = (rect: Rect): void => {
    if (!this.options.tiles) {
      return;
    }
    // The tiles take the item in when idle; the base shows it at once, from a patch.
    this.tiles.invalidate(rect);
    this.patches.push(makeRect(rect.l, rect.t, rect.r, rect.b));
  };
  private readonly dropItem = (item: SceneItem): void => {
    if (this.options.tiles) {
      this.dropItems.push(item);
    }
  };
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
    this.patchPath = this.resources.own(new ck.PathBuilder());
    this.backgroundPaint = this.resources.own(new ck.Paint());
    this.backgroundPaint.setColor(this.background);
    this.picture = this.resources.own(new ck.Paint());
    this.picture.setAntiAlias(true);
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
    // New tiles leave live items out: nothing is left to mask.
    this.masks.length = 0;
    this.patches.length = 0;
    this.dropItems.length = 0;
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
    this.patchPixels = 0;
    this.layered = layer !== null;
    this.drainScene();
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

  /**
   * Takes the scene's changes: dirty rectangles invalidate tiles and damage
   * the base, lifts and drops become patches and masks. Once nothing is
   * live, the tiles under the masks are invalidated so idle time re-renders
   * them; the base already hides the lifted items there.
   */
  private drainScene(): void {
    this.scene.drainDirty(this.invalidate);
    this.scene.drainLifts(this.lift, this.drop, this.dropItem);
    this.scene.drainRemoved(this.forget);
    if (this.masks.length > 0 && !this.scene.hasLive) {
      for (const mask of this.masks) {
        this.tiles.invalidate(mask);
      }
      this.masks.length = 0;
    }
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
    if (this.regionCount === 0 && this.dropItems.length === 0) {
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
    this.counters.patches = this.drawDropped(canvas);
    canvas.restore();
    if (this.counters.patches > 0) {
      unionInto(u, this.dropUnion);
    }
    if (u.r <= u.l || u.b <= u.t) {
      return 0;
    }
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
    c.patches = 0;
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
      regions: this.regionCount + c.patches,
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
    // The tiles may still show lifted items: idle time masks them again. Dropped items are in the dirty regions.
    this.requeueMasks();
    this.dropItems.length = 0;
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

  /**
   * Draws the items dropped since the last base draw over the base, so the
   * drop shows at once; their queued patches put them in z-order in idle
   * time. Leaves their bounds in `dropUnion`; returns how many it drew.
   */
  private drawDropped(canvas: Canvas): number {
    const items = this.dropItems;
    const count = items.length;
    if (count === 0) {
      return 0;
    }
    const u = setRect(this.dropUnion, Infinity, Infinity, -Infinity, -Infinity);
    for (const item of items) {
      unionInto(u, item.bounds);
    }
    setRect(
      u,
      Math.max(u.l, this.view.l),
      Math.max(u.t, this.view.t),
      Math.min(u.r, this.view.r),
      Math.min(u.b, this.view.b),
    );
    this.drawScene(canvas, this.view, false, items);
    items.length = 0;
    return count;
  }

  /** Queues every mask for idle time again, after a whole draw of the base from the tiles. */
  private requeueMasks(): void {
    this.patches.length = 0;
    this.patches.push(...this.masks);
  }

  /**
   * Draws queued patches on the base in chunks of at most `PATCH_CHUNK`
   * rectangles from one tile-sized cell, so a chunk meets few items, until `deadline`, `yieldNow` or the frame's
   * budget of flushed pixels (`patchBudget`; at least one chunk per frame), flushing each cell's chunks on their own;
   * returns how many chunks it drew. The base must be placed at the current
   * view; otherwise the realignment queues the masks again.
   */
  private patchIdle(deadline: number, yieldNow: () => boolean): number {
    const scale = this.viewport.zoom * this.dpr;
    if (
      this.patches.length === 0 ||
      this.needsShift() ||
      this.tiles.scale !== scale ||
      this.patchPixels >= this.patchBudget()
    ) {
      return 0;
    }
    const canvas = this.surface.getCanvas();
    const size = this.tiles.worldSize;
    this.patches.sort((a, b) => cellOf(a, size) - cellOf(b, size));
    // During an interaction each cell's chunks are flushed together, apart from other cells': every commit of the
    // base canvas costs the browser an upload through software GL, which a drag frame then waits for, so a flush
    // covers one cell, not the union of distant ones. Outside one, a single flush per call costs least.
    const group = setRect(this.patchGroup, Infinity, Infinity, -Infinity, -Infinity);
    let cell = Number.NaN;
    let chunks = 0;
    while (this.patches.length > 0) {
      if (
        chunks > 0 &&
        (this.now() >= deadline || yieldNow() || this.patchPixels + this.areaOf(group, scale) >= this.patchBudget())
      ) {
        break;
      }
      const first = this.patches[0] as Rect;
      if (cellOf(first, size) !== cell && this.interacting) {
        this.flushPatches(canvas, scale);
        cell = cellOf(first, size);
      }
      const chunk = this.patches.splice(0, this.chunkLength(size));
      canvas.save();
      canvas.scale(scale, scale);
      canvas.translate(-Math.round(this.viewport.x * scale) / scale, -Math.round(this.viewport.y * scale) / scale);
      if (this.drawPatch(canvas, chunk, this.view) > 0) {
        unionInto(group, this.patchUnion);
      }
      canvas.restore();
      chunks++;
    }
    this.flushPatches(canvas, scale);
    return chunks;
  }

  /**
   * The device pixels patches may flush per animation frame during an
   * interaction: one tile's. A
   * flush of the base is an upload on the browser's single software-GL
   * thread, which also composites the page, so patches are paced by it —
   * lift masks then never hold a drag frame up by more than this (M-13).
   */
  private patchBudget(): number {
    // Outside an interaction no animation waits on the patches: the idle deadline bounds them alone.
    return this.interacting ? this.options.tileSize * this.options.tileSize : Number.POSITIVE_INFINITY;
  }

  /** The device pixels the world rectangle `rect` covers at `scale`; 0 when empty. */
  private areaOf(rect: Rect, scale: number): number {
    return rect.r > rect.l && rect.b > rect.t ? (rect.r - rect.l) * (rect.b - rect.t) * scale * scale : 0;
  }

  /** Flushes the patches drawn since the last flush (`patchGroup`) and counts their pixels against the frame's budget. */
  private flushPatches(canvas: Canvas, scale: number): void {
    const group = this.patchGroup;
    if (group.r > group.l && group.b > group.t) {
      const d = this.deviceRect(this.toArray(group, this.dst), scale);
      this.patchPixels += Math.max(0, (d[2] ?? 0) - (d[0] ?? 0)) * Math.max(0, (d[3] ?? 0) - (d[1] ?? 0));
      this.flushBase(canvas, d);
    }
    setRect(group, Infinity, Infinity, -Infinity, -Infinity);
  }

  /** Whether patches wait only for the next animation frame: this frame's budget of them is spent. */
  get patchesWaitForFrame(): boolean {
    return this.patches.length > 0 && this.patchPixels >= this.patchBudget();
  }

  /** Starts a new animation frame's budget of patches without drawing a frame; `frame` does the same. */
  nextPatchFrame(): void {
    this.patchPixels = 0;
  }

  /**
   * How many of the queued patches, from the first, share its cell: at least
   * one, at most `PATCH_CHUNK` during an interaction (the grain a drag frame
   * may wait for) and `REFRESH_CHUNK` outside one, where nothing waits and
   * fewer passes draw the long links that meet the pieces fewer times.
   */
  private chunkLength(size: number): number {
    const first = this.patches[0];
    const cell = first ? cellOf(first, size) : 0;
    const most = this.interacting ? PATCH_CHUNK : REFRESH_CHUNK;
    let end = 1;
    for (; end < most && end < this.patches.length; end++) {
      const next = this.patches[end];
      if (!next || cellOf(next, size) !== cell) {
        break;
      }
    }
    return end;
  }

  /**
   * Redraws the scene without live items inside the union of `rects`
   * clipped to `within`: one clip of every rectangle and one batched draw of
   * the static items that meet any of them, so the many thin pieces along
   * lifted or dropped links cost one pass over few items, not a redraw of
   * the regions they merge into. Leaves the union in `patchUnion`; returns
   * how many rectangles it drew.
   */
  private drawPatch(canvas: Canvas, rects: readonly Rect[], within: Rect): number {
    const u = setRect(this.patchUnion, Infinity, Infinity, -Infinity, -Infinity);
    const clips = this.patchClip;
    let count = 0;
    for (const rect of rects) {
      let clip = clips[count];
      if (!clip) {
        clip = makeRect(0, 0, 0, 0);
        clips.push(clip);
      }
      if (!intersectInto(clip, rect, within)) {
        continue;
      }
      this.patchPath.addRect(this.toArray(clip, this.patchRect));
      unionInto(u, clip);
      count++;
    }
    if (count === 0) {
      return 0;
    }
    const list = this.patchItemsIn(count);
    const path = this.patchPath.detach();
    canvas.save();
    canvas.clipPath(path, this.ck.ClipOp.Intersect, false);
    canvas.drawPaint(this.backgroundPaint);
    this.calls++;
    this.drawScene(canvas, u, false, list);
    canvas.restore();
    path.delete();
    list.length = 0;
    return count;
  }

  /** The static items, in z-order, that meet any of the first `count` patch rectangles. */
  private patchItemsIn(count: number): SceneItem[] {
    const list = this.patchItems;
    const clips = this.patchClip;
    list.length = 0;
    for (const item of this.scene.items()) {
      if (item.live || !intersects(item.bounds, this.patchUnion)) {
        continue;
      }
      for (let i = 0; i < count; i++) {
        const clip = clips[i];
        if (clip && intersects(item.bounds, clip)) {
          list.push(item);
          break;
        }
      }
    }
    return list;
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
    this.pictures.length = 0;
    for (const item of list) {
      this.addItem(canvas, item);
    }
    this.calls += this.batches.flush(canvas);
    for (const item of this.pictures) {
      this.drawPicture(canvas, item.spec);
    }
    for (const item of this.texts) {
      this.drawText(canvas, item.spec);
    }
  }

  /** Decodes `bytes` and keeps the image under `key`; an existing key is left as it is. */
  setImage(key: string, bytes: Uint8Array): boolean {
    if (this.images.has(key)) {
      return true;
    }
    const image = this.ck.MakeImageFromEncoded(bytes);
    if (image === null) {
      return false;
    }
    this.images.set(key, this.resources.own(image));
    return true;
  }

  /**
   * The world rectangle `rect` as an encoded PNG `width` pixels wide, rendered from the scene on a
   * surface of its own (STU-052, A5.6); null when the surface or the encoding is unavailable.
   */
  thumbnail(rect: Rect, width: number): Uint8Array | null {
    const w = Math.max(1, Math.round(width));
    const h = Math.max(1, Math.round((w * (rect.b - rect.t)) / Math.max(1, rect.r - rect.l)));
    const surface = this.ck.MakeSurface(w, h);
    if (surface === null) {
      return null;
    }
    const canvas = surface.getCanvas();
    canvas.clear(this.background);
    canvas.scale(w / (rect.r - rect.l), w / (rect.r - rect.l));
    canvas.translate(-rect.l, -rect.t);
    this.drawScene(canvas, rect, false);
    surface.flush();
    const image = surface.makeImageSnapshot();
    const png = image.encodeToBytes();
    image.delete();
    surface.delete();
    return png;
  }

  private drawPicture(canvas: Canvas, spec: ItemSpec): void {
    const image = spec.kind === "rect" && spec.image !== undefined ? this.images.get(spec.image) : undefined;
    if (spec.kind !== "rect" || image === undefined) {
      return;
    }
    const dst = this.toArray(setRect(this.tileWorld, spec.x, spec.y, spec.x + spec.w, spec.y + spec.h), this.dst);
    const src = this.src;
    src[0] = 0;
    src[1] = 0;
    src[2] = image.width();
    src[3] = image.height();
    canvas.save();
    canvas.clipRRect(this.ck.RRectXY(dst, spec.radius, spec.radius), this.ck.ClipOp.Intersect, true);
    canvas.drawImageRectOptions(image, src, dst, this.ck.FilterMode.Linear, this.ck.MipmapMode.None, this.picture);
    canvas.restore();
    this.calls++;
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
        if (spec.image !== undefined) {
          this.pictures.push(item);
        }
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
    // At least this many chunks are left; cells may split them into more.
    const patches = Math.ceil(this.patches.length / PATCH_CHUNK);
    return patches + this.tiles.pending(this.view, this.interacting, this.pendingTiles, this.scratchTiles).length;
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
    this.viewChanged = false;
    let rendered = this.patchIdle(deadline, yieldNow);
    const visible = this.tiles.collect(this.view, this.visible);
    for (const tile of this.tiles.pending(this.view, this.interacting, this.pendingTiles, this.scratchTiles)) {
      if (rendered > 0 && (this.now() >= deadline || yieldNow())) {
        break;
      }
      this.renderTile(tile, deadline, yieldNow);
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
    this.drainScene();
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
    // The tile may still show lifted items: idle time masks them again.
    const world = this.tiles.tileRect(tile, this.tileWorld);
    for (const mask of this.masks) {
      if (intersects(mask, world)) {
        this.patches.push(mask);
      }
    }
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
    this.requeueMasks();
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

  private renderTile(tile: Tile<Image>, deadline: number, yieldNow: () => boolean): void {
    this.partial = null;
    const surface = this.tileSurface;
    if (!surface) {
      this.shownTiles.delete(tile);
      return;
    }
    if (this.refreshTile(tile, surface, deadline, yieldNow)) {
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
   * Re-renders only the dirty pieces of a full-resolution tile over its
   * cached pixels — each snapped to whole tile pixels, drawn behind one
   * non-antialiased clip per chunk of `REFRESH_CHUNK` pieces, not their union —
   * in chunks until `deadline` or `yieldNow` (at least one chunk); pieces
   * left over keep the tile dirty for the next idle slice. Returns false
   * when the tile needs a whole render. A tile the surface shows with all
   * damage drawn stays shown: its pixels are already on the surface.
   */
  private refreshTile(tile: Tile<Image>, surface: Surface, deadline: number, yieldNow: () => boolean): boolean {
    const image = tile.image;
    const pieces = tile.pieces;
    if (image === null || tile.dirty === null || pieces.length === 0 || tile.lowRes || this.interacting) {
      return false;
    }
    const start = this.now();
    const px = Math.max(1, Math.round(this.options.tileSize));
    const world = this.tiles.tileRect(tile, this.tileTarget);
    const current = this.shownTiles.has(tile) && this.damage.length === 0;
    const fromBase = current && this.baseIsCurrent();
    const canvas = surface.getCanvas();
    canvas.save();
    canvas.clipRect(this.src.fill(0).fill(px, 2), this.ck.ClipOp.Intersect, false);
    canvas.drawImage(image, 0, 0);
    let done = 0;
    while (done < pieces.length) {
      if (done > 0 && (this.now() >= deadline || yieldNow())) {
        break;
      }
      const end = Math.min(pieces.length, done + REFRESH_CHUNK);
      this.refreshPieces(canvas, pieces, done, end, world, px, fromBase);
      done = end;
    }
    canvas.restore();
    this.irect.set([0, 0, px, px]);
    const snapshot = this.resources.own(surface.makeImageSnapshot(this.irect));
    if (done < pieces.length) {
      this.tiles.storePartial(tile, snapshot, pieces.slice(done));
    } else {
      this.tiles.store(tile, snapshot, false);
    }
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

  /**
   * Redraws `pieces[from..to)` of the tile at `world` ([px] pixels square)
   * on `canvas`, which holds its cached image: each piece snapped to whole
   * pixels, one clip of all of them, the background, then the static items
   * that meet any of them, in z-order.
   */
  private refreshPieces(
    canvas: Canvas,
    pieces: readonly Rect[],
    from: number,
    to: number,
    world: Rect,
    px: number,
    fromBase: boolean,
  ): void {
    const scale = this.tiles.scale;
    const u = setRect(this.patchUnion, Infinity, Infinity, -Infinity, -Infinity);
    const clips = this.patchClip;
    const bounds = this.pieceBounds.fill(0);
    bounds[0] = px;
    bounds[1] = px;
    let count = 0;
    for (let i = from; i < to; i++) {
      const piece = pieces[i] as Rect;
      const l = Math.max(0, Math.floor((piece.l - world.l) * scale));
      const t = Math.max(0, Math.floor((piece.t - world.t) * scale));
      const r = Math.min(px, Math.ceil((piece.r - world.l) * scale));
      const b = Math.min(px, Math.ceil((piece.b - world.t) * scale));
      if (r <= l || b <= t) {
        continue;
      }
      let clip = clips[count];
      if (!clip) {
        clip = makeRect(0, 0, 0, 0);
        clips.push(clip);
      }
      setRect(clip, world.l + l / scale, world.t + t / scale, world.l + r / scale, world.t + b / scale);
      this.patchRect.set([l, t, r, b]);
      this.patchPath.addRect(this.patchRect);
      unionInto(u, clip);
      bounds.set([
        Math.min(bounds[0] ?? l, l),
        Math.min(bounds[1] ?? t, t),
        Math.max(bounds[2] ?? r, r),
        Math.max(bounds[3] ?? b, b),
      ]);
      count++;
    }
    if (count === 0) {
      return;
    }
    if (fromBase && this.copyFromBase(canvas, world, bounds)) {
      return;
    }
    const list = this.itemsMeeting(u, count);
    const path = this.patchPath.detach();
    canvas.save();
    canvas.clipPath(path, this.ck.ClipOp.Intersect, false);
    canvas.clear(this.background);
    canvas.scale(scale, scale);
    canvas.translate(-world.l, -world.t);
    this.drawScene(canvas, u, false, list);
    canvas.restore();
    path.delete();
    list.length = 0;
  }

  /** The static items, in z-order, inside `union` that meet any of the first `count` clips in `patchClip`. */
  private itemsMeeting(union: Rect, count: number): SceneItem[] {
    const list = this.patchItems;
    list.length = 0;
    for (const item of this.scene.query(union, this.items, false)) {
      for (let i = 0; i < count; i++) {
        const clip = this.patchClip[i];
        if (clip && intersects(item.bounds, clip)) {
          list.push(item);
          break;
        }
      }
    }
    return list;
  }

  /**
   * Whether the base surface holds exactly what a full render shows at the
   * tiles' scale: a CPU raster base (a snapshot of a GL surface stalls
   * software GL), placed at the current view, with no damage, patch or
   * dropped item left to draw and nothing live.
   */
  private baseIsCurrent(): boolean {
    return (
      !this.surface.reportBackendTypeIsGPU() &&
      !this.needsShift() &&
      this.tiles.scale === this.viewport.zoom * this.dpr &&
      this.damage.length === 0 &&
      this.patches.length === 0 &&
      this.dropItems.length === 0 &&
      this.masks.length === 0 &&
      !this.scene.hasLive
    );
  }

  /**
   * Copies the tile pixels `bounds` ([l, t, r, b]) from the base surface,
   * behind the clip of the pieces already in `patchPath`, when the base
   * covers them: base and tile pixels map one to one at the tiles' scale, so
   * a refresh of what the base already shows needs no redraw. Returns
   * whether it copied; only then is the clip path consumed.
   */
  private copyFromBase(canvas: Canvas, world: Rect, bounds: Float32Array): boolean {
    const scale = this.tiles.scale;
    const bx = Math.round(world.l * scale) - Math.round(this.viewport.x * scale);
    const by = Math.round(world.t * scale) - Math.round(this.viewport.y * scale);
    const [l = 0, t = 0, r = 0, b = 0] = bounds;
    if (bx + l < 0 || by + t < 0 || bx + r > this.surface.width() || by + b > this.surface.height()) {
      return false;
    }
    this.irect.set([bx + l, by + t, bx + r, by + b]);
    const snapshot = this.surface.makeImageSnapshot(this.irect);
    const path = this.patchPath.detach();
    canvas.save();
    canvas.clipPath(path, this.ck.ClipOp.Intersect, false);
    canvas.drawImage(snapshot, l, t);
    canvas.restore();
    path.delete();
    snapshot.delete();
    this.copiedRefreshes++;
    return true;
  }

  /** Tile refreshes copied from the base instead of redrawn, since creation; for tests and the benchmark. */
  get copiedTileRefreshes(): number {
    return this.copiedRefreshes;
  }

  /** Deletes every CanvasKit object the renderer owns. */
  dispose(): void {
    this.tiles.clear();
    this.paragraphs.clear();
    this.images.clear();
    this.resources.dispose();
  }
}
