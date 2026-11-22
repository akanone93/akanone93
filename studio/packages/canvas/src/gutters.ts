// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Routing through the gaps between boxes (STU-043, #206). When neither the direct curve nor a curve through one or
 * two waypoints clears the boxes in its way, the arrow follows the gaps: a shortest path with few bends on the
 * grid of lines that run along every box's edges (an orthogonal visibility grid, as diagram tools use), drawn with
 * rounded corners as cubic pieces, so it stays one smooth curve that leaves and enters perpendicular to its sides
 * and passes under no box. Before this, a path meeting more than two boxes was drawn straight through them, and on a
 * row of screens nobody could tell which arrow went where.
 *
 * Pure and deterministic: the result is a function of the query; ties go to the lowest grid index.
 */

import type { Cubic } from "./arrows.ts";
import type { Rect } from "./geometry.ts";
import type { Point } from "./protocol.ts";

/** Grids larger than this are not searched: the arrow keeps its best curve instead (the search's cost cap). */
export const MAX_GUTTER_NODES = 24_000;

/** Direction of travel: 0 up, 1 right, 2 down, 3 left (the side indices of `Side`). */
export type Heading = 0 | 1 | 2 | 3;

/** Control-arm length of a quarter circle as a cubic, per unit of radius. */
const KAPPA = 0.5523;

/** One gap search. */
export interface GutterQuery {
  /** Where the path starts, just outside the source box, and the heading it arrives there with. */
  readonly from: Point;
  readonly fromHeading: Heading;
  /** Where the path ends, just outside the target box, and the heading it must leave there with. */
  readonly to: Point;
  readonly toHeading: Heading;
  /** The boxes to keep out of, already grown by the distance to keep: the path may run along their edges. */
  readonly obstacles: readonly Rect[];
  /** The area searched. */
  readonly area: Rect;
  /** What a bend costs, in units of length. */
  readonly bend: number;
}

/** The sorted, distinct values of `values` within [lo, hi]. */
function axis(values: readonly number[], lo: number, hi: number): Float64Array {
  const sorted = Float64Array.from(values).sort();
  const out = new Float64Array(sorted.length);
  let n = 0;
  for (const v of sorted) {
    if (v >= lo && v <= hi && (n === 0 || v - (out[n - 1] as number) > 1e-6)) {
      out[n++] = v;
    }
  }
  return out.subarray(0, n);
}

/** The first index whose value is at least `v`. */
function lowerBound(a: Float64Array, v: number): number {
  let lo = 0;
  let hi = a.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if ((a[mid] as number) < v) {
      lo = mid + 1;
    } else {
      hi = mid;
    }
  }
  return lo;
}

/** The first index whose value is greater than `v`. */
function upperBound(a: Float64Array, v: number): number {
  let lo = 0;
  let hi = a.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if ((a[mid] as number) <= v) {
      lo = mid + 1;
    } else {
      hi = mid;
    }
  }
  return lo;
}

/** The index of exactly `v` on `a`. */
function indexOf(a: Float64Array, v: number): number {
  const i = lowerBound(a, v - 1e-6);
  return i < a.length && Math.abs((a[i] as number) - v) <= 1e-6 ? i : -1;
}

/** The grid of one search: its lines, and which nodes and edges lie inside an obstacle. */
interface Grid {
  readonly xs: Float64Array;
  readonly ys: Float64Array;
  /** Node (i, j) at j * nx + i lies inside an obstacle. */
  readonly node: Uint8Array;
  /** The edge from (i, j) to (i + 1, j) crosses an obstacle. */
  readonly right: Uint8Array;
  /** The edge from (i, j) to (i, j + 1) crosses an obstacle. */
  readonly down: Uint8Array;
}

function grid(q: GutterQuery): Grid | undefined {
  const xv = [q.from.x, q.to.x, q.area.l, q.area.r];
  const yv = [q.from.y, q.to.y, q.area.t, q.area.b];
  for (const o of q.obstacles) {
    xv.push(o.l, o.r);
    yv.push(o.t, o.b);
  }
  const xs = axis(xv, q.area.l, q.area.r);
  const ys = axis(yv, q.area.t, q.area.b);
  const nx = xs.length;
  const ny = ys.length;
  if (nx * ny > MAX_GUTTER_NODES) {
    return undefined;
  }
  const g: Grid = {
    xs,
    ys,
    node: new Uint8Array(nx * ny),
    right: new Uint8Array(nx * ny),
    down: new Uint8Array(nx * ny),
  };
  for (const o of q.obstacles) {
    block(g, o);
  }
  return g;
}

/**
 * Marks the nodes and edges of `g` strictly inside obstacle `o`; lines on its edges stay free, so paths run along it.
 */
function block(g: Grid, o: Rect): void {
  const { xs, ys, node, right, down } = g;
  const nx = xs.length;
  const i0 = upperBound(xs, o.l);
  const i1 = lowerBound(xs, o.r);
  const j0 = upperBound(ys, o.t);
  const j1 = lowerBound(ys, o.b);
  for (let j = j0; j < j1; j++) {
    right.fill(1, j * nx + Math.max(0, i0 - 1), j * nx + Math.min(nx - 1, i1));
    node.fill(1, j * nx + i0, j * nx + i1);
  }
  for (let i = i0; i < i1; i++) {
    for (let j = Math.max(0, j0 - 1); j < Math.min(ys.length - 1, j1); j++) {
      down[j * nx + i] = 1;
    }
  }
}

/** A binary min-heap of (priority, state) pairs; equal priorities pop the lowest state first. */
class Heap {
  private readonly keys: number[] = [];
  private readonly states: number[] = [];

  get size(): number {
    return this.keys.length;
  }

  get min(): number {
    return this.keys[0] as number;
  }

  push(key: number, state: number): void {
    const keys = this.keys;
    const states = this.states;
    let i = keys.length;
    keys.push(key);
    states.push(state);
    while (i > 0) {
      const parent = (i - 1) >> 1;
      if (!this.less(i, parent)) {
        break;
      }
      this.swap(i, parent);
      i = parent;
    }
  }

  pop(): number {
    const keys = this.keys;
    const states = this.states;
    const top = states[0] as number;
    const lastKey = keys.pop() as number;
    const lastState = states.pop() as number;
    if (keys.length > 0) {
      keys[0] = lastKey;
      states[0] = lastState;
      let i = 0;
      for (;;) {
        const l = 2 * i + 1;
        const r = l + 1;
        let m = i;
        if (l < keys.length && this.less(l, m)) {
          m = l;
        }
        if (r < keys.length && this.less(r, m)) {
          m = r;
        }
        if (m === i) {
          break;
        }
        this.swap(i, m);
        i = m;
      }
    }
    return top;
  }

  private less(a: number, b: number): boolean {
    const ka = this.keys[a] as number;
    const kb = this.keys[b] as number;
    return ka < kb || (ka === kb && (this.states[a] as number) < (this.states[b] as number));
  }

  private swap(a: number, b: number): void {
    const k = this.keys[a] as number;
    this.keys[a] = this.keys[b] as number;
    this.keys[b] = k;
    const s = this.states[a] as number;
    this.states[a] = this.states[b] as number;
    this.states[b] = s;
  }
}

/**
 * The corners of the cheapest path through the gaps from `from` to `to` — length plus `bend` per turn, a turn at
 * either end included — with `from` and `to` as its first and last points; undefined when the area holds none or
 * its grid is too large to search.
 */
export function gutterRoute(q: GutterQuery): Point[] | undefined {
  const g = grid(q);
  if (g === undefined) {
    return undefined;
  }
  const nx = g.xs.length;
  const start = indexOf(g.ys, q.from.y) * nx + indexOf(g.xs, q.from.x);
  const goal = indexOf(g.ys, q.to.y) * nx + indexOf(g.xs, q.to.x);
  if (g.node[start] || g.node[goal]) {
    return undefined;
  }
  const search = new Search(g, q, goal);
  const best = search.run(start * 4 + q.fromHeading);
  return best < 0 ? undefined : corners(search.path(best));
}

/**
 * An A* search over (node, heading) states: a step costs its length, plus `bend` when it turns; the estimate adds the
 * fewest turns still needed to the distance left, so it never overestimates and the path found is a cheapest one.
 */
class Search {
  private readonly cost: Float64Array;
  private readonly prev: Int32Array;
  private readonly heap = new Heap();

  constructor(
    private readonly g: Grid,
    private readonly q: GutterQuery,
    private readonly goal: number,
  ) {
    const states = g.xs.length * g.ys.length * 4;
    this.cost = new Float64Array(states).fill(Number.POSITIVE_INFINITY);
    this.prev = new Int32Array(states).fill(-1);
  }

  /** The cheapest state arriving at the goal from `first`, or -1. */
  run(first: number): number {
    const { cost, heap, q } = this;
    const back = (q.toHeading + 2) % 4;
    cost[first] = 0;
    heap.push(this.estimate(first), first);
    let best = Number.POSITIVE_INFINITY;
    let bestState = -1;
    while (heap.size > 0 && heap.min < best) {
      const key = heap.min;
      const s = heap.pop();
      const c = cost[s] as number;
      const h = s & 3;
      if (key > c + this.estimate(s) + 1e-9) {
        // A stale entry: the state was reached more cheaply since.
      } else if (s >> 2 === this.goal && h !== back) {
        const total = c + (h === q.toHeading ? 0 : q.bend);
        if (total < best) {
          best = total;
          bestState = s;
        }
      } else {
        this.expand(s, c);
      }
    }
    return bestState;
  }

  /** The nodes of the path ending in state `last`, from the start. */
  path(last: number): Point[] {
    const out: Point[] = [];
    for (let s = last; s >= 0; s = this.prev[s] as number) {
      out.push(this.at(s >> 2));
    }
    return out.reverse();
  }

  private at(n: number): Point {
    const nx = this.g.xs.length;
    const i = n % nx;
    return { x: this.g.xs[i] as number, y: this.g.ys[(n - i) / nx] as number };
  }

  /** Distance left plus a turn's cost per turn still needed from state `s`. */
  private estimate(s: number): number {
    const p = this.at(s >> 2);
    const to = this.q.to;
    return Math.abs(p.x - to.x) + Math.abs(p.y - to.y) + this.q.bend * turnsLeft(p, s & 3, to, this.q.toHeading);
  }

  /** Relaxes the steps from state `s` (reached at cost `c`) to its neighbours, never straight back. */
  private expand(s: number, c: number): void {
    const n = s >> 2;
    const h = s & 3;
    const nx = this.g.xs.length;
    const i = n % nx;
    const j = (n - i) / nx;
    const p = this.at(n);
    for (let d = 0; d < 4; d++) {
      const m = d === (h + 2) % 4 ? -1 : neighbour(this.g, n, i, j, d);
      if (m < 0) {
        continue;
      }
      const q = this.at(m);
      const next = c + Math.abs(q.x - p.x) + Math.abs(q.y - p.y) + (d === h ? 0 : this.q.bend);
      const ms = m * 4 + d;
      if (next < (this.cost[ms] as number)) {
        this.cost[ms] = next;
        this.prev[ms] = s;
        this.heap.push(next + this.estimate(ms), ms);
      }
    }
  }
}

/** Whether `to` lies straight ahead of `p` heading `h`. */
function ahead(p: Point, h: number, to: Point): boolean {
  switch (h) {
    case 0:
      return p.x === to.x && to.y <= p.y;
    case 1:
      return p.y === to.y && to.x >= p.x;
    case 2:
      return p.x === to.x && to.y >= p.y;
    default:
      return p.y === to.y && to.x <= p.x;
  }
}

/**
 * The fewest turns still needed from heading `h` at `p` to arrive at `to` heading `t`: none when the goal lies straight
 * ahead on the right heading, one from a perpendicular heading, else two. A lower bound, so the search stays exact
 * while it explores far fewer nodes.
 */
function turnsLeft(p: Point, h: number, to: Point, t: number): number {
  if (h === t) {
    return ahead(p, h, to) ? 0 : 2;
  }
  return (h + t) % 2 === 1 ? 1 : 2;
}

/** The neighbour of node `n` (at column `i`, row `j`) towards `t`, or -1 when it or the edge to it is blocked. */
function neighbour(g: Grid, n: number, i: number, j: number, t: number): number {
  const m = step(g, n, i, j, t);
  return m >= 0 && !g.node[m] ? m : -1;
}

/** The node next to `n` towards `t` when the edge to it is free, else -1. */
function step(g: Grid, n: number, i: number, j: number, t: number): number {
  const nx = g.xs.length;
  switch (t) {
    case 0:
      return j > 0 && !g.down[n - nx] ? n - nx : -1;
    case 1:
      return i + 1 < nx && !g.right[n] ? n + 1 : -1;
    case 2:
      return j + 1 < g.ys.length && !g.down[n] ? n + nx : -1;
    default:
      return i > 0 && !g.right[n - 1] ? n - 1 : -1;
  }
}

/** The points of `path` where it turns, with its two ends. */
function corners(path: readonly Point[]): Point[] {
  const out: Point[] = [path[0] as Point];
  for (let k = 1; k + 1 < path.length; k++) {
    const a = out.at(-1) as Point;
    const b = path[k] as Point;
    const c = path[k + 1] as Point;
    const straight = (a.x === b.x && b.x === c.x) || (a.y === b.y && b.y === c.y);
    if (!straight) {
      out.push(b);
    }
  }
  if (path.length > 1) {
    out.push(path.at(-1) as Point);
  }
  return out;
}

function line(a: Point, b: Point): Cubic {
  const dx = (b.x - a.x) / 3;
  const dy = (b.y - a.y) / 3;
  return { from: a, c1: { x: a.x + dx, y: a.y + dy }, c2: { x: b.x - dx, y: b.y - dy }, to: b };
}

/** An orthogonal path as cubic pieces, with its middle piece: the longest straight run. */
export interface RoundedPath {
  readonly segments: Cubic[];
  readonly middle: number;
}

/**
 * The orthogonal polyline `polyline` drawn with corners rounded to at most `radius` (and never past the middle of a
 * leg, so neighbouring corners do not overlap): straight pieces and quarter turns, each a cubic, joined smoothly.
 */
export function roundedPath(polyline: readonly Point[], radius: number): RoundedPath {
  const points = corners(polyline);
  const segments: Cubic[] = [];
  let middle = 0;
  let longest = -1;
  const push = (a: Point, b: Point): void => {
    const length = Math.hypot(b.x - a.x, b.y - a.y);
    if (length < 0.5) {
      return;
    }
    if (length > longest) {
      longest = length;
      middle = segments.length;
    }
    segments.push(line(a, b));
  };
  let at = points[0] as Point;
  for (let k = 1; k + 1 < points.length; k++) {
    const a = points[k - 1] as Point;
    const p = points[k] as Point;
    const b = points[k + 1] as Point;
    const lin = Math.hypot(p.x - a.x, p.y - a.y);
    const lout = Math.hypot(b.x - p.x, b.y - p.y);
    // The first and last legs have no other corner to share with.
    const r = Math.min(radius, k === 1 ? lin : lin / 2, k + 2 === points.length ? lout : lout / 2);
    const u = { x: (p.x - a.x) / lin, y: (p.y - a.y) / lin };
    const v = { x: (b.x - p.x) / lout, y: (b.y - p.y) / lout };
    const enter = { x: p.x - u.x * r, y: p.y - u.y * r };
    const leave = { x: p.x + v.x * r, y: p.y + v.y * r };
    push(at, enter);
    segments.push({
      from: enter,
      c1: { x: enter.x + u.x * r * KAPPA, y: enter.y + u.y * r * KAPPA },
      c2: { x: leave.x - v.x * r * KAPPA, y: leave.y - v.y * r * KAPPA },
      to: leave,
    });
    at = leave;
  }
  push(at, points.at(-1) as Point);
  return { segments, middle };
}
