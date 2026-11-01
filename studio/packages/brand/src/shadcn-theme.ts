// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Studio's theme (ADR-0103, amended 2026-11-01): shadcn/ui's `zinc` base colour with its `indigo` theme, both from
 * shadcn-ui/ui `apps/v4/registry/themes.ts` (MIT): neutral surfaces, one indigo accent. On top, Studio sets the
 * shell's surfaces a clear step apart (the sidebar darkest, then the page, then the cards; in light and dark alike), so
 * the content area reads as a raised panel beside the sidebar, gives the current item and hovers an indigo wash (`accent`),
 * raises the dark cards a step above the page, and uses five solid Tailwind hues (indigo, violet, emerald, amber,
 * rose; 600 on light, 500 on dark) for charts and the small coloured marks. It adds the contrast theme of `STU-011` and
 * the status colours, which shadcn has no theme for.
 */

export type ShadcnVariable =
  | "background"
  | "foreground"
  | "card"
  | "card-foreground"
  | "popover"
  | "popover-foreground"
  | "primary"
  | "primary-foreground"
  | "secondary"
  | "secondary-foreground"
  | "muted"
  | "muted-foreground"
  | "accent"
  | "accent-foreground"
  | "destructive"
  | "border"
  | "input"
  | "ring"
  | "chart-1"
  | "chart-2"
  | "chart-3"
  | "chart-4"
  | "chart-5"
  | "sidebar"
  | "sidebar-foreground"
  | "sidebar-primary"
  | "sidebar-primary-foreground"
  | "sidebar-accent"
  | "sidebar-accent-foreground"
  | "sidebar-border"
  | "sidebar-ring";

export type ShadcnTheme = Readonly<Record<ShadcnVariable, string>>;

/** shadcn's corner radius for the theme; the radius scale is derived from it. */
export const shadcnRadius = "0.625rem";

export const shadcnLight: ShadcnTheme = {
  background: "oklch(0.985 0.001 286)",
  foreground: "oklch(0.141 0.005 285.823)",
  card: "oklch(1 0 0)",
  "card-foreground": "oklch(0.141 0.005 285.823)",
  popover: "oklch(1 0 0)",
  "popover-foreground": "oklch(0.141 0.005 285.823)",
  primary: "oklch(0.511 0.262 276.966)",
  "primary-foreground": "oklch(0.985 0 0)",
  secondary: "oklch(0.967 0.001 286.375)",
  "secondary-foreground": "oklch(0.21 0.006 285.885)",
  muted: "oklch(0.967 0.001 286.375)",
  "muted-foreground": "oklch(0.52 0.016 286)",
  accent: "oklch(0.962 0.018 272.314)",
  "accent-foreground": "oklch(0.398 0.195 277.366)",
  destructive: "oklch(0.577 0.245 27.325)",
  border: "oklch(0.92 0.004 286.32)",
  input: "oklch(0.92 0.004 286.32)",
  ring: "oklch(0.585 0.233 277.117)",
  "chart-1": "oklch(0.511 0.262 276.966)",
  "chart-2": "oklch(0.541 0.281 293.009)",
  "chart-3": "oklch(0.596 0.145 163.225)",
  "chart-4": "oklch(0.666 0.179 58.318)",
  "chart-5": "oklch(0.586 0.253 17.585)",
  sidebar: "oklch(0.948 0.004 286)",
  "sidebar-foreground": "oklch(0.141 0.005 285.823)",
  "sidebar-primary": "oklch(0.511 0.262 276.966)",
  "sidebar-primary-foreground": "oklch(0.985 0 0)",
  "sidebar-accent": "oklch(0.915 0.03 277)",
  "sidebar-accent-foreground": "oklch(0.398 0.195 277.366)",
  "sidebar-border": "oklch(0.89 0.005 286)",
  "sidebar-ring": "oklch(0.585 0.233 277.117)",
};

export const shadcnDark: ShadcnTheme = {
  background: "oklch(0.178 0.005 286)",
  foreground: "oklch(0.985 0 0)",
  card: "oklch(0.218 0.006 286)",
  "card-foreground": "oklch(0.985 0 0)",
  popover: "oklch(0.24 0.006 286)",
  "popover-foreground": "oklch(0.985 0 0)",
  primary: "oklch(0.511 0.262 276.966)",
  "primary-foreground": "oklch(0.985 0 0)",
  secondary: "oklch(0.29 0.006 286)",
  "secondary-foreground": "oklch(0.985 0 0)",
  muted: "oklch(0.29 0.006 286)",
  "muted-foreground": "oklch(0.705 0.015 286.067)",
  accent: "oklch(0.257 0.09 281.288)",
  "accent-foreground": "oklch(0.93 0.034 272.788)",
  destructive: "oklch(0.704 0.191 22.216)",
  border: "oklch(1 0 0 / 10%)",
  input: "oklch(1 0 0 / 15%)",
  ring: "oklch(0.585 0.233 277.117)",
  "chart-1": "oklch(0.585 0.233 277.117)",
  "chart-2": "oklch(0.606 0.25 292.717)",
  "chart-3": "oklch(0.696 0.17 162.48)",
  "chart-4": "oklch(0.769 0.188 70.08)",
  "chart-5": "oklch(0.645 0.246 16.439)",
  sidebar: "oklch(0.115 0.004 286)",
  "sidebar-foreground": "oklch(0.985 0 0)",
  "sidebar-primary": "oklch(0.511 0.262 276.966)",
  "sidebar-primary-foreground": "oklch(0.985 0 0)",
  "sidebar-accent": "oklch(0.257 0.09 281.288)",
  "sidebar-accent-foreground": "oklch(0.93 0.034 272.788)",
  "sidebar-border": "oklch(1 0 0 / 10%)",
  "sidebar-ring": "oklch(0.585 0.233 277.117)",
};

/**
 * The contrast theme (`STU-011`): the light theme with secondary text, borders and the focus ring darkened so each
 * reaches 7:1 or 3:1 against white. Only the values that differ from light are listed.
 */
export const shadcnContrast: Partial<Record<ShadcnVariable, string>> = {
  background: "oklch(1 0 0)",
  sidebar: "oklch(1 0 0)",
  "accent-foreground": "oklch(0 0 0)",
  "sidebar-accent-foreground": "oklch(0 0 0)",
  foreground: "oklch(0 0 0)",
  "card-foreground": "oklch(0 0 0)",
  "popover-foreground": "oklch(0 0 0)",
  primary: "oklch(0 0 0)",
  "muted-foreground": "oklch(0.35 0 0)",
  border: "oklch(0.45 0 0)",
  input: "oklch(0.45 0 0)",
  ring: "oklch(0 0 0)",
  "sidebar-foreground": "oklch(0 0 0)",
  "sidebar-border": "oklch(0.45 0 0)",
};
