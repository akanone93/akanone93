// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { brotliCompressSync } from "node:zlib";
import { median, serve } from "./bench-lcp.ts";

let dist = "";

beforeAll(() => {
  dist = mkdtempSync(join(tmpdir(), "bench-lcp-"));
  mkdirSync(join(dist, "assets"));
  writeFileSync(join(dist, "index.html"), "<main>shell</main>");
  writeFileSync(join(dist, "assets/app.js"), "console.log(1)");
  writeFileSync(join(dist, "assets/app.js.br"), brotliCompressSync(Buffer.from("console.log(1)")));
  writeFileSync(join(dist, "assets/plain.css"), "a{}");
});

afterAll(() => rmSync(dist, { recursive: true, force: true }));

const get = (path: string, encoding = "br"): Response =>
  serve(dist, new Request(`http://x${path}`, { headers: { "accept-encoding": encoding } }));

describe("median", () => {
  test("takes the middle of the sorted values", () => {
    expect(median([30, 10, 20])).toBe(20);
    expect(median([5])).toBe(5);
    expect(median([])).toBe(0);
  });
});

describe("serve", () => {
  test("sends the Brotli sibling to a client that accepts it", async () => {
    const response = get("/assets/app.js");
    expect(response.headers.get("content-encoding")).toBe("br");
    expect(response.headers.get("content-type")).toBe("text/javascript");
    expect((await response.arrayBuffer()).byteLength).toBeGreaterThan(0);
  });

  test("sends the file as it is without Brotli support or a sibling", async () => {
    expect(get("/assets/app.js", "gzip").headers.get("content-encoding")).toBeNull();
    const css = get("/assets/plain.css");
    expect(css.headers.get("content-encoding")).toBeNull();
    expect(await css.text()).toBe("a{}");
  });

  test("answers the API with 401 and unknown paths with the app shell", async () => {
    expect(get("/api/plux.v1.Foo/Bar").status).toBe(401);
    expect(await get("/some/route").text()).toContain("<main>");
  });

  test("never leaves the build directory", async () => {
    expect(await get("/../../etc/passwd").text()).toContain("<main>");
  });
});
