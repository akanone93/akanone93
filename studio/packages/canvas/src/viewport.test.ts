// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { makeRect } from "./geometry.ts";
import { clampZoom, MAX_ZOOM, MIN_ZOOM, Viewport } from "./viewport.ts";

describe("STU-120 viewport math", () => {
  test("zooming about a point keeps the world point under it fixed", () => {
    const viewport = new Viewport(800, 600);
    viewport.apply({ op: "set", x: 100, y: 50, zoom: 1 });
    const before = viewport.toWorld(300, 200);
    viewport.zoomAbout(1.7, 300, 200);
    const after = viewport.toWorld(300, 200);
    expect(after.x).toBeCloseTo(before.x, 9);
    expect(after.y).toBeCloseTo(before.y, 9);
    expect(viewport.zoom).toBeCloseTo(1.7, 9);
  });

  test("zoom is clamped to 10–400 % and still keeps the point fixed", () => {
    const viewport = new Viewport(800, 600);
    viewport.apply({ op: "zoom", factor: 100, x: 400, y: 300 });
    expect(viewport.zoom).toBe(MAX_ZOOM);
    expect(viewport.toWorld(400, 300).x).toBeCloseTo(400, 9);
    viewport.apply({ op: "zoom", factor: 0.0001, x: 0, y: 0 });
    expect(viewport.zoom).toBe(MIN_ZOOM);
    viewport.apply({ op: "set", x: 0, y: 0, zoom: 9 });
    expect(viewport.zoom).toBe(MAX_ZOOM);
    expect(clampZoom(0.5)).toBe(0.5);
  });

  test("panning moves content by CSS pixels at the current zoom", () => {
    const viewport = new Viewport(800, 600);
    viewport.apply({ op: "set", x: 0, y: 0, zoom: 2 });
    viewport.apply({ op: "pan", dx: 100, dy: -50 });
    expect(viewport.x).toBe(-50);
    expect(viewport.y).toBe(25);
    expect(viewport.worldRect(makeRect(0, 0, 0, 0))).toEqual({ l: -50, t: 25, r: 350, b: 325 });
  });
});
