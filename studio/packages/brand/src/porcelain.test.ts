// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";

import { contrastRatio, isHexColor, minimumComponentContrast, minimumTextContrast } from "./contrast.ts";
import { primitives, semanticColors, type ThemeName, themeNames, typeRoles } from "./porcelain.ts";
import { themeCss } from "./theme-css.ts";

type Color = Readonly<Record<ThemeName, string>>;

function semantic(name: string): Color {
  const found = semanticColors.find((c) => c.name === name);
  if (!found) {
    throw new Error(`unknown semantic token ${name}`);
  }
  return found;
}

/** A text/background pair of semantic tokens and its minimum ratio. */
interface Pair {
  readonly fg: string;
  readonly bg: string;
  readonly min: number;
}

const solidSurfaces = ["surface.canvas", "surface.app", "surface.raised", "surface.selected"];
const text = minimumTextContrast;
const component = (theme: ThemeName) => (theme === "contrast" ? 4.5 : minimumComponentContrast);

/** Pairs every theme must satisfy: text on solid surfaces, labels, status, UI parts. */
const pairs = (theme: ThemeName): Pair[] => {
  const textMin = theme === "contrast" ? 7 : text;
  const onSolid = solidSurfaces.flatMap((bg) =>
    ["text.primary", "text.secondary", "text.tertiary"].map((fg) => ({ fg, bg, min: textMin })),
  );
  return [
    ...onSolid,
    { fg: "text.onAccent", bg: "accent.primary", min: theme === "contrast" ? 7 : text },
    { fg: "accent.selectionText", bg: "accent.selectionSurface", min: textMin },
    { fg: "status.success", bg: "surface.raised", min: textMin },
    { fg: "status.warning", bg: "surface.raised", min: textMin },
    { fg: "status.danger", bg: "surface.raised", min: textMin },
    { fg: "focus.ring", bg: "surface.canvas", min: component(theme) },
    { fg: "canvas.arrow", bg: "surface.canvas", min: component(theme) },
    { fg: "canvas.arrowActive", bg: "surface.canvas", min: component(theme) },
    { fg: "accent.primary", bg: "surface.canvas", min: component(theme) },
  ];
};

/** U2.5, light theme: the measured pairs, as primitives with their stated ratios. */
const measured: readonly { fg: string; bg: string; ratio: number }[] = [
  { fg: "navy.900", bg: "white", ratio: 18.15 },
  { fg: "navy.900", bg: "porcelain.100", ratio: 16.49 },
  { fg: "navy.700", bg: "white", ratio: 9.78 },
  { fg: "navy.500", bg: "white", ratio: 6.55 },
  { fg: "navy.500", bg: "porcelain.100", ratio: 5.95 },
  { fg: "navy.400", bg: "porcelain.100", ratio: 3.47 },
  { fg: "violet.700", bg: "violet.50", ratio: 6.35 },
  { fg: "green.700", bg: "green.50", ratio: 4.66 },
  { fg: "green.700", bg: "white", ratio: 5.32 },
  { fg: "amber.800", bg: "amber.50", ratio: 5.52 },
];

describe("Porcelain U2.5 measured pairs [STU-010]", () => {
  for (const { fg, bg, ratio } of measured) {
    test(`${fg} on ${bg} is ${ratio}:1`, () => {
      const f = primitives[fg as keyof typeof primitives];
      const b = primitives[bg as keyof typeof primitives];
      expect(contrastRatio(f, b)).toBeCloseTo(ratio, 1);
    });
  }

  test("white on brand.violet is 6.12:1", () => {
    expect(contrastRatio(primitives.white, semantic("accent.primary").light)).toBeCloseTo(6.12, 1);
  });

  test("text pairs meet 4.5:1 and arrows meet 3:1", () => {
    for (const { fg, bg, ratio } of measured) {
      const need = fg === "navy.400" ? component("light") : text;
      expect(ratio, `${fg} on ${bg}`).toBeGreaterThanOrEqual(need);
    }
  });
});

for (const theme of themeNames) {
  describe(`Porcelain ${theme} theme contrast [STU-010]`, () => {
    test("every colour a pair uses is a hex colour", () => {
      for (const { fg, bg } of pairs(theme)) {
        expect(isHexColor(semantic(fg)[theme]), `${fg} ${theme}`).toBe(true);
        expect(isHexColor(semantic(bg)[theme]), `${bg} ${theme}`).toBe(true);
      }
    });

    test("every text and component pair meets its ratio", () => {
      for (const { fg, bg, min } of pairs(theme)) {
        const ratio = contrastRatio(semantic(fg)[theme], semantic(bg)[theme]);
        expect(ratio, `${fg} on ${bg} (${theme})`).toBeGreaterThanOrEqual(min);
      }
    });
  });
}

describe("Porcelain tokens [STU-010, STU-016]", () => {
  test("every semantic token has a unique name and key", () => {
    expect(new Set(semanticColors.map((c) => c.name)).size).toBe(semanticColors.length);
    expect(new Set(semanticColors.map((c) => c.key)).size).toBe(semanticColors.length);
  });

  test("serif roles never run below 17 px (U7)", () => {
    for (const role of typeRoles.filter((r) => r.family === "serif")) {
      expect(role.size).toBeGreaterThanOrEqual(17);
    }
  });

  test("the stylesheet carries every theme, density and the reduced-motion rule", () => {
    const css = themeCss();
    for (const marker of [
      "[data-theme=dark] {",
      "[data-theme=contrast] {",
      "@media (prefers-contrast: more)",
      "@media (forced-colors: active)",
      "Highlight",
      "[data-density=comfortable] {",
      "@media (prefers-reduced-motion: reduce)",
      "--duration-fast: 0ms;",
      "@utility glass {",
    ]) {
      expect(css).toContain(marker);
    }
    for (const c of semanticColors) {
      expect(css).toContain(`--color-${c.key}: `);
    }
  });
});
