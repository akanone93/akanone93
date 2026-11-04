// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { argbToRgba, colorSchemeFromSeed } from "@plux/layout";
import { brandsOf, paintThemeOf, schemeOf, tokenValue } from "./paint-theme.ts";

const hex = (digits: string) => `#${digits}`;
const tokens = {
  brands: { acme: { color: { $type: "color", primary: { $value: hex("FF5722") } } } },
  brand: { zen: { color: { primary: { $value: hex("00897B") } } } },
  color: {
    $type: "color",
    primary: { $value: hex("6750A4"), $extensions: { "dev.plux.dark": hex("D0BCFF") } },
    onSurface: { $value: hex("1D1B20") },
    accent: { $value: "{color.primary}" },
  },
};

describe("[THM-001] [THM-002] [THM-003] the theme's colours as the runtime reads them", () => {
  test("a token's dark value, a brand's value and aliases are followed", () => {
    expect(tokenValue(tokens, "color.primary", false, "")).toBe(hex("6750A4"));
    expect(tokenValue(tokens, "color.primary", true, "")).toBe(hex("D0BCFF"));
    expect(tokenValue(tokens, "color.primary", false, "acme")).toBe(hex("FF5722"));
    expect(tokenValue(tokens, "color.accent", true, "")).toBe(hex("D0BCFF"));
    expect(brandsOf(tokens)).toEqual(["acme", "zen"]);
  });

  test("the scheme grows from color.primary and every color.<role> token replaces its role", () => {
    const light = schemeOf(tokens, false, "");
    expect(light.primary).toBe(0xff6750a4);
    expect(light.secondary).toBe(colorSchemeFromSeed(0xff6750a4, false).secondary);
    expect(light.onSurface).toBe(0xff1d1b20);
    expect(schemeOf(tokens, false, "acme").primary).toBe(0xffff5722);
    expect(schemeOf(tokens, false, "acme").tertiary).toBe(colorSchemeFromSeed(0xffff5722, false).tertiary);
    expect(schemeOf({}, true, "").brightness).toBe("dark");
  });

  test("the paint theme carries the theme's other colour tokens, resolved", () => {
    const paint = paintThemeOf(tokens, false, "");
    expect(paint.tokens?.["color.accent"]).toBe(argbToRgba(0xff6750a4));
  });
});
