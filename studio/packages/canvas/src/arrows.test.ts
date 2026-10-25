// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { ArrowCanvas } from "./arrow-layer.ts";
import {
  type ArrowBox,
  ArrowRouter,
  type Bundle,
  type Cubic,
  chooseSides,
  cubicAt,
  deriveAppBundles,
  deriveBundles,
  derivePluginBundles,
  type NavigateEdge,
  type Route,
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

describe("STU-040 STU-041 derivation and bundling", () => {
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

  test("STU-051 plugin canvas bundles screen-to-screen links leaving its own screens", () => {
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

describe("STU-042 side choice, hysteresis and anchor spread", () => {
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

describe("STU-043 perpendicular Béziers routed around boxes", () => {
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
    const boxes = [box("a", 0, 0), box("o", 300, 0), box("b", 600, 0)];
    const router = new ArrowRouter(boxes, deriveBundles([edge("x", "a", "b")]));
    const route = router.routes.get("a→b") as Route;
    expect(route.clear).toBe(true);
    expect(route.segments.length).toBeGreaterThan(1);
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

describe("STU-045 determinism and incremental re-routing", () => {
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

describe("STU-044 STU-046 STU-048 lanes, picking and level of detail", () => {
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

  test("low zoom: straight lines, no labels, lanes merged; moves patch only re-routed bundles", () => {
    expect(arrowLodForZoom(ARROW_DETAIL_BELOW - 0.01)).toEqual({ straight: true, labels: false, lanes: false });
    expect(arrowLodForZoom(1)).toEqual({ straight: false, labels: true, lanes: true });
    const router = new ArrowRouter(boxes, bundles);
    const arrows = new ArrowCanvas(router);
    const index = new HitIndex();
    index.apply(arrows.render());
    expect(arrows.setZoom(1)).toEqual([]);
    index.apply(arrows.setZoom(0.3));
    const line = index.spec("arrow:a→b:s0");
    expect(line?.kind === "link" && line.c1).toBeUndefined();
    expect(index.spec("arrow:a→b:badge")).toBeUndefined();
    index.apply(arrows.setFocus("a→b", 0));
    index.apply(arrows.frame(EXPAND_MS));
    expect(index.spec("arrow:a→b:l0s0")).toBeUndefined();
    const ops = arrows.moveBox("b", 520, 10);
    expect(ops.every((op) => op.op !== "replace" || op.item.id.startsWith("arrow:a→b:"))).toBe(true);
    expect(arrows.moveBox("missing", 0, 0)).toEqual([]);
    expect(arrows.itemsAt("b")).toEqual([
      "arrow:a→b:s0",
      "arrow:a→b:h0",
      "arrow:a→b:h1",
      "arrow:a→b:s1",
      "arrow:a→b:s2",
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
