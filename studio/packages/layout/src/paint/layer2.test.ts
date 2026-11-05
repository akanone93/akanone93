// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed } from "../material-color/scheme.ts";
import { layer2Paint, layer2Parts } from "./layer2.ts";
import { argbToRgba, type PaintTheme } from "./paint.ts";

const scheme = colorSchemeFromSeed(0xff6750a4, false);
const theme: PaintTheme = { scheme };
const role = (r: keyof typeof scheme) => argbToRgba(scheme[r] as number);
const r = { x: 0, y: 0, w: 320, h: 180 };

describe("[STU-005] [WGT-020] Layer 2 components as plux_components builds them", () => {
  test("account and payment cards: a primary to secondary gradient, corners of 16, texts in onPrimary", () => {
    const paint = layer2Paint("AccountCard", r, theme);
    expect(paint?.boxes[0]).toMatchObject({
      radii: [16, 16, 16, 16],
      gradient: { kind: "linear", colors: [role("primary"), role("secondary")] },
    });
    expect(paint?.parts).toEqual({ title: role("onPrimary"), body: role("onPrimary") });
  });

  test("product cards are Material cards; promo banners primaryContainer; inputs underlined; states bare", () => {
    expect(layer2Paint("ProductCard", r, theme)?.boxes[0]).toMatchObject({ x: 4, fill: role("surfaceContainerLow") });
    expect(layer2Paint("PromoBanner", r, theme)?.parts.title).toBe(role("onPrimaryContainer"));
    expect(layer2Paint("IbanInput", r, theme)?.boxes[0]?.border).toEqual({
      kind: "sides",
      bottom: { color: role("onSurfaceVariant"), width: 1 },
    });
    expect(layer2Paint("EmptyState", r, theme)?.boxes).toEqual([]);
    expect(layer2Paint("Text", r, theme)).toBeUndefined();
    expect(layer2Parts(theme)).toEqual({ title: role("onSurface"), body: role("onSurfaceVariant") });
  });
});
