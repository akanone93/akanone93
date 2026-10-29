// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { pseudoCatalogue, pseudoCompiled } from "./pseudo.ts";

describe("pseudo-locales of compiled messages [I18N-010] [STU-012]", () => {
  test("en-XA accents the literal parts around a placeholder and keeps the placeholder", () => {
    const out = pseudoCompiled("en-XA", ["Hello ", ["name"], "!"]);
    expect(Array.isArray(out)).toBe(true);
    const parts = out as unknown[];
    expect(parts[0]).toBe("[");
    expect(parts[2]).toEqual(["name"]);
    expect(String(parts[1])).not.toBe("Hello ");
    expect(String(parts[parts.length - 1])).toMatch(/^~+\]$/);
  });

  test("en-XA reaches into the branches of a plural form", () => {
    const out = pseudoCompiled("en-XA", [["count", "plural", { one: ["#", " item"], other: ["#", " items"] }]]);
    const plural = (out as unknown[])[1] as [string, string, { one: unknown[]; other: unknown[] }];
    expect(plural[0]).toBe("count");
    expect(plural[2].one[0]).toBe("#");
    expect(plural[2].one[1]).not.toBe(" item");
  });

  test("ar-XB wraps the whole message right-to-left and marks every literal part", () => {
    const out = pseudoCompiled("ar-XB", ["Open ", ["file"]]) as unknown[];
    expect(out[0]).toBe("‫");
    expect(out[out.length - 1]).toBe("‬");
    expect(String(out[1]).startsWith("‏")).toBe(true);
    expect(out[2]).toEqual(["file"]);
  });

  test("plain strings keep working and every message of the catalogue is derived", () => {
    const catalogue = pseudoCatalogue("en-XA", { a: "Save", b: ["Hi ", ["n"]] });
    const { a, b } = catalogue;
    expect(a).toMatch(/^\[.+~+\]$/);
    expect(Array.isArray(b)).toBe(true);
  });
});
