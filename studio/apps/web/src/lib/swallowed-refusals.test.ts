// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const root = join(import.meta.dir, "..");

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) {
      return name === "node_modules" || name === "locales" ? [] : sources(path);
    }
    return /\.(ts|tsx)$/.test(name) && !/\.test\.tsx?$/.test(name) && name !== "test-upstream.ts" ? [path] : [];
  });
}

/** The body of the `try` block that ends right before `catch {` at `at`, found by matching braces backwards. */
function tryBody(text: string, at: number): string {
  let depth = 0;
  for (let i = at - 1; i >= 0; i--) {
    const c = text[i];
    if (c === "}") {
      depth++;
    } else if (c === "{") {
      depth--;
      if (depth === 0) {
        return text.slice(i, at);
      }
    }
  }
  return "";
}

/** A call to the server inside the try: an awaited RPC client method or a mutation, which can be refused. */
const serverCall = /await\s+[\w.]+\.(?!json\(|text\(|arrayBuffer\()[a-z]\w*\(\{|await\s+mutateAsync\(|createClient\(/;

describe("refusals are never swallowed [SRV-006] [SEC-103]", () => {
  test("a catch that drops the error of a server call says why it may", () => {
    const found: string[] = [];
    for (const file of sources(root)) {
      const text = readFileSync(file, "utf8");
      for (const m of text.matchAll(/catch \{/g)) {
        const at = m.index ?? 0;
        const body = tryBody(text, at);
        if (!serverCall.test(body)) {
          continue;
        }
        // An intended one names why on the line right above the catch: `// refusal-ok: <why>`.
        const lines = text.slice(0, at).split("\n");
        if (/\/\/ refusal-ok: \S/.test(lines[lines.length - 2] ?? "")) {
          continue;
        }
        found.push(`${relative(root, file)}:${text.slice(0, at).split("\n").length}`);
      }
    }
    // Each of these drops a refusal, so the person sees a generic failure without the reason or the step-up it needs.
    // Catch the error and show it with `withReason`, or say why dropping it is right with `// refusal-ok: <why>`.
    expect(found).toEqual([]);
  });
});
