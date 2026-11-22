// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import dagre from "@dagrejs/dagre";

/**
 * **Arrange** for both canvases (STU-032, STU-050, #241): plugins on the app canvas, screens on the plugin canvas.
 *
 * Each connected group of boxes gets a layered graph layout (Sugiyama's method as dagre implements it: ranking by
 * network simplex, crossing reduction by barycentres, Brandes–Köpf coordinates), so links mostly run one way, from the
 * entry outwards, with few crossings. Before, one layered layout took the whole graph: every box without links was
 * stacked in one column, a hub's neighbours made one very tall column, and screens were laid in a single row.
 * Now:
 *
 * - links between two boxes count once, oriented away from the entry (breadth-first depth), so a link back to the
 *   entry or a pair of links both ways does not stretch the layout;
 * - a dense group (more than `DENSE_PAIRS` linked pairs) is laid out on a spanning tree of its links plus a few short
 *   ones: a layered layout turns every link spanning several layers into a chain of dummy nodes, and on 200 boxes
 *   with 1,000 random links the full graph took 9 s, the tree 35 ms;
 * - each group is laid out left to right and top to bottom, and keeps the one whose shape comes closer to the view's
 *   (left to right unless the other is clearly closer: flows read that way);
 * - a layer with many boxes (a hub's neighbours) is folded into several columns side by side, and a layout much wider
 *   (or taller) than the view is wrapped: its layers are cut into consecutive bands stacked one under the other (or
 *   side by side), as ELK's layer wrapping does, so a long chain of screens is not one long row — each only when it
 *   brings the shape clearly closer to the view's;
 * - the groups, then the boxes without links in a compact grid, are packed in rows towards the view's shape, the
 *   entry's group first.
 *
 * Pure and deterministic: the same boxes, links and options give the same places. It takes a few milliseconds for a
 * plugin's screens and under the frame budget for the largest app canvas (tested).
 */

export interface ArrangeBox {
  readonly id: string;
  readonly w: number;
  readonly h: number;
}

export interface ArrangeLink {
  readonly from: string;
  readonly to: string;
}

export interface ArrangeOptions {
  /** The least horizontal and vertical space between two boxes. */
  readonly gapX: number;
  readonly gapY: number;
  /** The space between two groups (and the grid of boxes without links). */
  readonly groupGap: number;
  /** The width-to-height ratio the whole should come close to: the view's. */
  readonly aspect: number;
  /** Where the flow starts (the entry plugin or screen): laid first. */
  readonly entry?: string | undefined;
}

/** The shape Arrange aims for on both canvases: about the view's, wider than tall. */
export const VIEW_ASPECT = 1.6;

/** Groups with more linked pairs than this are laid out on a spanning tree and `EXTRA_PAIRS` short links. */
export const DENSE_PAIRS = 120;
const EXTRA_PAIRS = 50;

export interface Placed {
  readonly x: number;
  readonly y: number;
}

/** A laid-out block: its boxes' places relative to its top-left corner, and its size. */
interface Block {
  readonly at: Map<string, Placed>;
  readonly w: number;
  readonly h: number;
  /** The first box's index in the input: ties keep the input's order. */
  readonly first: number;
  readonly entry: boolean;
}

/** The boxes' top-left corners, by id. */
export function arrange(
  boxes: readonly ArrangeBox[],
  links: readonly ArrangeLink[],
  options: ArrangeOptions,
): Map<string, Placed> {
  const index = new Map(boxes.map((b, i) => [b.id, i]));
  const depth = depths(boxes, links, index, options.entry);
  const pairs = orientedPairs(links, index, depth);
  const groups = components(boxes.length, pairs, index);
  const blocks: Block[] = [];
  const alone: ArrangeBox[] = [];
  for (const group of groups) {
    if (group.length === 1) {
      alone.push(boxes[group[0] as number] as ArrangeBox);
      continue;
    }
    const members = group.map((i) => boxes[i] as ArrangeBox);
    const inGroup = new Set(members.map((b) => b.id));
    const groupLinks = pairs.filter((p) => inGroup.has(p.from));
    const dense = groupLinks.length > DENSE_PAIRS;
    const laid = dense ? backbone(members.length, groupLinks, depth) : groupLinks;
    const only = dense ? estimatedDirection(members, depth, options) : undefined;
    blocks.push(layered(members, laid, options, group[0] as number, only));
  }
  if (alone.length > 0) {
    blocks.push(grid(alone, options, index));
  }
  return pack(blocks, options);
}

/**
 * Each box's breadth-first depth along the links: from the entry, then from each box not yet reached, in input order.
 */
function depths(
  boxes: readonly ArrangeBox[],
  links: readonly ArrangeLink[],
  index: ReadonlyMap<string, number>,
  entry: string | undefined,
): Map<string, number> {
  const out = new Map<string, string[]>();
  for (const l of links) {
    if (index.has(l.from) && index.has(l.to) && l.from !== l.to) {
      const list = out.get(l.from);
      if (list === undefined) {
        out.set(l.from, [l.to]);
      } else {
        list.push(l.to);
      }
    }
  }
  const depth = new Map<string, number>();
  const walk = (start: string): void => {
    if (depth.has(start)) {
      return;
    }
    depth.set(start, 0);
    const queue = [start];
    for (let k = 0; k < queue.length; k++) {
      const id = queue[k] as string;
      for (const next of out.get(id) ?? []) {
        if (!depth.has(next)) {
          depth.set(next, (depth.get(id) as number) + 1);
          queue.push(next);
        }
      }
    }
  };
  if (entry !== undefined && index.has(entry)) {
    walk(entry);
  }
  for (const b of boxes) {
    walk(b.id);
  }
  return depth;
}

/**
 * Each pair of linked boxes once, oriented from the box nearer the entry to the farther one; equal depths keep the
 * first link's own direction.
 */
function orientedPairs(
  links: readonly ArrangeLink[],
  index: ReadonlyMap<string, number>,
  depth: ReadonlyMap<string, number>,
): ArrangeLink[] {
  const seen = new Set<string>();
  const pairs: ArrangeLink[] = [];
  for (const l of links) {
    if (!(index.has(l.from) && index.has(l.to)) || l.from === l.to) {
      continue;
    }
    const [a, b] = (index.get(l.from) as number) < (index.get(l.to) as number) ? [l.from, l.to] : [l.to, l.from];
    const key = `${a}\u0000${b}`;
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    const da = depth.get(l.from) as number;
    const db = depth.get(l.to) as number;
    pairs.push(da <= db ? { from: l.from, to: l.to } : { from: l.to, to: l.from });
  }
  return pairs;
}

/**
 * The links a dense group is laid out on: a spanning tree that takes the shortest links first (between neighbouring
 * layers, then within one, then longer; in order within each), then up to `EXTRA_PAIRS` more links between
 * neighbouring layers, which order the layers without adding dummy nodes.
 */
function backbone(count: number, pairs: readonly ArrangeLink[], depth: ReadonlyMap<string, number>): ArrangeLink[] {
  const span = (p: ArrangeLink) => (depth.get(p.to) as number) - (depth.get(p.from) as number);
  // Links between neighbouring layers first (a breadth-first tree is made of them), then within a layer, then longer.
  const cost = (d: number) => (d === 1 ? 0 : d === 0 ? 1 : d);
  const ranked = pairs.map((p, i) => ({ p, i, d: cost(span(p)) })).sort((a, b) => a.d - b.d || a.i - b.i);
  const parent = new Map<string, string>();
  const root = (id: string): string => {
    let r = id;
    for (let next = parent.get(r); next !== undefined; next = parent.get(r)) {
      r = next;
    }
    return r;
  };
  const tree: ArrangeLink[] = [];
  const rest: ArrangeLink[] = [];
  for (const { p } of ranked) {
    const a = root(p.from);
    const b = root(p.to);
    if (a !== b && tree.length < count - 1) {
      parent.set(a, b);
      tree.push(p);
    } else {
      rest.push(p);
    }
  }
  const extra = rest.filter((p) => span(p) === 1).slice(0, EXTRA_PAIRS);
  return [...tree, ...extra];
}

/** The connected groups of box indices, each in input order, ordered by their first box. */
function components(count: number, pairs: readonly ArrangeLink[], index: ReadonlyMap<string, number>): number[][] {
  const parent = Array.from({ length: count }, (_, i) => i);
  const root = (i: number): number => {
    let r = i;
    while (parent[r] !== r) {
      r = parent[r] as number;
    }
    return r;
  };
  for (const p of pairs) {
    const a = root(index.get(p.from) as number);
    const b = root(index.get(p.to) as number);
    if (a !== b) {
      parent[Math.max(a, b)] = Math.min(a, b);
    }
  }
  const groups = new Map<number, number[]>();
  for (let i = 0; i < count; i++) {
    const r = root(i);
    const group = groups.get(r);
    if (group === undefined) {
      groups.set(r, [i]);
    } else {
      group.push(i);
    }
  }
  return [...groups.values()];
}

/** A layout across (left to right) wins unless the other's shape is this much closer to the view's (a factor of 1.5). */
const PREFER_ACROSS = Math.log(1.5);
/** A fold or a wrap must bring the shape this much closer to the view's (a factor of 1.5): both cost some reading. */
const CLEARLY = Math.log(1.5);
/** A wrapped band keeps at least this many layers; a folded layer, columns of at least this many boxes. */
const MIN_BAND_LAYERS = 3;
const MIN_FOLD = 3;

/** A laid-out group: its boxes' places from its top-left corner, and its size. */
interface Laid {
  readonly at: Map<string, Placed>;
  readonly w: number;
  readonly h: number;
}

/**
 * `laid` with its layers wrapped into bands when that brings its shape closer to the view's: left to right, runs of
 * consecutive layers (columns) stacked one under the other; top to bottom, runs of rows set side by side. Each band
 * keeps its boxes' places within it; bands are `groupGap` apart.
 */
function wrap(laid: Laid, boxes: readonly ArrangeBox[], dir: "LR" | "TB", o: ArrangeOptions): Laid {
  const along = (p: Placed) => Math.round(dir === "LR" ? p.x : p.y);
  const size = new Map(boxes.map((b) => [b.id, b]));
  const starts = [...new Set([...laid.at.values()].map(along))].sort((a, b) => a - b);
  const layerOf = new Map([...laid.at].map(([id, p]) => [id, starts.indexOf(along(p))]));
  let best: Laid = laid;
  // Each band keeps at least three layers, and a wrap must bring the shape clearly closer: a wrapped flow reads less
  // easily than a straight one.
  for (let bands = 2; Math.ceil(starts.length / bands) >= MIN_BAND_LAYERS; bands++) {
    const next = banded(laid, size, layerOf, Math.ceil(starts.length / bands), dir, o);
    if (offShape(next.w, next.h, o) + CLEARLY < offShape(best.w, best.h, o)) {
      best = next;
    }
  }
  return best;
}

/**
 * `laid` with each over-full layer folded into columns set side by side along the flow (rows, top to bottom), in the
 * order the crossing reduction gave it, when that brings the shape closer to the view's: a hub's twenty neighbours
 * are one layer, which would otherwise be one very tall column. The other layers keep their boxes' places within
 * them; every layer is then centred across the flow, and layers after a folded one move along.
 */
function fold(laid: Laid, boxes: readonly ArrangeBox[], dir: "LR" | "TB", o: ArrangeOptions): Laid {
  const size = new Map(boxes.map((b) => [b.id, b]));
  const layers = layersOf(laid, size, dir);
  const fullest = Math.max(...layers.map((l) => l.length));
  let best: Laid = laid;
  for (let per = fullest - 1; per >= MIN_FOLD; per--) {
    const next = folded(layers, per, dir, o);
    if (offShape(next.w, next.h, o) + CLEARLY < offShape(best.w, best.h, o)) {
      best = next;
    }
  }
  return best;
}

/** The layers of `laid` along the flow, each with its boxes in flow coordinates, sorted across the flow. */
function layersOf(laid: Laid, size: ReadonlyMap<string, ArrangeBox>, dir: "LR" | "TB"): Banded[][] {
  const byStart = new Map<number, Banded[]>();
  for (const [id, p] of laid.at) {
    const b = size.get(id) as ArrangeBox;
    const e: Banded =
      dir === "LR" ? { id, u: p.x, v: p.y, du: b.w, dv: b.h } : { id, u: p.y, v: p.x, du: b.h, dv: b.w };
    const key = Math.round(e.u + e.du / 2);
    const layer = byStart.get(key);
    if (layer === undefined) {
      byStart.set(key, [e]);
    } else {
      layer.push(e);
    }
  }
  return [...byStart.entries()].sort((a, b) => a[0] - b[0]).map(([, l]) => l.sort((a, b) => a.v - b.v));
}

/** The layers with every layer of more than `per` boxes folded into columns of `per`. */
function folded(layers: readonly Banded[][], per: number, dir: "LR" | "TB", o: ArrangeOptions): Laid {
  const [gapAlong, gapAcross] = dir === "LR" ? [o.gapX, o.gapY] : [o.gapY, o.gapX];
  // Each layer on its own first, across the flow from 0: a folded one in columns, the others as they were laid out.
  const placed: { e: Banded; u: number; v: number }[][] = [];
  let u = 0;
  for (const layer of layers) {
    const wide = Math.max(...layer.map((e) => e.du));
    const tall = Math.max(...layer.map((e) => e.dv));
    const u0 = Math.min(...layer.map((e) => e.u));
    const v0 = Math.min(...layer.map((e) => e.v));
    const columns = Math.ceil(layer.length / per);
    placed.push(
      layer.map((e, i) =>
        columns === 1
          ? { e, u: u + (e.u - u0), v: e.v - v0 }
          : { e, u: u + Math.floor(i / per) * (wide + gapAlong), v: (i % per) * (tall + gapAcross) },
      ),
    );
    u += columns * (wide + gapAlong);
  }
  // Then every layer centred on one middle line: a folded layer is much shorter than the column it was, and the
  // others, left where the tall column put them, would sit far from it.
  const extent = (l: { e: Banded; v: number }[]) => Math.max(...l.map((p) => p.v + p.e.dv));
  const thick = Math.max(...placed.map(extent));
  const at = new Map<string, Placed>();
  let end = 0;
  for (const layer of placed) {
    const shift = (thick - extent(layer)) / 2;
    for (const p of layer) {
      const v = p.v + shift;
      at.set(p.e.id, dir === "LR" ? { x: p.u, y: v } : { x: v, y: p.u });
      end = Math.max(end, p.u + p.e.du);
    }
  }
  return dir === "LR" ? { at, w: end, h: thick } : { at, w: thick, h: end };
}

/** One box of a band, in flow coordinates: `u` along the flow, `v` across it, and its extent in each. */
interface Banded {
  readonly id: string;
  readonly u: number;
  readonly v: number;
  readonly du: number;
  readonly dv: number;
}

/** `laid` cut into runs of `per` consecutive layers, each moved to the start of the flow and past the one before. */
function banded(
  laid: Laid,
  size: ReadonlyMap<string, ArrangeBox>,
  layerOf: ReadonlyMap<string, number>,
  per: number,
  dir: "LR" | "TB",
  o: ArrangeOptions,
): Laid {
  const bands = new Map<number, Banded[]>();
  for (const [id, p] of laid.at) {
    const b = size.get(id) as ArrangeBox;
    const e: Banded =
      dir === "LR" ? { id, u: p.x, v: p.y, du: b.w, dv: b.h } : { id, u: p.y, v: p.x, du: b.h, dv: b.w };
    const k = Math.floor((layerOf.get(id) as number) / per);
    const band = bands.get(k);
    if (band === undefined) {
      bands.set(k, [e]);
    } else {
      band.push(e);
    }
  }
  const at = new Map<string, Placed>();
  let offset = 0;
  let longest = 0;
  for (const k of [...bands.keys()].sort((a, b) => a - b)) {
    const band = bands.get(k) as Banded[];
    const u0 = Math.min(...band.map((e) => e.u));
    const v0 = Math.min(...band.map((e) => e.v));
    const uEnd = Math.max(...band.map((e) => e.u + e.du));
    const vEnd = Math.max(...band.map((e) => e.v + e.dv));
    for (const e of band) {
      const nu = e.u - u0;
      const nv = e.v - v0 + offset;
      at.set(e.id, dir === "LR" ? { x: nu, y: nv } : { x: nv, y: nu });
    }
    offset += vEnd - v0 + o.groupGap;
    longest = Math.max(longest, uEnd - u0);
  }
  const thick = offset - o.groupGap;
  return dir === "LR" ? { at, w: longest, h: thick } : { at, w: thick, h: longest };
}

/** How far a shape's width-to-height ratio is from the view's. */
function offShape(w: number, h: number, o: ArrangeOptions): number {
  return Math.abs(Math.log(w / h / o.aspect));
}

/**
 * One group laid out in layers, left to right or top to bottom, whichever comes closer to the view's shape; `only`
 * lays it out one way (a dense group, whose layout is the costly part).
 */
function layered(
  boxes: readonly ArrangeBox[],
  links: readonly ArrangeLink[],
  o: ArrangeOptions,
  first: number,
  only?: "LR" | "TB",
): Block {
  const shaped = (dir: "LR" | "TB") => wrap(fold(layout(boxes, links, o, dir), boxes, dir, o), boxes, dir, o);
  const across = only === "TB" ? undefined : shaped("LR");
  const down = only === "LR" ? undefined : shaped("TB");
  const pick =
    across === undefined
      ? (down as Laid)
      : down === undefined || offShape(down.w, down.h, o) + PREFER_ACROSS >= offShape(across.w, across.h, o)
        ? across
        : down;
  return { ...pick, first, entry: o.entry !== undefined && pick.at.has(o.entry) };
}

/**
 * The direction a dense group is laid out in, from its layers as breadth-first depth gives them: left to right, the
 * layers are columns as tall as the fullest one; top to bottom, rows as wide. The one whose shape comes closer to the
 * view's.
 */
function estimatedDirection(
  boxes: readonly ArrangeBox[],
  depth: ReadonlyMap<string, number>,
  o: ArrangeOptions,
): "LR" | "TB" {
  const perLayer = new Map<number, number>();
  for (const b of boxes) {
    const d = depth.get(b.id) ?? 0;
    perLayer.set(d, (perLayer.get(d) ?? 0) + 1);
  }
  const layers = perLayer.size;
  const fullest = Math.max(...perLayer.values());
  const w = Math.max(...boxes.map((b) => b.w));
  const h = Math.max(...boxes.map((b) => b.h));
  const across = offShape(layers * (w + o.gapX), fullest * (h + o.gapY), o);
  const down = offShape(fullest * (w + o.gapX), layers * (h + o.gapY), o);
  return down < across ? "TB" : "LR";
}

function layout(
  boxes: readonly ArrangeBox[],
  links: readonly ArrangeLink[],
  o: ArrangeOptions,
  rankdir: "LR" | "TB",
): Laid {
  const g = new dagre.graphlib.Graph();
  // Within a layer boxes sit side by side across the flow; layers follow each other along it.
  const [nodesep, ranksep] = rankdir === "LR" ? [o.gapY, o.gapX] : [o.gapX, o.gapY];
  g.setGraph({ rankdir, nodesep, ranksep, marginx: 0, marginy: 0 });
  g.setDefaultEdgeLabel(() => ({}));
  for (const b of boxes) {
    g.setNode(b.id, { width: b.w, height: b.h });
  }
  for (const l of links) {
    g.setEdge(l.from, l.to);
  }
  dagre.layout(g);
  let l = Number.POSITIVE_INFINITY;
  let t = Number.POSITIVE_INFINITY;
  let r = Number.NEGATIVE_INFINITY;
  let bottom = Number.NEGATIVE_INFINITY;
  const centres = boxes.map((b) => {
    const n = g.node(b.id);
    const x = (n.x ?? 0) - b.w / 2;
    const y = (n.y ?? 0) - b.h / 2;
    l = Math.min(l, x);
    t = Math.min(t, y);
    r = Math.max(r, x + b.w);
    bottom = Math.max(bottom, y + b.h);
    return { id: b.id, x, y };
  });
  const at = new Map(centres.map((c) => [c.id, { x: c.x - l, y: c.y - t }]));
  return { at, w: r - l, h: bottom - t };
}

/** Boxes without links in a grid of rows and columns, close to the view's shape. */
function grid(boxes: readonly ArrangeBox[], o: ArrangeOptions, index: ReadonlyMap<string, number>): Block {
  const cellW = Math.max(...boxes.map((b) => b.w)) + o.gapX;
  const cellH = Math.max(...boxes.map((b) => b.h)) + o.gapY;
  const columns = Math.max(1, Math.min(boxes.length, Math.round(Math.sqrt((boxes.length * o.aspect * cellH) / cellW))));
  const rows = Math.ceil(boxes.length / columns);
  const at = new Map(boxes.map((b, i) => [b.id, { x: (i % columns) * cellW, y: Math.floor(i / columns) * cellH }]));
  return {
    at,
    w: columns * cellW - o.gapX,
    h: rows * cellH - o.gapY,
    first: index.get((boxes[0] as ArrangeBox).id) as number,
    entry: o.entry !== undefined && at.has(o.entry),
  };
}

/**
 * The blocks in rows, as wide as the view's shape asks for the area they cover (at least the widest block): the
 * entry's block first, then the larger before the smaller, ties in input order.
 */
function pack(blocks: readonly Block[], o: ArrangeOptions): Map<string, Placed> {
  const order = [...blocks].sort(
    (a, b) => Number(b.entry) - Number(a.entry) || b.w * b.h - a.w * a.h || a.first - b.first,
  );
  const area = order.reduce((sum, b) => sum + (b.w + o.groupGap) * (b.h + o.groupGap), 0);
  const width = Math.max(...order.map((b) => b.w), Math.sqrt(area * o.aspect));
  const out = new Map<string, Placed>();
  let x = 0;
  let y = 0;
  let row = 0;
  for (const b of order) {
    if (x > 0 && x + b.w > width) {
      x = 0;
      y += row + o.groupGap;
      row = 0;
    }
    for (const [id, p] of b.at) {
      out.set(id, { x: Math.round(x + p.x), y: Math.round(y + p.y) });
    }
    x += b.w + o.groupGap;
    row = Math.max(row, b.h);
  }
  return out;
}
