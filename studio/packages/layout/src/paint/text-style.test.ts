// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { colorSchemeFromSeed } from "../material-color/scheme.ts";
import { argbToRgba } from "./paint.ts";
import { M3_TYPE_SCALE, resolveTextStyle, typeScaleOf, weightOf } from "./text-style.ts";

const theme = { scheme: colorSchemeFromSeed(0xff6750a4, false) };

describe("[STU-006] [THM-004] text styles as Flutter resolves them", () => {
  test("the type scale is Material 3's (englishLike2021 sizes, M3 line heights and tracking)", () => {
    expect(M3_TYPE_SCALE.displayLarge).toEqual({ size: 57, weight: 400, height: 1.12, letterSpacing: -0.25 });
    expect(M3_TYPE_SCALE.titleMedium).toEqual({ size: 16, weight: 500, height: 1.5, letterSpacing: 0.15 });
    expect(M3_TYPE_SCALE.labelSmall).toEqual({ size: 11, weight: 500, height: 1.45, letterSpacing: 0.5 });
  });

  test("a text with no style is bodyMedium; a token picks its role; fields override", () => {
    expect(resolveTextStyle(undefined, theme)).toEqual({ ...M3_TYPE_SCALE.bodyMedium, italic: false });
    expect(resolveTextStyle({ $token: "typography.headlineSmall" }, theme).size).toBe(24);
    const custom = resolveTextStyle(
      { $token: "typography.titleLarge", fontWeight: "w700", fontStyle: "italic", decoration: "underline" },
      theme,
    );
    expect(custom).toMatchObject({ size: 22, weight: 700, italic: true, decoration: "underline" });
  });

  test("a colour from the scheme, and the theme's own typography replace Material's", () => {
    expect(resolveTextStyle({ color: { $token: "color.primary" } }, theme).color).toBe(
      argbToRgba(theme.scheme.primary),
    );
    const own = { ...theme, typography: { bodyMedium: { size: 15, weight: 400, height: 1.4, letterSpacing: 0 } } };
    expect(typeScaleOf(own).bodyMedium.size).toBe(15);
    expect(resolveTextStyle(undefined, own).size).toBe(15);
    const named = { ...theme, textTokens: { "type.headline": { size: 24, weight: 600 } } };
    expect(resolveTextStyle({ $token: "type.headline" }, named)).toMatchObject({ size: 24, weight: 600, height: 1.43 });
  });

  test("weights by Flutter name or number, clamped to the hundreds", () => {
    expect(weightOf("bold")).toBe(700);
    expect(weightOf("semi-bold")).toBe(600);
    expect(weightOf("w300")).toBe(300);
    expect(weightOf(651)).toBe(700);
    expect(weightOf(2000)).toBe(900);
    expect(weightOf("heavy")).toBe(900);
    expect(weightOf("loud")).toBeUndefined();
  });
});
