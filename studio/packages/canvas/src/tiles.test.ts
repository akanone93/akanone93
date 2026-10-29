// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { makeRect } from "./geometry.ts";
import { MAX_TILE_PIECES, type Tile, TileCache } from "./tiles.ts";

interface FakeImage {
  readonly id: number;
  released: boolean;
}

function cache(maxTiles = 48): { tiles: TileCache<FakeImage>; images: FakeImage[] } {
  const images: FakeImage[] = [];
  const tiles = new TileCache<FakeImage>(
    512,
    (image) => {
      image.released = true;
    },
    maxTiles,
  );
  return { tiles, images };
}

function render(tiles: TileCache<FakeImage>, images: FakeImage[], list: Tile<FakeImage>[], lowRes = false): void {
  for (const tile of list) {
    const image = { id: images.length, released: false };
    images.push(image);
    tiles.store(tile, image, lowRes);
  }
}

describe("[STU-120] tile cache (M-12)", () => {
  test("tiles cover the view at the current scale", () => {
    const { tiles } = cache();
    expect(tiles.setScale(1)).toBe(true);
    const out: Tile<FakeImage>[] = [];
    tiles.collect(makeRect(0, 0, 1280, 800), out);
    expect(out.map((t) => [t.ix, t.iy])).toEqual([
      [0, 0],
      [1, 0],
      [2, 0],
      [0, 1],
      [1, 1],
      [2, 1],
    ]);
    expect(tiles.tileRect(out[4] as Tile<FakeImage>, makeRect(0, 0, 0, 0))).toEqual({
      l: 512,
      t: 512,
      r: 1024,
      b: 1024,
    });
    expect(tiles.collect(makeRect(0, 0, 1, 1), out, 1)).toHaveLength(9);
  });

  test("a dirty rectangle invalidates exactly the tiles it overlaps", () => {
    const { tiles, images } = cache();
    tiles.setScale(1);
    const view = makeRect(0, 0, 1280, 800);
    const out: Tile<FakeImage>[] = [];
    render(tiles, images, tiles.collect(view, out));
    tiles.invalidate(makeRect(500, 100, 520, 120));
    tiles.invalidate(makeRect(505, 110, 530, 130));
    const dirty = tiles.collect(view, out).filter((t) => t.dirty !== null);
    expect(dirty.map((t) => [t.ix, t.iy])).toEqual([
      [0, 0],
      [1, 0],
    ]);
    expect(dirty[0]?.dirty).toEqual({ l: 500, t: 100, r: 530, b: 130 });
    const pending: Tile<FakeImage>[] = [];
    expect(tiles.pending(view, false, pending, [])).toHaveLength(2 + 14);
    render(tiles, images, [dirty[0] as Tile<FakeImage>]);
    expect(images[0]?.released).toBe(true);
    expect(dirty[0]?.dirty).toBeNull();
  });

  test("M-13 a tile keeps its dirty pieces apart, clipped to it, and merges them beyond the cap", () => {
    const { tiles, images } = cache();
    tiles.setScale(1);
    const view = makeRect(0, 0, 1280, 800);
    const out: Tile<FakeImage>[] = [];
    render(tiles, images, tiles.collect(view, out));
    tiles.invalidate(makeRect(10, 10, 20, 20));
    tiles.invalidate(makeRect(400, 400, 600, 410));
    const tile = tiles.collect(view, out)[0] as Tile<FakeImage>;
    expect(tile.pieces).toEqual([
      { l: 10, t: 10, r: 20, b: 20 },
      { l: 400, t: 400, r: 512, b: 410 },
    ]);
    for (let i = 0; i < MAX_TILE_PIECES; i++) {
      tiles.invalidate(makeRect(i * 4, 100, i * 4 + 2, 102));
    }
    expect(tile.pieces).toHaveLength(MAX_TILE_PIECES);
    // A partial refresh keeps the pieces it did not redraw dirty.
    const rest = tile.pieces.slice(60);
    tiles.storePartial(tile, { id: 999, released: false }, rest);
    expect(tile.pieces).toEqual(rest);
    expect(tile.dirty).not.toBeNull();
    expect(tiles.needsWork(tile, false)).toBe(true);
  });

  test("panning within 25 % of zoom reuses tiles; a larger change re-tiles and keeps stale tiles until covered", () => {
    const { tiles, images } = cache();
    tiles.setScale(1);
    const view = makeRect(0, 0, 1000, 1000);
    const out: Tile<FakeImage>[] = [];
    render(tiles, images, tiles.collect(view, out));
    expect(tiles.setScale(1.2)).toBe(false);
    expect(tiles.setScale(0.8)).toBe(false);
    expect(tiles.setScale(1.3)).toBe(true);
    expect(tiles.hasStale).toBe(true);
    expect(tiles.stale(view, out)).toHaveLength(4);
    expect(tiles.imageCount).toBe(4);
    tiles.invalidate(makeRect(0, 0, 10, 10));
    expect(tiles.stale(view, out)).toHaveLength(3);
    tiles.dropStaleIfCovered(view, out);
    expect(tiles.hasStale).toBe(true);
    render(tiles, images, tiles.collect(view, []));
    tiles.dropStaleIfCovered(view, out);
    expect(tiles.hasStale).toBe(false);
    tiles.dropStaleIfCovered(view, out);
    expect(images.slice(0, 4).every((image) => image.released)).toBe(true);
  });

  test("low-resolution tiles are refined only when the interaction ends", () => {
    const { tiles, images } = cache();
    tiles.setScale(1);
    const view = makeRect(0, 0, 100, 100);
    render(tiles, images, tiles.collect(view, [], 1), true);
    expect(tiles.pending(view, true, [], [])).toHaveLength(0);
    expect(tiles.pending(view, false, [], [])).toHaveLength(9);
  });

  test("eviction keeps tiles near the view and clear releases everything", () => {
    const { tiles, images } = cache(4);
    tiles.setScale(1);
    render(tiles, images, tiles.collect(makeRect(0, 0, 3000, 600), []));
    tiles.evict(makeRect(0, 0, 100, 100));
    expect(tiles.imageCount).toBe(1);
    tiles.evict(makeRect(0, 0, 100, 100));
    tiles.clear();
    expect(tiles.imageCount).toBe(0);
    expect(images.every((image) => image.released)).toBe(true);
    expect(tiles.scale).toBe(0);
  });
});
