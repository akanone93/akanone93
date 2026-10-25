// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The retained scene graph of Plux Canvas (STU-003, ADR-0013): frames and
 * items with bounds, z-order and dirty tracking. The scene changes only
 * through `apply` (patches from the edit pipeline); every change records the
 * world rectangles it touched, which the renderer turns into tile
 * invalidations. Queries fill caller-owned arrays, so the draw path
 * allocates nothing.
 */

import { copyRect, intersects, type Rect, setRect } from "./geometry.ts";
import type { ItemSpec, LinkSpec, Point, SceneOp, StrokeStyle } from "./protocol.ts";

/** Extra bounds around a shadowed frame for its pre-rendered shadow. */
export const SHADOW_MARGIN = 24;
/** Text height as a multiple of its size, for bounds before layout. */
export const TEXT_HEIGHT_FACTOR = 1.4;

/** A scene item: its spec plus derived bounds and batching keys. */
export interface SceneItem {
  spec: ItemSpec;
  readonly bounds: Rect;
  readonly seq: number;
  /** Live items are being dragged: drawn every frame, never into tiles. */
  live: boolean;
  fillKey: string | null;
  strokeKey: string | null;
}

/** The batching key of a fill colour. */
export function fillKey(color: number): string {
  return `f${color}`;
}

/** The batching key of a stroke style. */
export function strokeKey(stroke: StrokeStyle): string {
  return `s${stroke.color}:${stroke.width}`;
}

function linkBounds(spec: LinkSpec, out: Rect): Rect {
  setRect(out, spec.from.x, spec.from.y, spec.from.x, spec.from.y);
  for (const p of [spec.to, spec.c1, spec.c2]) {
    if (p) {
      setRect(out, Math.min(out.l, p.x), Math.min(out.t, p.y), Math.max(out.r, p.x), Math.max(out.b, p.y));
    }
  }
  const half = spec.stroke.width / 2 + 1;
  return setRect(out, out.l - half, out.t - half, out.r + half, out.b + half);
}

/** The world bounds of `spec`, including stroke and shadow, written into `out`. */
export function boundsOf(spec: ItemSpec, out: Rect): Rect {
  switch (spec.kind) {
    case "link":
      return linkBounds(spec, out);
    case "text":
      return setRect(out, spec.x, spec.y, spec.x + spec.w, spec.y + spec.size * TEXT_HEIGHT_FACTOR);
    default: {
      const pad =
        (spec.stroke ? spec.stroke.width / 2 : 0) + (spec.kind === "frame" && spec.shadow ? SHADOW_MARGIN : 0);
      return setRect(out, spec.x - pad, spec.y - pad, spec.x + spec.w + pad, spec.y + spec.h + pad);
    }
  }
}

function shift(p: Point, dx: number, dy: number): Point {
  return { x: p.x + dx, y: p.y + dy };
}

/** Antialiasing allowance around a moved box edge, in world units. */
const EDGE_AA = 1;

function sameItem(a: ItemSpec, b: ItemSpec): boolean {
  const ka = Object.keys(a) as (keyof ItemSpec)[];
  if (ka.length !== Object.keys(b).length) {
    return false;
  }
  for (const key of ka) {
    const va: unknown = a[key];
    const vb: unknown = (b as unknown as Record<string, unknown>)[key];
    if (va !== vb && !(typeof va === "object" && typeof vb === "object" && sameShallow(va, vb))) {
      return false;
    }
  }
  return true;
}

function sameShallow(a: unknown, b: unknown): boolean {
  if (a === null || b === null || typeof a !== "object" || typeof b !== "object") {
    return a === b;
  }
  const ra = a as Record<string, unknown>;
  const rb = b as Record<string, unknown>;
  const keys = Object.keys(ra);
  return keys.length === Object.keys(rb).length && keys.every((k) => ra[k] === rb[k]);
}

/** Whether two boxes are drawn alike apart from their geometry. */
function sameBoxStyle(a: ItemSpec, b: ItemSpec): boolean {
  if (a.kind !== b.kind || (a.kind !== "rect" && a.kind !== "frame") || (b.kind !== "rect" && b.kind !== "frame")) {
    return false;
  }
  if (a.kind === "frame" && b.kind === "frame" && (a.shadow || b.shadow || a.name !== b.name)) {
    return false;
  }
  return (
    a.z === b.z &&
    a.radius === b.radius &&
    a.fill === b.fill &&
    a.stroke?.color === b.stroke?.color &&
    a.stroke?.width === b.stroke?.width &&
    (a.kind !== "rect" || b.kind !== "rect" || (a.frame === b.frame && a.label === b.label))
  );
}

/**
 * Pushes to `out` the world rectangles whose pixels change when `old` (with
 * bounds `oldBounds`) becomes `next` (with bounds `nextBounds`). A box that
 * only moved an edge, with the same fill and stroke, damages just a strip
 * around each moved edge — its interior paints the same — so widening a
 * column does not dirty the whole column; anything else damages the old and
 * the new bounds.
 */
export function damageOf(old: ItemSpec, oldBounds: Rect, next: ItemSpec, nextBounds: Rect, out: Rect[]): void {
  if (sameItem(old, next)) {
    return;
  }
  if (
    !sameBoxStyle(old, next) ||
    old.kind === "link" ||
    old.kind === "text" ||
    next.kind === "link" ||
    next.kind === "text"
  ) {
    out.push(copyRect(oldBounds), copyRect(nextBounds));
    return;
  }
  const pad = (next.stroke ? next.stroke.width / 2 : 0) + next.radius + EDGE_AA;
  const t = Math.min(oldBounds.t, nextBounds.t) - pad;
  const b = Math.max(oldBounds.b, nextBounds.b) + pad;
  const l = Math.min(oldBounds.l, nextBounds.l) - pad;
  const r = Math.max(oldBounds.r, nextBounds.r) + pad;
  const edges: readonly [number, number, boolean][] = [
    [old.x, next.x, false],
    [old.x + old.w, next.x + next.w, false],
    [old.y, next.y, true],
    [old.y + old.h, next.y + next.h, true],
  ];
  const strips: Rect[] = [];
  let stripArea = 0;
  for (const [a, z, horizontal] of edges) {
    if (a === z) {
      continue;
    }
    const lo = Math.min(a, z) - pad;
    const hi = Math.max(a, z) + pad;
    const strip = horizontal ? { l, t: lo, r, b: hi } : { l: lo, t, r: hi, b };
    stripArea += area(strip);
    strips.push(strip);
  }
  // Strips only pay off when they cover less than the two boxes (a resize, not a move of a small box).
  if (stripArea >= area(oldBounds) + area(nextBounds)) {
    out.push(copyRect(oldBounds), copyRect(nextBounds));
  } else {
    out.push(...strips);
  }
}

function area(rect: Rect): number {
  return Math.max(0, rect.r - rect.l) * Math.max(0, rect.b - rect.t);
}

/** `spec` moved by `dx`, `dy`. */
export function moveSpec(spec: ItemSpec, dx: number, dy: number): ItemSpec {
  if (spec.kind !== "link") {
    return { ...spec, x: spec.x + dx, y: spec.y + dy };
  }
  return {
    ...spec,
    from: shift(spec.from, dx, dy),
    to: shift(spec.to, dx, dy),
    ...(spec.c1 ? { c1: shift(spec.c1, dx, dy) } : {}),
    ...(spec.c2 ? { c2: shift(spec.c2, dx, dy) } : {}),
  };
}

function keysOf(spec: ItemSpec): { fill: string | null; stroke: string | null } {
  switch (spec.kind) {
    case "link":
      return { fill: null, stroke: strokeKey(spec.stroke) };
    case "text":
      return { fill: fillKey(spec.color), stroke: null };
    default:
      return {
        fill: spec.fill === undefined ? null : fillKey(spec.fill),
        stroke: spec.stroke ? strokeKey(spec.stroke) : null,
      };
  }
}

/** The retained scene graph. */
export class Scene {
  /** Incremented by every `apply`. */
  version = 0;
  private readonly byId = new Map<string, SceneItem>();
  private readonly ordered: SceneItem[] = [];
  private orderDirty = false;
  private seq = 0;
  private readonly dirty: Rect[] = [];
  private readonly removed: string[] = [];
  /** Ids marked live, kept so an item added mid-drag (a re-routed arrow's new piece) is live too. */
  private readonly liveIds = new Set<string>();

  /** Applies patches in order. */
  apply(ops: readonly SceneOp[]): void {
    for (const op of ops) {
      this.applyOp(op);
    }
    this.version++;
  }

  private applyOp(op: SceneOp): void {
    switch (op.op) {
      case "add":
      case "replace":
        this.put(op.item);
        break;
      case "move": {
        const item = this.byId.get(op.id);
        if (item) {
          this.put(moveSpec(item.spec, op.dx, op.dy));
        }
        break;
      }
      case "remove":
        this.remove(op.id);
        break;
      case "clear":
        for (const id of [...this.byId.keys()]) {
          this.remove(id);
        }
        break;
    }
  }

  private put(spec: ItemSpec): void {
    const existing = this.byId.get(spec.id);
    if (existing) {
      if (existing.spec.z !== spec.z) {
        this.orderDirty = true;
      }
      const old = existing.spec;
      const oldBounds = copyRect(existing.bounds);
      existing.spec = spec;
      this.derive(existing);
      if (!existing.live) {
        damageOf(old, oldBounds, spec, existing.bounds, this.dirty);
      }
      return;
    }
    const item: SceneItem = {
      spec,
      bounds: { l: 0, t: 0, r: 0, b: 0 },
      seq: this.seq++,
      live: this.liveIds.has(spec.id),
      fillKey: null,
      strokeKey: null,
    };
    this.derive(item);
    this.byId.set(spec.id, item);
    this.ordered.push(item);
    this.orderDirty = true;
    this.markDirty(item);
  }

  private derive(item: SceneItem): void {
    boundsOf(item.spec, item.bounds);
    const keys = keysOf(item.spec);
    item.fillKey = keys.fill;
    item.strokeKey = keys.stroke;
  }

  private remove(id: string): void {
    const item = this.byId.get(id);
    if (!item) {
      return;
    }
    this.markDirty(item);
    this.byId.delete(id);
    this.ordered.splice(this.ordered.indexOf(item), 1);
    this.removed.push(id);
  }

  private markDirty(item: SceneItem): void {
    if (!item.live) {
      this.dirty.push(copyRect(item.bounds));
    }
  }

  /** The item with `id`, if any. */
  get(id: string): SceneItem | undefined {
    return this.byId.get(id);
  }

  /** The number of items. */
  get size(): number {
    return this.byId.size;
  }

  /** All items in z-order (z, then insertion order). */
  items(): readonly SceneItem[] {
    if (this.orderDirty) {
      this.ordered.sort((a, b) => a.spec.z - b.spec.z || a.seq - b.seq);
      this.orderDirty = false;
    }
    return this.ordered;
  }

  /** Fills `out` with the items overlapping `rect`, in z-order; live items only if asked. */
  query(rect: Rect, out: SceneItem[], includeLive: boolean): SceneItem[] {
    out.length = 0;
    for (const item of this.items()) {
      if ((includeLive || !item.live) && intersects(item.bounds, rect)) {
        out.push(item);
      }
    }
    return out;
  }

  /** Fills `out` with the live items, in z-order. */
  liveItems(out: SceneItem[]): SceneItem[] {
    out.length = 0;
    for (const item of this.items()) {
      if (item.live) {
        out.push(item);
      }
    }
    return out;
  }

  /** Marks items live (dragged) or static; both transitions dirty the item's bounds. */
  setLive(ids: readonly string[], live: boolean): void {
    for (const id of ids) {
      if (live) {
        this.liveIds.add(id);
      } else {
        this.liveIds.delete(id);
      }
      const item = this.byId.get(id);
      if (item && item.live !== live) {
        item.live = false;
        this.markDirty(item);
        item.live = live;
      }
    }
  }

  /** Whether rectangles wait to be drained: the static content changed. */
  get hasDirty(): boolean {
    return this.dirty.length > 0;
  }

  /** Passes every dirty world rectangle since the last drain to `visit`. */
  drainDirty(visit: (rect: Rect) => void): void {
    for (const rect of this.dirty) {
      visit(rect);
    }
    this.dirty.length = 0;
  }

  /** Passes every id removed since the last drain to `visit`. */
  drainRemoved(visit: (id: string) => void): void {
    for (const id of this.removed) {
      visit(id);
    }
    this.removed.length = 0;
  }
}
