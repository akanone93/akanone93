// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { fromMap, mapFrame, onMap, panTo, viewRect } from "./view-map.ts";

describe("the view map above the zoom (C5)", () => {
  const screens = [
    { l: 0, t: 0, r: 390, b: 844 },
    { l: 600, t: 0, r: 990, b: 844 },
  ];

  test("the view's frame and every item fit inside the map, centred", () => {
    const view = viewRect({ x: -200, y: -100, zoom: 0.5 }, { w: 1000, h: 600 });
    expect(view).toEqual({ l: -200, t: -100, r: 1800, b: 1100 });
    const f = mapFrame(screens, view, { w: 180, h: 112 });
    for (const r of [...screens, view]) {
      const m = onMap(f, r);
      expect(m.x).toBeGreaterThanOrEqual(0);
      expect(m.y).toBeGreaterThanOrEqual(0);
      expect(m.x + m.w).toBeLessThanOrEqual(180.0001);
      expect(m.y + m.h).toBeLessThanOrEqual(112.0001);
    }
    // A map point leads back to the world point drawn there.
    const p = fromMap(f, { x: onMap(f, screens[1] ?? view).x, y: onMap(f, screens[1] ?? view).y });
    expect(p.x).toBeCloseTo(600, 6);
    expect(p.y).toBeCloseTo(0, 6);
  });

  test("a pan centres the view on the world point dragged to", () => {
    const view = { x: 100, y: 50, zoom: 2 };
    const size = { w: 800, h: 400 };
    const { dx, dy } = panTo(view, size, { x: 700, y: 300 });
    // The surface moves the content by (dx, dy): the view's origin moves by the opposite, in world units.
    const moved = { x: view.x - dx / view.zoom, y: view.y - dy / view.zoom, zoom: 2 };
    const r = viewRect(moved, size);
    expect((r.l + r.r) / 2).toBeCloseTo(700, 6);
    expect((r.t + r.b) / 2).toBeCloseTo(300, 6);
  });
});
