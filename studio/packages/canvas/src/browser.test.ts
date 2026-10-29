// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { CanvasKit } from "canvaskit-wasm";
import { type BrowserScope, browserWorkerDeps } from "./browser.ts";

const UNMASKED = 0x9246;
const RENDERER = 0x1f01;

/** A scope whose WebGL reports `renderer` and refuses caveat-free contexts when `caveat` is set. */
function scope(renderer: string | null, caveat = false): BrowserScope {
  class Canvas {
    constructor(
      readonly width: number,
      readonly height: number,
    ) {}
    getContext(_type: "webgl2", attributes?: { failIfMajorPerformanceCaveat: boolean }) {
      if (renderer === null || (caveat && attributes?.failIfMajorPerformanceCaveat)) {
        return null;
      }
      return {
        RENDERER,
        getExtension: (name: string) =>
          name === "WEBGL_debug_renderer_info" ? { UNMASKED_RENDERER_WEBGL: UNMASKED } : { loseContext: () => {} },
        getParameter: (parameter: number) => (parameter === UNMASKED ? renderer : "WebKit WebGL"),
      };
    }
  }
  return {
    requestAnimationFrame: () => 0,
    setTimeout: () => 0,
    performance: { now: () => 0 },
    OffscreenCanvas: Canvas,
  };
}

const probe = (s: BrowserScope) =>
  browserWorkerDeps(s, () => Promise.reject(new Error("unused")) as Promise<CanvasKit>).probeGl();

describe("[STU-120] software GL detection (M-12)", () => {
  test("a SwiftShader or llvmpipe renderer string marks the GL as software", () => {
    expect(probe(scope("ANGLE (Google, Vulkan 1.3.0 (SwiftShader Device (Subzero)), SwiftShader driver)"))).toEqual({
      available: true,
      software: true,
      renderer: "ANGLE (Google, Vulkan 1.3.0 (SwiftShader Device (Subzero)), SwiftShader driver)",
      majorPerformanceCaveat: false,
    });
    expect(probe(scope("llvmpipe (LLVM 17.0.6, 256 bits)")).software).toBe(true);
  });

  test("a context refused for a major performance caveat marks the GL as software", () => {
    expect(probe(scope("ANGLE (Intel, Mesa Intel(R) UHD Graphics 620)", true))).toMatchObject({
      available: true,
      software: true,
      majorPerformanceCaveat: true,
    });
  });

  test("a hardware renderer is not software; no WebGL or no OffscreenCanvas is unavailable", () => {
    expect(probe(scope("ANGLE (NVIDIA, NVIDIA GeForce RTX 3060)")).software).toBe(false);
    expect(probe(scope(null)).available).toBe(false);
    const { OffscreenCanvas: _omitted, ...noOffscreen } = scope(null);
    expect(probe(noOffscreen).available).toBe(false);
  });
});
