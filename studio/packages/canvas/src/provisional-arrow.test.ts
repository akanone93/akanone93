// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { defaultArrowStyle, ProvisionalArrow } from "./arrow-layer.ts";

describe("provisional arrow [STU-047]", () => {
  test("draws a shaft and two head strokes, then moves them", () => {
    const arrow = new ProvisionalArrow();
    const first = arrow.update({ x: 0, y: 0 }, { x: 100, y: 0 });
    expect(first.map((o) => o.op)).toEqual(["add", "add", "add"]);
    const second = arrow.update({ x: 0, y: 0 }, { x: 100, y: 50 });
    expect(second.map((o) => o.op)).toEqual(["replace", "replace", "replace"]);
    const shaft = second[0];
    expect(shaft?.op === "replace" && shaft.item.kind === "link" ? shaft.item.to : undefined).toEqual({
      x: 100,
      y: 50,
    });
  });

  test("uses the focus stroke and a zero-length drag does not produce NaN", () => {
    const ops = new ProvisionalArrow().update({ x: 5, y: 5 }, { x: 5, y: 5 });
    for (const o of ops) {
      if (o.op === "add" && o.item.kind === "link") {
        expect(o.item.stroke).toEqual(defaultArrowStyle.focus);
        expect(Number.isFinite(o.item.to.x + o.item.to.y)).toBe(true);
      }
    }
  });

  test("clear removes what was drawn, once", () => {
    const arrow = new ProvisionalArrow();
    expect(arrow.clear()).toEqual([]);
    arrow.update({ x: 0, y: 0 }, { x: 1, y: 1 });
    expect(arrow.clear()).toHaveLength(3);
    expect(arrow.clear()).toEqual([]);
  });
});
