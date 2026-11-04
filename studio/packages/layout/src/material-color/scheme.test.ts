// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { type ColorRole, colorRoleNames, colorSchemeFromSeed } from "./scheme.ts";
import vectors from "./scheme-vectors.json";

const hex = (argb: number) => `#${(argb >>> 0).toString(16).padStart(8, "0")}`;

describe("[THM-001] [THM-002] Material colour scheme from a seed, as the device builds it", () => {
  test("every role of every vector matches material_color_utilities 0.13.0 exactly", () => {
    expect(vectors.length).toBeGreaterThan(20);
    for (const v of vectors) {
      const scheme = colorSchemeFromSeed(Number.parseInt(v.seed.slice(1), 16), v.dark);
      const roles = v.roles as Record<ColorRole, string>;
      for (const role of colorRoleNames) {
        expect(`${v.seed} ${v.dark ? "dark" : "light"} ${role} ${hex(scheme[role])}`).toBe(
          `${v.seed} ${v.dark ? "dark" : "light"} ${role} ${roles[role]}`,
        );
      }
    }
  });

  test("the scheme says whether it is light or dark", () => {
    expect(colorSchemeFromSeed(0xff6750a4, false).brightness).toBe("light");
    expect(colorSchemeFromSeed(0xff6750a4, true).brightness).toBe("dark");
  });
});
