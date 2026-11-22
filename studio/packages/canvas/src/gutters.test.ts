// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ArrowCanvas } from "./arrow-layer.ts";
import { type ArrowBox, ArrowRouter, cubicAt, deriveBundles, type NavigateEdge, type Route } from "./arrows.ts";
import type { Rect } from "./geometry.ts";
import { type GutterQuery, gutterRoute, MAX_GUTTER_NODES, roundedPath } from "./gutters.ts";
import type { LinkSpec, Point } from "./protocol.ts";

const rect = (l: number, t: number, r: number, b: number): Rect => ({ l, t, r, b });

/** Whether the axis-parallel segment p–q passes through the inside of `o`. */
function through(p: Point, q: Point, o: Rect): boolean {
  if (p.y === q.y) {
    return p.y > o.t && p.y < o.b && Math.max(p.x, q.x) > o.l && Math.min(p.x, q.x) < o.r;
  }
  return p.x > o.l && p.x < o.r && Math.max(p.y, q.y) > o.t && Math.min(p.y, q.y) < o.b;
}

function query(obstacles: readonly Rect[], extra: Partial<GutterQuery> = {}): GutterQuery {
  return {
    from: { x: 0, y: 50 },
    fromHeading: 1,
    to: { x: 400, y: 50 },
    toHeading: 1,
    obstacles,
    area: rect(-100, -200, 500, 300),
    bend: 60,
    ...extra,
  };
}

describe("[STU-043] paths through the gaps between boxes (#206)", () => {
  test("goes round a box in the way, along its edges, with as few turns as the cost asks", () => {
    const wall = rect(150, 0, 250, 100);
    const path = gutterRoute(query([wall])) as Point[];
    expect(path[0]).toEqual({ x: 0, y: 50 });
    expect(path.at(-1)).toEqual({ x: 400, y: 50 });
    for (let i = 1; i < path.length; i++) {
      const p = path[i - 1] as Point;
      const q = path[i] as Point;
      // Orthogonal legs that never enter the obstacle.
      expect(p.x === q.x || p.y === q.y).toBe(true);
      expect(through(p, q, wall)).toBe(false);
    }
    // Over (or under) the wall: four turns, the fewest that clear it and arrive heading right.
    expect(path).toHaveLength(6);
    expect([0, 100]).toContain(path[2]?.y ?? Number.NaN);
  });

  test("is a function of the query: the same path whatever the order of the obstacles", () => {
    const boxes = [rect(150, 0, 250, 100), rect(60, -150, 120, 40), rect(280, 60, 340, 260)];
    const a = gutterRoute(query(boxes));
    const b = gutterRoute(query([...boxes].reverse()));
    expect(a).toBeDefined();
    expect(b).toEqual(a);
  });

  test("finds none when the target is walled in, or when the grid is too large to search", () => {
    // Walls overlapping at the corners: a path may run along an edge, but no edge leads inside.
    const walled = [
      rect(330, -20, 470, 0),
      rect(330, 100, 470, 120),
      rect(330, -20, 350, 120),
      rect(450, -20, 470, 120),
    ];
    expect(gutterRoute(query(walled))).toBeUndefined();
    const many = Array.from({ length: 200 }, (_, i) => rect(i * 3, i * 3, i * 3 + 1, i * 3 + 1));
    expect(MAX_GUTTER_NODES).toBeLessThan(400 * 400);
    expect(gutterRoute(query(many, { area: rect(-10, -10, 700, 700) }))).toBeUndefined();
  });

  test("rounds the corners into one smooth path of cubic pieces, its badge on the longest straight run", () => {
    const { segments, middle } = roundedPath(
      [
        { x: 0, y: 0 },
        { x: 0, y: -40 },
        { x: 300, y: -40 },
        { x: 300, y: 0 },
      ],
      30,
    );
    expect(segments).toHaveLength(5);
    for (let i = 1; i < segments.length; i++) {
      const a = segments[i - 1];
      const b = segments[i];
      // Each piece starts where the one before ends, in the same direction (G1).
      expect(b?.from).toEqual(a?.to as Point);
      const out = { x: (a?.to.x ?? 0) - (a?.c2.x ?? 0), y: (a?.to.y ?? 0) - (a?.c2.y ?? 0) };
      const into = { x: (b?.c1.x ?? 0) - (b?.from.x ?? 0), y: (b?.c1.y ?? 0) - (b?.from.y ?? 0) };
      expect(Math.abs(out.x * into.y - out.y * into.x)).toBeLessThan(1e-9);
    }
    expect(segments[middle]?.from).toEqual({ x: 30, y: -40 });
    expect(segments[middle]?.to).toEqual({ x: 270, y: -40 });
  });
});

function edge(id: string, source: string, target: string): NavigateEdge {
  return { id, source, target, trigger: `${id} · onTap`, title: `${id} → ${target}` };
}

/** The boxes (other than its ends) a route passes through. */
function crossed(route: Route, boxes: readonly ArrowBox[]): string[] {
  const hit = new Set<string>();
  for (const seg of route.segments) {
    for (let i = 0; i <= 200; i++) {
      const p = cubicAt(seg, i / 200);
      for (const b of boxes) {
        const end = b.id === route.bundle.source || b.id === route.bundle.target;
        if (!end && p.x > b.x && p.x < b.x + b.w && p.y > b.y && p.y < b.y + b.h) {
          hit.add(b.id);
        }
      }
    }
  }
  return [...hit];
}

/** A row of six screens, as the plugin canvas lays them out. */
const row: ArrowBox[] = Array.from({ length: 6 }, (_, i) => ({ id: `s${i}`, x: i * 470, y: 0, w: 390, h: 844 }));

describe("[STU-043] [STU-045] arrows across a row of screens (#206)", () => {
  test("an arrow from the first screen to the last goes round the screens between them, not through them", () => {
    const router = new ArrowRouter(row, deriveBundles([edge("far", "s0", "s5"), edge("near", "s1", "s2")]));
    const far = router.routes.get("s0→s5") as Route;
    expect(far.clear).toBe(true);
    expect(crossed(far, row)).toEqual([]);
    expect(far.middle).toBeDefined();
    // The neighbour keeps its direct curve.
    expect((router.routes.get("s1→s2") as Route).segments).toHaveLength(1);
  });

  test("a drag skips the search, its drop catches up, and the result equals a full re-route", () => {
    const bundles = deriveBundles([edge("far", "s0", "s5"), edge("back", "s4", "s1")]);
    const live = new ArrowRouter(row, bundles);
    live.beginDrag("s3");
    live.moveBox("s3", 3 * 470, 120);
    live.moveBox("s3", 3 * 470, 300);
    live.endDrag();
    const moved = row.map((b) => (b.id === "s3" ? { ...b, y: 300 } : b));
    const full = new ArrowRouter(moved, bundles);
    for (const [id, route] of full.routes) {
      expect(live.routes.get(id)?.segments).toEqual(route.segments);
      expect(crossed(route, moved)).toEqual([]);
    }
  });

  test("a box moved only inside a route's search area routes it again, as a full re-route would", () => {
    const bundles = deriveBundles([edge("far", "s0", "s5")]);
    const live = new ArrowRouter(row, bundles);
    // s2 leaves the row: the way through its old place opens.
    live.moveBox("s2", 2 * 470, 2000);
    const moved = row.map((b) => (b.id === "s2" ? { ...b, y: 2000 } : b));
    expect(live.routes.get("s0→s5")?.segments).toEqual(new ArrowRouter(moved, bundles).routes.get("s0→s5")?.segments);
  });
});

describe("[STU-043] one scene item per arrow, and the arrows of a box highlighted (#206)", () => {
  test("a route through the gaps is drawn as one link with further pieces", () => {
    const arrows = new ArrowCanvas(new ArrowRouter(row, deriveBundles([edge("far", "s0", "s5")])));
    const items = arrows.render().flatMap((op) => (op.op === "replace" ? [op.item] : []));
    const shafts = items.filter((i) => i.kind === "link" && i.id.startsWith("arrow:s0→s5:s"));
    expect(shafts.map((i) => i.id)).toEqual(["arrow:s0→s5:s0"]);
    expect(((shafts[0] as LinkSpec).via ?? []).length).toBeGreaterThan(2);
  });

  test("an arrow starts with a dot touching its source box and ends with its head (#206)", () => {
    const arrows = new ArrowCanvas(new ArrowRouter(row, deriveBundles([edge("a", "s0", "s1")])));
    const items = arrows.render().flatMap((op) => (op.op === "replace" ? [op.item] : []));
    const shaft = items.find((i) => i.id === "arrow:s0→s1:s0") as LinkSpec;
    // Part of the shaft's own item, on the right side of s0 where the shaft starts, facing out of it, and kept at
    // least 4 pixels on screen.
    expect(shaft.dot).toMatchObject({ x: shaft.from.x, y: shaft.from.y, nx: 1, ny: 0, minScreenRadius: 4 });
    expect(shaft.dot?.x).toBeCloseTo(390, 6);
    expect(items.some((i) => i.id === "arrow:s0→s1:h0")).toBe(true);
  });

  test("the arrows of the highlighted boxes take the focus style and the rest dim, until the highlight ends", () => {
    const style = {
      stroke: { color: 0x111111ff, width: 2 },
      dimmed: { color: 0x22222240, width: 2 },
      focus: { color: 0x3333ffff, width: 3 },
      badgeFill: 0x3333ffff,
      badgeText: 0xffffffff,
      labelText: 0x000000ff,
      z: 0,
    };
    const arrows = new ArrowCanvas(
      new ArrowRouter(row, deriveBundles([edge("a", "s0", "s1"), edge("b", "s2", "s3")])),
      style,
    );
    arrows.render();
    const shaft = (ops: ReturnType<ArrowCanvas["render"]>, id: string) =>
      ops.flatMap((op) => (op.op === "replace" && op.item.id === id && op.item.kind === "link" ? [op.item] : []))[0];
    const lit = arrows.setHighlight(["s1"]);
    expect(shaft(lit, "arrow:s0→s1:s0")?.stroke.color).toBe(0x3333ffff);
    expect(shaft(lit, "arrow:s2→s3:s0")?.stroke.color).toBe(0x22222240);
    expect(arrows.setHighlight(["s1", "s1"])).toEqual([]);
    const off = arrows.setHighlight([]);
    expect(shaft(off, "arrow:s0→s1:s0")?.stroke.color).toBe(0x111111ff);
    expect(shaft(off, "arrow:s2→s3:s0")?.stroke.color).toBe(0x111111ff);
  });
});
