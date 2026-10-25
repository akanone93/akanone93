// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Hit-testing over item bounds in world space (ADR-0013, M-12). The index is
 * a uniform grid of buckets, kept up to date from the same patch stream the
 * scene receives, so a point or rectangle query answers synchronously on the
 * thread that holds it, without a render round-trip. Items spanning more
 * than `MAX_CELLS` cells (long links, large frames) sit in one list checked on
 * every query instead of filling hundreds of buckets.
 *
 * Results are topmost first: higher `z`, then later insertion, as drawn. An
 * item inside a frame is clipped to the frame's rectangle; a link is hit near
 * its path, not anywhere in its bounding box.
 */

import { intersects, makeRect, type Rect, setRect } from "./geometry.ts";
import type { ItemSpec, LinkSpec, Point, SceneOp } from "./protocol.ts";
import { boundsOf, moveSpec } from "./scene.ts";

/** The default bucket size in world units. */
export const HIT_CELL = 256;
/** An item spanning more cells than this is kept in the list of large items. */
export const MAX_CELLS = 64;
/** Segments a cubic link is flattened into for hit-testing. */
const CURVE_STEPS = 16;

interface Entry {
  spec: ItemSpec;
  readonly bounds: Rect;
  readonly seq: number;
  /** The cells the entry is in; null while it is in the list of large items. */
  cells: number[] | null;
}

/** How a rectangle query selects items. */
export type RectMode = "intersect" | "contain";

function cellKey(ix: number, iy: number): number {
  // 2^21 cells per axis either side of the origin: far beyond any canvas.
  return (ix + 0x100000) * 0x200000 + (iy + 0x100000);
}

function frameOf(spec: ItemSpec): string | null {
  return spec.kind === "rect" || spec.kind === "text" ? (spec.frame ?? null) : null;
}

function distanceToSegment(px: number, py: number, a: Point, b: Point): number {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const length = dx * dx + dy * dy;
  const t = length === 0 ? 0 : Math.max(0, Math.min(1, ((px - a.x) * dx + (py - a.y) * dy) / length));
  return Math.hypot(px - (a.x + t * dx), py - (a.y + t * dy));
}

function cubicAt(spec: LinkSpec, c1: Point, c2: Point, t: number): Point {
  const u = 1 - t;
  const a = u * u * u;
  const b = 3 * u * u * t;
  const c = 3 * u * t * t;
  const d = t * t * t;
  return {
    x: a * spec.from.x + b * c1.x + c * c2.x + d * spec.to.x,
    y: a * spec.from.y + b * c1.y + c * c2.y + d * spec.to.y,
  };
}

function nearLink(spec: LinkSpec, x: number, y: number, tolerance: number): boolean {
  const reach = spec.stroke.width / 2 + tolerance;
  const { c1, c2 } = spec;
  if (!c1 || !c2) {
    return distanceToSegment(x, y, spec.from, spec.to) <= reach;
  }
  let previous = spec.from;
  for (let i = 1; i <= CURVE_STEPS; i++) {
    const next = cubicAt(spec, c1, c2, i / CURVE_STEPS);
    if (distanceToSegment(x, y, previous, next) <= reach) {
      return true;
    }
    previous = next;
  }
  return false;
}

/** The rectangle `spec` occupies for hit-testing: its shape, without shadow. */
export function shapeOf(spec: ItemSpec, out: Rect): Rect {
  if (spec.kind === "frame") {
    return setRect(out, spec.x, spec.y, spec.x + spec.w, spec.y + spec.h);
  }
  return boundsOf(spec, out);
}

/** A spatial index of the scene's items for point and rectangle queries. */
export class HitIndex {
  private readonly entries = new Map<string, Entry>();
  private readonly buckets = new Map<number, Entry[]>();
  private readonly large: Entry[] = [];
  private seq = 0;
  private readonly shape = makeRect(0, 0, 0, 0);
  private readonly clip = makeRect(0, 0, 0, 0);
  private readonly probe = makeRect(0, 0, 0, 0);

  constructor(readonly cell = HIT_CELL) {}

  /** The number of indexed items. */
  get size(): number {
    return this.entries.size;
  }

  /** The spec of the item `id`, if indexed. */
  spec(id: string): ItemSpec | undefined {
    return this.entries.get(id)?.spec;
  }

  /** The hit shape of item `id` in world units written into `out`, or null. */
  shapeOf(id: string, out: Rect): Rect | null {
    const entry = this.entries.get(id);
    return entry ? shapeOf(entry.spec, out) : null;
  }

  /** Applies scene patches in order, exactly as the scene does. */
  apply(ops: readonly SceneOp[]): void {
    for (const op of ops) {
      switch (op.op) {
        case "add":
        case "replace":
          this.put(op.item);
          break;
        case "move": {
          const entry = this.entries.get(op.id);
          if (entry) {
            this.put(moveSpec(entry.spec, op.dx, op.dy));
          }
          break;
        }
        case "remove":
          this.remove(op.id);
          break;
        case "clear":
          this.entries.clear();
          this.buckets.clear();
          this.large.length = 0;
          break;
      }
    }
  }

  private put(spec: ItemSpec): void {
    const existing = this.entries.get(spec.id);
    if (existing) {
      this.unlink(existing);
      existing.spec = spec;
      boundsOf(spec, existing.bounds);
      this.link(existing);
      return;
    }
    const entry: Entry = { spec, bounds: boundsOf(spec, makeRect(0, 0, 0, 0)), seq: this.seq++, cells: null };
    this.entries.set(spec.id, entry);
    this.link(entry);
  }

  private remove(id: string): void {
    const entry = this.entries.get(id);
    if (entry) {
      this.unlink(entry);
      this.entries.delete(id);
    }
  }

  private range(rect: Rect): [number, number, number, number] {
    const c = this.cell;
    return [Math.floor(rect.l / c), Math.floor(rect.t / c), Math.floor(rect.r / c), Math.floor(rect.b / c)];
  }

  private link(entry: Entry): void {
    const [x0, y0, x1, y1] = this.range(entry.bounds);
    if ((x1 - x0 + 1) * (y1 - y0 + 1) > MAX_CELLS) {
      entry.cells = null;
      this.large.push(entry);
      return;
    }
    const cells: number[] = [];
    for (let ix = x0; ix <= x1; ix++) {
      for (let iy = y0; iy <= y1; iy++) {
        const key = cellKey(ix, iy);
        let bucket = this.buckets.get(key);
        if (!bucket) {
          bucket = [];
          this.buckets.set(key, bucket);
        }
        bucket.push(entry);
        cells.push(key);
      }
    }
    entry.cells = cells;
  }

  private unlink(entry: Entry): void {
    if (entry.cells === null) {
      this.large.splice(this.large.indexOf(entry), 1);
      return;
    }
    for (const key of entry.cells) {
      const bucket = this.buckets.get(key);
      if (!bucket) {
        continue;
      }
      bucket.splice(bucket.indexOf(entry), 1);
      if (bucket.length === 0) {
        this.buckets.delete(key);
      }
    }
  }

  /** Whether the frame of `entry`, if any, clips `rect` away entirely. */
  private clipped(entry: Entry, rect: Rect): boolean {
    const frame = frameOf(entry.spec);
    const owner = frame === null ? undefined : this.entries.get(frame);
    if (!owner) {
      return false;
    }
    shapeOf(owner.spec, this.clip);
    return !(rect.l <= this.clip.r && this.clip.l <= rect.r && rect.t <= this.clip.b && this.clip.t <= rect.b);
  }

  private candidates(rect: Rect): Entry[] {
    const found = new Set<Entry>();
    const [x0, y0, x1, y1] = this.range(rect);
    for (let ix = x0; ix <= x1; ix++) {
      for (let iy = y0; iy <= y1; iy++) {
        for (const entry of this.buckets.get(cellKey(ix, iy)) ?? []) {
          found.add(entry);
        }
      }
    }
    for (const entry of this.large) {
      found.add(entry);
    }
    return [...found].sort((a, b) => b.spec.z - a.spec.z || b.seq - a.seq);
  }

  /**
   * The ids of the items under world point (`x`, `y`), topmost first.
   * `tolerance` (world units) widens the hit area, e.g. 3 CSS pixels / zoom.
   */
  hitPoint(x: number, y: number, tolerance = 0): string[] {
    const probe = setRect(this.probe, x - tolerance, y - tolerance, x + tolerance, y + tolerance);
    const out: string[] = [];
    for (const entry of this.candidates(probe)) {
      const spec = entry.spec;
      if (spec.kind === "link") {
        if (!nearLink(spec, x, y, tolerance)) {
          continue;
        }
      } else {
        const shape = shapeOf(spec, this.shape);
        if (x < shape.l - tolerance || x > shape.r + tolerance || y < shape.t - tolerance || y > shape.b + tolerance) {
          continue;
        }
      }
      if (!this.clipped(entry, setRect(this.probe, x, y, x, y))) {
        out.push(spec.id);
      }
    }
    return out;
  }

  /**
   * The ids of the items meeting world rectangle `rect`, topmost first:
   * overlapping it (`intersect`, the default) or wholly inside it (`contain`).
   * Only the visible part of an item clipped by its frame counts.
   */
  hitRect(rect: Rect, mode: RectMode = "intersect"): string[] {
    const out: string[] = [];
    for (const entry of this.candidates(rect)) {
      const shape = shapeOf(entry.spec, this.shape);
      const frame = frameOf(entry.spec);
      const owner = frame === null ? undefined : this.entries.get(frame);
      if (owner) {
        const clip = shapeOf(owner.spec, this.clip);
        setRect(
          shape,
          Math.max(shape.l, clip.l),
          Math.max(shape.t, clip.t),
          Math.min(shape.r, clip.r),
          Math.min(shape.b, clip.b),
        );
        if (shape.l >= shape.r || shape.t >= shape.b) {
          continue;
        }
      }
      const hit =
        mode === "contain"
          ? shape.l >= rect.l && shape.r <= rect.r && shape.t >= rect.t && shape.b <= rect.b
          : intersects(shape, rect);
      if (hit) {
        out.push(entry.spec.id);
      }
    }
    return out;
  }
}
