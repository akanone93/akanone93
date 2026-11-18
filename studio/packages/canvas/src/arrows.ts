// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Bundled navigation arrows: derivation and routing (spec §21.5, STU-040 to
 * STU-043, STU-045, STU-051). Pure and deterministic: the same boxes, links and
 * side history give the same routes — no clock, randomness or hash order.
 *
 * Derivation turns the navigate edges of the reference graph (SCH-041) into
 * bundles: all links from one box to another in one direction. Routing
 * attaches each end at the point of its side nearest the other box, picks the
 * side pair whose anchors are closest (with hysteresis), keeps the anchors that
 * share a side apart in an order that keeps parallel bundles from crossing, and draws
 * cubic Béziers that leave and enter perpendicular to their sides, detouring
 * around other boxes through at most two waypoints. A drag re-routes only the
 * bundles it can affect (`moveBox`); the result equals a full re-route with
 * the same side history.
 */

import type { Rect } from "./geometry.ts";
import type { Point } from "./protocol.ts";

/** One navigate action of the reference graph (STU-040). */
export interface NavigateEdge {
  /** Stable id of the navigate action. */
  readonly id: string;
  /** The node holding the action: a screen, or a native route. */
  readonly source: string;
  /** The navigation target: a screen, or a native route. */
  readonly target: string;
  /** The trigger: widget and event, e.g. "Apply now · onTap". */
  readonly trigger: string;
  /** The lane title, e.g. "Apply now → Loan form" (STU-044). */
  readonly title: string;
}

/** All links from one box to another in one direction (STU-041). */
export interface Bundle {
  readonly id: string;
  readonly source: string;
  readonly target: string;
  readonly links: readonly NavigateEdge[];
}

/** A box arrows attach to: a plugin, a screen, a ghost screen or a native target. */
export interface ArrowBox {
  readonly id: string;
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
}

/** A box side: 0 top, 1 right, 2 bottom, 3 left. */
export type Side = 0 | 1 | 2 | 3;

/** One cubic Bézier piece of a route. */
export interface Cubic {
  readonly from: Point;
  readonly c1: Point;
  readonly c2: Point;
  readonly to: Point;
}

/** A routed bundle. */
export interface Route {
  readonly bundle: Bundle;
  readonly sourceSide: Side;
  readonly targetSide: Side;
  /** Pieces from the source anchor to the target anchor, G1-continuous. */
  readonly segments: readonly Cubic[];
  /** False when no candidate avoided every other box (STU-043 is a SHOULD). */
  readonly clear: boolean;
  /** Bounds of every candidate tried: a moved box outside it cannot change the route. */
  readonly envelope: Rect;
}

/** Router tuning, in world units. */
export interface RouterOptions {
  /** Distance kept from other boxes. */
  readonly clearance?: number;
  /** A side pair is kept until another is shorter by this fraction... */
  readonly hysteresis?: number;
  /** ...plus this many units. */
  readonly hysteresisMin?: number;
  /** Anchors keep this distance from a side's corners. */
  readonly cornerInset?: number;
  /** Anchors sharing a side keep at least this distance apart. */
  readonly anchorGap?: number;
}

/** The id of the bundle from box `source` to box `target`. */
export function bundleId(source: string, target: string): string {
  return `${source}→${target}`;
}

function byId(a: { readonly id: string }, b: { readonly id: string }): number {
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
}

/**
 * Bundles from navigate edges (STU-040, STU-041). `boxOf` maps a graph node to
 * the box it is drawn in — its plugin on the app canvas, itself on the plugin
 * canvas — or undefined to leave the edge out. Links within one box are not
 * arrows. Bundles and their links are sorted by id.
 */
export function deriveBundles(
  edges: readonly NavigateEdge[],
  boxOf: (node: string) => string | undefined = (node) => node,
): Bundle[] {
  const groups = new Map<string, { source: string; target: string; links: NavigateEdge[] }>();
  for (const edge of edges) {
    const source = boxOf(edge.source);
    const target = boxOf(edge.target);
    if (source === undefined || target === undefined || source === target) {
      continue;
    }
    const id = bundleId(source, target);
    let group = groups.get(id);
    if (!group) {
      group = { source, target, links: [] };
      groups.set(id, group);
    }
    group.links.push(edge);
  }
  const out: Bundle[] = [];
  for (const [id, group] of groups) {
    out.push({ id, source: group.source, target: group.target, links: group.links.sort(byId) });
  }
  return out.sort(byId);
}

/** App canvas: a screen is drawn in its plugin box; other nodes (native targets) as themselves. */
export function deriveAppBundles(edges: readonly NavigateEdge[], pluginOf: ReadonlyMap<string, string>): Bundle[] {
  return deriveBundles(edges, (node) => pluginOf.get(node) ?? node);
}

/**
 * Plugin canvas (STU-051): the links leaving the screens of `plugin`, to its
 * own screens, ghost screens of other plugins (box id = screen id) or native
 * targets, under the same bundling rules.
 */
export function derivePluginBundles(
  edges: readonly NavigateEdge[],
  plugin: string,
  pluginOf: ReadonlyMap<string, string>,
): Bundle[] {
  return deriveBundles(edges.filter((edge) => pluginOf.get(edge.source) === plugin));
}

const NX = [0, 1, 0, -1] as const;
const NY = [-1, 0, 1, 0] as const;
const SIDES: readonly Side[] = [0, 1, 2, 3];
/** Added to a side pair's length per side facing away from the other box. */
const BACKFACE = 1000;
const GRID = 256;
const MIN_BEND = 24;
const MAX_BEND = 240;
/** Flattening step for collision checks. */
const STEP = 24;
/** Paths meeting more boxes than this are not detoured. */
const MAX_DETOURED = 2;

interface MBox {
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

interface BundleState {
  readonly bundle: Bundle;
  sa: Side;
  sb: Side;
  fa: number;
  fb: number;
  route: Route | undefined;
  /** Flattened polylines (x, y, x, y, …) of every candidate tried for `route`. */
  paths: number[][];
  /**
   * The straight connections of other side pairs tried while the bundle looked for a clear one, as far
   * as each was checked, with their bounds: a box moved across them can change the sides it takes.
   */
  probes: number[][];
  probeBounds: Rect | undefined;
}

interface Member {
  readonly state: BundleState;
  /** 0: the bundle's source end; 1: its target end. */
  readonly end: 0 | 1;
}

/** The outward normal of `side`. */
export function sideNormal(side: Side): Point {
  return { x: NX[side], y: NY[side] };
}

/** The point at fraction `f` along `side` of `box` (left to right, top to bottom). */
export function sidePoint(box: ArrowBox, side: Side, f: number): Point {
  switch (side) {
    case 0:
      return { x: box.x + box.w * f, y: box.y };
    case 1:
      return { x: box.x + box.w, y: box.y + box.h * f };
    case 2:
      return { x: box.x + box.w * f, y: box.y + box.h };
    default:
      return { x: box.x, y: box.y + box.h * f };
  }
}

/** Whether `side` runs along x (top and bottom) rather than y. */
function horizontal(side: Side): boolean {
  return side === 0 || side === 2;
}

/** The stretch of `side` an anchor may use, along its axis: the side less its corner insets. */
function usable(box: ArrowBox, side: Side, cornerInset: number): [number, number] {
  const [start, length] = horizontal(side) ? [box.x, box.w] : [box.y, box.h];
  const inset = Math.min(cornerInset, length / 4);
  return [start + inset, start + length - inset];
}

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v));
}

/**
 * Where on `side` of `here` an end attaches to reach `other` by the shortest way, along the side's axis
 * (STU-042): facing parallel sides that overlap meet in the middle of their overlap, so the connection is
 * straight; otherwise the end goes to the point of its side nearest the other box's centre.
 */
export function attachAlong(here: ArrowBox, side: Side, other: ArrowBox, otherSide: Side, cornerInset = 12): number {
  const [lo, hi] = usable(here, side, cornerInset);
  if (horizontal(side) === horizontal(otherSide)) {
    const [olo, ohi] = usable(other, otherSide, cornerInset);
    const from = Math.max(lo, olo);
    const to = Math.min(hi, ohi);
    if (from <= to) {
      return (from + to) / 2;
    }
  }
  return clamp(horizontal(side) ? other.x + other.w / 2 : other.y + other.h / 2, lo, hi);
}

/** The point on `side` of `box` at `along` on its axis. */
function sideAt(box: ArrowBox, side: Side, along: number): Point {
  switch (side) {
    case 0:
      return { x: along, y: box.y };
    case 1:
      return { x: box.x + box.w, y: along };
    case 2:
      return { x: along, y: box.y + box.h };
    default:
      return { x: box.x, y: along };
  }
}

/** The length of the connection between the anchors of side pair (`sa`, `sb`), plus a penalty per end facing away. */
function pairCost(a: ArrowBox, sa: Side, b: ArrowBox, sb: Side, cornerInset: number): number {
  const pa = sideAt(a, sa, attachAlong(a, sa, b, sb, cornerInset));
  const pb = sideAt(b, sb, attachAlong(b, sb, a, sa, cornerInset));
  const dx = pb.x - pa.x;
  const dy = pb.y - pa.y;
  let cost = Math.hypot(dx, dy);
  if (NX[sa] * dx + NY[sa] * dy < 0) {
    cost += BACKFACE;
  }
  if (NX[sb] * -dx + NY[sb] * -dy < 0) {
    cost += BACKFACE;
  }
  return cost;
}

/**
 * The side pair from `a` to `b` whose anchors are closest (STU-042), keeping `previous` (a pair encoded
 * `sa * 4 + sb`) unless the best is shorter by the hysteresis margin, so nearly equidistant boxes do not
 * flicker between sides. Ties go to the first pair in side order, so the choice is a function of the boxes.
 */
export function chooseSides(
  a: ArrowBox,
  b: ArrowBox,
  previous: number | undefined,
  hysteresis = 0.03,
  hysteresisMin = 12,
  cornerInset = 12,
): [Side, Side] {
  let best = Number.POSITIVE_INFINITY;
  let pick: [Side, Side] = [1, 3];
  for (const sa of SIDES) {
    for (const sb of SIDES) {
      const cost = pairCost(a, sa, b, sb, cornerInset);
      if (cost < best) {
        best = cost;
        pick = [sa, sb];
      }
    }
  }
  if (previous !== undefined) {
    const pa = (previous >> 2) as Side;
    const pb = (previous & 3) as Side;
    if (pairCost(a, pa, b, pb, cornerInset) <= best * (1 + hysteresis) + hysteresisMin) {
      return [pa, pb];
    }
  }
  return pick;
}

/**
 * Places anchors wanted at `want` (sorted) on a stretch [`lo`, `hi`] at least `gap` apart, each as near
 * its wish as the others allow (least squares, by pooling adjacent violators): with the i-th shifted
 * back by i gaps, the positions must rise, and their best rising fit clamped to the stretch is the
 * answer. When the stretch is too short for the gaps, the anchors are spaced evenly over it.
 */
export function placeAnchors(want: readonly number[], lo: number, hi: number, gap: number): number[] {
  const n = want.length;
  if (n === 0) {
    return [];
  }
  if ((n - 1) * gap > hi - lo) {
    return want.map((_, i) => (n === 1 ? (lo + hi) / 2 : lo + ((hi - lo) * i) / (n - 1)));
  }
  const blocks: { sum: number; count: number }[] = [];
  for (let i = 0; i < n; i++) {
    blocks.push({ sum: (want[i] as number) - i * gap, count: 1 });
    while (blocks.length > 1) {
      const last = blocks[blocks.length - 1] as { sum: number; count: number };
      const prev = blocks[blocks.length - 2] as { sum: number; count: number };
      if (prev.sum / prev.count <= last.sum / last.count) {
        break;
      }
      prev.sum += last.sum;
      prev.count += last.count;
      blocks.pop();
    }
  }
  const out: number[] = [];
  const top = hi - (n - 1) * gap;
  for (const block of blocks) {
    const z = clamp(block.sum / block.count, lo, top);
    for (let k = 0; k < block.count; k++) {
      out.push(z + out.length * gap);
    }
  }
  return out;
}

/**
 * The sign that orders a bundle's offset along `side` so the two bundles of a
 * box pair (one per direction) stay parallel instead of crossing (STU-041).
 */
function laneSign(side: Side, atCanonicalSource: boolean): number {
  const start = side === 0 || side === 1 ? 1 : -1;
  return atCanonicalSource ? start : -start;
}

function add(p: Point, dx: number, dy: number): Point {
  return { x: p.x + dx, y: p.y + dy };
}

function unit(dx: number, dy: number): Point {
  const len = Math.hypot(dx, dy);
  return len > 0 ? { x: dx / len, y: dy / len } : { x: 1, y: 0 };
}

/**
 * A cubic from `p` leaving along `u` to `q` arriving along `v` (both unit, direction of travel). Its arms
 * are never longer than half the chord, so a short hop between close waypoints cannot overshoot into a
 * loop or a kink.
 */
function piece(p: Point, u: Point, q: Point, v: Point): Cubic {
  const d = Math.hypot(q.x - p.x, q.y - p.y);
  const k = Math.min(MAX_BEND, Math.max(Math.min(MIN_BEND, d / 2), d * 0.4));
  return { from: p, c1: add(p, u.x * k, u.y * k), c2: add(q, -v.x * k, -v.y * k), to: q };
}

/** A path from `p0` (leaving along `n0`) through `waypoints` to `p3` (entering against `n3`). */
function pathThrough(p0: Point, n0: Point, waypoints: readonly Point[], p3: Point, n3: Point): Cubic[] {
  const points = [p0, ...waypoints, p3];
  const out: Cubic[] = [];
  let u = n0;
  for (let i = 0; i + 1 < points.length; i++) {
    const p = points[i] as Point;
    const q = points[i + 1] as Point;
    const next = points[i + 2];
    const v = next ? unit(next.x - p.x, next.y - p.y) : { x: -n3.x, y: -n3.y };
    out.push(piece(p, u, q, v));
    u = v;
  }
  return out;
}

/** The point of `c` at parameter `t`. */
export function cubicAt(c: Cubic, t: number): Point {
  const s = 1 - t;
  const a = s * s * s;
  const b = 3 * s * s * t;
  const d = 3 * s * t * t;
  const e = t * t * t;
  return {
    x: a * c.from.x + b * c.c1.x + d * c.c2.x + e * c.to.x,
    y: a * c.from.y + b * c.c1.y + d * c.c2.y + e * c.to.y,
  };
}

function controlBounds(c: Cubic, out: Rect): void {
  out.l = Math.min(out.l, c.from.x, c.c1.x, c.c2.x, c.to.x);
  out.t = Math.min(out.t, c.from.y, c.c1.y, c.c2.y, c.to.y);
  out.r = Math.max(out.r, c.from.x, c.c1.x, c.c2.x, c.to.x);
  out.b = Math.max(out.b, c.from.y, c.c1.y, c.c2.y, c.to.y);
}

/** Whether segment p–q meets rectangle [l, r] × [t, b] (Liang–Barsky). */
function segmentHits(
  px: number,
  py: number,
  qx: number,
  qy: number,
  l: number,
  t: number,
  r: number,
  b: number,
): boolean {
  let t0 = 0;
  let t1 = 1;
  const dx = qx - px;
  const dy = qy - py;
  const edges = [
    [-dx, px - l],
    [dx, r - px],
    [-dy, py - t],
    [dy, b - py],
  ] as const;
  for (const [p, q] of edges) {
    if (p === 0) {
      if (q < 0) {
        return false;
      }
      continue;
    }
    const ratio = q / p;
    if (p < 0) {
      if (ratio > t1) {
        return false;
      }
      t0 = Math.max(t0, ratio);
    } else {
      if (ratio < t0) {
        return false;
      }
      t1 = Math.min(t1, ratio);
    }
  }
  return t0 <= t1;
}

interface Attempt {
  readonly segments: Cubic[];
  readonly hit: Collision;
}

interface Collision {
  /** Distinct boxes the path meets, in order along it. */
  readonly boxes: MBox[];
  /** Index of the first segment that meets a box, or -1. */
  readonly segment: number;
  /** The flattened path, x and y interleaved. */
  readonly points: number[];
}

function cellKey(ix: number, iy: number): number {
  return (ix + 32768) * 65536 + (iy + 32768);
}

/**
 * Routes bundles between boxes (STU-042, STU-043, STU-045). Holds the side
 * history that hysteresis needs; `routes` is the current result.
 */
export class ArrowRouter {
  private readonly clearance: number;
  private readonly hysteresis: number;
  private readonly hysteresisMin: number;
  private readonly cornerInset: number;
  private readonly anchorGap: number;
  private readonly boxes = new Map<string, MBox>();
  private readonly grid = new Map<number, MBox[]>();
  private readonly states = new Map<string, BundleState>();
  private readonly byBox = new Map<string, BundleState[]>();
  private readonly members = new Map<string, Member[]>();
  private readonly history = new Map<string, number>();
  private readonly current = new Map<string, Route>();
  private drag: { id: string; start: Rect; pending: Set<BundleState> } | undefined;

  constructor(boxes: readonly ArrowBox[], bundles: readonly Bundle[], options: RouterOptions = {}) {
    this.clearance = options.clearance ?? 12;
    this.hysteresis = options.hysteresis ?? 0.03;
    this.hysteresisMin = options.hysteresisMin ?? 12;
    this.cornerInset = options.cornerInset ?? 12;
    this.anchorGap = options.anchorGap ?? 20;
    this.setGraph(boxes, bundles);
  }

  /** The current routes by bundle id, in bundle id order. */
  get routes(): ReadonlyMap<string, Route> {
    return this.current;
  }

  /** The ids of the bundles attached to box `id`, sorted. */
  bundlesAt(id: string): string[] {
    return (this.byBox.get(id) ?? []).map((state) => state.bundle.id).sort();
  }

  /** The box `id`, if present. */
  box(id: string): ArrowBox | undefined {
    return this.boxes.get(id);
  }

  /**
   * Replaces boxes and bundles and routes everything, keeping the side history
   * of bundles that remain. Bundles whose boxes are missing are not routed.
   */
  setGraph(boxes: readonly ArrowBox[], bundles: readonly Bundle[]): void {
    this.boxes.clear();
    this.grid.clear();
    for (const box of boxes) {
      const copy = { id: box.id, x: box.x, y: box.y, w: box.w, h: box.h };
      this.boxes.set(box.id, copy);
      this.gridInsert(copy);
    }
    this.states.clear();
    this.byBox.clear();
    const keep = new Set<string>();
    for (const bundle of [...bundles].sort(byId)) {
      if (!this.boxes.has(bundle.source) || !this.boxes.has(bundle.target)) {
        continue;
      }
      const state: BundleState = {
        bundle,
        sa: 1,
        sb: 3,
        fa: 0.5,
        fb: 0.5,
        route: undefined,
        paths: [],
        probes: [],
        probeBounds: undefined,
      };
      this.states.set(bundle.id, state);
      keep.add(bundle.id);
      for (const end of [bundle.source, bundle.target]) {
        const list = this.byBox.get(end) ?? [];
        list.push(state);
        this.byBox.set(end, list);
      }
    }
    for (const id of [...this.history.keys()]) {
      if (!keep.has(id)) {
        this.history.delete(id);
      }
    }
    this.routeAll();
  }

  /** Re-selects every side (with hysteresis) and re-routes every bundle. */
  routeAll(): void {
    for (const state of this.states.values()) {
      this.selectSides(state);
    }
    this.members.clear();
    for (const state of this.states.values()) {
      this.join(state);
    }
    for (const key of [...this.members.keys()]) {
      this.spread(key);
    }
    this.current.clear();
    for (const state of this.states.values()) {
      this.curve(state);
      this.current.set(state.bundle.id, state.route as Route);
    }
    this.unblock(this.states.values());
  }

  /**
   * Starts dragging box `id` (STU-045): until `endDrag`, `moveBox` re-routes
   * only the bundles attached to it, so everything else stays still on the
   * base layer; `endDrag` then catches up the rest.
   */
  beginDrag(id: string): void {
    const box = this.boxes.get(id);
    this.drag = box ? { id, start: this.inflated(box), pending: new Set() } : undefined;
  }

  /**
   * Ends the drag: re-routes the bundles it held back — those sharing a side
   * with the dragged box's bundles, and those whose candidates meet its start
   * or end place. Returns their ids, sorted; the result equals a full re-route.
   */
  endDrag(): string[] {
    const drag = this.drag;
    this.drag = undefined;
    const box = drag ? this.boxes.get(drag.id) : undefined;
    if (!(drag && box)) {
      return [];
    }
    const dirty = new Set<BundleState>();
    const met = new Set(drag.pending);
    this.collectMet(met, drag.start, this.inflated(box), new Set(this.byBox.get(drag.id)));
    this.reselect(met, dirty);
    return this.recurve(dirty);
  }

  /**
   * Moves box `id` to (`x`, `y`) and re-routes only the bundles that can
   * change: those attached to it, those sharing a side with them, and those
   * whose candidates meet its old or new place. Returns their ids, sorted.
   * While `id` is being dragged, only its own bundles are re-routed.
   */
  moveBox(id: string, x: number, y: number): string[] {
    const box = this.boxes.get(id);
    if (!box) {
      return [];
    }
    const old = this.inflated(box);
    this.gridRemove(box);
    box.x = x;
    box.y = y;
    this.gridInsert(box);
    const own = new Set(this.byBox.get(id));
    const touched = new Set<string>();
    for (const state of own) {
      this.leave(state, touched);
      this.selectSides(state);
      this.join(state, touched);
    }
    const dirty = new Set(own);
    const dragging = this.drag?.id === id;
    for (const key of touched) {
      this.spread(key);
      for (const member of this.members.get(key) ?? []) {
        (dragging && !own.has(member.state) ? this.drag?.pending : dirty)?.add(member.state);
      }
    }
    if (!dragging) {
      const met = new Set<BundleState>();
      this.collectMet(met, old, this.inflated(box), own);
      this.reselect(met, dirty);
    }
    return this.recurve(dirty);
  }

  /**
   * Chooses the sides of `states` again, as a full re-route would, and adds them to `dirty`; a bundle
   * that changes sides leaves and joins its sides, whose members are spread again and added too.
   */
  private reselect(states: ReadonlySet<BundleState>, dirty: Set<BundleState>): void {
    const touched = new Set<string>();
    for (const state of [...states].sort((x, y) => byId(x.bundle, y.bundle))) {
      dirty.add(state);
      const [sa, sb] = [state.sa, state.sb];
      this.selectSides(state);
      if (state.sa === sa && state.sb === sb) {
        continue;
      }
      const [na, nb] = [state.sa, state.sb];
      state.sa = sa;
      state.sb = sb;
      this.leave(state, touched);
      state.sa = na;
      state.sb = nb;
      this.join(state, touched);
    }
    for (const key of [...touched].sort()) {
      this.spread(key);
      for (const member of this.members.get(key) ?? []) {
        dirty.add(member.state);
      }
    }
  }

  private inflated(box: MBox): Rect {
    const c = this.clearance;
    return { l: box.x - c, t: box.y - c, r: box.x + box.w + c, b: box.y + box.h + c };
  }

  /** Adds the bundles (other than `skip`) whose tried candidates meet `a` or `b`. */
  private collectMet(dirty: Set<BundleState>, a: Rect, b: Rect, skip: ReadonlySet<BundleState>): void {
    for (const state of this.states.values()) {
      const env = state.route?.envelope;
      if (
        env &&
        !skip.has(state) &&
        !dirty.has(state) &&
        (this.affects(state, env, a) || this.affects(state, env, b))
      ) {
        dirty.add(state);
      }
    }
  }

  /** Whether a box moved to or from `rect` can change `state`: it crosses a candidate or a probe. */
  private affects(state: BundleState, env: Rect, rect: Rect): boolean {
    return (
      meets(state.paths, env, rect) || (state.probeBounds !== undefined && meets(state.probes, state.probeBounds, rect))
    );
  }

  private recurve(dirty: ReadonlySet<BundleState>): string[] {
    const ids = new Set<string>();
    for (const state of dirty) {
      this.curve(state);
      this.current.set(state.bundle.id, state.route as Route);
      ids.add(state.bundle.id);
    }
    // While a box is dragged, bundles not attached to it wait for the drop.
    const own = this.drag ? new Set(this.byBox.get(this.drag.id)) : undefined;
    for (const state of this.unblock(dirty, own)) {
      ids.add(state.bundle.id);
    }
    return [...ids].sort();
  }

  /**
   * Hysteresis keeps a bundle's side pair only while it routes clear and straight: a bundle that is
   * blocked, or needs a detour, takes the shortest pair that routes clear and straight, and the
   * bundles sharing the sides it leaves or joins are spread and routed again. Each bundle switches at most
   * once, so this ends. Bundles not in `own` (when given) are left for the drag to catch up. Returns the
   * bundles routed again.
   */
  private unblock(states: Iterable<BundleState>, own?: ReadonlySet<BundleState>): BundleState[] {
    const touched = new Set<string>();
    const switched = [...states].filter((state) => this.giveWay(state, touched, own));
    if (switched.length === 0) {
      return [];
    }
    const again = new Set<BundleState>(switched);
    for (const key of [...touched].sort()) {
      this.spread(key);
      for (const member of this.members.get(key) ?? []) {
        if (own === undefined || own.has(member.state)) {
          again.add(member.state);
        } else {
          this.drag?.pending.add(member.state);
        }
      }
    }
    const out = [...again].sort((x, y) => byId(x.bundle, y.bundle));
    for (const state of out) {
      this.curve(state);
      this.current.set(state.bundle.id, state.route as Route);
    }
    return out;
  }

  /**
   * Moves a blocked or detoured bundle to the shortest side pair that routes straight and clear (or the
   * shortest of all when none does), noting the sides it leaves and joins in `touched`. False when the
   * bundle keeps its sides.
   */
  private giveWay(state: BundleState, touched: Set<string>, own?: ReadonlySet<BundleState>): boolean {
    const route = state.route;
    if (own !== undefined && !own.has(state)) {
      return false;
    }
    if (route === undefined || (route.clear && route.segments.length === 1)) {
      state.probes = [];
      state.probeBounds = undefined;
      return false;
    }
    const a = this.boxes.get(state.bundle.source) as MBox;
    const b = this.boxes.get(state.bundle.target) as MBox;
    const [sa, sb] = this.clearestSides(a, b, state);
    if (sa === state.sa && sb === state.sb) {
      return false;
    }
    // The history keeps the geometric choice: the way round a box is worked out again from the boxes
    // each time, so where a box was dragged through does not decide where a bundle ends up.
    this.leave(state, touched);
    state.sa = sa;
    state.sb = sb;
    this.join(state, touched);
    return true;
  }

  /**
   * The shortest side pair from `a` to `b` whose connection runs straight and clear of every box, or,
   * when none does, the shortest of all. Pairs are tried by length, then in side order.
   */
  private clearestSides(a: MBox, b: MBox, state: BundleState): [Side, Side] {
    const pairs: { sa: Side; sb: Side; cost: number }[] = [];
    for (const sa of SIDES) {
      for (const sb of SIDES) {
        const cost = pairCost(a, sa, b, sb, this.cornerInset);
        if (cost < BACKFACE) {
          pairs.push({ sa, sb, cost });
        }
      }
    }
    pairs.sort((x, y) => x.cost - y.cost || x.sa * 4 + x.sb - (y.sa * 4 + y.sb));
    state.probes = [];
    const bounds: Rect = { l: Number.POSITIVE_INFINITY, t: Number.POSITIVE_INFINITY, r: -Infinity, b: -Infinity };
    state.probeBounds = bounds;
    for (const { sa, sb } of pairs) {
      if (this.straightClear(a, sa, b, sb, state.probes, bounds)) {
        return [sa, sb];
      }
    }
    return chooseSides(a, b, undefined, 0, 0, this.cornerInset);
  }

  /**
   * Whether the undetoured curve between the nearest anchors of side pair (`sa`, `sb`) meets no box;
   * the points it checked go to `probes` and their bounds to `bounds`.
   */
  private straightClear(a: MBox, sa: Side, b: MBox, sb: Side, probes: number[][], bounds: Rect): boolean {
    const p0 = sideAt(a, sa, attachAlong(a, sa, b, sb, this.cornerInset));
    const p3 = sideAt(b, sb, attachAlong(b, sb, a, sa, this.cornerInset));
    const segments = pathThrough(p0, sideNormal(sa), [], p3, sideNormal(sb));
    for (const segment of segments) {
      controlBounds(segment, bounds);
    }
    const hit = this.collide(segments, [a, b]);
    probes.push(hit.points);
    return hit.boxes.length === 0 && !this.entersEnds(hit.points, [a, b]);
  }

  private selectSides(state: BundleState): void {
    const a = this.boxes.get(state.bundle.source) as MBox;
    const b = this.boxes.get(state.bundle.target) as MBox;
    const [sa, sb] = chooseSides(
      a,
      b,
      this.history.get(state.bundle.id),
      this.hysteresis,
      this.hysteresisMin,
      this.cornerInset,
    );
    state.sa = sa;
    state.sb = sb;
    this.history.set(state.bundle.id, sa * 4 + sb);
  }

  private join(state: BundleState, touched?: Set<string>): void {
    for (const end of [0, 1] as const) {
      const key = this.sideKey(state, end);
      const list = this.members.get(key) ?? [];
      list.push({ state, end });
      this.members.set(key, list);
      touched?.add(key);
    }
  }

  private leave(state: BundleState, touched: Set<string>): void {
    for (const end of [0, 1] as const) {
      const key = this.sideKey(state, end);
      const list = this.members.get(key);
      if (list) {
        this.members.set(
          key,
          list.filter((m) => m.state !== state),
        );
      }
      touched.add(key);
    }
  }

  private sideKey(state: BundleState, end: 0 | 1): string {
    return end === 0 ? `${state.bundle.source}\u0000${state.sa}` : `${state.bundle.target}\u0000${state.sb}`;
  }

  /** Spreads the anchors on one side, ordered to minimise crossings (STU-042). */
  /**
   * Places the anchors sharing one side (STU-042): each where it reaches its other box by the shortest
   * way, kept `anchorGap` apart in an order that keeps them from crossing — by where they want to be,
   * then the two directions of a box pair side by side as parallel lanes.
   */
  private spread(key: string): void {
    const list = this.members.get(key);
    if (!list || list.length === 0) {
      this.members.delete(key);
      return;
    }
    const keyed = list.map((member) => {
      const { bundle, sa, sb } = member.state;
      const [side, otherSide] = member.end === 0 ? [sa, sb] : [sb, sa];
      const here = this.boxes.get(member.end === 0 ? bundle.source : bundle.target) as MBox;
      const other = this.boxes.get(member.end === 0 ? bundle.target : bundle.source) as MBox;
      const want = attachAlong(here, side, other, otherSide, this.cornerInset);
      const canonicalSource = bundle.source < bundle.target ? bundle.source : bundle.target;
      const offset = bundle.source < bundle.target ? -1 : 1;
      const tie = offset * laneSign(side, here.id === canonicalSource);
      return { member, want, tie };
    });
    keyed.sort(
      (a, b) =>
        a.want - b.want ||
        a.tie - b.tie ||
        byId(a.member.state.bundle, b.member.state.bundle) ||
        a.member.end - b.member.end,
    );
    const first = keyed[0]?.member as Member;
    const side = first.end === 0 ? first.state.sa : first.state.sb;
    const box = this.boxes.get(first.end === 0 ? first.state.bundle.source : first.state.bundle.target) as MBox;
    const [lo, hi] = usable(box, side, this.cornerInset);
    const [start, length] = horizontal(side) ? [box.x, box.w] : [box.y, box.h];
    const placed = placeAnchors(
      keyed.map((k) => k.want),
      lo,
      hi,
      this.anchorGap,
    );
    keyed.forEach(({ member }, i) => {
      const f = ((placed[i] as number) - start) / length;
      if (member.end === 0) {
        member.state.fa = f;
      } else {
        member.state.fb = f;
      }
    });
    this.members.set(
      key,
      keyed.map((k) => k.member),
    );
  }

  /** Builds the curve of one bundle, detouring around other boxes (STU-043). */
  private curve(state: BundleState): void {
    const { bundle } = state;
    const a = this.boxes.get(bundle.source) as MBox;
    const b = this.boxes.get(bundle.target) as MBox;
    const p0 = sidePoint(a, state.sa, state.fa);
    const p3 = sidePoint(b, state.sb, state.fb);
    const n0 = sideNormal(state.sa);
    const n3 = sideNormal(state.sb);
    const envelope: Rect = { l: p0.x, t: p0.y, r: p0.x, b: p0.y };
    const tried: Attempt[] = [];
    const attempt = (waypoints: readonly Point[]): Attempt => {
      const segments = pathThrough(p0, n0, waypoints, p3, n3);
      for (const s of segments) {
        controlBounds(s, envelope);
      }
      const hit = this.collide(segments, [a, b]);
      // A curve that dips back into its own box hides under it: not clear (the anchors sit on the edges).
      const own = hit.boxes.length === 0 && this.entersEnds(hit.points, [a, b]);
      const result = { segments, hit: own ? { boxes: [a], segment: 0, points: hit.points } : hit };
      tried.push(result);
      return result;
    };
    const first = attempt([]);
    const blocked = first.hit.boxes.length;
    // A path through a field of boxes cannot be fixed with two waypoints: drawn as is.
    const found = blocked === 0 ? first : blocked <= MAX_DETOURED ? this.detour(first, p0, p3, attempt) : undefined;
    // No clear candidate: the one meeting the fewest boxes, earliest first.
    const pick = found ?? tried.reduce((x, y) => (y.hit.boxes.length < x.hit.boxes.length ? y : x));
    state.paths = tried.map((t) => t.hit.points);
    state.route = {
      bundle,
      sourceSide: state.sa,
      targetSide: state.sb,
      segments: pick.segments,
      clear: found !== undefined,
      envelope,
    };
  }

  /** Whether a flattened path enters the inside of one of its end boxes, a little in from their edges. */
  private entersEnds(points: readonly number[], ends: readonly MBox[]): boolean {
    const inset = 2;
    for (let i = 0; i + 1 < points.length; i += 2) {
      const x = points[i] as number;
      const y = points[i + 1] as number;
      for (const box of ends) {
        if (x > box.x + inset && x < box.x + box.w - inset && y > box.y + inset && y < box.y + box.h - inset) {
          return true;
        }
      }
    }
    return false;
  }

  /** Up to six detours: one waypoint around the first box in the way, then a second around the next. */
  private detour(
    first: Attempt,
    p0: Point,
    p3: Point,
    attempt: (waypoints: readonly Point[]) => Attempt,
  ): Attempt | undefined {
    for (const w of this.detours(first.hit.boxes[0] as MBox, p0, p3)) {
      const second = attempt([w]);
      const found = second.hit.boxes.length === 0 ? second : this.around(second, w, p0, p3, attempt);
      if (found) {
        return found;
      }
    }
    return undefined;
  }

  /** A second waypoint around the box blocking the path through `w`. */
  private around(
    second: Attempt,
    w: Point,
    p0: Point,
    p3: Point,
    attempt: (waypoints: readonly Point[]) => Attempt,
  ): Attempt | undefined {
    const early = second.hit.segment === 0;
    for (const w2 of this.detours(second.hit.boxes[0] as MBox, early ? p0 : w, early ? w : p3)) {
      const third = attempt(early ? [w2, w] : [w, w2]);
      if (third.hit.boxes.length === 0) {
        return this.oneBend(third, w, w2, attempt);
      }
    }
    return undefined;
  }

  /** Two waypoints this close wiggle the curve: their midpoint, where it routes clear, is one bend. */
  private oneBend(clear: Attempt, w: Point, w2: Point, attempt: (waypoints: readonly Point[]) => Attempt): Attempt {
    if (Math.hypot(w2.x - w.x, w2.y - w.y) >= 2 * MIN_BEND) {
      return clear;
    }
    const one = attempt([{ x: (w.x + w2.x) / 2, y: (w.y + w2.y) / 2 }]);
    return one.hit.boxes.length === 0 ? one : clear;
  }

  /** Two waypoints beside `box`, across the chord p–q; the side the box leans away from first. */
  private detours(box: MBox, p: Point, q: Point): Point[] {
    const d = unit(q.x - p.x, q.y - p.y);
    const nx = -d.y;
    const ny = d.x;
    const cx = box.x + box.w / 2;
    const cy = box.y + box.h / 2;
    const half = (Math.abs(nx) * box.w) / 2 + (Math.abs(ny) * box.h) / 2 + this.clearance * 3;
    const lean = d.x * (cy - p.y) - d.y * (cx - p.x);
    const signs = lean > 0 ? [-1, 1] : [1, -1];
    return signs.map((s) => ({ x: cx + nx * half * s, y: cy + ny * half * s }));
  }

  /** The boxes (other than `skip`) the flattened path meets, inflated by the clearance. */
  private collide(segments: readonly Cubic[], skip: readonly MBox[]): Collision {
    const found: MBox[] = [];
    const points: number[] = [];
    let first = -1;
    for (let s = 0; s < segments.length; s++) {
      const seg = segments[s] as Cubic;
      const poly =
        Math.hypot(seg.c1.x - seg.from.x, seg.c1.y - seg.from.y) +
        Math.hypot(seg.c2.x - seg.c1.x, seg.c2.y - seg.c1.y) +
        Math.hypot(seg.to.x - seg.c2.x, seg.to.y - seg.c2.y);
      const n = Math.min(256, Math.max(8, Math.ceil(poly / STEP)));
      let prev = seg.from;
      if (s === 0) {
        points.push(prev.x, prev.y);
      }
      for (let i = 1; i <= n; i++) {
        const p = cubicAt(seg, i / n);
        const step = this.stepHits(prev, p, skip, found);
        if (step.length > 0) {
          // Grid order depends on move history; id order keeps the result a function of the boxes.
          found.push(...step.sort(byId));
          if (first < 0) {
            first = s;
          }
        }
        points.push(p.x, p.y);
        prev = p;
      }
    }
    return { boxes: found, segment: first, points };
  }

  /** The boxes not yet `found` (nor skipped) that segment prev–p meets, by id. */
  private stepHits(prev: Point, p: Point, skip: readonly MBox[], found: readonly MBox[]): MBox[] {
    const c = this.clearance;
    const ix0 = Math.floor((Math.min(prev.x, p.x) - c) / GRID);
    const ix1 = Math.floor((Math.max(prev.x, p.x) + c) / GRID);
    const iy0 = Math.floor((Math.min(prev.y, p.y) - c) / GRID);
    const iy1 = Math.floor((Math.max(prev.y, p.y) + c) / GRID);
    const step: MBox[] = [];
    for (let ix = ix0; ix <= ix1; ix++) {
      for (let iy = iy0; iy <= iy1; iy++) {
        for (const box of this.grid.get(cellKey(ix, iy)) ?? []) {
          if (
            !(skip.includes(box) || found.includes(box) || step.includes(box)) &&
            segmentHits(prev.x, prev.y, p.x, p.y, box.x - c, box.y - c, box.x + box.w + c, box.y + box.h + c)
          ) {
            step.push(box);
          }
        }
      }
    }
    return step;
  }

  private cells(box: MBox): number[] {
    const out: number[] = [];
    const c = this.clearance;
    for (let ix = Math.floor((box.x - c) / GRID); ix <= Math.floor((box.x + box.w + c) / GRID); ix++) {
      for (let iy = Math.floor((box.y - c) / GRID); iy <= Math.floor((box.y + box.h + c) / GRID); iy++) {
        out.push(cellKey(ix, iy));
      }
    }
    return out;
  }

  private gridInsert(box: MBox): void {
    for (const key of this.cells(box)) {
      const list = this.grid.get(key) ?? [];
      list.push(box);
      this.grid.set(key, list);
    }
  }

  private gridRemove(box: MBox): void {
    for (const key of this.cells(box)) {
      const list = this.grid.get(key);
      if (list) {
        const index = list.indexOf(box);
        if (index >= 0) {
          list.splice(index, 1);
        }
      }
    }
  }
}

/** Whether a candidate tried for `state` meets `rect`: only then can a box there change its route. */
function meets(paths: readonly number[][], envelope: Rect, rect: Rect): boolean {
  if (!overlaps(envelope, rect)) {
    return false;
  }
  for (const path of paths) {
    for (let i = 2; i + 1 < path.length; i += 2) {
      const px = path[i - 2] as number;
      const py = path[i - 1] as number;
      if (segmentHits(px, py, path[i] as number, path[i + 1] as number, rect.l, rect.t, rect.r, rect.b)) {
        return true;
      }
    }
  }
  return false;
}

/** Closed-interval overlap: a curve touching a box's inflated edge still counts. */
function overlaps(a: Rect, b: Rect): boolean {
  return a.l <= b.r && b.l <= a.r && a.t <= b.b && b.t <= a.b;
}
