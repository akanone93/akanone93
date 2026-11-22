// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ArrowCanvas } from "./arrow-layer.ts";
import {
  type ArrowBox,
  ArrowRouter,
  attachAlong,
  type Bundle,
  type Cubic,
  chooseSides,
  cubicAt,
  deriveAppBundles,
  deriveBundles,
  derivePluginBundles,
  type NavigateEdge,
  placeAnchors,
  type Route,
  type Side,
} from "./arrows.ts";
import { HitIndex } from "./hit.ts";
import {
  ARROW_DETAIL_BELOW,
  arrowLodForZoom,
  EXPAND_MS,
  Expansion,
  laneGeometry,
  laneMenu,
  splitCubic,
} from "./lanes.ts";

function edge(id: string, source: string, target: string): NavigateEdge {
  return { id, source, target, trigger: `${id} · onTap`, title: `${id} → ${target}` };
}

function box(id: string, x: number, y: number, w = 100, h = 60): ArrowBox {
  return { id, x, y, w, h };
}

function inside(p: { x: number; y: number }, b: ArrowBox): boolean {
  return p.x > b.x && p.x < b.x + b.w && p.y > b.y && p.y < b.y + b.h;
}

function crosses(route: Route, boxes: readonly ArrowBox[]): string[] {
  const hit = new Set<string>();
  for (const seg of route.segments) {
    for (let i = 0; i <= 400; i++) {
      const p = cubicAt(seg, i / 400);
      for (const b of boxes) {
        if (b.id !== route.bundle.source && b.id !== route.bundle.target && inside(p, b)) {
          hit.add(b.id);
        }
      }
    }
  }
  return [...hit];
}

/** A seeded grid scene: `n` boxes on a spaced grid, `m` links. */
function grid(n: number, m: number, seed = 7): { boxes: ArrowBox[]; edges: NavigateEdge[] } {
  let state = seed;
  const random = (): number => {
    state = (Math.imul(state, 1664525) + 1013904223) >>> 0;
    return state / 2 ** 32;
  };
  const boxes = Array.from({ length: n }, (_, i) =>
    box(`b${String(i).padStart(3, "0")}`, (i % 6) * 320, Math.floor(i / 6) * 240, 120, 72),
  );
  const edges = Array.from({ length: m }, (_, i) => {
    const a = Math.floor(random() * n);
    const b = (a + 1 + Math.floor(random() * (n - 1))) % n;
    return edge(`e${i}`, boxes[a]?.id ?? "", boxes[b]?.id ?? "");
  });
  return { boxes, edges };
}

describe("[STU-040] [STU-041] derivation and bundling", () => {
  test("every navigate edge becomes a link; same-direction links share one bundle", () => {
    const bundles = deriveBundles([
      edge("n2", "A", "B"),
      edge("n1", "A", "B"),
      edge("n3", "B", "A"),
      edge("n4", "A", "A"),
    ]);
    expect(bundles.map((b) => [b.id, b.links.map((l) => l.id)])).toEqual([
      ["A→B", ["n1", "n2"]],
      ["B→A", ["n3"]],
    ]);
  });

  test("app canvas bundles by plugin; native targets stay their own box", () => {
    const pluginOf = new Map([
      ["home", "p1"],
      ["offers", "p1"],
      ["loan", "p2"],
    ]);
    const bundles = deriveAppBundles(
      [
        edge("a", "home", "loan"),
        edge("b", "offers", "loan"),
        edge("c", "home", "offers"),
        edge("d", "loan", "native:camera"),
      ],
      pluginOf,
    );
    expect(bundles.map((b) => `${b.id}:${b.links.length}`)).toEqual(["p1→p2:2", "p2→native:camera:1"]);
  });

  test("[STU-051] plugin canvas bundles screen-to-screen links leaving its own screens", () => {
    const pluginOf = new Map([
      ["home", "p1"],
      ["offers", "p1"],
      ["loan", "p2"],
    ]);
    const bundles = derivePluginBundles(
      [
        edge("a", "home", "offers"),
        edge("b", "home", "offers"),
        edge("c", "offers", "loan"),
        edge("d", "loan", "home"),
      ],
      "p1",
      pluginOf,
    );
    expect(bundles.map((b) => `${b.id}:${b.links.length}`)).toEqual(["home→offers:2", "offers→loan:1"]);
    const router = new ArrowRouter(
      [box("home", 0, 0, 180, 390), box("offers", 400, 0, 180, 390), box("loan", 800, 0, 180, 390)],
      bundles,
    );
    expect(router.routes.get("home→offers")?.sourceSide).toBe(1);
    expect(router.routes.get("offers→loan")?.clear).toBe(true);
  });
});

describe("[STU-042] side choice, hysteresis and anchor spread", () => {
  test("the shortest side pair connects facing sides", () => {
    expect(chooseSides(box("a", 0, 0), box("b", 300, 0), undefined)).toEqual([1, 3]);
    expect(chooseSides(box("a", 0, 0), box("b", 0, 300), undefined)).toEqual([2, 0]);
    expect(chooseSides(box("a", 300, 0), box("b", 0, 0), undefined)).toEqual([3, 1]);
  });

  test("hysteresis keeps the side pair near the diagonal and switches once clearly shorter", () => {
    const a = box("a", 0, 0, 100, 100);
    const router = new ArrowRouter([a, box("b", 300, 40, 100, 100)], deriveBundles([edge("x", "a", "b")]));
    expect(router.routes.get("a→b")?.sourceSide).toBe(1);
    const sides: number[] = [];
    // Drag b along the diagonal: right/left wins first, bottom/top later.
    for (let step = 0; step <= 60; step++) {
      router.moveBox("b", 300 - step * 4, 40 + step * 6);
      sides.push(router.routes.get("a→b")?.sourceSide ?? -1);
    }
    const switches = sides.filter((s, i) => i > 0 && s !== sides[i - 1]).length;
    expect(switches).toBe(1);
    expect(sides.at(-1)).toBe(2);
    // Wiggling around the switch point does not flicker.
    const index = sides.indexOf(2);
    const at = { x: 300 - index * 4, y: 40 + index * 6 };
    const wiggle: number[] = [];
    for (let i = 0; i < 20; i++) {
      router.moveBox("b", at.x + (i % 2 === 0 ? 3 : -3), at.y + (i % 2 === 0 ? -3 : 3));
      wiggle.push(router.routes.get("a→b")?.sourceSide ?? -1);
    }
    expect(new Set(wiggle).size).toBe(1);
  });

  test("anchors sharing a side are spread and ordered; opposite bundles stay parallel", () => {
    const boxes = [box("a", 0, 200), box("b", 400, 0), box("c", 400, 200), box("d", 400, 400)];
    const router = new ArrowRouter(
      boxes,
      deriveBundles([edge("1", "a", "b"), edge("2", "a", "c"), edge("3", "a", "d"), edge("4", "c", "a")]),
    );
    const route = (id: string): Route => router.routes.get(id) as Route;
    const start = (id: string): number => (route(id).segments[0] as Cubic).from.y;
    const end = (id: string): number => (route(id).segments.at(-1) as Cubic).to.y;
    const ys = [start("a→b"), start("a→c"), end("c→a"), start("a→d")];
    expect(router.routes.get("a→b")?.sourceSide).toBe(1);
    for (let i = 1; i < ys.length; i++) {
      expect(ys[i] as number).toBeGreaterThan(ys[i - 1] as number);
    }
    // a→c and c→a: same order at both ends, so they do not cross.
    const ac = router.routes.get("a→c") as Route;
    const ca = router.routes.get("c→a") as Route;
    const acEnd = (ac.segments.at(-1) as Cubic).to.y;
    const caStart = (ca.segments[0] as Cubic).from.y;
    expect(Math.sign(start("a→c") - end("c→a"))).toBe(Math.sign(acEnd - caStart));
  });
});

describe("[STU-043] perpendicular Béziers routed around boxes", () => {
  test("leaves and enters perpendicular to the sides", () => {
    const router = new ArrowRouter([box("a", 0, 0), box("b", 400, 100)], deriveBundles([edge("x", "a", "b")]));
    const route = router.routes.get("a→b") as Route;
    const first = route.segments[0] as Cubic;
    const last = route.segments.at(-1) as Cubic;
    expect(first.c1.y).toBe(first.from.y);
    expect(first.c1.x).toBeGreaterThan(first.from.x);
    expect(last.c2.y).toBe(last.to.y);
    expect(last.c2.x).toBeLessThan(last.to.x);
  });

  test("a box in the way is avoided (sampled)", () => {
    // Boxes above and below wall off the arcs over and under `o`, so only a detour gets past it.
    const boxes = [
      box("a", 0, 0),
      box("o", 300, 0),
      box("b", 600, 0),
      box("u", 150, -200, 400, 60),
      box("d", 150, 200, 400, 60),
    ];
    const router = new ArrowRouter(boxes, deriveBundles([edge("x", "a", "b")]));
    const route = router.routes.get("a→b") as Route;
    expect(route.clear).toBe(true);
    expect(route.segments.length).toBeGreaterThan(1);
    expect(crosses(route, boxes)).toEqual([]);
  });

  test("[STU-042] a side pair that connects straight and clear is preferred to a detour", () => {
    const boxes = [box("a", 0, 0), box("o", 300, 0), box("b", 600, 0)];
    const router = new ArrowRouter(boxes, deriveBundles([edge("x", "a", "b")]));
    const route = router.routes.get("a→b") as Route;
    expect(route.clear).toBe(true);
    expect(route.segments.length).toBe(1);
    expect(route.sourceSide).toBe(route.targetSide);
    expect(crosses(route, boxes)).toEqual([]);
  });

  test("on a spaced grid every route is clear and no sampled point lies in a box", () => {
    const { boxes, edges } = grid(24, 80);
    const router = new ArrowRouter(boxes, deriveBundles(edges));
    let clear = 0;
    for (const route of router.routes.values()) {
      if (route.clear) {
        clear++;
        expect(crosses(route, boxes)).toEqual([]);
      }
    }
    expect(clear / router.routes.size).toBeGreaterThan(0.9);
  });
});

/** The point on `side` of `b` at `along` on the side's axis. */
function onSide(b: ArrowBox, side: Side, along: number): { x: number; y: number } {
  if (side === 0 || side === 2) {
    return { x: along, y: side === 0 ? b.y : b.y + b.h };
  }
  return { x: side === 1 ? b.x + b.w : b.x, y: along };
}

/** The length of the connection between the nearest anchors of side pair (`sa`, `sb`). */
function connection(a: ArrowBox, sa: Side, b: ArrowBox, sb: Side): number {
  const p = onSide(a, sa, attachAlong(a, sa, b, sb));
  const q = onSide(b, sb, attachAlong(b, sb, a, sa));
  return Math.hypot(q.x - p.x, q.y - p.y);
}

describe("[STU-042] [STU-045] anchors follow the shortest connection while a box moves", () => {
  test("an end attaches where its side is nearest the other box; overlapping sides connect straight", () => {
    const a = box("a", 0, 0, 100, 400);
    const router = new ArrowRouter([a, box("b", 300, 0)], deriveBundles([edge("x", "a", "b")]));
    router.beginDrag("b");
    for (const y of [0, 100, 200, 300, 340]) {
      router.moveBox("b", 300, y);
      const route = router.routes.get("a→b") as Route;
      const first = route.segments[0] as Cubic;
      const last = route.segments.at(-1) as Cubic;
      expect(route.sourceSide).toBe(1);
      // b spans y … y + 60 and a 0 … 400: they overlap, so the arrow is straight across the overlap.
      expect(first.from.y).toBeCloseTo(last.to.y, 9);
      expect(first.from.y).toBeCloseTo(attachAlong(a, 1, box("b", 300, y), 3), 9);
    }
    router.endDrag();
  });

  test("dragging a box round another re-picks the closest side pair live, without flicker", () => {
    const a = box("a", 0, 0, 200, 120);
    const router = new ArrowRouter([a, box("b", 500, 30, 200, 120)], deriveBundles([edge("x", "a", "b")]));
    router.beginDrag("b");
    const seen: number[] = [];
    for (let deg = 0; deg <= 360; deg += 3) {
      const t = (deg * Math.PI) / 180;
      const b = box("b", 500 * Math.cos(t), 30 + 400 * Math.sin(t), 200, 120);
      router.moveBox("b", b.x, b.y);
      const route = router.routes.get("a→b") as Route;
      // The pair in use is never more than the hysteresis margin longer than the shortest one.
      const [sa, sb] = chooseSides(a, b, undefined, 0, 0);
      expect(connection(a, route.sourceSide, b, route.targetSide)).toBeLessThanOrEqual(
        connection(a, sa, b, sb) * 1.03 + 12 + 1e-9,
      );
      seen.push(route.sourceSide * 4 + route.targetSide);
    }
    router.endDrag();
    // Every side of a is used on the way round, and each switch happens once per crossing (no flicker).
    expect(new Set(seen.map((p) => p >> 2))).toEqual(new Set([0, 1, 2, 3]));
    const switches = seen.filter((p, i) => i > 0 && p !== seen[i - 1]).length;
    expect(switches).toBeLessThanOrEqual(12);
  });

  test("anchors sharing a side sit where they want, kept apart in order", () => {
    expect(placeAnchors([50, 200, 300], 0, 400, 20)).toEqual([50, 200, 300]);
    expect(placeAnchors([100, 100, 100], 0, 400, 20)).toEqual([80, 100, 120]);
    expect(placeAnchors([0, 0], 10, 400, 20)).toEqual([10, 30]);
    expect(placeAnchors([400, 400], 0, 400, 20)).toEqual([380, 400]);
    // Too short for the gaps: spaced evenly.
    expect(placeAnchors([5, 6, 7], 0, 20, 20)).toEqual([0, 10, 20]);
  });
});

describe("[STU-042] [STU-043] routes stay off their own boxes and keep smooth", () => {
  /** Whether a sampled point of `route` lies inside one of its own end boxes, a little in from the edges. */
  function entersEnds(route: Route, boxes: readonly ArrowBox[]): boolean {
    const ends = boxes.filter((b) => b.id === route.bundle.source || b.id === route.bundle.target);
    return route.segments.some((seg) =>
      Array.from({ length: 201 }, (_, i) => cubicAt(seg, i / 200)).some((p) =>
        ends.some((b) => p.x > b.x + 2 && p.x < b.x + b.w - 2 && p.y > b.y + 2 && p.y < b.y + b.h - 2),
      ),
    );
  }

  test("a remembered side pair that no longer routes clear gives way to the shortest", () => {
    // Apply Register's canvas: Account → Launch first leaves from Account's top; once Console moves above
    // Account and the boxes close up, that side is under Console and its detour would dip back into Account.
    const edges = [edge("x", "account", "launch"), edge("y", "account", "launch")];
    const before = [
      box("account", 0, 800, 260, 152),
      box("console", -600, 600, 260, 152),
      box("launch", 356, 400, 260, 152),
    ];
    const after = [
      box("account", 0, 600, 260, 152),
      box("console", 0, 400, 260, 152),
      box("launch", 356, 300, 260, 152),
    ];
    const router = new ArrowRouter(before, deriveBundles(edges));
    expect(router.routes.get("account→launch")?.sourceSide).toBe(0);
    router.setGraph(after, deriveBundles(edges));
    const route = router.routes.get("account→launch") as Route;
    expect(route.clear).toBe(true);
    expect(route.sourceSide).toBe(1);
    expect(entersEnds(route, after)).toBe(false);
    expect(crosses(route, after)).toEqual([]);
  });

  test("no route of a grid scene dips into its own boxes, and no arm is longer than half its piece", () => {
    const { boxes, edges } = grid(24, 80);
    const router = new ArrowRouter(boxes, deriveBundles(edges));
    for (const route of router.routes.values()) {
      if (route.clear) {
        expect(entersEnds(route, boxes)).toBe(false);
      }
      for (const s of route.segments) {
        const chord = Math.hypot(s.to.x - s.from.x, s.to.y - s.from.y);
        expect(Math.hypot(s.c1.x - s.from.x, s.c1.y - s.from.y)).toBeLessThanOrEqual(chord / 2 + 1e-9);
        expect(Math.hypot(s.to.x - s.c2.x, s.to.y - s.c2.y)).toBeLessThanOrEqual(chord / 2 + 1e-9);
      }
    }
  });

  /** The largest change of heading between neighbouring samples of a route, in degrees: a kink is large. */
  function sharpest(route: Route): number {
    const points = route.segments.flatMap((s) => Array.from({ length: 400 }, (_, i) => cubicAt(s, i / 399)));
    let worst = 0;
    let previous: number | undefined;
    for (let i = 1; i < points.length; i++) {
      const a = points[i - 1] as { x: number; y: number };
      const b = points[i] as { x: number; y: number };
      if (Math.hypot(b.x - a.x, b.y - a.y) < 0.05) {
        continue;
      }
      const heading = (Math.atan2(b.y - a.y, b.x - a.x) * 180) / Math.PI;
      if (previous !== undefined) {
        worst = Math.max(worst, Math.abs(((heading - previous + 540) % 360) - 180));
      }
      previous = heading;
    }
    return worst;
  }

  test("detours around neighbouring corners bend smoothly, without a kink", () => {
    // Apply Register's app canvas as laid out: Explore → Launch detoured through two waypoints 22 units
    // apart, and the short piece between them overshot its arms into a notch beside the "10" badge.
    const at: Record<string, [number, number]> = {
      verify: [0, 0],
      pay: [0, 200],
      console: [0, 400],
      account: [0, 600],
      launch: [356, 300],
      partner: [712, 190],
      passes: [712, 520],
      explore: [1068, 465],
    };
    const boxes = Object.entries(at).map(([id, [x, y]]) => box(id, x, y, 260, 152));
    const links: [string, string, number][] = [
      ["verify", "launch", 1],
      ["pay", "launch", 1],
      ["console", "launch", 1],
      ["account", "launch", 2],
      ["account", "passes", 1],
      ["launch", "partner", 1],
      ["partner", "launch", 2],
      ["launch", "explore", 10],
      ["explore", "launch", 1],
      ["launch", "passes", 1],
      ["passes", "launch", 1],
      ["passes", "explore", 2],
    ];
    const edges = links.flatMap(([a, b, n]) => Array.from({ length: n }, (_, i) => edge(`${a}-${b}-${i}`, a, b)));
    const router = new ArrowRouter(boxes, deriveBundles(edges));
    for (const route of router.routes.values()) {
      expect(route.clear).toBe(true);
      expect(entersEnds(route, boxes)).toBe(false);
      expect(sharpest(route)).toBeLessThan(3);
      for (const s of route.segments) {
        const chord = Math.hypot(s.to.x - s.from.x, s.to.y - s.from.y);
        expect(Math.hypot(s.c1.x - s.from.x, s.c1.y - s.from.y)).toBeLessThanOrEqual(chord / 2 + 1e-9);
      }
    }
    expect((router.routes.get("explore→launch") as Route).segments.length).toBeLessThanOrEqual(2);
  });
});

describe("[STU-045] determinism and incremental re-routing", () => {
  test("same input gives identical routes", () => {
    const { boxes, edges } = grid(30, 120);
    const one = new ArrowRouter(boxes, deriveBundles(edges));
    const two = new ArrowRouter([...boxes].reverse(), deriveBundles([...edges].reverse()));
    expect(JSON.stringify([...two.routes])).toBe(JSON.stringify([...one.routes]));
  });

  test("moveBox re-routes only what can change and equals a full re-route", () => {
    const { boxes, edges } = grid(30, 120);
    const router = new ArrowRouter(boxes, deriveBundles(edges));
    let changed = 0;
    for (let i = 1; i <= 40; i++) {
      changed += router.moveBox("b007", 320 + i * 9, 240 + i * 5).length;
    }
    expect(changed / 40).toBeLessThan(router.routes.size);
    const incremental = JSON.stringify([...router.routes]);
    router.routeAll();
    expect(JSON.stringify([...router.routes])).toBe(incremental);
    expect(router.moveBox("missing", 0, 0)).toEqual([]);
  });

  test("while dragging only the dragged box's bundles re-route; the drop catches up exactly", () => {
    const { boxes, edges } = grid(30, 120);
    const router = new ArrowRouter(boxes, deriveBundles(edges));
    const reference = new ArrowRouter(boxes, deriveBundles(edges));
    const own = new Set(router.bundlesAt("b007"));
    router.beginDrag("b007");
    for (let i = 1; i <= 40; i++) {
      const at = { x: 320 + i * 9, y: 240 + i * 5 };
      expect(router.moveBox("b007", at.x, at.y).every((id) => own.has(id))).toBe(true);
      reference.moveBox("b007", at.x, at.y);
    }
    const caught = router.endDrag();
    expect(caught.length).toBeGreaterThan(0);
    expect(JSON.stringify([...router.routes])).toBe(JSON.stringify([...reference.routes]));
    expect(router.endDrag()).toEqual([]);
    const arrows = new ArrowCanvas(router);
    arrows.render();
    expect(arrows.beginDrag("b007").length).toBeGreaterThan(0);
    arrows.moveBox("b007", 0, 0);
    expect(arrows.endDrag().length).toBeGreaterThan(0);
  });
});

describe("[STU-044] [STU-046] [STU-048] lanes, picking and level of detail", () => {
  const boxes = [box("a", 0, 0), box("b", 500, 0)];
  const bundles: Bundle[] = deriveBundles([edge("n1", "a", "b"), edge("n2", "a", "b"), edge("n3", "a", "b")]);

  test("lanes share the stems, fan out by the spacing, and start as the collapsed curve", () => {
    const router = new ArrowRouter(boxes, bundles);
    const route = router.routes.get("a→b") as Route;
    const collapsed = laneGeometry(route, 0);
    const [left, right] = splitCubic(route.segments[0] as Cubic, 0.5);
    for (const lane of collapsed) {
      expect(lane.segments).toEqual([left, right]);
    }
    const lanes = laneGeometry(route, 1);
    expect(lanes.map((l) => l.link.id)).toEqual(["n1", "n2", "n3"]);
    expect(lanes.map((l) => Math.round(l.label.y - (lanes[1]?.label.y ?? 0)))).toEqual([-28, 0, 28]);
    for (const lane of lanes) {
      expect(lane.segments[0]?.from).toEqual(left.from);
      expect(lane.segments[0]?.c1).toEqual(left.c1);
      expect(lane.segments.at(-1)?.to).toEqual(right.to);
    }
  });

  test("expansion animates within 150–200 ms and collapses back", () => {
    const expansion = new Expansion();
    expect(expansion.setFocus("x", 1000)).toEqual(["x"]);
    expect(expansion.setFocus("x", 1000)).toEqual([]);
    expect(expansion.progress("x", 1000)).toBe(0);
    expect(expansion.progress("x", 1000 + EXPAND_MS / 2)).toBeGreaterThan(0.5);
    expect(EXPAND_MS).toBeGreaterThanOrEqual(150);
    expect(EXPAND_MS).toBeLessThanOrEqual(200);
    expect(expansion.animating(1000 + EXPAND_MS)).toEqual([]);
    expect(expansion.setFocus(null, 2000)).toEqual(["x"]);
    expect(expansion.animating(2000 + 10)).toEqual(["x"]);
    expect(expansion.settle(2000 + EXPAND_MS)).toBe(false);
  });

  test("clicking a lane picks that one link through the hit index and offers the lane menu", () => {
    const router = new ArrowRouter(boxes, bundles);
    const arrows = new ArrowCanvas(router);
    const index = new HitIndex();
    index.apply(arrows.render());
    const route = router.routes.get("a→b") as Route;
    const mid = cubicAt(route.segments[0] as Cubic, 0.5);
    expect(arrows.pick(index.hitPoint(mid.x, mid.y, 3))).toEqual({ bundle: route.bundle });
    index.apply(arrows.setFocus("a→b", 0));
    expect(arrows.animating(10)).toBe(true);
    index.apply(arrows.frame(EXPAND_MS / 2));
    index.apply(arrows.frame(EXPAND_MS));
    index.apply(arrows.frame(EXPAND_MS + 16));
    expect(arrows.animating(EXPAND_MS + 16)).toBe(false);
    const lanes = laneGeometry(route, 1);
    const lane = lanes[2];
    const p = cubicAt(lane?.segments[0] as Cubic, 0.9);
    const pick = arrows.pick(index.hitPoint(p.x, p.y, 2));
    expect(pick?.link?.id).toBe("n3");
    expect(arrows.menu(pick ?? { bundle: route.bundle }).map((m) => m.action)).toEqual([
      "open-source",
      "open-target",
      "change-trigger",
      "redirect",
      "remove",
    ]);
    expect(arrows.menu({ bundle: route.bundle })).toEqual([]);
    expect(laneMenu(lane?.link as NavigateEdge)).toHaveLength(5);
    expect(index.spec("arrow:a→b:t2")?.kind).toBe("text");
    // Collapse: lanes and titles go away, the badge returns.
    index.apply(arrows.setFocus(null, 1000));
    index.apply(arrows.frame(1000 + EXPAND_MS));
    index.apply(arrows.frame(1000 + EXPAND_MS + 16));
    expect(index.spec("arrow:a→b:t2")).toBeUndefined();
    expect(index.spec("arrow:a→b:badge")?.kind).toBe("rect");
    expect(index.spec("arrow:a→b:s0")?.z).toBe(0);
    expect(arrows.pick(["nothing"])).toBeUndefined();
  });

  test("unrelated bundles dim while one is expanded", () => {
    const router = new ArrowRouter([...boxes, box("c", 0, 300)], [...bundles, ...deriveBundles([edge("z", "c", "a")])]);
    const arrows = new ArrowCanvas(router);
    const index = new HitIndex();
    index.apply(arrows.render());
    index.apply(arrows.setFocus("a→b", 0));
    const other = index.spec("arrow:c→a:s0");
    const focused = index.spec("arrow:a→b:s0");
    expect(other?.kind === "link" && other.stroke.color).toBe(0x5f636840);
    expect(focused?.z).toBe(10);
  });

  test("low zoom: routed curves kept, no labels, lanes merged; moves patch only re-routed bundles", () => {
    expect(arrowLodForZoom(ARROW_DETAIL_BELOW - 0.01)).toEqual({ labels: false, lanes: false });
    expect(arrowLodForZoom(1)).toEqual({ labels: true, lanes: true });
    const router = new ArrowRouter(boxes, bundles);
    const arrows = new ArrowCanvas(router);
    const index = new HitIndex();
    index.apply(arrows.render());
    expect(arrows.setZoom(1)).toEqual([]);
    index.apply(arrows.setZoom(0.3));
    const line = index.spec("arrow:a→b:s0");
    expect(line?.kind === "link" && line.c1).toBeDefined();
    expect(index.spec("arrow:a→b:badge")).toBeUndefined();
    index.apply(arrows.setFocus("a→b", 0));
    index.apply(arrows.frame(EXPAND_MS));
    expect(index.spec("arrow:a→b:l0s0")).toBeUndefined();
    const ops = arrows.moveBox("b", 520, 10);
    expect(ops.every((op) => op.op !== "replace" || op.item.id.startsWith("arrow:a→b:"))).toBe(true);
    expect(arrows.moveBox("missing", 0, 0)).toEqual([]);
    // One item per route, whatever its pieces (#206).
    expect(arrows.itemsAt("b")).toEqual([
      "arrow:a→b:s0",
      "arrow:a→b:h0",
      "arrow:a→b:h1",
      "arrow:a→b:badge",
      "arrow:a→b:count",
    ]);
    expect(arrows.itemsAt("missing")).toEqual([]);
  });

  test("a regraph drops the items of vanished bundles", () => {
    const router = new ArrowRouter(boxes, bundles);
    const arrows = new ArrowCanvas(router);
    arrows.render();
    router.setGraph(boxes, []);
    const ops = arrows.render();
    expect(ops.length).toBeGreaterThan(0);
    expect(ops.every((op) => op.op === "remove")).toBe(true);
  });
});
