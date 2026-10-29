// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The tile cache (ADR-0013 M-12): the static scene rendered into square
 * device-pixel tiles at one scale, reused while panning. A tile is
 * re-rendered when content in it changes (its dirty rectangle) or when the
 * scale drifts by more than 25 %; until then the old image, or the previous
 * scale's tiles, stay on screen. Rendering happens from an idle-time queue,
 * outside the frame. The cache is generic over the image type so it can be
 * tested without CanvasKit.
 */

import { intersects, makeRect, type Rect, setRect, unionInto } from "./geometry.ts";

/** Re-tile when the scale differs from the tiles' scale by more than this fraction. */
export const RETILE_THRESHOLD = 0.25;
/** The default tile size in device pixels. */
export const DEFAULT_TILE_SIZE = 512;

const OFFSET = 1 << 15;

/** One tile: its grid position, image, and what changed since it was rendered. */
export interface Tile<T> {
  readonly ix: number;
  readonly iy: number;
  image: T | null;
  /** Rendered at reduced resolution during an interaction. */
  lowRes: boolean;
  /** The world rectangle changed since the image was rendered: the union of the changes, `pieces` unclipped. */
  dirty: Rect | null;
  /**
   * The changed world rectangles themselves, clipped to the tile, at most
   * `MAX_TILE_PIECES` (beyond that a new one merges into the piece it grows
   * least): a refresh redraws only these, not their union.
   */
  readonly pieces: Rect[];
}

/** The most dirty pieces a tile keeps apart; more are merged. */
export const MAX_TILE_PIECES = 64;

function area(r: Rect): number {
  return (r.r - r.l) * (r.b - r.t);
}

/** Adds the world rectangle `rect` to `pieces`, merging it into the piece it grows least once there are `MAX_TILE_PIECES`. */
function addPiece(pieces: Rect[], rect: Rect): void {
  if (pieces.length < MAX_TILE_PIECES) {
    pieces.push(makeRect(rect.l, rect.t, rect.r, rect.b));
    return;
  }
  let best = 0;
  let growth = Infinity;
  for (let i = 0; i < pieces.length; i++) {
    const p = pieces[i] as Rect;
    const merged =
      (Math.max(p.r, rect.r) - Math.min(p.l, rect.l)) * (Math.max(p.b, rect.b) - Math.min(p.t, rect.t)) - area(p);
    if (merged < growth) {
      growth = merged;
      best = i;
    }
  }
  unionInto(pieces[best] as Rect, rect);
}

function keyOf(ix: number, iy: number): number {
  return (ix + OFFSET) * 65536 + (iy + OFFSET);
}

/** A grid of tiles at one scale, plus the previous scale's tiles as a fallback. */
export class TileCache<T> {
  /** Device pixels per world unit of the current grid; 0 before the first `setScale`. */
  scale = 0;
  private staleScaleValue = 0;
  private readonly tiles = new Map<number, Tile<T>>();
  private readonly staleTiles = new Map<number, Tile<T>>();
  private readonly scratch: Rect = makeRect(0, 0, 0, 0);
  private readonly piece: Rect = makeRect(0, 0, 0, 0);

  constructor(
    readonly tileSize: number,
    private readonly release: (image: T) => void,
    readonly maxTiles = 48,
  ) {}

  /** The world size of a tile of the current grid. */
  get worldSize(): number {
    return this.tileSize / this.scale;
  }

  /** The number of images held, current and stale. */
  get imageCount(): number {
    let count = 0;
    for (const map of [this.tiles, this.staleTiles]) {
      for (const tile of map.values()) {
        count += tile.image === null ? 0 : 1;
      }
    }
    return count;
  }

  /** Whether the previous scale's tiles are still shown as a fallback. */
  get hasStale(): boolean {
    return this.staleTiles.size > 0;
  }

  /** Sets the scale; re-tiles (keeping the old tiles as stale) when it drifts by more than 25 %. */
  setScale(scale: number): boolean {
    if (this.scale > 0 && Math.abs(scale / this.scale - 1) <= RETILE_THRESHOLD) {
      return false;
    }
    this.dropStale();
    for (const [key, tile] of this.tiles) {
      if (tile.image !== null) {
        this.staleTiles.set(key, tile);
      }
    }
    this.tiles.clear();
    this.staleScaleValue = this.scale;
    this.scale = scale;
    return true;
  }

  /** The world rectangle of `tile` (of the current grid, or the stale one). */
  tileRect(tile: Tile<T>, out: Rect, stale = false): Rect {
    const size = this.tileSize / (stale ? this.staleScaleValue : this.scale);
    return setRect(out, tile.ix * size, tile.iy * size, (tile.ix + 1) * size, (tile.iy + 1) * size);
  }

  /** Fills `out` with the tiles covering `view` grown by `margin` tiles, creating missing ones. */
  collect(view: Rect, out: Tile<T>[], margin = 0): Tile<T>[] {
    out.length = 0;
    const size = this.worldSize;
    const ix0 = Math.floor(view.l / size) - margin;
    const iy0 = Math.floor(view.t / size) - margin;
    const ix1 = Math.ceil(view.r / size) + margin;
    const iy1 = Math.ceil(view.b / size) + margin;
    for (let iy = iy0; iy < iy1; iy++) {
      for (let ix = ix0; ix < ix1; ix++) {
        out.push(this.tileAt(ix, iy));
      }
    }
    return out;
  }

  private tileAt(ix: number, iy: number): Tile<T> {
    const key = keyOf(ix, iy);
    let tile = this.tiles.get(key);
    if (!tile) {
      tile = { ix, iy, image: null, lowRes: false, dirty: null, pieces: [] };
      this.tiles.set(key, tile);
    }
    return tile;
  }

  /** Records that the world rectangle `rect` changed: overlapping tiles become dirty, stale ones are dropped. */
  invalidate(rect: Rect): void {
    for (const tile of this.tiles.values()) {
      const world = this.tileRect(tile, this.scratch);
      if (tile.image !== null && intersects(world, rect)) {
        const piece = setRect(
          this.piece,
          Math.max(rect.l, world.l),
          Math.max(rect.t, world.t),
          Math.min(rect.r, world.r),
          Math.min(rect.b, world.b),
        );
        tile.dirty = tile.dirty ? unionInto(tile.dirty, rect) : makeRect(rect.l, rect.t, rect.r, rect.b);
        addPiece(tile.pieces, piece);
      }
    }
    for (const [key, tile] of this.staleTiles) {
      if (intersects(this.tileRect(tile, this.scratch, true), rect)) {
        this.drop(this.staleTiles, key, tile);
      }
    }
  }

  /** Whether `tile` needs rendering: no image, dirty, or low-res after the interaction. */
  needsWork(tile: Tile<T>, interacting: boolean): boolean {
    return tile.image === null || tile.dirty !== null || (tile.lowRes && !interacting);
  }

  /** Fills `out` with the tiles needing work: those in `view` first, then a one-tile margin. */
  pending(view: Rect, interacting: boolean, out: Tile<T>[], scratch: Tile<T>[]): Tile<T>[] {
    out.length = 0;
    for (const tile of this.collect(view, scratch)) {
      if (this.needsWork(tile, interacting)) {
        out.push(tile);
      }
    }
    for (const tile of this.collect(view, scratch, 1)) {
      if (this.needsWork(tile, interacting) && !out.includes(tile)) {
        out.push(tile);
      }
    }
    return out;
  }

  /** Stores a freshly rendered image for `tile`, releasing the old one. */
  store(tile: Tile<T>, image: T, lowRes: boolean): void {
    if (tile.image !== null) {
      this.release(tile.image);
    }
    tile.image = image;
    tile.lowRes = lowRes;
    tile.dirty = null;
    tile.pieces.length = 0;
  }

  /**
   * Stores an image of `tile` in which only some of its dirty pieces were
   * redrawn: `rest` (taken from the tile's own pieces) stays dirty.
   */
  storePartial(tile: Tile<T>, image: T, rest: readonly Rect[]): void {
    const kept = rest.slice();
    this.store(tile, image, false);
    for (const piece of kept) {
      tile.pieces.push(piece);
      tile.dirty = tile.dirty ? unionInto(tile.dirty, piece) : makeRect(piece.l, piece.t, piece.r, piece.b);
    }
  }

  /** Fills `out` with the stale tiles overlapping `view`. */
  stale(view: Rect, out: Tile<T>[]): Tile<T>[] {
    out.length = 0;
    for (const tile of this.staleTiles.values()) {
      if (intersects(this.tileRect(tile, this.scratch, true), view)) {
        out.push(tile);
      }
    }
    return out;
  }

  /** Drops the stale tiles once every tile in `view` has a clean image. */
  dropStaleIfCovered(view: Rect, scratch: Tile<T>[]): void {
    if (!this.hasStale) {
      return;
    }
    for (const tile of this.collect(view, scratch)) {
      if (tile.image === null || tile.dirty !== null) {
        return;
      }
    }
    this.dropStale();
  }

  /** Releases tiles away from `keep` while more than `maxTiles` are held. */
  evict(keep: Rect): void {
    if (this.tiles.size <= this.maxTiles) {
      return;
    }
    for (const [key, tile] of this.tiles) {
      if (!intersects(this.tileRect(tile, this.scratch), keep)) {
        this.drop(this.tiles, key, tile);
      }
    }
  }

  /** Releases every image and forgets every tile. */
  clear(): void {
    this.dropStale();
    for (const [key, tile] of this.tiles) {
      this.drop(this.tiles, key, tile);
    }
    this.scale = 0;
  }

  private dropStale(): void {
    for (const [key, tile] of this.staleTiles) {
      this.drop(this.staleTiles, key, tile);
    }
  }

  private drop(map: Map<number, Tile<T>>, key: number, tile: Tile<T>): void {
    if (tile.image !== null) {
      this.release(tile.image);
      tile.image = null;
    }
    map.delete(key);
  }
}
