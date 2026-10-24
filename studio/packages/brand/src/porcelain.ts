// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The Porcelain design tokens (studio-ui-language U2-U7, U13-U15; STU-010,
 * STU-016). Three layers (studio-development §4.1): the brand tokens of
 * `tokens.ts` and the primitives below, the semantic tokens components use,
 * and the scalar tokens (shape, elevation, space, type, motion). Every theme
 * is a column of the same table, so a missing value is a type error.
 * Brand tokens stay the source of the wordmark and accents (O-13).
 */

import { brandColors } from "./tokens.ts";

/** The three themes (U14, U15). */
export type ThemeName = "light" | "dark" | "contrast";

/** The themes in output order. */
export const themeNames: readonly ThemeName[] = ["light", "dark", "contrast"];

/** A token's value in each theme. */
export interface ThemedValues {
  readonly light: string;
  readonly dark: string;
  readonly contrast: string;
}

/** U2.2 primitive colour tokens (light theme). */
export const primitives = {
  "navy.900": "#11142A",
  "navy.700": "#3C4262",
  "navy.650": "#30364F",
  "navy.500": "#555C7C",
  "navy.400": "#7A81A3",
  "navy.alpha16": "rgba(17,20,42,.16)",
  "navy.alpha08": "rgba(17,20,42,.08)",
  "navy.alpha05": "rgba(17,20,42,.05)",
  "porcelain.100": "#F2F4F9",
  white: "#FFFFFF",
  "violet.700": "#4A2DE6",
  "violet.50": "#EEEAFE",
  "violet.halo": "rgba(91,61,245,.12)",
  "green.700": "#0B7A5A",
  "green.50": "#E2F4EC",
  "amber.800": "#8F5300",
  "amber.50": "#FFF1D6",
  "red.600": "#D92D20",
  "red.700": "#B42318",
  "red.50": "#FEECEB",
} as const;

const violet = brandColors["brand.violet"];
const surface = brandColors["brand.surface"];

/** One semantic colour: its UI-language name, its Tailwind key and its values. */
export interface SemanticColor extends ThemedValues {
  /** The name used in the UI language, e.g. `surface.canvas`. */
  readonly name: string;
  /** The Tailwind colour key, e.g. `canvas` makes `bg-canvas`. */
  readonly key: string;
}

const darkGlassText = "#B4BAD0";

/**
 * U2.3 semantic colours. The contrast column is U15: solid surfaces, text of
 * at least 7:1; values the UI language leaves open are chosen here and
 * proven by the tests.
 */
export const semanticColors: readonly SemanticColor[] = [
  {
    name: "surface.canvas",
    key: "canvas",
    light: primitives["porcelain.100"],
    dark: "#0B0D17",
    contrast: primitives.white,
  },
  { name: "surface.app", key: "app", light: surface.light, dark: "#0B0D17", contrast: primitives.white },
  { name: "surface.raised", key: "raised", light: primitives.white, dark: "#141726", contrast: primitives.white },
  {
    name: "surface.glass",
    key: "glass",
    light: "rgba(255,255,255,.66)",
    dark: "rgba(20,23,38,.80)",
    contrast: primitives.white,
  },
  {
    name: "surface.card",
    key: "card",
    light: "rgba(255,255,255,.70)",
    dark: "rgba(26,30,48,.80)",
    contrast: primitives.white,
  },
  {
    name: "surface.hover",
    key: "hover",
    light: primitives["navy.alpha05"],
    dark: "rgba(255,255,255,.06)",
    contrast: "#E6E8F0",
  },
  { name: "surface.selected", key: "selected", light: primitives.white, dark: "#22263A", contrast: "#E2DDFB" },
  {
    name: "text.primary",
    key: "primary",
    light: primitives["navy.900"],
    dark: "#F5F6FA",
    contrast: primitives["navy.900"],
  },
  {
    name: "text.secondary",
    key: "secondary",
    light: primitives["navy.700"],
    dark: darkGlassText,
    contrast: primitives["navy.900"],
  },
  {
    name: "text.secondaryOnGlass",
    key: "secondary-glass",
    light: primitives["navy.650"],
    dark: darkGlassText,
    contrast: primitives["navy.900"],
  },
  {
    name: "text.tertiary",
    key: "tertiary",
    light: primitives["navy.500"],
    dark: "#A3AAC2",
    contrast: primitives["navy.650"],
  },
  {
    name: "text.onAccent",
    key: "on-accent",
    light: primitives.white,
    dark: primitives["navy.900"],
    contrast: primitives.white,
  },
  {
    name: "border.hairline",
    key: "hairline",
    light: primitives["navy.alpha08"],
    dark: "rgba(255,255,255,.08)",
    contrast: primitives["navy.500"],
  },
  {
    name: "border.strong",
    key: "strong",
    light: primitives["navy.alpha16"],
    dark: "rgba(255,255,255,.16)",
    contrast: primitives["navy.900"],
  },
  { name: "accent.primary", key: "accent", light: violet.light, dark: violet.dark, contrast: primitives["violet.700"] },
  {
    name: "accent.primaryHover",
    key: "accent-hover",
    light: primitives["violet.700"],
    dark: "#7257FF",
    contrast: "#3A22B8",
  },
  {
    name: "accent.halo",
    key: "accent-halo",
    light: primitives["violet.halo"],
    dark: "rgba(139,116,255,.20)",
    contrast: "rgba(74,45,230,.28)",
  },
  {
    name: "accent.selection",
    key: "selection",
    light: violet.light,
    dark: violet.dark,
    contrast: primitives["violet.700"],
  },
  {
    name: "accent.selectionText",
    key: "selection-text",
    light: primitives["violet.700"],
    dark: "#C9BFFF",
    contrast: "#3A22B8",
  },
  {
    name: "accent.selectionSurface",
    key: "selection-surface",
    light: primitives["violet.50"],
    dark: "#2A2250",
    contrast: "#E2DDFB",
  },
  { name: "focus.ring", key: "focus", light: violet.light, dark: violet.dark, contrast: primitives["violet.700"] },
  { name: "status.success", key: "success", light: primitives["green.700"], dark: "#38D9A9", contrast: "#065F46" },
  {
    name: "status.success.surface",
    key: "success-surface",
    light: primitives["green.50"],
    dark: "rgba(56,217,169,.12)",
    contrast: primitives["green.50"],
  },
  { name: "status.warning", key: "warning", light: primitives["amber.800"], dark: "#FFC53D", contrast: "#6B3F00" },
  {
    name: "status.warning.surface",
    key: "warning-surface",
    light: primitives["amber.50"],
    dark: "rgba(255,197,61,.12)",
    contrast: primitives["amber.50"],
  },
  { name: "status.danger", key: "danger", light: primitives["red.700"], dark: "#F97066", contrast: "#8E1B10" },
  {
    name: "status.danger.surface",
    key: "danger-surface",
    light: primitives["red.50"],
    dark: "rgba(249,112,102,.12)",
    contrast: primitives["red.50"],
  },
  {
    name: "status.danger.fill",
    key: "danger-fill",
    light: primitives["red.600"],
    dark: "#F97066",
    contrast: "#8E1B10",
  },
  {
    name: "canvas.arrow",
    key: "arrow",
    light: primitives["navy.400"],
    dark: "#6B7390",
    contrast: primitives["navy.900"],
  },
  {
    name: "canvas.arrowActive",
    key: "arrow-active",
    light: violet.light,
    dark: violet.dark,
    contrast: primitives["violet.700"],
  },
];

/** A scalar token with a theme-dependent value (shadows, glass, strokes). */
export interface ThemedScalar extends ThemedValues {
  readonly name: string;
  readonly key: string;
}

/** U4 radii in px, keyed by the Tailwind `rounded-*` suffix. */
export const radii: Readonly<Record<string, { readonly name: string; readonly value: string }>> = {
  xs: { name: "radius.xs", value: "6px" },
  sm: { name: "radius.sm", value: "10px" },
  md: { name: "radius.md", value: "12px" },
  lg: { name: "radius.lg", value: "18px" },
  xl: { name: "radius.xl", value: "24px" },
  full: { name: "radius.full", value: "9999px" },
};

const darkEdge = "0 0 0 1px rgba(0,0,0,.40)";

/** U5 elevation; dark is the same geometry at double opacity plus the outer edge. */
export const elevations: readonly ThemedScalar[] = [
  {
    name: "elevation.1",
    key: "e1",
    light: "0 1px 2px rgba(17,20,42,.08), 0 2px 8px -2px rgba(17,20,42,.10)",
    dark: `0 1px 2px rgba(0,0,0,.16), 0 2px 8px -2px rgba(0,0,0,.20), ${darkEdge}`,
    contrast: "0 0 0 2px rgb(17,20,42)",
  },
  {
    name: "elevation.2",
    key: "e2",
    light: "0 1px 1px rgba(30,24,90,.04), 0 16px 40px -8px rgba(48,34,150,.14)",
    dark: `0 1px 1px rgba(0,0,0,.08), 0 16px 40px -8px rgba(0,0,0,.28), ${darkEdge}`,
    contrast: "0 0 0 2px rgb(17,20,42)",
  },
  {
    name: "elevation.frame",
    key: "frame",
    light: "0 0 0 1px rgba(17,20,42,.05), 0 24px 48px -16px rgba(48,34,150,.20)",
    dark: `0 0 0 1px rgba(0,0,0,.10), 0 24px 48px -16px rgba(0,0,0,.40), ${darkEdge}`,
    contrast: "0 0 0 2px rgb(17,20,42)",
  },
  {
    name: "elevation.frameActive",
    key: "frame-active",
    light:
      "0 0 0 1px rgba(17,20,42,.05), 0 24px 48px -16px rgba(48,34,150,.20), 0 0 0 7px rgba(91,61,245,.12), 0 0 0 1px #5B3DF5",
    dark: `0 0 0 1px rgba(0,0,0,.10), 0 24px 48px -16px rgba(0,0,0,.40), 0 0 0 7px rgba(139,116,255,.20), 0 0 0 1px #8B74FF`,
    contrast: "0 0 0 3px #4A2DE6",
  },
  {
    name: "elevation.primary",
    key: "primary",
    light: "0 6px 16px -6px rgba(91,61,245,.60)",
    dark: "0 6px 16px -6px rgba(139,116,255,.60)",
    contrast: "0 0 0 2px #11142A",
  },
  {
    name: "elevation.4",
    key: "e4",
    light: "0 24px 64px rgba(17,20,42,.18), 0 4px 12px rgba(17,20,42,.08)",
    dark: `0 24px 64px rgba(0,0,0,.36), 0 4px 12px rgba(0,0,0,.16), ${darkEdge}`,
    contrast: "0 0 0 2px rgb(17,20,42)",
  },
];

/** U3 glass and the stroke widths of U15; the contrast theme has no glass. */
export const surfaceScalars: readonly ThemedScalar[] = [
  {
    name: "glass.fill",
    key: "glass-fill",
    light: "rgba(255,255,255,.66)",
    dark: "rgba(20,23,38,.80)",
    contrast: "#FFFFFF",
  },
  {
    name: "glass.solid",
    key: "glass-solid",
    light: "rgba(255,255,255,.96)",
    dark: "rgba(20,23,38,.96)",
    contrast: "#FFFFFF",
  },
  {
    name: "glass.backdrop",
    key: "glass-backdrop",
    light: "blur(28px) saturate(180%)",
    dark: "blur(28px) saturate(140%)",
    contrast: "none",
  },
  {
    name: "glass.inner",
    key: "glass-inner",
    light: "rgba(255,255,255,.72)",
    dark: "rgba(255,255,255,.06)",
    contrast: "transparent",
  },
  {
    name: "glass.outer",
    key: "glass-outer",
    light: "rgba(17,20,42,.05)",
    dark: "rgba(0,0,0,.40)",
    contrast: "#11142A",
  },
  {
    name: "scrim.fill",
    key: "scrim-fill",
    light: "rgba(17,20,42,.24)",
    dark: "rgba(0,0,0,.48)",
    contrast: "rgba(17,20,42,.60)",
  },
  { name: "stroke.field", key: "stroke-field", light: "1px", dark: "1px", contrast: "2px" },
  { name: "stroke.focus", key: "stroke-focus", light: "2px", dark: "2px", contrast: "3px" },
  { name: "stroke.arrow", key: "stroke-arrow", light: "1.5px", dark: "1.5px", contrast: "2px" },
];

/** U6 spacing scale in px, keyed by the Tailwind suffix (`p-0.5`, `p-4`). */
export const space: Readonly<Record<string, string>> = {
  "0.5": "2px",
  "1": "4px",
  "2": "8px",
  "3": "12px",
  "4": "16px",
  "5": "20px",
  "6": "24px",
  "8": "32px",
  "10": "40px",
  "12": "48px",
  "16": "64px",
};

/** U6 density sets. */
export type DensityName = "compact" | "comfortable";

/** One density-dependent size. */
export interface DensityToken {
  readonly name: string;
  readonly key: string;
  readonly compact: string;
  readonly comfortable: string;
}

export const densityTokens: readonly DensityToken[] = [
  { name: "density.capsule", key: "capsule", compact: "48px", comfortable: "52px" },
  { name: "density.capsuleButton", key: "capsule-button", compact: "36px", comfortable: "40px" },
  { name: "density.field", key: "field", compact: "30px", comfortable: "36px" },
  { name: "density.tabTrack", key: "tab-track", compact: "28px", comfortable: "32px" },
  { name: "density.treeRow", key: "tree-row", compact: "30px", comfortable: "36px" },
  { name: "density.tableRow", key: "table-row", compact: "30px", comfortable: "36px" },
  { name: "density.cardPaddingY", key: "card-padding-y", compact: "12px", comfortable: "16px" },
  { name: "density.cardPaddingX", key: "card-padding-x", compact: "14px", comfortable: "18px" },
];

/** U13 motion; `ease.standard` is the only easing. */
export const motion = {
  durations: { fast: "140ms", base: "180ms", enter: "420ms", bundle: "220ms" },
  easings: { standard: "cubic-bezier(0.2, 0, 0, 1)" },
} as const;

/** U7 font families; the Arabic faces follow the Latin ones so each script falls through (M-1). */
export const fontFamilies = {
  sans: '"Instrument Sans", "Noto Sans Arabic", system-ui, sans-serif',
  serif: '"Instrument Serif", "Noto Naskh Arabic", serif',
  mono: '"DM Mono", ui-monospace, monospace',
  wordmark: '"Sora", system-ui, sans-serif',
} as const;

/** A U7 type role. */
export interface TypeRole {
  /** The Tailwind key: `text-<key>` sets size, line height and weight. */
  readonly key: string;
  readonly name: string;
  readonly family: keyof typeof fontFamilies;
  readonly size: number;
  readonly line: number;
  readonly weight: number;
}

/**
 * U7 roles. Ranges in the UI language take these defaults: label 12 / 16 at
 * 500, caption 12 / 16 at 400, tag 10 / 12 at 500 (the data role uses 400).
 */
export const typeRoles: readonly TypeRole[] = [
  { key: "screen", name: "type.screen", family: "serif", size: 26, line: 28, weight: 400 },
  { key: "title", name: "type.title", family: "serif", size: 24, line: 26, weight: 400 },
  { key: "plugin", name: "type.plugin", family: "serif", size: 19, line: 22, weight: 400 },
  { key: "section", name: "type.section", family: "serif", size: 17, line: 20, weight: 400 },
  { key: "body", name: "type.body", family: "sans", size: 13, line: 18, weight: 400 },
  { key: "label", name: "type.label", family: "sans", size: 12, line: 16, weight: 500 },
  { key: "caption", name: "type.caption", family: "sans", size: 12, line: 16, weight: 400 },
  { key: "data", name: "type.data", family: "mono", size: 11, line: 16, weight: 400 },
  { key: "tag", name: "type.tag", family: "mono", size: 10, line: 12, weight: 500 },
];

/** Arabic adds 2 px of line height at `type.body` and below (U7). */
export const arabicLineHeightExtra = 2;

/** Roles at body size and below, which get the Arabic line-height increase. */
export const arabicAdjustedRoles: ReadonlySet<string> = new Set(["body", "label", "caption", "data", "tag"]);

/** Number of distinct tokens a theme resolves, for reporting. */
export function tokenCount(): number {
  return (
    semanticColors.length +
    Object.keys(radii).length +
    elevations.length +
    surfaceScalars.length +
    Object.keys(space).length +
    densityTokens.length +
    Object.keys(motion.durations).length +
    Object.keys(motion.easings).length +
    typeRoles.length +
    Object.keys(fontFamilies).length
  );
}
