// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { insetOf, withInset } from "./properties-model.ts";

describe("PC-33 the insets editor reads every EdgeInsets form [WGT-010]", () => {
  test("each side's inset from the side, start/end, horizontal/vertical or all", () => {
    const sides = (o: Record<string, unknown>) =>
      (["left", "top", "right", "bottom"] as const).map((s) => insetOf(o, s));
    expect(sides({ all: 32 })).toEqual([32, 32, 32, 32]);
    expect(sides({ horizontal: 16, vertical: 8 })).toEqual([16, 8, 16, 8]);
    expect(sides({ all: 4, top: 12 })).toEqual([4, 12, 4, 4]);
    expect(sides({ start: 20, end: 10 })).toEqual([20, undefined, 10, undefined]);
    expect(sides({})).toEqual([undefined, undefined, undefined, undefined]);
  });

  test("editing one side spreads a shorthand, so the others keep their values", () => {
    expect(withInset({ all: 32 }, "top", 8)).toEqual({ left: 32, top: 8, right: 32, bottom: 32 });
    expect(withInset({ horizontal: 16 }, "left", undefined)).toEqual({ right: 16 });
    expect(withInset({ left: 4 }, "left", undefined)).toBeUndefined();
  });

  test("directional insets stay directional, so they still mirror right to left", () => {
    expect(withInset({ start: 20, end: 10 }, "left", 24)).toEqual({ start: 24, end: 10 });
    expect(withInset({ start: 20, all: 2 }, "top", 6)).toEqual({ start: 20, top: 6, end: 2, bottom: 2 });
  });
});
