// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { FrameStats, ScreenDocument, ScreenStyle } from "../src/protocol.ts";
import { measurePropertyChange, PROPERTY_BUDGET_MS, type PropertyHost } from "./property.ts";

const screens: ScreenDocument[] = [
  { id: "s", title: "S", x: 0, y: 0, root: { id: "r", type: "Text", props: { data: "a" } } },
];
const style = {} as ScreenStyle;

describe("STU-121 property change to canvas frame measurement", () => {
  test("times each edit from hand-over to the frame after the layout answer, after the warm-up", async () => {
    const calls: string[] = [];
    let clock = 0;
    const host: PropertyHost = {
      layoutScreens: async () => {
        calls.push("layout");
        clock += 10;
        return [];
      },
      patch: () => calls.push("patch"),
      nextFrame: async () => {
        calls.push("frame");
        clock += 5;
        return { ms: 2, flushMs: 1 } as FrameStats;
      },
    };
    const seen: number[] = [];
    const summary = await measurePropertyChange(
      host,
      screens,
      style,
      (s, i) => {
        seen.push(i);
        return s;
      },
      { warmup: 2, runs: 5, now: () => clock },
    );
    expect(seen).toEqual([0, 1, 2, 3, 4, 5, 6]);
    expect(calls.slice(0, 4)).toEqual(["layout", "layout", "patch", "frame"]);
    expect(summary).toEqual({
      runs: 5,
      p50: 15,
      p95: 15,
      max: 15,
      layoutP95: 10,
      frameP95: 2,
      flushP95: 1,
      budgetMs: PROPERTY_BUDGET_MS,
    });
  });

  test("the budget is 50 ms at p95", () => {
    expect(PROPERTY_BUDGET_MS).toBe(50);
  });
});
