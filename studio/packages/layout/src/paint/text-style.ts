// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The text style a widget's text is laid out and drawn in (STU-005, STU-006, THM-001): Material 3's type scale
 * (Flutter's `Typography.englishLike2021` sizes with the M3 line heights and tracking), replaced role by role by the
 * theme's `typography.<role>` tokens, then the widget's own `style` — a type-scale token, or the fields of a
 * `TextStyle`. One style serves the layout's measuring and the canvas's drawing, so a line breaks in the same place.
 */

import { m3Metrics } from "../flutter/material/theme.ts";
import type { PaintTheme, Rgba } from "./paint.ts";
import { resolveColor } from "./paint.ts";

export const typeRoles = [
  "displayLarge",
  "displayMedium",
  "displaySmall",
  "headlineLarge",
  "headlineMedium",
  "headlineSmall",
  "titleLarge",
  "titleMedium",
  "titleSmall",
  "bodyLarge",
  "bodyMedium",
  "bodySmall",
  "labelLarge",
  "labelMedium",
  "labelSmall",
] as const;

export type TypeRole = (typeof typeRoles)[number];

/** A text style's measured fields: size, weight (100–900), line height as a multiple of the size, tracking. */
export interface TypeSpec {
  readonly size: number;
  readonly weight: number;
  /** The line height as a multiple of the font size (Flutter's `height`). */
  readonly height: number;
  readonly letterSpacing: number;
  readonly family?: string;
}

const spec = (role: TypeRole): TypeSpec => {
  const [size, weight, letterSpacing, height] = m3Metrics(role);
  return { size, weight, height, letterSpacing };
};

/** Material 3's type scale (Flutter's `Typography.englishLike2021` with the M3 metrics): each role's metrics. */
export const M3_TYPE_SCALE: Readonly<Record<TypeRole, TypeSpec>> = Object.fromEntries(
  typeRoles.map((r) => [r, spec(r)]),
) as Record<TypeRole, TypeSpec>;

/** How a text is drawn: its measured fields and what only painting needs. */
export interface ResolvedText extends TypeSpec {
  readonly italic: boolean;
  readonly color?: Rgba;
  readonly decoration?: "underline" | "lineThrough" | "overline";
}

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

/** Weight names: Flutter's (`bold`), and the W3C token names the runtime reads (`semi-bold`, `heavy`). */
const WEIGHT_NAMES: Readonly<Record<string, number>> = {
  thin: 100,
  hairline: 100,
  "extra-light": 200,
  "ultra-light": 200,
  light: 300,
  normal: 400,
  regular: 400,
  book: 400,
  medium: 500,
  "semi-bold": 600,
  "demi-bold": 600,
  bold: 700,
  "extra-bold": 800,
  "ultra-bold": 800,
  black: 900,
  heavy: 900,
};

/** A weight as a document writes it (`w600`), a token names it (`semi-bold`) or a number, rounded to the hundreds. */
export function weightOf(v: unknown): number | undefined {
  const n =
    typeof v === "number" ? v : typeof v === "string" ? (WEIGHT_NAMES[v] ?? Number(v.replace(/^w/, ""))) : Number.NaN;
  return Number.isFinite(n) ? Math.min(900, Math.max(100, Math.round(n / 100) * 100)) : undefined;
}

/** The type scale of the theme: Material 3's, with each role the theme's typography tokens define. */
export function typeScaleOf(theme: PaintTheme | undefined): Readonly<Record<TypeRole, TypeSpec>> {
  return theme?.typography === undefined ? M3_TYPE_SCALE : { ...M3_TYPE_SCALE, ...theme.typography };
}

/** The type-scale role a `typography.<role>` token path names. */
export const typeRoleOf = (path: string): TypeRole | undefined => {
  const name = path.startsWith("typography.") ? path.slice("typography.".length) : "";
  return (typeRoles as readonly string[]).includes(name) ? (name as TypeRole) : undefined;
};

/**
 * The style of a text whose `style` prop is `value`, over `base` (the default text style where it sits: Material's
 * `bodyMedium`). A type-scale token replaces the base; a `TextStyle` object replaces the fields it sets, and may name
 * its own type-scale token to start from.
 */
export function resolveTextStyle(value: unknown, theme: PaintTheme | undefined, base?: ResolvedText): ResolvedText {
  const scale = typeScaleOf(theme);
  const start: ResolvedText = base ?? { ...scale.bodyMedium, italic: false };
  if (!isObj(value)) {
    return start;
  }
  const from = typeof value["$token"] === "string" ? fromToken(value["$token"], start, scale, theme) : start;
  return { ...measuredOf(value, from), ...paintedOf(value, from, theme) };
}

/**
 * A text style bound to a token: a type-scale role's full style (the runtime reads it from the text theme), or the
 * fields another typography token sets over the default style.
 */
function fromToken(
  path: string,
  start: ResolvedText,
  scale: Readonly<Record<TypeRole, TypeSpec>>,
  theme: PaintTheme | undefined,
): ResolvedText {
  const role = typeRoleOf(path);
  if (role !== undefined) {
    return { ...start, ...scale[role] };
  }
  return { ...start, ...(theme?.textTokens?.[path] ?? {}) };
}

function measuredOf(value: Obj, from: ResolvedText): TypeSpec {
  const family = typeof value["fontFamily"] === "string" ? value["fontFamily"] : from.family;
  return {
    size: num(value["fontSize"]) ?? from.size,
    weight: weightOf(value["fontWeight"]) ?? from.weight,
    height: num(value["height"]) ?? from.height,
    letterSpacing: num(value["letterSpacing"]) ?? from.letterSpacing,
    ...(family === undefined ? {} : { family }),
  };
}

function paintedOf(value: Obj, from: ResolvedText, theme: PaintTheme | undefined): Omit<ResolvedText, keyof TypeSpec> {
  const color = (theme === undefined ? undefined : resolveColor(value["color"], theme)) ?? from.color;
  const decoration = decorationOf(value["decoration"]) ?? from.decoration;
  return {
    italic: value["fontStyle"] === undefined ? from.italic : value["fontStyle"] === "italic",
    ...(color === undefined ? {} : { color }),
    ...(decoration === undefined ? {} : { decoration }),
  };
}

/**
 * The fields a `style` value sets itself, without a base: a type-scale role's metrics for its token, another
 * typography token's fields, then its own fields. The component a text sits in gives the rest.
 */
export function ownTextStyle(value: unknown, theme: PaintTheme | undefined): Partial<TypeSpec> & { italic?: boolean } {
  if (!isObj(value)) {
    return {};
  }
  const token = typeof value["$token"] === "string" ? value["$token"] : undefined;
  const role = token === undefined ? undefined : typeRoleOf(token);
  const fromToken =
    role !== undefined ? typeScaleOf(theme)[role] : token === undefined ? {} : (theme?.textTokens?.[token] ?? {});
  const set = (name: string, v: number | undefined) => (v === undefined ? {} : { [name]: v });
  return {
    ...fromToken,
    ...set("size", num(value["fontSize"])),
    ...set("weight", weightOf(value["fontWeight"])),
    ...set("height", num(value["height"])),
    ...set("letterSpacing", num(value["letterSpacing"])),
    ...(typeof value["fontFamily"] === "string" ? { family: value["fontFamily"] } : {}),
    ...(value["fontStyle"] === undefined ? {} : { italic: value["fontStyle"] === "italic" }),
  };
}

function decorationOf(v: unknown): ResolvedText["decoration"] {
  return v === "underline" || v === "lineThrough" || v === "overline" ? v : undefined;
}
