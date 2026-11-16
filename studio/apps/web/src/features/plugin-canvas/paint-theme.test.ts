// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { argbToRgba, colorSchemeFromSeed } from "@plux/layout";
import {
  brandsOf,
  dimensionsOf,
  paintThemeOf,
  schemeOf,
  textTokensOf,
  themeTokens,
  tokenValue,
  typographyOf,
} from "./paint-theme.ts";

const hex = (digits: string) => `#${digits}`;
const isObjectish = (v: unknown) => typeof v === "object" && v !== null;
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
    // `brand.<name>` is an ordinary group: the device reads overlays from `brands` only, and so does the canvas.
    expect(brandsOf(tokens)).toEqual(["acme"]);
    expect(tokenValue(tokens, "color.primary", false, "zen")).toBe(hex("6750A4"));
  });

  test("aliases inside a composite are followed and a loop reads as nothing, as on the device", () => {
    const t = {
      font: { base: { $type: "fontFamily", $value: ["Inter"] } },
      size: { body: { $type: "dimension", $value: 16 } },
      typography: {
        bodyLarge: { $type: "typography", $value: { fontFamily: "{font.base}", fontSize: "{size.body}" } },
      },
      loop: { a: { $value: "{loop.b}" }, b: { $value: "{loop.a}" } },
      wide: { $value: Object.fromEntries(Array.from({ length: 20 }, (_, i) => [`k${i}`, "{wide}"])) },
    };
    expect(tokenValue(t, "typography.bodyLarge", false, "")).toEqual({ fontFamily: ["Inter"], fontSize: 16 });
    expect(tokenValue(t, "loop.a", false, "")).toBeUndefined();
    expect(isObjectish(tokenValue(t, "wide", false, ""))).toBe(true);
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

  test("[THM-004] typography tokens replace the type scale's roles, in pixels or multiples", () => {
    const type = {
      typography: {
        bodyMedium: { $value: { fontFamily: ["Inter", "sans-serif"], fontSize: "15px", lineHeight: 1.4 } },
        titleLarge: { $value: { fontSize: { value: 20, unit: "px" }, fontWeight: 600, lineHeight: "28px" } },
      },
    };
    const scale = typographyOf(type, false, "");
    expect(scale.bodyMedium).toMatchObject({ size: 15, height: 1.4, family: "Inter", weight: 400 });
    // The runtime takes a line height as a multiple only; a dimension leaves Material's.
    expect(scale.titleLarge).toMatchObject({ size: 20, weight: 600, height: 1.27 });
    expect(scale.labelSmall).toBeUndefined();
    const named = { type: { $type: "typography", hero: { $value: { fontSize: "32px", fontWeight: "semi-bold" } } } };
    expect(textTokensOf(named, false, "")).toEqual({ "type.hero": { size: 32, weight: 600 } });
    const space = {
      space: { $type: "dimension", md: { $value: { value: 12, unit: "px" } }, lg: { $value: "1.5rem" } },
    };
    expect(dimensionsOf({ ...space, ...tokens }, false, "")).toEqual({ "space.md": 12 });
  });
});

describe("PC-35 the theme as an edit session holds it [THM-001]", () => {
  test("a document's unsaved copy wins over its saved bytes, so a theme edit is drawn at once", () => {
    const saved = { kind: "theme", tokens: { color: { primary: { $value: hex("6750A4") } } } };
    const edited = { kind: "theme", tokens: { color: { primary: { $value: hex("00897B") } } } };
    const bytes = new TextEncoder().encode(JSON.stringify(saved));
    expect(themeTokens([{ kind: "theme", content: bytes }])).toEqual(saved.tokens);
    expect(themeTokens([{ kind: "theme", content: bytes, parsed: edited }])).toEqual(edited.tokens);
  });
});
