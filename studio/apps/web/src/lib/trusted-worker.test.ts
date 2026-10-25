// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { CANVAS_WORKER_POLICY, canvasWorkerScriptURL, isCanvasWorkerURL } from "./trusted-worker.ts";

const origin = "https://studio.example.org";

describe("the canvas worker's Trusted Types policy", () => {
  test("admits only the built worker on this origin", () => {
    expect(isCanvasWorkerURL("/assets/canvas-worker-mnAtbjO5.js", origin, false)).toBe(true);
    expect(isCanvasWorkerURL(`${origin}/assets/canvas-worker-a_b-1.js`, origin, false)).toBe(true);
    for (const bad of [
      "https://evil.example/assets/canvas-worker-x.js",
      "/assets/other-x.js",
      "/assets/canvas-worker-x.js?y=1",
      "/assets/canvas-worker-x.js#a",
      "/assets/../assets/canvas-worker-x.js/../evil.js",
      "data:text/javascript,1",
      "blob:https://studio.example.org/1",
      "javascript:alert(1)",
      "/src/features/plugin-canvas/canvas-worker.ts",
    ]) {
      expect(isCanvasWorkerURL(bad, origin, false)).toBe(false);
    }
  });

  test("the dev server's module path is admitted only in dev", () => {
    const dev = "/src/features/plugin-canvas/canvas-worker.ts";
    expect(isCanvasWorkerURL(dev, origin, true)).toBe(true);
    expect(isCanvasWorkerURL("https://evil.example/canvas-worker.ts", origin, true)).toBe(false);
  });

  test("goes through the named policy where Trusted Types exist, and refuses other addresses", () => {
    const names: string[] = [];
    const trustedTypes = {
      createPolicy(name: string, rules: { createScriptURL(input: string): string }) {
        names.push(name);
        return { createScriptURL: (input: string) => ({ trusted: rules.createScriptURL(input) }) };
      },
    };
    const scope = { location: { origin }, trustedTypes };
    expect(canvasWorkerScriptURL("/assets/canvas-worker-x.js", scope, false)).toEqual({
      trusted: "/assets/canvas-worker-x.js",
    } as never);
    expect(names).toEqual([CANVAS_WORKER_POLICY]);
    expect(() => canvasWorkerScriptURL("https://evil.example/x.js", scope, false)).toThrow(TypeError);
    expect(() => canvasWorkerScriptURL("/x.js", { location: { origin } }, false)).toThrow(TypeError);
  });
});
