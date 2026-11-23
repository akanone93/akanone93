// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { canvasViewKey, rememberedView, rememberView } from "./canvas-view-memory.ts";

afterEach(() => sessionStorage.clear());

describe("where a canvas was left, per tab (build log entry 249)", () => {
  test("a view kept for a canvas comes back for that canvas only", () => {
    const app = canvasViewKey("o1", "a1");
    const plugin = canvasViewKey("o1", "a1", "p1");
    rememberView(app, { x: 120.5, y: -40, zoom: 0.35 });
    expect(rememberedView(app)).toEqual({ x: 120.5, y: -40, zoom: 0.35 });
    expect(rememberedView(plugin)).toBeUndefined();
    expect(rememberedView(canvasViewKey("o1", "a2"))).toBeUndefined();
  });

  test("a view that is not one is neither kept nor read back", () => {
    const key = canvasViewKey("o1", "a1");
    rememberView(key, { x: Number.NaN, y: 0, zoom: 1 });
    rememberView(key, { x: 0, y: 0, zoom: 0 });
    expect(rememberedView(key)).toBeUndefined();
    for (const raw of ["{", "null", '{"x":"1","y":0,"zoom":1}', '{"x":1,"y":2,"zoom":-1}', "[1,2,3]"]) {
      sessionStorage.setItem(key, raw);
      expect(rememberedView(key)).toBeUndefined();
    }
  });
});
