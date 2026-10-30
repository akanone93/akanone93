// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Studio's theme: shadcn/ui's stock `neutral` theme, as `shadcn init` writes it for the `base-nova` style (ADR-0103).
 * The light and dark values are shadcn's own, copied unchanged from shadcn-ui/ui `apps/v4/registry/themes.ts` (MIT).
 * Studio adds only what shadcn has no theme for: the contrast theme of `STU-011` and the status colours.
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
  background: "oklch(1 0 0)",
  foreground: "oklch(0.145 0 0)",
  card: "oklch(1 0 0)",
  "card-foreground": "oklch(0.145 0 0)",
  popover: "oklch(1 0 0)",
  "popover-foreground": "oklch(0.145 0 0)",
  primary: "oklch(0.205 0 0)",
  "primary-foreground": "oklch(0.985 0 0)",
  secondary: "oklch(0.97 0 0)",
  "secondary-foreground": "oklch(0.205 0 0)",
  muted: "oklch(0.97 0 0)",
  "muted-foreground": "oklch(0.556 0 0)",
  accent: "oklch(0.97 0 0)",
  "accent-foreground": "oklch(0.205 0 0)",
  destructive: "oklch(0.577 0.245 27.325)",
  border: "oklch(0.922 0 0)",
  input: "oklch(0.922 0 0)",
  ring: "oklch(0.708 0 0)",
  "chart-1": "oklch(0.87 0 0)",
  "chart-2": "oklch(0.556 0 0)",
  "chart-3": "oklch(0.439 0 0)",
  "chart-4": "oklch(0.371 0 0)",
  "chart-5": "oklch(0.269 0 0)",
  sidebar: "oklch(0.985 0 0)",
  "sidebar-foreground": "oklch(0.145 0 0)",
  "sidebar-primary": "oklch(0.205 0 0)",
  "sidebar-primary-foreground": "oklch(0.985 0 0)",
  "sidebar-accent": "oklch(0.97 0 0)",
  "sidebar-accent-foreground": "oklch(0.205 0 0)",
  "sidebar-border": "oklch(0.922 0 0)",
  "sidebar-ring": "oklch(0.708 0 0)",
};

export const shadcnDark: ShadcnTheme = {
  background: "oklch(0.145 0 0)",
  foreground: "oklch(0.985 0 0)",
  card: "oklch(0.205 0 0)",
  "card-foreground": "oklch(0.985 0 0)",
  popover: "oklch(0.205 0 0)",
  "popover-foreground": "oklch(0.985 0 0)",
  primary: "oklch(0.922 0 0)",
  "primary-foreground": "oklch(0.205 0 0)",
  secondary: "oklch(0.269 0 0)",
  "secondary-foreground": "oklch(0.985 0 0)",
  muted: "oklch(0.269 0 0)",
  "muted-foreground": "oklch(0.708 0 0)",
  accent: "oklch(0.269 0 0)",
  "accent-foreground": "oklch(0.985 0 0)",
  destructive: "oklch(0.704 0.191 22.216)",
  border: "oklch(1 0 0 / 10%)",
  input: "oklch(1 0 0 / 15%)",
  ring: "oklch(0.556 0 0)",
  "chart-1": "oklch(0.87 0 0)",
  "chart-2": "oklch(0.556 0 0)",
  "chart-3": "oklch(0.439 0 0)",
  "chart-4": "oklch(0.371 0 0)",
  "chart-5": "oklch(0.269 0 0)",
  sidebar: "oklch(0.205 0 0)",
  "sidebar-foreground": "oklch(0.985 0 0)",
  "sidebar-primary": "oklch(0.488 0.243 264.376)",
  "sidebar-primary-foreground": "oklch(0.985 0 0)",
  "sidebar-accent": "oklch(0.269 0 0)",
  "sidebar-accent-foreground": "oklch(0.985 0 0)",
  "sidebar-border": "oklch(1 0 0 / 10%)",
  "sidebar-ring": "oklch(0.556 0 0)",
};

/**
 * The contrast theme (`STU-011`): the light theme with secondary text, borders and the focus ring darkened so each
 * reaches 7:1 or 3:1 against white. Only the values that differ from light are listed.
 */
export const shadcnContrast: Partial<Record<ShadcnVariable, string>> = {
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
