// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  dashedOutline,
  deriveGhosts,
  GHOST,
  ghostItem,
  ghostItems,
  ghostOfHit,
  PULSE_MS,
  placeGhosts,
  pulseSteps,
  readAppScreens,
  SceneSync,
} from "./ghosts.ts";
import { ghostStyle } from "./scene.ts";

const page = (id: string, route: string, title: string) =>
  new TextEncoder().encode(
    JSON.stringify({
      id,
      route,
      kind: "page",
      pageKind: "screen",
      root: { id: `${id}-r`, type: "Text", props: { data: title } },
    }),
  );

describe("[STU-052] ghost screens", () => {
  const app = readAppScreens([
    { pluginId: "loans", kind: "page", content: page("s1", "/loan", "Loan form") },
    { pluginId: "cards", kind: "page", content: page("s2", "/card", "Card") },
    { pluginId: "cards", kind: "plugin", content: new TextEncoder().encode("{}") },
    { pluginId: "cards", kind: "page", content: new TextEncoder().encode("not json") },
  ]);

  test("reads the screens of every plugin by route", () => {
    expect(app.map((s) => [s.id, s.route, s.title, s.pluginId])).toEqual([
      ["s1", "/loan", "Loan form", "loans"],
      ["s2", "/card", "Card", "cards"],
    ]);
  });

  const own = [
    { id: "a", route: "/home", title: "Home", targets: ["/loan", "/loan", "/card", "/pay", "/home", "/nowhere"] },
    { id: "b", route: "/other", title: "Other", targets: ["/home"] },
  ];
  const graph = deriveGhosts({ plugin: "cards", screens: own, appScreens: app, nativeRoutes: ["/pay"] });

  test("a route of another plugin becomes a ghost with its link count; own routes do not", () => {
    expect(graph.targets.map((t) => [t.id, t.kind, t.pluginId, t.screenId, t.links])).toEqual([
      ["ghost:s1", "screen", "loans", "s1", 2],
      ["native:/pay", "native", undefined, undefined, 1],
    ]);
  });

  test("native routes are native nodes [STU-054]; unknown routes get no box", () => {
    expect(graph.edges.map((e) => e.target)).toEqual(["ghost:s1", "ghost:s1", "native:/pay", "a", "a"]);
  });

  test("edges name their source, trigger and title", () => {
    expect(graph.edges[0]).toEqual({
      id: "a#0",
      source: "a",
      target: "ghost:s1",
      trigger: "navigate",
      title: "Home → Loan form",
    });
  });

  test("ghosts stack in a column right of the screens", () => {
    const placed = placeGhosts(graph.targets, 1000, 20);
    expect(placed[0]).toMatchObject({ x: 1000 + GHOST.offset, y: 20, w: GHOST.w, h: GHOST.h });
    expect(placed[1]?.y).toBe(20 + GHOST.h + GHOST.gap);
    expect(placed[1]?.h).toBe(GHOST.nativeH);
  });

  test("the frame is dashed: separate segments, none longer than a dash", () => {
    const dashes = dashedOutline(0, 0, 100, 60);
    expect(dashes.length).toBeGreaterThan(8);
    for (const d of dashes) {
      expect(Math.hypot(d.to.x - d.from.x, d.to.y - d.from.y)).toBeLessThanOrEqual(10.0001);
    }
  });

  test("a ghost draws a card with its thumbnail, dashed frame, title and link count", () => {
    const g = placeGhosts(graph.targets, 0, 0)[0];
    if (g === undefined) {
      throw new Error("no ghost");
    }
    const items = ghostItems(g, ghostStyle, { links: (n) => `${n} links` }, "hash1");
    const card = items.find((i) => i.id === ghostItem(g.id));
    expect(card).toMatchObject({ kind: "rect", image: "hash1" });
    expect(items.filter((i) => i.kind === "link").length).toBe(dashedOutline(g.x, g.y, g.w, g.h).length);
    expect(items.filter((i) => i.kind === "text").map((i) => (i.kind === "text" ? i.text : ""))).toEqual([
      "Loan form",
      "2 links",
    ]);
    expect(
      ghostItems(g, ghostStyle, { links: String }, undefined).find((i) => i.id === ghostItem(g.id)),
    ).not.toHaveProperty("image");
  });

  test("a hit on any part of a ghost finds the ghost", () => {
    expect(ghostOfHit(["x", `${ghostItem("ghost:s1")}/title`], graph.targets)?.id).toBe("ghost:s1");
    expect(ghostOfHit(["screen:a"], graph.targets)).toBeUndefined();
  });
});

describe("scene sync", () => {
  const rect = (id: string, x: number) => ({ kind: "rect" as const, id, x, y: 0, w: 1, h: 1, radius: 0, z: 0 });

  test("adds new items, replaces changed ones, removes gone ones and leaves the rest", () => {
    const sync = new SceneSync();
    expect(sync.sync([rect("a", 0), rect("b", 0)]).map((o) => o.op)).toEqual(["add", "add"]);
    expect(sync.sync([rect("a", 0), rect("b", 0)])).toEqual([]);
    expect(sync.sync([rect("a", 5)]).map((o) => o.op)).toEqual(["replace", "remove"]);
  });
});

describe("[STU-053] pulse", () => {
  test("the ring appears, blinks and is gone after 2 s", () => {
    const steps = pulseSteps("p", { l: 0, t: 0, r: 10, b: 10 }, 1);
    expect(steps[0]?.ops[0]?.op).toBe("add");
    expect(steps.at(-1)).toMatchObject({ at: PULSE_MS, ops: [{ op: "remove", id: "p" }] });
    expect(steps.map((s) => s.at)).toEqual([...steps.map((s) => s.at)].sort((a, b) => a - b));
    expect(PULSE_MS).toBe(2000);
  });
});
