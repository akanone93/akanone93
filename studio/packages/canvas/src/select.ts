// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Renderer selection at start-up (ADR-0013 M-12). The canvas probes WebGL
 * with `failIfMajorPerformanceCaveat` and reads the GL renderer string; on a
 * hardware GL it uses WebGL, and on a software GL (SwiftShader, llvmpipe, or
 * a context refused for a major performance caveat) it uses CanvasKit's CPU
 * raster backend. A start-up measurement cannot make this choice: on software
 * GL the WebGL cost lands in the GPU process after each recentre (stalls of
 * 400-870 ms under SwiftShader), invisible to timings taken in the worker.
 */

import type { RendererPreference } from "./protocol.ts";

/** What the WebGL probe found. */
export interface GlProbe {
  readonly available: boolean;
  readonly software: boolean;
  readonly renderer: string;
  readonly majorPerformanceCaveat: boolean;
}

const softwareGl = /swiftshader|llvmpipe|softpipe|software|basic render/i;

/** Whether a GL renderer string names a software implementation. */
export function isSoftwareGl(renderer: string): boolean {
  return softwareGl.test(renderer);
}

/** The renderer to use for a preference and a WebGL probe: a forced choice wins; otherwise WebGL only on hardware GL. */
export function chooseRenderer(preference: RendererPreference, probe: GlProbe): "webgl" | "raster" {
  if (preference !== "auto") {
    return preference;
  }
  return probe.available && !probe.software ? "webgl" : "raster";
}
