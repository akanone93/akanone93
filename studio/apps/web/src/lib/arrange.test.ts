// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  type ArrangeBox,
  type ArrangeLink,
  type ArrangeOptions,
  arrange,
  DENSE_PAIRS,
  type Placed,
} from "./arrange.ts";

const options: ArrangeOptions = { gapX: 120, gapY: 64, groupGap: 160, aspect: 1.6 };

const boxes = (n: number, w = 260, h = 152): ArrangeBox[] =>
  Array.from({ length: n }, (_, i) => ({ id: `b${i}`, w, h }));

/** The bounds of the boxes placed. */
function bounds(at: ReadonlyMap<string, Placed>, list: readonly ArrangeBox[]) {
  const xs = list.map((b) => at.get(b.id)?.x ?? 0);
  const ys = list.map((b) => at.get(b.id)?.y ?? 0);
  const rs = list.map((b) => (at.get(b.id)?.x ?? 0) + b.w);
  const bs = list.map((b) => (at.get(b.id)?.y ?? 0) + b.h);
  return { w: Math.max(...rs) - Math.min(...xs), h: Math.max(...bs) - Math.min(...ys) };
}

/** No two boxes overlap or come closer than the gaps. */
function apart(at: ReadonlyMap<string, Placed>, list: readonly ArrangeBox[], o: ArrangeOptions): boolean {
  for (const a of list) {
    for (const b of list) {
      if (a === b) {
        continue;
      }
      const p = at.get(a.id) as Placed;
      const q = at.get(b.id) as Placed;
      const clear =
        p.x + a.w + o.gapX <= q.x + 0.5 ||
        q.x + b.w + o.gapX <= p.x + 0.5 ||
        p.y + a.h + o.gapY <= q.y + 0.5 ||
        q.y + b.h + o.gapY <= p.y + 0.5;
      if (!clear) {
        return false;
      }
    }
  }
  return true;
}

describe("[STU-032] [STU-050] Arrange: layered, wrapped and packed (#241)", () => {
  test("a short chain reads left to right from the entry", () => {
    const list = boxes(3);
    const at = arrange(
      list,
      [
        { from: "b0", to: "b1" },
        { from: "b1", to: "b2" },
      ],
      { ...options, entry: "b0" },
    );
    expect((at.get("b0")?.x ?? 0) < (at.get("b1")?.x ?? 0)).toBe(true);
    expect((at.get("b1")?.x ?? 0) < (at.get("b2")?.x ?? 0)).toBe(true);
    expect(apart(at, list, options)).toBe(true);
  });

  test("a long chain of screens is wrapped into bands, not laid out as one row", () => {
    const list = boxes(12, 390, 844);
    const chain = list.slice(1).map((b, i) => ({ from: `b${i}`, to: b.id }));
    const o = { gapX: 200, gapY: 320, groupGap: 320, aspect: 1.6, entry: "b0" };
    const at = arrange(list, chain, o);
    const { w, h } = bounds(at, list);
    // One row would be 12 screens wide: 6,880 by 844.
    expect(w / h).toBeLessThan(3);
    expect(apart(at, list, o)).toBe(true);
    // Each band runs left to right: the next screen is to the right, or starts a band below.
    for (const { from, to } of chain) {
      const a = at.get(from) as Placed;
      const b = at.get(to) as Placed;
      expect(b.x > a.x || b.y > a.y).toBe(true);
    }
  });

  test("boxes without links sit in a grid beside the linked ones, not in one column", () => {
    const list = boxes(20);
    const at = arrange(list, [{ from: "b0", to: "b1" }], { ...options, entry: "b0" });
    const alone = list.slice(2);
    const columns = new Set(alone.map((b) => at.get(b.id)?.x));
    const rows = new Set(alone.map((b) => at.get(b.id)?.y));
    expect(columns.size).toBeGreaterThan(2);
    expect(rows.size).toBeGreaterThan(1);
    expect(apart(at, list, options)).toBe(true);
    // The entry's group comes first.
    expect(at.get("b0")).toEqual({ x: 0, y: at.get("b0")?.y ?? 0 });
  });

  test("a hub's links both ways count once, and the hub's neighbours do not form one tall column", () => {
    const list = boxes(13);
    const links: ArrangeLink[] = list.slice(1).flatMap((b) => [
      { from: "b0", to: b.id },
      { from: b.id, to: "b0" },
    ]);
    const at = arrange(list, links, { ...options, entry: "b0" });
    const { w, h } = bounds(at, list);
    expect(w / h).toBeGreaterThan(0.5);
    expect(apart(at, list, options)).toBe(true);
  });

  test("a dense graph is laid out on a backbone, keeps every box apart and is deterministic", () => {
    const list = boxes(80);
    const links: ArrangeLink[] = [];
    for (let i = 0; i < 400; i++) {
      // A fixed pseudo-random graph: no clock, no Math.random.
      links.push({ from: `b${(i * 37) % 80}`, to: `b${(i * 53 + 11) % 80}` });
    }
    expect(links.length).toBeGreaterThan(DENSE_PAIRS);
    const at = arrange(list, links, { ...options, entry: "b0" });
    expect(at.size).toBe(80);
    expect(apart(at, list, options)).toBe(true);
    expect(arrange(list, links, { ...options, entry: "b0" })).toEqual(at);
  });
});
