// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Preferences } from "@plux/api";
import { useEffect } from "react";
import { applyPreferences, asDensity, asTheme } from "../../lib/preferences.ts";
import { useApp } from "../../lib/use-app.tsx";

/** Applies stored preferences to the document whenever they change (STU-014); also used by the shell, so the choice holds on every page. */
export function useAppliedPreferences(preferences: Preferences | undefined): void {
  const { studio } = useApp();
  const loaded = preferences !== undefined;
  const theme = preferences?.theme ?? "";
  const density = preferences?.density ?? "";
  const language = preferences?.language ?? "";
  useEffect(() => {
    if (!loaded) {
      return;
    }
    const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    void (async () => {
      const locale = language === "" ? studio.locale() : await studio.activate(language);
      applyPreferences(
        document.documentElement,
        { theme: asTheme(theme), density: asDensity(density), locale },
        prefersDark,
      );
    })();
  }, [loaded, theme, density, language, studio]);
}
