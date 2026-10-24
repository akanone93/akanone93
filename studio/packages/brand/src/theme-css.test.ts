// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";

import { themeCss } from "./theme-css.ts";
import { brandColors } from "./tokens.ts";

describe("themeCss [STU-016]", () => {
  const css = themeCss();

  test("resets every Tailwind default before declaring tokens", () => {
    const reset = css.indexOf("--*: initial;");
    expect(reset).toBeGreaterThan(css.indexOf("@theme"));
    expect(reset).toBeLessThan(css.indexOf("--color-brand-ink"));
  });

  test("declares every brand token in both themes", () => {
    for (const [name, color] of Object.entries(brandColors)) {
      const variable = `--color-${name.replaceAll(".", "-")}`;
      expect(css).toContain(`  ${variable}: ${color.light};`);
      expect(css).toContain(`  ${variable}: ${color.dark};`);
    }
  });

  test("wires the dark variant to the data-theme attribute", () => {
    expect(css).toContain("@custom-variant dark (&:where([data-theme=dark], [data-theme=dark] *));");
    expect(css).toContain("[data-theme=dark] {");
  });

  test("is deterministic", () => {
    expect(themeCss()).toBe(themeCss());
  });

  test("contains no default palette names", () => {
    expect(css).not.toMatch(/--color-(red|blue|green|gray|slate|zinc|neutral|stone|indigo|violet)-\d{2,3}/);
    expect(css).not.toContain("--spacing");
    expect(css).not.toContain("--font-sans");
  });
});
