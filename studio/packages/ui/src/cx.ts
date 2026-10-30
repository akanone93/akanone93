// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Joins class names, skipping falsy entries. Tailwind utilities here are token-backed only (studio-development 4.2). */
export function cx(...parts: readonly (string | false | null | undefined)[]): string {
  return parts.filter((p): p is string => typeof p === "string" && p !== "").join(" ");
}

/** Visible focus ring from the focus token (STU-011). */
export const focusRing = "outline-none focus-visible:ring-3 focus-visible:ring-ring/50";
