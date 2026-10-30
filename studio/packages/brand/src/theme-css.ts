// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Generates Studio's Tailwind v4 theme (ADR-0103): shadcn/ui's stock `neutral` theme for the `base-nova` style, mapped
 * the way `shadcn init` maps it, with Tailwind's own defaults left in place. Older Studio token names that do not clash
 * with shadcn's stay available as aliases of shadcn's variables, so screens not yet rebuilt on shadcn follow its look.
 */

import {
  arabicAdjustedRoles,
  arabicLineHeightExtra,
  densityTokens,
  dialogWidths,
  iconSizes,
  motion,
  semanticColors,
  typeRoles,
} from "./porcelain.ts";
import { type ShadcnVariable, shadcnContrast, shadcnDark, shadcnLight, shadcnRadius } from "./shadcn-theme.ts";
import { brandColors } from "./tokens.ts";

/** shadcn's font: Inter, as `@fontsource-variable/inter` names it; Arabic falls through to Noto Sans Arabic. */
const fontSans = "'Inter Variable', 'Noto Sans Arabic', sans-serif";

/** Studio colours shadcn has no variable for; they keep their measured light, dark and contrast values. */
const keptColors: ReadonlySet<string> = new Set([
  "selection",
  "selection-text",
  "selection-surface",
  "success",
  "success-surface",
  "warning",
  "warning-surface",
  "danger-surface",
  "arrow",
  "arrow-active",
  "hero-liquid-a",
  "hero-liquid-b",
  "hero-liquid-c",
  "sample-loan",
  "sample-shop",
  "sample-bank",
  "sample-delivery",
  "sample-fresh",
  "sample-on-app",
]);

/** Older Studio colour names, now aliases of shadcn's variables. */
const colorAliases: Readonly<Record<string, string>> = {
  app: "var(--background)",
  canvas: "var(--muted)",
  raised: "var(--card)",
  glass: "var(--popover)",
  hover: "var(--accent)",
  selected: "var(--accent)",
  tertiary: "var(--muted-foreground)",
  "secondary-glass": "var(--muted-foreground)",
  "on-accent": "var(--primary-foreground)",
  "accent-hover": "color-mix(in oklch, var(--primary) 90%, var(--background))",
  "accent-halo": "color-mix(in oklch, var(--ring) 50%, transparent)",
  hairline: "var(--border)",
  strong: "var(--input)",
  focus: "var(--ring)",
  link: "var(--primary)",
  danger: "var(--destructive)",
  "danger-fill": "var(--destructive)",
  "button-text": "var(--foreground)",
};

/** Older Studio shadow names, as Tailwind's default shadows. */
const shadowAliases: Readonly<Record<string, string>> = {
  e1: "0 1px 2px 0 rgb(0 0 0 / 0.05)",
  soft: "0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1)",
  e2: "0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)",
  frame: "0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)",
  "frame-active": "0 0 0 2px var(--ring)",
  primary: "0 1px 2px 0 rgb(0 0 0 / 0.05)",
  float: "0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)",
  e4: "0 20px 25px -5px rgb(0 0 0 / 0.1), 0 8px 10px -6px rgb(0 0 0 / 0.1)",
};

/** Glass, kept for the canvas chrome: a neutral popover surface with a light blur. */
const glass: Readonly<Record<string, string>> = {
  "glass-fill": "color-mix(in oklch, var(--popover) 88%, transparent)",
  "glass-solid": "var(--popover)",
  "glass-backdrop": "blur(12px)",
  "glass-inner": "var(--border)",
  "glass-outer": "transparent",
  "glass-deep-backdrop": "blur(16px)",
  "glass-sheen": "none",
  "scrim-fill": "oklch(0 0 0 / 40%)",
  "stroke-field": "1px",
  "stroke-focus": "2px",
  "stroke-arrow": "1.5px",
};

const shadcnVariables = Object.keys(shadcnLight) as ShadcnVariable[];

function themeVars(values: Partial<Record<ShadcnVariable, string>>): string[] {
  return shadcnVariables.flatMap((v) => (values[v] === undefined ? [] : [`  --${v}: ${values[v]};`]));
}

/** The kept Studio colours of one theme; dark and contrast list only what differs from light. */
function keptColorVars(theme: "light" | "dark" | "contrast"): string[] {
  const brand = Object.entries(brandColors).flatMap(([key, value]) => {
    const v = theme === "dark" ? value.dark : value.light;
    return theme !== "light" && v === value.light ? [] : [`  --color-${key.replaceAll(".", "-")}: ${v};`];
  });
  const kept = semanticColors
    .filter((c) => keptColors.has(c.key))
    .flatMap((c) => {
      const v = theme === "contrast" ? c.contrast : c[theme];
      return theme !== "light" && v === c.light ? [] : [`  --color-${c.key}: ${v};`];
    });
  return [...brand, ...kept];
}

function typeRoleVars(): string[] {
  return typeRoles.flatMap((role) => [
    `  --text-${role.key}: ${role.size}px;`,
    `  --text-${role.key}--line-height: ${role.line}px;`,
    `  --text-${role.key}--font-weight: ${role.weight};`,
    ...(role.tracking === undefined ? [] : [`  --text-${role.key}--letter-spacing: ${role.tracking}em;`]),
  ]);
}

function densityDeclarations(density: "compact" | "comfortable"): string[] {
  return densityTokens.map((t) => `  --density-${t.key}: ${t[density]};`);
}

function arabicDeclarations(): string[] {
  const lineHeights = typeRoles
    .filter((role) => arabicAdjustedRoles.has(role.key))
    .map((role) => `  --text-${role.key}--line-height: ${role.line + arabicLineHeightExtra}px;`);
  const tracking = typeRoles
    .filter((role) => role.tracking !== undefined)
    .map((role) => `  --text-${role.key}--letter-spacing: 0em;`);
  return [...lineHeights, ...tracking];
}

const forcedColors: Readonly<Partial<Record<ShadcnVariable, string>>> = {
  background: "Canvas",
  foreground: "CanvasText",
  card: "Canvas",
  "card-foreground": "CanvasText",
  popover: "Canvas",
  "popover-foreground": "CanvasText",
  primary: "ButtonText",
  "primary-foreground": "ButtonFace",
  "muted-foreground": "CanvasText",
  accent: "Highlight",
  "accent-foreground": "HighlightText",
  border: "CanvasText",
  input: "CanvasText",
  ring: "Highlight",
};

/**
 * Returns the stylesheet text. The output is a pure function of the tokens: the same tokens give the same bytes.
 */
export function themeCss(): string {
  return [
    "/* SPDX-FileCopyrightText: 2026 Plux contributors */",
    "/* SPDX-License-Identifier: AGPL-3.0-only */",
    "/* Generated by studio/scripts/gen-theme.ts from @plux/brand. Do not edit. */",
    "",
    "/* shadcn's dark variant, keyed to the theme Studio writes on <html data-theme> (STU-014). */",
    "@custom-variant dark (&:where([data-theme=dark], [data-theme=dark] *));",
    "",
    "/* shadcn/ui: the theme mapping `shadcn init` writes (ADR-0103). */",
    "@theme inline {",
    `  --font-sans: ${fontSans};`,
    "  --font-heading: var(--font-sans);",
    ...shadcnVariables.map((v) => `  --color-${v}: var(--${v});`),
    "  --radius-sm: calc(var(--radius) * 0.6);",
    "  --radius-md: calc(var(--radius) * 0.8);",
    "  --radius-lg: var(--radius);",
    "  --radius-xl: calc(var(--radius) * 1.4);",
    "  --radius-2xl: calc(var(--radius) * 1.8);",
    "  --radius-3xl: calc(var(--radius) * 2.2);",
    "  --radius-4xl: calc(var(--radius) * 2.6);",
    "}",
    "",
    "/* Older Studio names, as aliases of shadcn's variables; one family per Tailwind namespace. */",
    "@theme inline {",
    `  --font-serif: ${fontSans};`,
    `  --font-wordmark: ${fontSans};`,
    ...Object.entries(colorAliases).map(([k, v]) => `  --color-${k}: ${v};`),
    ...Object.entries(shadowAliases).map(([k, v]) => `  --shadow-${k}: ${v};`),
    ...Object.entries(glass).map(([k, v]) => `  --surface-${k}: ${v};`),
    "}",
    "",
    "/* Studio values shadcn has no token for: status, selection, arrows, the sample apps and the type roles. */",
    "@theme static {",
    ...keptColorVars("light"),
    ...typeRoleVars(),
    ...Object.entries(iconSizes).map(([k, v]) => `  --spacing-${k}: ${v};`),
    ...Object.entries(dialogWidths).map(([k, v]) => `  --container-${k}: ${v};`),
    ...Object.entries(motion.durations).map(([k, v]) => `  --duration-${k}: ${v};`),
    ...Object.entries(motion.easings).map(([k, v]) => `  --ease-${k}: ${v};`),
    "}",
    "",
    ":root {",
    `  --radius: ${shadcnRadius};`,
    ...themeVars(shadcnLight),
    ...densityDeclarations("compact"),
    "}",
    "",
    "[data-density=comfortable] {",
    ...densityDeclarations("comfortable"),
    "}",
    "",
    "/* The contrast theme applies on request of the system; an explicit theme below wins (STU-011). */",
    "@media (prefers-contrast: more) {",
    "  :root {",
    ...themeVars(shadcnContrast).map((d) => `  ${d}`),
    ...keptColorVars("contrast").map((d) => `  ${d}`),
    "  }",
    "}",
    "",
    "/* Forced colours use system colours; forced-color-adjust stays auto. */",
    "@media (forced-colors: active) {",
    "  :root {",
    ...themeVars(forcedColors).map((d) => `  ${d}`),
    "  }",
    "}",
    "",
    "[data-theme=dark] {",
    ...themeVars(shadcnDark),
    ...keptColorVars("dark"),
    "}",
    "",
    "[data-theme=contrast] {",
    ...themeVars(shadcnContrast),
    ...keptColorVars("contrast"),
    "}",
    "",
    ":lang(ar) {",
    ...arabicDeclarations(),
    "}",
    "",
    "@utility glass {",
    "  background-color: var(--surface-glass-fill);",
    "  backdrop-filter: var(--surface-glass-backdrop);",
    "  box-shadow:",
    "    0 0 0 1px var(--surface-glass-inner),",
    "    var(--shadow-e2);",
    "}",
    "",
    "@utility glass-deep {",
    "  background-color: var(--surface-glass-fill);",
    "  backdrop-filter: var(--surface-glass-deep-backdrop);",
    "  box-shadow:",
    "    0 0 0 1px var(--surface-glass-inner),",
    "    var(--shadow-float);",
    "}",
    "",
    "@media (prefers-reduced-transparency: reduce) {",
    "  :root {",
    "    --surface-glass-fill: var(--popover);",
    "    --surface-glass-backdrop: none;",
    "    --surface-glass-deep-backdrop: none;",
    "  }",
    "}",
    "",
    "/* Every animation becomes an instant change. */",
    "@media (prefers-reduced-motion: reduce) {",
    "  :root {",
    ...Object.keys(motion.durations).map((name) => `    --duration-${name}: 0ms;`),
    "  }",
    "}",
    "",
  ].join("\n");
}
