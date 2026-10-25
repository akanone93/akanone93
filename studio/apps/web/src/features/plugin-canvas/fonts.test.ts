// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";

const dir = new URL("./fonts/", import.meta.url);
const pinned = readFileSync(new URL("VERSION", dir), "utf-8")
  .split("\n")
  .filter((line) => line.startsWith("file "))
  .map((line) => line.split(" ").slice(1) as [string, string]);

describe("STU-006 canvas fonts", () => {
  test("four fonts are pinned", () => {
    expect(pinned.map(([name]) => name).sort()).toEqual([
      "NotoSans_400Regular.ttf",
      "NotoSans_700Bold.ttf",
      "Roboto_400Regular.ttf",
      "Roboto_700Bold.ttf",
    ]);
  });

  for (const [name, sha] of pinned) {
    test(`${name} matches its pinned SHA-256`, () => {
      expect(createHash("sha256").update(readFileSync(new URL(name, dir))).digest("hex")).toBe(sha);
    });
  }
});
