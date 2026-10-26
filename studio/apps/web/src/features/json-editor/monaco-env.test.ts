// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { WORKER_POLICY } from "../../lib/trusted-worker.ts";
import { MONACO_POLICY_NAMES, monacoPolicy } from "./monaco-env.ts";

describe("STU-063 Monaco under the CSP and Trusted Types", () => {
  test("answers Monaco's rendering policies through the one allowed policy name, and no other", () => {
    const names: string[] = [];
    const factory = {
      createPolicy: (name: string) => {
        names.push(name);
        return { name };
      },
    };
    for (const wanted of MONACO_POLICY_NAMES) {
      expect(monacoPolicy(wanted, { createHTML: (v: string) => v }, factory)).toEqual({ name: WORKER_POLICY });
    }
    expect(names.every((n) => n === WORKER_POLICY)).toBe(true);
    expect(monacoPolicy("markdownRenderer", {}, factory)).toBeUndefined();
    expect(monacoPolicy("defaultWorkerFactory", {}, factory)).toBeUndefined();
    expect(names).toHaveLength(MONACO_POLICY_NAMES.size);
    expect(monacoPolicy("editorViewLayer", {}, undefined)).toBeUndefined();
  });
});
