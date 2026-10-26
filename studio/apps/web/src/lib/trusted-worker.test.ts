// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  canvasWorkerScriptURL,
  isCanvasWorkerURL,
  isLockWorkerURL,
  isMonacoWorkerURL,
  isValidatorWorkerURL,
  lockWorkerScriptURL,
  monacoWorkerScriptURL,
  validatorWorkerScriptURL,
  WORKER_POLICY,
} from "./trusted-worker.ts";

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
    expect(names).toEqual([WORKER_POLICY]);
    expect(() => canvasWorkerScriptURL("https://evil.example/x.js", scope, false)).toThrow(TypeError);
    expect(() => canvasWorkerScriptURL("/x.js", { location: { origin } }, false)).toThrow(TypeError);
  });
});

describe("the validator worker through the same policy (S2.1)", () => {
  test("admits only its own built asset, and its dev module path in dev", () => {
    expect(isValidatorWorkerURL("/assets/validator-worker-Ab_1-9.js", origin, false)).toBe(true);
    for (const bad of [
      "/assets/canvas-worker-x.js",
      "/assets/validator-worker-x.js?y=1",
      "https://evil.example/assets/validator-worker-x.js",
      "javascript:alert(1)",
      "/src/features/problems/validator-worker.ts",
    ]) {
      expect(isValidatorWorkerURL(bad, origin, false)).toBe(false);
    }
    expect(isValidatorWorkerURL("/src/features/problems/validator-worker.ts", origin, true)).toBe(true);
  });

  test("creates no policy of its own name and refuses the canvas worker's address", () => {
    const names: string[] = [];
    const scope = {
      location: { origin },
      trustedTypes: {
        createPolicy(name: string, rules: { createScriptURL(input: string): string }) {
          names.push(name);
          return { createScriptURL: (input: string) => rules.createScriptURL(input) };
        },
      },
    };
    expect(validatorWorkerScriptURL("/assets/validator-worker-x.js", scope, false)).toBe(
      "/assets/validator-worker-x.js",
    );
    expect(names).toEqual([WORKER_POLICY]);
    expect(() => validatorWorkerScriptURL("/assets/canvas-worker-x.js", scope, false)).toThrow(TypeError);
    expect(validatorWorkerScriptURL("/assets/validator-worker-x.js", { location: { origin } }, false)).toBe(
      "/assets/validator-worker-x.js",
    );
  });
});

describe("the lock heartbeat worker through the same policy (S2.3)", () => {
  test("admits only its own built asset, and its dev module path in dev", () => {
    expect(isLockWorkerURL("/assets/lock-worker-Ab_1-9.js", origin, false)).toBe(true);
    for (const bad of [
      "/assets/canvas-worker-x.js",
      "/assets/validator-worker-x.js",
      "/assets/lock-worker-x.js?y=1",
      "https://evil.example/assets/lock-worker-x.js",
      "javascript:alert(1)",
      "/src/features/editor/lock-worker.ts",
    ]) {
      expect(isLockWorkerURL(bad, origin, false)).toBe(false);
    }
    expect(isLockWorkerURL("/src/features/editor/lock-worker.ts", origin, true)).toBe(true);
  });

  test("goes through the one plux-worker policy and refuses another worker's address", () => {
    const names: string[] = [];
    const scope = {
      location: { origin },
      trustedTypes: {
        createPolicy(name: string, rules: { createScriptURL(input: string): string }) {
          names.push(name);
          return { createScriptURL: (input: string) => rules.createScriptURL(input) };
        },
      },
    };
    expect(String(lockWorkerScriptURL("/assets/lock-worker-a1.js", scope, false))).toBe("/assets/lock-worker-a1.js");
    expect(() => lockWorkerScriptURL("/assets/canvas-worker-a1.js", scope, false)).toThrow(TypeError);
    expect(names.every((n) => n === WORKER_POLICY)).toBe(true);
  });
});

describe("the Monaco workers' Trusted Types policy (S5.1)", () => {
  test("admits only each worker's own built asset on this origin", () => {
    expect(isMonacoWorkerURL("editor", "/assets/monaco-editor-worker-a1_B-2.js", origin, false)).toBe(true);
    expect(isMonacoWorkerURL("json", `${origin}/assets/monaco-json-worker-a1.js`, origin, false)).toBe(true);
    for (const bad of [
      "/assets/monaco-json-worker-a1.js",
      "/assets/monaco-editor-worker-a1.js?x=1",
      "/assets/monaco-editor-worker-a1.js#h",
      "/assets/other/monaco-editor-worker-a1.js",
      "https://evil.example/assets/monaco-editor-worker-a1.js",
      "javascript:alert(1)",
      "/src/features/json-editor/monaco-editor-worker.ts",
    ]) {
      expect(isMonacoWorkerURL("editor", bad, origin, false)).toBe(false);
    }
    expect(isMonacoWorkerURL("json", "/src/features/json-editor/monaco-json-worker.ts", origin, true)).toBe(true);
    expect(isMonacoWorkerURL("json", "not a url", "", false)).toBe(false);
  });

  test("goes through the one plux-worker policy and refuses the other Monaco worker's address", () => {
    const names: string[] = [];
    const scope = {
      location: { origin },
      trustedTypes: {
        createPolicy(name: string, rules: { createScriptURL(input: string): string }) {
          names.push(name);
          return { createScriptURL: (input: string) => rules.createScriptURL(input) };
        },
      },
    };
    expect(String(monacoWorkerScriptURL("json", "/assets/monaco-json-worker-a1.js", scope, false))).toBe(
      "/assets/monaco-json-worker-a1.js",
    );
    expect(() => monacoWorkerScriptURL("json", "/assets/monaco-editor-worker-a1.js", scope, false)).toThrow(TypeError);
    expect(names.every((n) => n === WORKER_POLICY)).toBe(true);
  });
});
