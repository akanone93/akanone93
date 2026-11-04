// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { iconFiles, iconSources } from "./gen-icons.ts";

const root = new URL("../../", import.meta.url);
const dir = new URL("studio/apps/web/src/features/plugin-canvas/icons/", root);

describe("[THM-005] the canvas icon fonts", () => {
  test("are the server's pinned fonts at their default instance, byte for byte as the script makes them", async () => {
    const files = iconFiles(await iconSources(root));
    expect(readdirSync(dir).sort()).toEqual([...files.keys()].sort());
    for (const [name, data] of files) {
      const committed = readFileSync(new URL(name, dir));
      expect(typeof data === "string" ? committed.toString("utf-8") : new Uint8Array(committed)).toEqual(
        typeof data === "string" ? data : new Uint8Array(data),
      );
    }
  });

  test("every font stays under the repository's 500 KB file limit", () => {
    for (const name of readdirSync(dir).filter((n) => n.endsWith(".ttf"))) {
      expect(readFileSync(new URL(name, dir)).length).toBeLessThan(500 * 1024);
    }
  });
});
