// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** What a `PhoneInput`'s region button shows on the canvas (build log row 295). */

import { callingCodes } from "./calling-codes.gen.ts";

/** The region of a locale tag such as `de-DE` (its first two-letter upper-case part after the language), or undefined. */
function regionOf(tag: string | undefined): string | undefined {
  return tag
    ?.split(/[-_]/)
    .slice(1)
    .find((p) => /^[A-Z]{2}$/.test(p));
}

/**
 * What a `PhoneInput`'s region button shows: the region and its calling code, as the runtime's "🇩🇪 +49" with the
 * region's code where the device draws its flag (the canvas has no flag glyphs); "+" when no region is known. The region
 * is the document's `region`, else the locale's, as `plux_components` picks it (build log row 295).
 */
export function phoneRegionLabel(region: string | undefined, locale: string | undefined): string {
  const r = (region ?? regionOf(locale))?.toUpperCase();
  const code = r === undefined ? undefined : callingCodes[r];
  return r === undefined || code === undefined ? "+" : `${r} +${code}`;
}
