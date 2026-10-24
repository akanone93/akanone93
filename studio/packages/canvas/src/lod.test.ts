// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { INTERACTION_RESOLUTION, lodForZoom, resolutionFor, SHADOWS_BELOW, STRAIGHT_LINKS_BELOW } from "./lod.ts";

describe("STU-120 level of detail by zoom (M-12)", () => {
  test("full detail at and above the link and text threshold", () => {
    expect(lodForZoom(1)).toEqual({ straightLinks: false, textBars: false, shadows: true });
    expect(lodForZoom(STRAIGHT_LINKS_BELOW)).toEqual({ straightLinks: false, textBars: false, shadows: true });
  });

  test("straight links and text bars below it, no shadows below the shadow threshold", () => {
    expect(lodForZoom(STRAIGHT_LINKS_BELOW - 0.01)).toEqual({ straightLinks: true, textBars: true, shadows: true });
    expect(lodForZoom(SHADOWS_BELOW)).toEqual({ straightLinks: true, textBars: true, shadows: true });
    expect(lodForZoom(SHADOWS_BELOW - 0.01)).toEqual({ straightLinks: true, textBars: true, shadows: false });
  });

  test("levels are shared constants and resolution halves while interacting", () => {
    expect(lodForZoom(2)).toBe(lodForZoom(3));
    expect(resolutionFor(true)).toBe(INTERACTION_RESOLUTION);
    expect(resolutionFor(false)).toBe(1);
  });
});
