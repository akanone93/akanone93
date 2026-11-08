// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { Environment } from "@plux/api";
import { devEnvironment } from "./use-dev-session.ts";

const env = (id: string, key: string, production: boolean) => ({ id, key, production }) as Environment;

describe("devEnvironment [DEV-002]", () => {
  test("opens the session in the app's development environment", () => {
    const envs = [env("p", "production", true), env("s", "staging", false), env("d", "development", false)];
    expect(devEnvironment(envs)?.id).toBe("d");
  });

  test("falls back to the first environment that is not production, and never to production", () => {
    expect(devEnvironment([env("p", "production", true), env("q", "qa", false)])?.id).toBe("q");
    expect(devEnvironment([env("p", "production", true)])).toBeUndefined();
    expect(devEnvironment([])).toBeUndefined();
  });
});
