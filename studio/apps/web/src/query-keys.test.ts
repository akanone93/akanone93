// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";

/** Every source file of the web app, tests left out. */
function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return sources(path);
    }
    return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

/**
 * One query definition: its key as the cache sees it (the literal first part and how many parts follow), and the shape
 * of what it caches (the RPC it calls and the field it takes from the answer, if any).
 */
interface Definition {
  readonly file: string;
  readonly key: string;
  readonly shape: string;
}

const KEY = /queryKey:\s*\[\s*("[^"]+")([^\]]*)\],\s*queryFn:/g;
const RPC = /\.((?:list|get|search|read)\w*)\(/g;

/**
 * The shape a query caches: the first RPC its function calls and the field taken from the answer, read by matching the
 * call's brackets — `(await client.listTeams({ … })).teams` is `listTeams.teams`, `client.listTeams({ … })` is
 * `listTeams`. Without an RPC, the function's first line.
 */
function shapeOf(fn: string): string {
  RPC.lastIndex = 0;
  const call = RPC.exec(fn);
  if (call === null) {
    return fn.split("\n")[0]?.trim() ?? "";
  }
  let at = call.index + call[0].length;
  for (let depth = 1; depth > 0 && at < fn.length; at++) {
    depth += fn[at] === "(" ? 1 : fn[at] === ")" ? -1 : 0;
  }
  const field = /^\)*\.(\w+)/.exec(fn.slice(at));
  return field === null ? (call[1] ?? "") : `${call[1]}.${field[1]}`;
}

function definitions(): Definition[] {
  const root = import.meta.dir;
  return sources(root).flatMap((file) => {
    const text = readFileSync(file, "utf8");
    return [...text.matchAll(KEY)].map((m) => {
      // Literal parts stay as written; any other part is a value, `_`.
      const parts = (m[2] ?? "")
        .split(",")
        .map((p) => p.trim())
        .filter((p) => p !== "")
        .map((p) => (/^"[^"]*"$/.test(p) ? p : "_"));
      return {
        file: relative(root, file),
        key: `[${[m[1], ...parts].join(", ")}]`,
        shape: shapeOf(text.slice((m.index ?? 0) + m[0].length, (m.index ?? 0) + m[0].length + 600)),
      };
    });
  });
}

describe("query cache keys [STU-011]", () => {
  test("a key fetched in several places caches the same shape everywhere", () => {
    const found = definitions();
    expect(found.length).toBeGreaterThan(50);
    const byKey = new Map<string, Definition[]>();
    for (const d of found) {
      byKey.set(d.key, [...(byKey.get(d.key) ?? []), d]);
    }
    // Two queries with one key and different shapes share one cache entry: whichever ran last decides what the
    // other reads, and a list read as an answer object crashed the organisation home after the way back from a canvas.
    const clashes = [...byKey.entries()].flatMap(([key, defs]) =>
      new Set(defs.map((d) => d.shape)).size > 1
        ? [`${key}: ${defs.map((d) => `${d.file} → ${d.shape}`).join(" | ")}`]
        : [],
    );
    expect(clashes).toEqual([]);
  });
});
