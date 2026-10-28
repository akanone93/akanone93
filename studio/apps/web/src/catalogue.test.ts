// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { build } from "vite";

describe("[STU-011] development-only catalogue page", () => {
  test("the production bundle has no catalogue code", async () => {
    // bun test sets NODE_ENV=test, which Vite would treat as a development build.
    const previous = process.env.NODE_ENV;
    process.env.NODE_ENV = "production";
    const result = await build({
      root: new URL("..", import.meta.url).pathname,
      logLevel: "silent",
      configFile: new URL("../vite.config.ts", import.meta.url).pathname,
      mode: "production",
      build: { write: false, sourcemap: false, manifest: false },
    });
    if (previous === undefined) {
      delete process.env.NODE_ENV;
    } else {
      process.env.NODE_ENV = previous;
    }
    const outputs = (Array.isArray(result) ? result : [result]).flatMap((r) => ("output" in r ? r.output : []));
    const code = outputs.map((o) => (o.type === "chunk" ? o.code : "")).join("\n");
    expect(code.length).toBeGreaterThan(0);
    expect(code).not.toContain("__catalogue");
    expect(code).not.toContain("data-case");
    expect(code).not.toContain("applyCatalogueParams");
  }, 120_000);
});
