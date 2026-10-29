// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const rules = ["no-inner-html", "no-raw-values", "logical-properties", "no-literal-ui-strings", "no-waitfor-tobenull"];
const studio = resolve(import.meta.dir, "..");

// Each fixture is linted inside a scratch project whose path matches the
// override scope (apps/web/src), with only the rule under test loaded.
function diagnostics(rule: string, kind: "good" | "bad"): number {
  const dir = mkdtempSync(join(tmpdir(), "plux-lint-"));
  try {
    mkdirSync(join(dir, "apps/web/src"), { recursive: true });
    copyFileSync(join(studio, "lint/fixtures", `${rule}.${kind}.tsx`), join(dir, "apps/web/src/f.tsx"));
    writeFileSync(
      join(dir, "biome.json"),
      JSON.stringify({ linter: { rules: { recommended: false } }, plugins: [join(studio, "lint", `${rule}.grit`)] }),
    );
    const p = Bun.spawnSync(
      [
        join(studio, "node_modules/.bin/biome"),
        "lint",
        "--colors=off",
        "--max-diagnostics=100",
        `--config-path=${dir}`,
        "apps/web/src/f.tsx",
      ],
      { cwd: dir, env: { ...process.env, PATH: process.env.PATH ?? "" } },
    );
    const out = p.stdout.toString() + p.stderr.toString();
    return (out.match(/plugin/g) ?? []).length;
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

for (const rule of rules) {
  test(`${rule}: bad fixture is reported`, () => {
    expect(diagnostics(rule, "bad")).toBeGreaterThanOrEqual(1);
  });
  test(`${rule}: good fixture is clean`, () => {
    expect(diagnostics(rule, "good")).toBe(0);
  });
}
