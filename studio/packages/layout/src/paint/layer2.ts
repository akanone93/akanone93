// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * What the Layer 2 components paint (STU-005, WGT-020), as the runtime's `plux_components` builds them from Material
 * 3: the account and payment cards' `primary` to `secondary` gradient, the product and live-ETA cards' Material card,
 * the promo banner in `primaryContainer`, the skeleton's `surfaceContainerHighest` blocks, the inputs' underline; and
 * the colours of the texts each lays out itself (its parts): titles and the rest.
 */

import type { ColorRole } from "../material-color/scheme.ts";
import { type BoxPaint, elevationShadows, type PaintTheme, type Rect, type Rgba, roleColor } from "./paint.ts";

/** What a Layer 2 component paints: its boxes, and the colours of its texts (titles of 16 pixels and more, the rest). */
export interface Layer2Paint {
  readonly boxes: readonly BoxPaint[];
  readonly parts: { readonly title: Rgba; readonly body: Rgba };
}

const corners = (r: number): readonly [number, number, number, number] => [r, r, r, r];

/** A Material 3 card: inside a 4 pixel margin, corners of 12, `surfaceContainerLow` at elevation 1. */
function card(r: Rect, theme: PaintTheme): BoxPaint {
  return {
    x: r.x + 4,
    y: r.y + 4,
    w: Math.max(0, r.w - 8),
    h: Math.max(0, r.h - 8),
    radii: corners(12),
    fill: roleColor(theme, "surfaceContainerLow"),
    shadows: elevationShadows(1, roleColor(theme, "shadow")),
  };
}

const CARDS: ReadonlySet<string> = new Set(["ProductCard", "LiveEtaCard", "CartSummary"]);
const GRADIENT_CARDS: ReadonlySet<string> = new Set(["AccountCard", "PaymentCard"]);
const INPUTS: ReadonlySet<string> = new Set([
  "AmountInput",
  "IbanInput",
  "PhoneInput",
  "SecureTextField",
  "SearchField",
  "CountryPicker",
  "CurrencyPicker",
]);
const STATES: ReadonlySet<string> = new Set(["EmptyState", "ErrorState"]);

function colours(theme: PaintTheme, title: ColorRole, body: ColorRole): Layer2Paint["parts"] {
  return { title: roleColor(theme, title), body: roleColor(theme, body) };
}

/** The boxes and text colours of a Layer 2 component of `type`, or undefined for a type that is not one. */
export function layer2Paint(type: string, r: Rect, theme: PaintTheme): Layer2Paint | undefined {
  if (GRADIENT_CARDS.has(type)) {
    const gradient = {
      kind: "linear" as const,
      colors: [roleColor(theme, "primary"), roleColor(theme, "secondary")],
      from: [r.x, r.y] as const,
      to: [r.x + r.w, r.y + r.h] as const,
      radius: 0,
    };
    return { boxes: [{ ...r, radii: corners(16), gradient }], parts: colours(theme, "onPrimary", "onPrimary") };
  }
  if (CARDS.has(type)) {
    return { boxes: [card(r, theme)], parts: colours(theme, "onSurface", "onSurfaceVariant") };
  }
  if (type === "PromoBanner") {
    return {
      boxes: [{ ...r, radii: corners(16), fill: roleColor(theme, "primaryContainer") }],
      parts: colours(theme, "onPrimaryContainer", "onPrimaryContainer"),
    };
  }
  if (type === "SkeletonLoader") {
    return {
      boxes: [{ ...r, radii: corners(4), fill: roleColor(theme, "surfaceContainerHighest") }],
      parts: colours(theme, "onSurface", "onSurfaceVariant"),
    };
  }
  if (INPUTS.has(type)) {
    const bottom = { color: roleColor(theme, "onSurfaceVariant"), width: 1 };
    return {
      boxes: [{ ...r, border: { kind: "sides", bottom } }],
      parts: colours(theme, "onSurface", "onSurfaceVariant"),
    };
  }
  if (STATES.has(type)) {
    return { boxes: [], parts: colours(theme, "onSurface", "onSurfaceVariant") };
  }
  return undefined;
}

/** The text colours of a Layer 2 component that paints nothing of its own: the page's. */
export function layer2Parts(theme: PaintTheme): Layer2Paint["parts"] {
  return colours(theme, "onSurface", "onSurfaceVariant");
}
