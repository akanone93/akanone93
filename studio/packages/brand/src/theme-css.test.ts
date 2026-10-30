// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";

import { shadcnDark, shadcnLight } from "./shadcn-theme.ts";
import { themeCss } from "./theme-css.ts";
import { brandColors } from "./tokens.ts";

describe("themeCss [STU-010] [STU-016]", () => {
  const css = themeCss();

  test("declares shadcn's neutral theme in light and dark, unchanged", () => {
    for (const [name, value] of Object.entries(shadcnLight)) {
      expect(css).toContain(`  --${name}: ${value};`);
      expect(css).toContain(`  --color-${name}: var(--${name});`);
    }
    const dark = css.slice(css.indexOf("[data-theme=dark] {"));
    for (const [name, value] of Object.entries(shadcnDark)) {
      expect(dark).toContain(`  --${name}: ${value};`);
    }
  });

  test("keeps Tailwind's defaults and derives the radius scale from --radius, as shadcn does", () => {
    expect(css).not.toContain("--*: initial;");
    expect(css).toContain("  --radius: 0.625rem;");
    expect(css).toContain("  --radius-lg: var(--radius);");
    expect(css).toContain("  --font-sans: 'Inter Variable', 'Noto Sans Arabic', sans-serif;");
  });

  test("declares every brand token in both themes", () => {
    for (const [name, color] of Object.entries(brandColors)) {
      const variable = `--color-${name.replaceAll(".", "-")}`;
      expect(css).toContain(`  ${variable}: ${color.light};`);
      if (color.dark !== color.light) {
        expect(css).toContain(`  ${variable}: ${color.dark};`);
      }
    }
  });

  test("older Studio names follow shadcn's variables and never redefine shadcn's own", () => {
    expect(css).toContain("  --color-raised: var(--card);");
    expect(css).toContain("  --color-tertiary: var(--muted-foreground);");
    for (const name of ["primary", "secondary", "accent", "card"]) {
      expect(css.match(new RegExp(`--color-${name}:`, "g"))?.length).toBe(1);
    }
  });

  test("wires the dark variant to the data-theme attribute", () => {
    expect(css).toContain("@custom-variant dark (&:where([data-theme=dark], [data-theme=dark] *));");
    expect(css).toContain("[data-theme=dark] {");
    expect(css).toContain("[data-theme=contrast] {");
  });

  test("is deterministic", () => {
    expect(themeCss()).toBe(themeCss());
  });
});
