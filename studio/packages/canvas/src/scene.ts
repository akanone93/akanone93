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

import { copyRect, intersects, makeRect, type Rect, setRect } from "./geometry.ts";
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
  /** A link's piece hulls (`liveDamageOf`), computed on first use by `itemMeets`; null until then or after a change. */
  pieces?: Rect[] | null;
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

/** The pieces a link's damage is split into on the interaction layer. */
export const LINK_PIECES = 8;

function lerp(a: Point, b: Point, t: number): Point {
  return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t };
}

/** `bounds` grown by the antialiasing allowance: an edge on a bound still tints the pixel beyond it. */
function antialiased(bounds: Rect): Rect {
  return makeRect(bounds.l - EDGE_AA, bounds.t - EDGE_AA, bounds.r + EDGE_AA, bounds.b + EDGE_AA);
}

/** Pushes the bounds of `LINK_PIECES` consecutive pieces of a cubic, each the hull of its control points, grown by `pad`. */
function pushPieces(p0: Point, c1: Point, c2: Point, p3: Point, pad: number, out: Rect[]): void {
  for (let i = 0; i < LINK_PIECES; i++) {
    const t1 = (i + 1) / LINK_PIECES;
    // De Casteljau: split at t1, then split the first part at t0 / t1 and keep its end.
    const ab = lerp(p0, c1, t1);
    const bc = lerp(c1, c2, t1);
    const cd = lerp(c2, p3, t1);
    const abc = lerp(ab, bc, t1);
    const end = lerp(abc, lerp(bc, cd, t1), t1);
    const s = i / (i + 1);
    const q1 = lerp(ab, abc, s);
    const q2 = lerp(abc, end, s);
    const r1 = lerp(q1, q2, s);
    const start = lerp(lerp(lerp(p0, ab, s), q1, s), r1, s);
    out.push(
      makeRect(
        Math.min(start.x, r1.x, q2.x, end.x) - pad,
        Math.min(start.y, r1.y, q2.y, end.y) - pad,
        Math.max(start.x, r1.x, q2.x, end.x) + pad,
        Math.max(start.y, r1.y, q2.y, end.y) + pad,
      ),
    );
  }
}

/**
 * Pushes the world rectangles `spec` covers on the interaction layer onto
 * `out`: its bounds, or for a link pieces along it — of its curve and of the
 * straight line levels of detail draw instead — so a long diagonal link
 * damages a band along itself rather than its whole bounding box.
 */
export function liveDamageOf(spec: ItemSpec, bounds: Rect, out: Rect[]): void {
  if (spec.kind !== "link") {
    out.push(antialiased(bounds));
    return;
  }
  const pad = spec.stroke.width / 2 + 1;
  pushPieces(spec.from, spec.from, spec.to, spec.to, pad, out);
  if (spec.c1 && spec.c2) {
    pushPieces(spec.from, spec.c1, spec.c2, spec.to, pad, out);
  }
}

/**
 * Whether `item` may draw inside `rect`: its bounds meet it and, for a link,
 * one of the piece hulls along its curve or straight line does. A cubic lies
 * in the hull of its control points, so a link whose pieces all miss `rect`
 * leaves every pixel of it untouched; a patch along other links then skips
 * the long links whose bounding box merely spans it.
 */
export function itemMeets(item: SceneItem, rect: Rect): boolean {
  if (!intersects(item.bounds, rect)) {
    return false;
  }
  const spec = item.spec;
  if (spec.kind !== "link") {
    return true;
  }
  for (const piece of piecesOf(item)) {
    if (intersects(piece, rect)) {
      return true;
    }
  }
  return false;
}

/**
 * The piece hulls of a link item (`liveDamageOf`): `LINK_PIECES` along its
 * straight line, then as many along its curve when it has one; piece `i` of
 * each spans the curve parameters [i, i + 1] / `LINK_PIECES`. Cached on the item.
 */
export function piecesOf(item: SceneItem): Rect[] {
  let pieces = item.pieces;
  if (!pieces) {
    pieces = [];
    liveDamageOf(item.spec, item.bounds, pieces);
    item.pieces = pieces;
  }
  return pieces;
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
 * column does not dirty the whole column; a link damages the pieces along
 * its old and new curves (`liveDamageOf`); anything else damages the old and
 * the new bounds.
 */
export function damageOf(old: ItemSpec, oldBounds: Rect, next: ItemSpec, nextBounds: Rect, out: Rect[]): void {
  if (sameItem(old, next)) {
    return;
  }
  if (old.kind === "link" && next.kind === "link") {
    // Pieces along both curves, not their bounding boxes: a rerouted long link re-renders only the tiles it crosses.
    liveDamageOf(old, oldBounds, out);
    liveDamageOf(next, nextBounds, out);
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
  /** World rectangles the live items changed in since the last drain: the interaction layer's damage. */
  private readonly liveDirty: Rect[] = [];
  private readonly removed: string[] = [];
  /** Where items were lifted (made live) since the last drain: the base hides them there, the tiles keep them. */
  private readonly lifted: Rect[] = [];
  /** Where items were dropped (made static) since the last drain: the tiles take them in there. */
  private readonly dropped: Rect[] = [];
  /** The items dropped since the last drain. */
  private readonly droppedItems: SceneItem[] = [];
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
      if (existing.live) {
        liveDamageOf(old, oldBounds, this.liveDirty);
        liveDamageOf(spec, existing.bounds, this.liveDirty);
      } else {
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
    item.pieces = null;
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

  /** Damages the item's bounds on the base, or on the interaction layer while it is live. */
  private markDirty(item: SceneItem): void {
    if (item.live) {
      liveDamageOf(item.spec, item.bounds, this.liveDirty);
    } else {
      this.dirty.push(antialiased(item.bounds));
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

  /**
   * Marks items live (dragged) or static; both transitions damage the item on
   * the layer, and record where it left or rejoined the base (`drainLifts`):
   * the renderer masks a lifted item out of the base without re-rendering
   * the tiles, and re-renders them only after the drop.
   */
  setLive(ids: readonly string[], live: boolean): void {
    for (const id of ids) {
      if (live) {
        this.liveIds.add(id);
      } else {
        this.liveIds.delete(id);
      }
      const item = this.byId.get(id);
      if (item && item.live !== live) {
        // Pieces along a link, not its bounding box: lifting long links re-renders only the tiles they cross.
        liveDamageOf(item.spec, item.bounds, live ? this.lifted : this.dropped);
        if (!live) {
          this.droppedItems.push(item);
        }
        liveDamageOf(item.spec, item.bounds, this.liveDirty);
        item.live = live;
      }
    }
  }

  /** Whether rectangles wait to be drained: the static content changed, or items were lifted or dropped. */
  get hasDirty(): boolean {
    return this.dirty.length > 0 || this.lifted.length > 0 || this.dropped.length > 0;
  }

  /** Whether any item is marked live. */
  get hasLive(): boolean {
    return this.liveIds.size > 0;
  }

  /**
   * Passes every world rectangle items were lifted from since the last
   * drain to `lift`, every one they were dropped at to `drop`, and each
   * dropped item still in the scene to `dropItem`.
   */
  drainLifts(
    lift: (rect: Rect) => void,
    drop: (rect: Rect) => void,
    dropItem: (item: SceneItem) => void = () => undefined,
  ): void {
    for (const rect of this.lifted) {
      lift(rect);
    }
    this.lifted.length = 0;
    for (const rect of this.dropped) {
      drop(rect);
    }
    this.dropped.length = 0;
    for (const item of this.droppedItems) {
      if (this.byId.get(item.spec.id) === item && !item.live) {
        dropItem(item);
      }
    }
    this.droppedItems.length = 0;
  }

  /** Passes every dirty world rectangle since the last drain to `visit`. */
  drainDirty(visit: (rect: Rect) => void): void {
    for (const rect of this.dirty) {
      visit(rect);
    }
    this.dirty.length = 0;
  }

  /** Passes every world rectangle the live items changed in since the last drain to `visit`. */
  drainLiveDirty(visit: (rect: Rect) => void): void {
    for (const rect of this.liveDirty) {
      visit(rect);
    }
    this.liveDirty.length = 0;
  }

  /** Passes every id removed since the last drain to `visit`. */
  drainRemoved(visit: (id: string) => void): void {
    for (const id of this.removed) {
      visit(id);
    }
    this.removed.length = 0;
  }
}
