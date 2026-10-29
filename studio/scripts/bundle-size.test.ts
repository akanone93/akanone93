// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { initialFiles, type Manifest, measure } from "./bundle-size.ts";

const manifest: Manifest = {
  "index.html": { file: "assets/index-aaa.js", isEntry: true, imports: ["_vendor.js"], dynamicImports: ["canvas.tsx"] },
  "_vendor.js": { file: "assets/vendor-bbb.js", imports: ["_shared.js"] },
  "_shared.js": { file: "assets/shared-ccc.js" },
  "canvas.tsx": { file: "assets/canvas-ddd.js", imports: ["_shared.js"] },
};

const contents: Record<string, string> = {
  "assets/index-aaa.js": "a".repeat(2000),
  "assets/vendor-bbb.js": "b".repeat(4000),
  "assets/shared-ccc.js": "c".repeat(1000),
  "assets/canvas-ddd.js": "d".repeat(100000),
};

const read = (file: string): Uint8Array<ArrayBuffer> => new TextEncoder().encode(contents[file] ?? "");

describe("initialFiles", () => {
  test("lists the entry and its static imports, never dynamic imports", () => {
    expect(initialFiles(manifest)).toEqual(["assets/index-aaa.js", "assets/vendor-bbb.js", "assets/shared-ccc.js"]);
  });

  test("ignores missing imports and non-script files", () => {
    const m: Manifest = {
      "index.html": { file: "assets/index.js", isEntry: true, imports: ["gone.js", "style.css"] },
      "style.css": { file: "assets/style.css" },
    };
    expect(initialFiles(m)).toEqual(["assets/index.js"]);
  });
});

describe("measure", () => {
  test("sums the gzip sizes and honours the budget", () => {
    const result = measure(manifest, read, 1_000_000);
    expect(result.files.map((f) => f.file)).toHaveLength(3);
    expect(result.total).toBe(result.files.reduce((sum, f) => sum + f.gzipBytes, 0));
    expect(result.total).toBeLessThan(500);
    expect(result.withinBudget).toBe(true);
    expect(measure(manifest, read, result.total - 1).withinBudget).toBe(false);
  });
});
