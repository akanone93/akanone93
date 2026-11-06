// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ScreenStyle } from "@plux/canvas";
import {
  BASELINE_SEED,
  type ColorRole,
  type ColorScheme,
  colorRoleNames,
  colorSchemeFromSeed,
  M3_TYPE_SCALE,
  type PaintTheme,
  parseHexColor,
  type Rgba,
  type TypeRole,
  type TypeSpec,
  typeRoles,
  weightOf,
  withColorRoles,
} from "@plux/layout";
import { useMemo } from "react";
import { parseDocument } from "./plugin-model.ts";

/**
 * The colours the canvas paints screens with (STU-005, THM-001–THM-003), read from the app's theme document as the
 * runtime reads it (`packages/plux_flutter/lib/src/render/tokens.dart`, `theme.dart`): a token at a dotted path in the
 * W3C token tree, its dark value from `$extensions["dev.plux.dark"]` in dark mode, a brand's value from
 * `brands.<brand>.<path>` first, `{a.b}` aliases followed; then the Material 3 scheme from the `color.primary` seed
 * (Flutter's baseline seed without one), with every `color.<role>` token replacing its role.
 */

interface DocLike {
  readonly kind: string;
  readonly content: Uint8Array;
  /** The document as an edit session holds it, unsaved edits included; absent for the server's copy. */
  readonly parsed?: unknown;
}

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/**
 * The token tree of the app's theme: the theme the app document names, else the only theme there is — as an edit
 * session holds it when one has it (`parsed`), so the canvas draws a theme edit at once (PC-35).
 */
export function themeTokens(documents: readonly DocLike[]): Obj {
  const docs = documents.map((d) => (d.parsed !== undefined ? d.parsed : parseDocument(d.content))).filter(isObj);
  const app = docs.find((d) => d["kind"] === "app");
  const themes = docs.filter((d) => d["kind"] === "theme");
  const ref = typeof app?.["theme"] === "string" ? app["theme"] : undefined;
  const theme =
    themes.find((t) => t["id"] === ref || t["key"] === ref) ?? (themes.length === 1 ? themes[0] : undefined);
  return isObj(theme?.["tokens"]) ? theme["tokens"] : {};
}

/** The brand overlays of the app's theme, for the preview's brand select (THM-003). */
export function useBrands(documents: readonly DocLike[] | undefined): readonly string[] {
  return useMemo(() => brandsOf(themeTokens(documents ?? [])), [documents]);
}

/** The brand overlays of the theme (`brands.<name>`, or `brand.<name>` as the Design area writes them). */
export function brandsOf(tokens: Obj): string[] {
  const names = (group: unknown) => (isObj(group) ? Object.keys(group).filter((k) => !k.startsWith("$")) : []);
  return [...new Set([...names(tokens["brands"]), ...names(tokens["brand"])])].sort();
}

/** The token object at a dotted path, or undefined. */
function tokenAt(tokens: Obj, path: string): Obj | undefined {
  let node: unknown = tokens;
  for (const part of path.split(".")) {
    node = isObj(node) ? node[part] : undefined;
  }
  return isObj(node) && "$value" in node ? node : undefined;
}

/** The raw value of the token at `path` for `dark` and `brand`, aliases followed; undefined when there is none. */
export function tokenValue(tokens: Obj, path: string, dark: boolean, brand: string, depth = 0): unknown {
  if (depth > 8) {
    return undefined;
  }
  const token =
    (brand === ""
      ? undefined
      : (tokenAt(tokens, `brands.${brand}.${path}`) ?? tokenAt(tokens, `brand.${brand}.${path}`))) ??
    tokenAt(tokens, path);
  if (token === undefined) {
    return undefined;
  }
  const ext = isObj(token["$extensions"]) ? token["$extensions"] : {};
  const nested = isObj(ext["dev"]) && isObj(ext["dev"]["plux"]) ? ext["dev"]["plux"]["dark"] : undefined;
  const darkValue = ext["dev.plux.dark"] ?? nested;
  const value = dark && darkValue !== undefined ? darkValue : token["$value"];
  if (typeof value === "string" && value.startsWith("{") && value.endsWith("}")) {
    return tokenValue(tokens, value.slice(1, -1), dark, brand, depth + 1);
  }
  return value;
}

/** RGBA as ARGB. */
const rgbaToArgb = (rgba: number): number => ((rgba >>> 8) | ((rgba & 0xff) << 24)) >>> 0;

function colorToken(tokens: Obj, path: string, dark: boolean, brand: string): Rgba | undefined {
  const v = tokenValue(tokens, path, dark, brand);
  return typeof v === "string" ? parseHexColor(v) : undefined;
}

/** Every token path of the tree outside the brand and contrast groups. */
function tokenPaths(node: unknown, prefix = ""): string[] {
  if (!isObj(node)) {
    return [];
  }
  if ("$value" in node) {
    return [prefix];
  }
  return Object.entries(node).flatMap(([k, v]) =>
    k.startsWith("$") || (prefix === "" && (k === "brands" || k === "brand" || k === "contrast"))
      ? []
      : tokenPaths(v, prefix === "" ? k : `${prefix}.${k}`),
  );
}

/** The Material 3 colour scheme of the theme, as the device builds it, for `dark` and `brand`. */
export function schemeOf(tokens: Obj, dark: boolean, brand: string): ColorScheme {
  const seed = colorToken(tokens, "color.primary", dark, brand);
  const base = colorSchemeFromSeed(seed === undefined ? BASELINE_SEED : rgbaToArgb(seed), dark);
  const overrides: Partial<Record<ColorRole, number>> = {};
  for (const role of colorRoleNames) {
    const c = colorToken(tokens, `color.${role}`, dark, brand);
    if (c !== undefined) {
      overrides[role] = rgbaToArgb(c);
    }
  }
  return withColorRoles(base, overrides);
}

const dimension = (v: unknown): number | undefined => {
  if (typeof v === "number") {
    return v;
  }
  if (typeof v === "string") {
    // As the runtime's `double.tryParse` after dropping a px, dp or pt unit.
    const digits = v.trim().replace(/(px|dp|pt)$/, "");
    const n = digits === "" ? Number.NaN : Number(digits);
    return Number.isFinite(n) ? n : undefined;
  }
  if (isObj(v) && typeof v["value"] === "number") {
    return v["unit"] === "rem" ? v["value"] * 16 : v["value"];
  }
  return undefined;
};

/**
 * The theme's `typography.<role>` tokens (W3C typography composites) as type specs: a role the theme defines replaces
 * Material 3's metrics of that role (the runtime's `textTheme.merge`); a field it leaves out keeps Material's.
 */
export function typographyOf(tokens: Obj, dark: boolean, brand: string): Partial<Record<TypeRole, TypeSpec>> {
  const out: Partial<Record<TypeRole, TypeSpec>> = {};
  for (const role of typeRoles) {
    const v = tokenValue(tokens, `typography.${role}`, dark, brand);
    if (isObj(v)) {
      out[role] = { ...M3_TYPE_SCALE[role], ...typeFieldsOf(v) };
    }
  }
  return out;
}

const TYPE_FIELDS = ["fontFamily", "fontSize", "fontWeight", "letterSpacing", "lineHeight"];

/**
 * The fields a W3C typography composite sets, as the runtime reads them (`tokens.dart` `_typography`): the first
 * family, dimensions in pixels (a rem is 16), a weight by number or name, and a line height as a multiple only.
 */
function typeFieldsOf(v: Obj): Partial<TypeSpec> {
  const families = Array.isArray(v["fontFamily"]) ? v["fontFamily"] : [v["fontFamily"]];
  const family = typeof families[0] === "string" ? families[0] : undefined;
  const size = dimension(v["fontSize"]);
  const weight = weightOf(v["fontWeight"]);
  const letterSpacing = dimension(v["letterSpacing"]);
  const height = v["lineHeight"];
  return {
    ...(family === undefined ? {} : { family }),
    ...(size === undefined ? {} : { size }),
    ...(weight === undefined ? {} : { weight }),
    ...(letterSpacing === undefined ? {} : { letterSpacing }),
    ...(typeof height === "number" ? { height } : {}),
  };
}

/** Every typography token of the theme by path, with the fields it sets (a text style bound to one, THM-004). */
export function textTokensOf(tokens: Obj, dark: boolean, brand: string): Record<string, Partial<TypeSpec>> {
  const out: Record<string, Partial<TypeSpec>> = {};
  for (const path of tokenPaths(tokens)) {
    const v = tokenValue(tokens, path, dark, brand);
    if (isObj(v) && TYPE_FIELDS.some((f) => f in v)) {
      out[path] = typeFieldsOf(v);
    }
  }
  return out;
}

/** Every dimension token of the theme by path, in logical pixels (a padding or a size bound to `space.md`). */
export function dimensionsOf(tokens: Obj, dark: boolean, brand: string): Record<string, number> {
  const out: Record<string, number> = {};
  for (const path of tokenPaths(tokens)) {
    const v = tokenValue(tokens, path, dark, brand);
    const n = typeof v === "string" && v.startsWith("#") ? undefined : dimension(v);
    if (n !== undefined) {
      out[path] = n;
    }
  }
  return out;
}

/** What the canvas paints screens with: the scheme and the theme's other colour tokens, resolved. */
export function paintThemeOf(
  tokens: Obj,
  dark: boolean,
  brand: string,
  platform: "material" | "cupertino" = "material",
): PaintTheme {
  const colors: Record<string, Rgba> = {};
  for (const path of tokenPaths(tokens)) {
    const c = colorToken(tokens, path, dark, brand);
    if (c !== undefined) {
      colors[path] = c;
    }
  }
  return {
    scheme: schemeOf(tokens, dark, brand),
    tokens: colors,
    typography: typographyOf(tokens, dark, brand),
    textTokens: textTokensOf(tokens, dark, brand),
    dimensions: dimensionsOf(tokens, dark, brand),
    platform,
  };
}

/**
 * The canvas style of screens painted with `paint`: the device's screen in the scheme's surface, text in `onSurface`,
 * widgets not painted yet as neutral boxes in the surface containers, and content beyond the frame faded.
 */
export function paintedStyle(base: ScreenStyle, paint: PaintTheme): ScreenStyle {
  const s = paint.scheme;
  return {
    ...base,
    paint,
    frame: roleRgba(s, "surface"),
    text: roleRgba(s, "onSurface"),
    box: roleRgba(s, "surfaceContainerHighest"),
    boxStroke: roleRgba(s, "outlineVariant"),
    placeholder: roleRgba(s, "primary", 0x1f),
    overflowBox: roleRgba(s, "surfaceContainerHighest", 0x66),
    overflowText: roleRgba(s, "onSurface", 0x80),
  };
}

/** A scheme role as the canvas's RGBA, with `alpha` (0–255) when given. */
export function roleRgba(scheme: ColorScheme, role: ColorRole, alpha = 0xff): Rgba {
  const argb = scheme[role];
  return (((argb << 8) & 0xffffff00) | alpha) >>> 0;
}
