// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { box, link, text } from "../test-support/canvaskit.ts";
import { makeRect } from "./geometry.ts";
import { HitIndex, MAX_CELLS } from "./hit.ts";
import type { FrameSpec } from "./protocol.ts";

function frame(id: string, x: number, y: number, w = 200, h = 300, z = 0): FrameSpec {
  return { kind: "frame", id, name: id, x, y, w, h, radius: 8, fill: 0xffffffff, shadow: true, z };
}

describe("HitIndex (ADR-0013 hit-testing)", () => {
  test("answers point queries topmost first: z, then insertion order", () => {
    const index = new HitIndex();
    index.apply([
      { op: "add", item: frame("f", 0, 0) },
      { op: "add", item: box("a", 10, 10, 0x3366ccff, 1) },
      { op: "add", item: box("b", 20, 20, 0x3366ccff, 1) },
      { op: "add", item: box("top", 30, 30, 0x3366ccff, 5) },
    ]);
    expect(index.hitPoint(35, 35)).toEqual(["top", "b", "a", "f"]);
    expect(index.hitPoint(15, 15)).toEqual(["a", "f"]);
    expect(index.hitPoint(-500, -500)).toEqual([]);
  });

  test("hits a frame by its shape, not its shadow, and widens hits by the tolerance", () => {
    const index = new HitIndex();
    index.apply([{ op: "add", item: frame("f", 0, 0) }]);
    expect(index.hitPoint(-10, 10)).toEqual([]);
    expect(index.hitPoint(-2, 10, 3)).toEqual(["f"]);
  });

  test("clips an item to its frame for points and rectangles", () => {
    const index = new HitIndex();
    index.apply([
      { op: "add", item: frame("f", 0, 0, 100, 100) },
      { op: "add", item: { ...box("inside", 80, 80), frame: "f" } },
    ]);
    // box() is 100 x 60: it overflows the frame at the right and bottom.
    expect(index.hitPoint(90, 90)).toEqual(["inside", "f"]);
    expect(index.hitPoint(150, 120)).toEqual([]);
    expect(index.hitRect(makeRect(120, 120, 200, 200))).toEqual([]);
    expect(index.hitRect(makeRect(70, 70, 200, 200), "contain")).toEqual(["inside"]);
  });

  test("hits links near their path only, straight and curved", () => {
    const index = new HitIndex();
    index.apply([
      { op: "add", item: link("l", 0, 0, 200, 200) },
      {
        op: "add",
        item: { ...link("c", 0, 300, 200, 300), c1: { x: 50, y: 200 }, c2: { x: 150, y: 200 } },
      },
    ]);
    expect(index.hitPoint(100, 100, 1)).toEqual(["l"]);
    expect(index.hitPoint(150, 20, 1)).toEqual([]);
    expect(index.hitPoint(100, 225, 2)).toEqual(["c"]);
    expect(index.hitPoint(100, 290, 2)).toEqual([]);
  });

  test("selects by marquee: intersecting or wholly contained", () => {
    const index = new HitIndex();
    index.apply([
      { op: "add", item: box("a", 0, 0) },
      { op: "add", item: box("b", 300, 0) },
      { op: "add", item: text("t", 0, 100) },
    ]);
    expect(index.hitRect(makeRect(50, 0, 350, 10))).toEqual(["b", "a"]);
    expect(index.hitRect(makeRect(-10, -10, 120, 200), "contain")).toEqual(["t", "a"]);
  });

  test("follows moves, replaces, removes and clears", () => {
    const index = new HitIndex();
    index.apply([
      { op: "add", item: box("a", 0, 0) },
      { op: "move", id: "a", dx: 1000, dy: 0 },
    ]);
    expect(index.hitPoint(10, 10)).toEqual([]);
    expect(index.hitPoint(1010, 10)).toEqual(["a"]);
    index.apply([{ op: "replace", item: box("a", 0, 0, 0x3366ccff, 1) }]);
    expect(index.hitPoint(10, 10)).toEqual(["a"]);
    index.apply([{ op: "remove", id: "a" }]);
    expect(index.size).toBe(0);
    expect(index.hitPoint(10, 10)).toEqual([]);
    index.apply([{ op: "add", item: box("b", 0, 0) }, { op: "clear" }]);
    expect(index.hitPoint(10, 10)).toEqual([]);
  });

  test("keeps items spanning many cells in the large list", () => {
    const index = new HitIndex(10);
    const span = Math.ceil(Math.sqrt(MAX_CELLS)) * 10 * 2;
    index.apply([{ op: "add", item: frame("big", 0, 0, span, span) }]);
    expect(index.hitPoint(span - 1, span - 1)).toEqual(["big"]);
    index.apply([{ op: "move", id: "big", dx: 5, dy: 0 }]);
    expect(index.hitPoint(2, 2)).toEqual([]);
    index.apply([{ op: "remove", id: "big" }]);
    expect(index.hitPoint(span, span)).toEqual([]);
  });

  test("queries 10 000 items without scanning them all", () => {
    const index = new HitIndex();
    const ops = [];
    for (let i = 0; i < 10_000; i++) {
      ops.push({ op: "add" as const, item: box(`b${i}`, (i % 100) * 150, Math.floor(i / 100) * 100) });
    }
    index.apply(ops);
    const start = performance.now();
    for (let i = 0; i < 1000; i++) {
      index.hitPoint((i % 100) * 150 + 5, 5);
    }
    expect(index.hitPoint(155, 105)).toEqual(["b101"]);
    expect(performance.now() - start).toBeLessThan(500);
  });
});
