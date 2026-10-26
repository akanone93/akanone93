// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { handleAnchor, screenAt, toCanvas, toWorld } from "./link-drag.ts";

describe("link drag geometry (STU-047)", () => {
  test("canvas and world points convert both ways", () => {
    const v = { x: 100, y: 50, zoom: 2 };
    expect(toWorld(v, { x: 20, y: 10 })).toEqual({ x: 110, y: 55 });
    expect(toCanvas(v, toWorld(v, { x: 33, y: 7 }))).toEqual({ x: 33, y: 7 });
  });

  test("the handle sits on the trailing edge, centred", () => {
    expect(handleAnchor({ l: 0, t: 10, r: 80, b: 30 })).toEqual({ x: 80, y: 20 });
  });

  test("the screen under a point is found, the topmost winning", () => {
    const frames = [
      { id: "a", box: { l: 0, t: 0, r: 100, b: 100 } },
      { id: "b", box: { l: 50, t: 50, r: 150, b: 150 } },
    ];
    expect(screenAt({ x: 10, y: 10 }, frames)).toBe("a");
    expect(screenAt({ x: 60, y: 60 }, frames)).toBe("b");
    expect(screenAt({ x: 500, y: 5 }, frames)).toBeUndefined();
  });
});
