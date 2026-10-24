// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { beforeAll, describe, expect, test } from "bun:test";
import type { CanvasKit } from "canvaskit-wasm";
import { loadCanvasKit, softwareProbe } from "../test-support/canvaskit.ts";
import { chooseRenderer, type GlProbe, isSoftwareGl, measureSurface } from "./select.ts";

let ck: CanvasKit;
beforeAll(async () => {
  ck = await loadCanvasKit();
});

const hardware: GlProbe = {
  available: true,
  software: false,
  renderer: "NVIDIA GeForce",
  majorPerformanceCaveat: false,
};
const none: GlProbe = { available: false, software: true, renderer: "", majorPerformanceCaveat: false };

describe("STU-120 renderer selection by measurement (M-12)", () => {
  test("software GL renderer strings are recognised", () => {
    expect(isSoftwareGl(softwareProbe.renderer)).toBe(true);
    expect(isSoftwareGl("llvmpipe (LLVM 17.0.6, 256 bits)")).toBe(true);
    expect(isSoftwareGl("Microsoft Basic Render Driver")).toBe(true);
    expect(isSoftwareGl("ANGLE (Intel, Mesa Intel(R) UHD Graphics 620)")).toBe(false);
  });

  test("WebGL is kept unless CPU raster is clearly faster; no GL means raster; a forced choice wins", () => {
    expect(chooseRenderer("auto", hardware, null)).toBe("webgl");
    expect(chooseRenderer("auto", none, null)).toBe("raster");
    expect(chooseRenderer("auto", softwareProbe, { webgl: 10, raster: 9 })).toBe("webgl");
    expect(chooseRenderer("auto", softwareProbe, { webgl: 10, raster: 7 })).toBe("raster");
    expect(chooseRenderer("raster", hardware, null)).toBe("raster");
    expect(chooseRenderer("webgl", none, null)).toBe("webgl");
  });

  test("the start-up workload measures a surface and frees what it made", () => {
    const surface = ck.MakeSurface(256, 256);
    expect(surface).not.toBeNull();
    if (surface) {
      expect(measureSurface(ck, surface, () => performance.now(), 3)).toBeGreaterThan(0);
      surface.delete();
    }
  });
});
