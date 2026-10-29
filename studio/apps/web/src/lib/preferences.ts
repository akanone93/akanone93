// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { directionOf } from "@plux/i18n";

export type ThemePreference = "system" | "light" | "dark" | "contrast";
export type DensityPreference = "comfortable" | "compact";

export const themes: readonly ThemePreference[] = ["system", "light", "dark", "contrast"];
export const densities: readonly DensityPreference[] = ["comfortable", "compact"];

export function asTheme(value: string): ThemePreference {
  return themes.find((t) => t === value) ?? "system";
}

export function asDensity(value: string): DensityPreference {
  return densities.find((d) => d === value) ?? "comfortable";
}

/** Writes the stored choices to the document: `data-theme`, `data-density`, `lang` and `dir` (STU-014). */
export function applyPreferences(
  root: HTMLElement,
  prefs: { theme: ThemePreference; density: DensityPreference; locale: string },
  prefersDark: boolean,
): void {
  root.setAttribute("data-theme", prefs.theme === "system" ? (prefersDark ? "dark" : "light") : prefs.theme);
  root.setAttribute("data-density", prefs.density);
  root.setAttribute("lang", prefs.locale);
  root.setAttribute("dir", directionOf(prefs.locale));
}
