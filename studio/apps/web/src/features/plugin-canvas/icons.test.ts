// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { codePointsOf, ICON_FAMILIES, ICON_FONTS, loadIconNames, mirroredOf } from "./icons.ts";

const fromFile = (url: string) => readFile(fileURLToPath(url), "utf-8");

describe("[THM-005] the canvas's icon sets", () => {
  test("name lists read as code points by name; the mirrored list as set and name", () => {
    expect(codePointsOf("home e9b2\nsearch ef7a\n")).toEqual({ home: 0xe9b2, search: 0xef7a });
    expect(mirroredOf("cupertino back\nmaterial arrow_back\n\n")).toEqual(["cupertino back", "material arrow_back"]);
  });

  test("the names load once, with the families of each set's fonts", async () => {
    const names = await loadIconNames(fromFile);
    expect(names.material["home"]).toBe(0xe9b2);
    expect(names.cupertino["heart"]).toBeGreaterThan(0xf000);
    expect(names.mirrored).toContain("material arrow_back");
    expect(names.families).toEqual(ICON_FAMILIES);
    expect(await loadIconNames(fromFile)).toBe(names);
    expect(ICON_FONTS.map((f) => f.family)).toEqual([...ICON_FAMILIES.material, ...ICON_FAMILIES.cupertino]);
  });
});
