// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Whether the page is dark: the theme Studio applied to the document (STU-014), else the system
 * preference. Third-party widgets such as Monaco follow this, not the system preference alone, so a
 * person who chose "light" on a dark system does not get a dark editor on a light page (STU-011).
 */
export function isDark(): boolean {
  const applied = globalThis.document?.documentElement.getAttribute("data-theme");
  if (applied !== null && applied !== undefined) {
    return applied === "dark";
  }
  return globalThis.matchMedia?.("(prefers-color-scheme: dark)").matches === true;
}
