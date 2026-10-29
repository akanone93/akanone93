// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { availableLocales } from "./locales.ts";

export type Direction = "ltr" | "rtl";

/** Reading direction of a locale; unknown tags are resolved by their language subtag. */
export function directionOf(tag: string): Direction {
  const all = availableLocales(true);
  const exact = all.find((l) => l.tag.toLowerCase() === tag.toLowerCase());
  if (exact !== undefined) {
    return exact.direction;
  }
  const language = tag.split("-")[0]?.toLowerCase();
  return all.find((l) => l.tag === language)?.direction ?? "ltr";
}
