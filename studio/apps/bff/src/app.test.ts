// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createApp } from "./app.ts";

let dir = "";

beforeAll(() => {
  dir = mkdtempSync(join(tmpdir(), "plux-bff-"));
  writeFileSync(join(dir, "hello.txt"), "hello studio");
});

afterAll(() => {
  rmSync(dir, { recursive: true, force: true });
});

describe("createApp", () => {
  test("GET /livez answers 200 ok as text/plain", async () => {
    const res = await createApp({ staticDir: dir }).request("/livez");
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type") ?? "").toContain("text/plain");
    expect(await res.text()).toBe("ok");
  });

  test("serves a file from the static directory", async () => {
    const res = await createApp({ staticDir: dir }).request("/hello.txt");
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("hello studio");
  });

  test("answers 404 for an unknown path", async () => {
    const res = await createApp({ staticDir: dir }).request("/missing.txt");
    expect(res.status).toBe(404);
  });
});
