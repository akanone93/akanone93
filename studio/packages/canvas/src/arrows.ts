// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Bundled navigation arrows: derivation and routing (spec §21.5, STU-040 to
 * STU-043, STU-045, STU-051). Pure and deterministic: the same boxes, links and
 * side history give the same routes — no clock, randomness or hash order.
 *
 * Derivation turns the navigate edges of the reference graph (SCH-041) into
 * bundles: all links from one box to another in one direction. Routing picks
 * the shortest side pair per bundle with hysteresis, spreads the anchors along
 * each side in an order that keeps parallel bundles from crossing, and draws
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

function pairCost(a: ArrowBox, sa: Side, b: ArrowBox, sb: Side): number {
  const pa = sidePoint(a, sa, 0.5);
  const pb = sidePoint(b, sb, 0.5);
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
 * The shortest side pair from `a` to `b` (STU-042), keeping `previous` (a
 * pair encoded `sa * 4 + sb`) unless the best is shorter by the hysteresis
 * margin, so nearly equidistant boxes do not flicker between sides.
 */
export function chooseSides(
  a: ArrowBox,
  b: ArrowBox,
  previous: number | undefined,
  hysteresis = 0.1,
  hysteresisMin = 8,
): [Side, Side] {
  let best = Number.POSITIVE_INFINITY;
  let pick: [Side, Side] = [1, 3];
  for (const sa of SIDES) {
    for (const sb of SIDES) {
      const cost = pairCost(a, sa, b, sb);
      if (cost < best) {
        best = cost;
        pick = [sa, sb];
      }
    }
  }
  if (previous !== undefined) {
    const pa = (previous >> 2) as Side;
    const pb = (previous & 3) as Side;
    if (pairCost(a, pa, b, pb) <= best * (1 + hysteresis) + hysteresisMin) {
      return [pa, pb];
    }
  }
  return pick;
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

/** A cubic from `p` leaving along `u` to `q` arriving along `v` (both unit, direction of travel). */
function piece(p: Point, u: Point, q: Point, v: Point): Cubic {
  const k = Math.min(MAX_BEND, Math.max(MIN_BEND, Math.hypot(q.x - p.x, q.y - p.y) * 0.4));
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
  private readonly boxes = new Map<string, MBox>();
  private readonly grid = new Map<number, MBox[]>();
  private readonly states = new Map<string, BundleState>();
  private readonly byBox = new Map<string, BundleState[]>();
  private readonly members = new Map<string, Member[]>();
  private readonly history = new Map<string, number>();
  private readonly current = new Map<string, Route>();

  constructor(boxes: readonly ArrowBox[], bundles: readonly Bundle[], options: RouterOptions = {}) {
    this.clearance = options.clearance ?? 12;
    this.hysteresis = options.hysteresis ?? 0.1;
    this.hysteresisMin = options.hysteresisMin ?? 8;
    this.cornerInset = options.cornerInset ?? 12;
    this.setGraph(boxes, bundles);
  }

  /** The current routes by bundle id, in bundle id order. */
  get routes(): ReadonlyMap<string, Route> {
    return this.current;
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
      const state: BundleState = { bundle, sa: 1, sb: 3, fa: 0.5, fb: 0.5, route: undefined };
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
  }

  /**
   * Moves box `id` to (`x`, `y`) and re-routes only the bundles that can
   * change: those attached to it, those sharing a side with them, and those
   * whose candidates came near its old or new place. Returns their ids, sorted.
   */
  moveBox(id: string, x: number, y: number): string[] {
    const box = this.boxes.get(id);
    if (!box) {
      return [];
    }
    const c = this.clearance;
    const old: Rect = { l: box.x - c, t: box.y - c, r: box.x + box.w + c, b: box.y + box.h + c };
    this.gridRemove(box);
    box.x = x;
    box.y = y;
    this.gridInsert(box);
    const moved: Rect = { l: x - c, t: y - c, r: x + box.w + c, b: y + box.h + c };
    const dirty = new Set<BundleState>();
    const touched = new Set<string>();
    for (const state of this.byBox.get(id) ?? []) {
      this.leave(state, touched);
      this.selectSides(state);
      this.join(state, touched);
      dirty.add(state);
    }
    for (const key of touched) {
      this.spread(key);
      for (const member of this.members.get(key) ?? []) {
        dirty.add(member.state);
      }
    }
    for (const state of this.states.values()) {
      const env = state.route?.envelope;
      if (env && !dirty.has(state) && (overlaps(env, old) || overlaps(env, moved))) {
        dirty.add(state);
      }
    }
    const ids: string[] = [];
    for (const state of dirty) {
      this.curve(state);
      this.current.set(state.bundle.id, state.route as Route);
      ids.push(state.bundle.id);
    }
    return ids.sort();
  }

  private selectSides(state: BundleState): void {
    const a = this.boxes.get(state.bundle.source) as MBox;
    const b = this.boxes.get(state.bundle.target) as MBox;
    const [sa, sb] = chooseSides(a, b, this.history.get(state.bundle.id), this.hysteresis, this.hysteresisMin);
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
  private spread(key: string): void {
    const list = this.members.get(key);
    if (!list || list.length === 0) {
      this.members.delete(key);
      return;
    }
    const keyed = list.map((member) => {
      const { bundle } = member.state;
      const side = member.end === 0 ? member.state.sa : member.state.sb;
      const other = this.boxes.get(member.end === 0 ? bundle.target : bundle.source) as MBox;
      const along = side === 0 || side === 2 ? other.x + other.w / 2 : other.y + other.h / 2;
      const here = member.end === 0 ? bundle.source : bundle.target;
      const canonicalSource = bundle.source < bundle.target ? bundle.source : bundle.target;
      const offset = bundle.source < bundle.target ? -1 : 1;
      const tie = offset * laneSign(side, here === canonicalSource);
      return { member, along, tie };
    });
    keyed.sort(
      (a, b) =>
        a.along - b.along ||
        a.tie - b.tie ||
        byId(a.member.state.bundle, b.member.state.bundle) ||
        a.member.end - b.member.end,
    );
    const n = keyed.length;
    for (let i = 0; i < n; i++) {
      const { member } = keyed[i] as (typeof keyed)[number];
      const side = member.end === 0 ? member.state.sa : member.state.sb;
      const box = this.boxes.get(member.end === 0 ? member.state.bundle.source : member.state.bundle.target) as MBox;
      const length = side === 0 || side === 2 ? box.w : box.h;
      const inset = Math.min(this.cornerInset, length / 4) / length;
      const f = inset + (1 - 2 * inset) * ((i + 1) / (n + 1));
      if (member.end === 0) {
        member.state.fa = f;
      } else {
        member.state.fb = f;
      }
    }
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
      const result = { segments, hit: this.collide(segments, [a, b]) };
      tried.push(result);
      return result;
    };
    const first = attempt([]);
    const found = first.hit.boxes.length === 0 ? first : this.detour(first, p0, p3, attempt);
    // No clear candidate: the one meeting the fewest boxes, earliest first.
    const pick = found ?? tried.reduce((x, y) => (y.hit.boxes.length < x.hit.boxes.length ? y : x));
    state.route = {
      bundle,
      sourceSide: state.sa,
      targetSide: state.sb,
      segments: pick.segments,
      clear: found !== undefined,
      envelope,
    };
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
        return third;
      }
    }
    return undefined;
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
    let first = -1;
    for (let s = 0; s < segments.length; s++) {
      const seg = segments[s] as Cubic;
      const poly =
        Math.hypot(seg.c1.x - seg.from.x, seg.c1.y - seg.from.y) +
        Math.hypot(seg.c2.x - seg.c1.x, seg.c2.y - seg.c1.y) +
        Math.hypot(seg.to.x - seg.c2.x, seg.to.y - seg.c2.y);
      const n = Math.min(256, Math.max(8, Math.ceil(poly / STEP)));
      let prev = seg.from;
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
        prev = p;
      }
    }
    return { boxes: found, segment: first };
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

/** Closed-interval overlap: a curve touching a box's inflated edge still counts. */
function overlaps(a: Rect, b: Rect): boolean {
  return a.l <= b.r && b.l <= a.r && a.t <= b.b && b.t <= a.b;
}
