// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

const roots = [join(import.meta.dir, ".."), join(import.meta.dir, "../../../../packages/ui/src")];

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return sources(path);
    }
    return /\.(tsx|css)$/.test(entry.name) && !/\.test\.|\.gen\./.test(entry.name) ? [path] : [];
  });
}

/** Tailwind utilities and CSS properties that fix a side instead of following the writing direction. */
const physical = [
  /(?:^|[\s"'`:])-?(?:m|p)[lr]-[\w[]/,
  /(?:^|[\s"'`:])-?(?:left|right)-[\w[]/,
  /(?:^|[\s"'`:])(?:rounded|border)-[lr](?:-|\s|"|'|`)/,
  /(?:^|[\s"'`:])text-(?:left|right)(?:\s|"|'|`)/,
  /(?:^|[\s"'`:])(?:float|clear)-(?:left|right)/,
  /(?:^|[\s"'`:])(?:space|divide)-x-/,
  /(?:margin|padding|border)-(?:left|right)\s*:/,
  /text-align\s*:\s*(?:left|right)/,
  /^\s*(?:left|right)\s*:\s*[-\w]/,
];

/** Intentionally physical: the link layer positions a handle at canvas coordinates, which the canvas never mirrors. */
const allowed = new Set(["features/plugin-canvas/link-layer.tsx"]);

describe("[STU-012] layout follows the writing direction", () => {
  test("no source uses a physical side where start and end exist", () => {
    const offenders: string[] = [];
    for (const root of roots) {
      for (const file of sources(root)) {
        const rel = file.slice(root.length + 1);
        if (allowed.has(rel)) {
          continue;
        }
        const lines = readFileSync(file, "utf8").split("\n");
        lines.forEach((line, i) => {
          if (physical.some((re) => re.test(line))) {
            offenders.push(`${rel}:${i + 1}: ${line.trim()}`);
          }
        });
      }
    }
    expect(offenders).toEqual([]);
  });
});
