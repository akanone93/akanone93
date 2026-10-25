// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { softwareProbe } from "../test-support/canvaskit.ts";
import { chooseRenderer, type GlProbe, isSoftwareGl } from "./select.ts";

const hardware: GlProbe = {
  available: true,
  software: false,
  renderer: "NVIDIA GeForce",
  majorPerformanceCaveat: false,
};
const none: GlProbe = { available: false, software: true, renderer: "", majorPerformanceCaveat: false };

describe("STU-120 renderer selection by GL probe (M-12)", () => {
  test("software GL renderer strings are recognised", () => {
    expect(isSoftwareGl(softwareProbe.renderer)).toBe(true);
    expect(isSoftwareGl("llvmpipe (LLVM 17.0.6, 256 bits)")).toBe(true);
    expect(isSoftwareGl("Microsoft Basic Render Driver")).toBe(true);
    expect(isSoftwareGl("ANGLE (Intel, Mesa Intel(R) UHD Graphics 620)")).toBe(false);
  });

  test("hardware GL renders with WebGL; software GL, a performance caveat or no GL with CPU raster", () => {
    expect(chooseRenderer("auto", hardware)).toBe("webgl");
    expect(chooseRenderer("auto", softwareProbe)).toBe("raster");
    expect(chooseRenderer("auto", { ...hardware, software: true, majorPerformanceCaveat: true })).toBe("raster");
    expect(chooseRenderer("auto", none)).toBe("raster");
  });

  test("a forced choice wins over the probe", () => {
    expect(chooseRenderer("raster", hardware)).toBe("raster");
    expect(chooseRenderer("webgl", softwareProbe)).toBe("webgl");
    expect(chooseRenderer("webgl", none)).toBe("webgl");
  });
});
