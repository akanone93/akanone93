// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * What a laid-out screen paints (STU-005, ADR-0013): for each widget, the boxes Flutter's painters would draw — fills,
 * gradients, borders, rounded and circular shapes, Material elevation shadows — with the opacity and clips its
 * ancestors apply, in paint order. Colours come from the widget's props, from theme tokens and from the Material 3
 * colour scheme the device builds (`../material-color/scheme.ts`). The layout engine places boxes; this module says how
 * they look. A widget type this module does not paint yet is reported as a placeholder, so the canvas still shows it.
 */

import { shadowsForElevation } from "../flutter/shadows.ts";
import type { ColorRole, ColorScheme } from "../material-color/scheme.ts";
import type { PaintText, PlacedGlyph } from "./fields.ts";
import type { TypeRole, TypeSpec } from "./text-style.ts";

/** A colour as the canvas takes it: 0xRRGGBBAA. */
export type Rgba = number;

/** The corner radii of a box, clockwise from the top left: top left, top right, bottom right, bottom left. */
export type Radii = readonly [number, number, number, number];

export interface Rect {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
}

/** A gradient's colours and stops along a line from `from` to `to` (linear), or round a centre (radial, sweep). */
export interface GradientPaint {
  readonly kind: "linear" | "radial" | "sweep";
  readonly colors: readonly Rgba[];
  readonly stops?: readonly number[];
  readonly from: readonly [number, number];
  readonly to: readonly [number, number];
  readonly radius: number;
}

export interface ShadowPaint {
  readonly color: Rgba;
  readonly dx: number;
  readonly dy: number;
  readonly blur: number;
  readonly spread: number;
}

export interface SidePaint {
  readonly color: Rgba;
  readonly width: number;
}

/** The outline of a box: one side for all four edges, or a side per edge. */
export type BorderPaint =
  | { readonly kind: "all"; readonly side: SidePaint }
  | {
      readonly kind: "sides";
      readonly top?: SidePaint;
      readonly right?: SidePaint;
      readonly bottom?: SidePaint;
      readonly left?: SidePaint;
    };

/** A shape descendants are clipped to. */
export interface PaintClip extends Rect {
  readonly radii?: Radii;
  readonly oval?: boolean;
}

/** One painted box. */
export interface BoxPaint extends Rect {
  readonly fill?: Rgba;
  readonly gradient?: GradientPaint;
  readonly radii?: Radii;
  readonly oval?: boolean;
  readonly border?: BorderPaint;
  readonly shadows?: readonly ShadowPaint[];
  /** Drawn over the widget's children (a foreground decoration). */
  readonly foreground?: boolean;
  /** An image painted inside the box's shape, over its fill (`Image`, a `CircleAvatar`'s image). */
  readonly image?: ImagePaint;
}

/** Flutter's `BoxFit`. */
export type BoxFitName = "fill" | "contain" | "cover" | "fitWidth" | "fitHeight" | "none" | "scaleDown";

/**
 * An image as Flutter's `paintImage` draws it: from an app asset (by id) or a network URL, fitted to the box and
 * aligned in it (`x` and `y` from -1 to 1). A network image is the device's to load; the canvas shows its placeholder.
 */
export interface ImagePaint {
  readonly asset?: string;
  readonly url?: string;
  readonly fit: BoxFitName;
  readonly alignX: number;
  readonly alignY: number;
}

/** How one widget is drawn: its boxes, the opacity and clips of its ancestors, and its text colour. */
export interface NodePaint {
  /**
   * `paint` draws boxes; `layout` draws nothing of its own (rows, padding, scroll views); `placeholder` is not painted
   * yet; `hidden` is not painted at all (the children an `IndexedStack` does not show).
   */
  readonly role: "paint" | "layout" | "placeholder" | "text" | "icon" | "hidden";
  readonly boxes: readonly BoxPaint[];
  /** Paint order among the screen's widgets, from 0; later is drawn over earlier. */
  readonly order: number;
  readonly opacity: number;
  readonly clips: readonly PaintClip[];
  /** The colour of the widget's text when it is a text. */
  readonly textColor?: Rgba;
  /** The glyph an `Icon` draws (THM-005). */
  readonly icon?: IconPaint;
  /** The texts and icons the widget places itself (a field's label and hint, its icons). */
  readonly texts?: readonly PaintText[];
  readonly glyphs?: readonly PlacedGlyph[];
}

/** An icon as Flutter's `Icon` draws it: a glyph of an icon set, centred in its box at `size`, in `color`. */
export interface IconPaint {
  readonly name: string;
  readonly set: "material" | "cupertino";
  readonly size: number;
  readonly color: Rgba;
}

/** What painters read colours from: the scheme of the previewed brightness and the theme's other colour tokens. */
export interface PaintTheme {
  readonly scheme: ColorScheme;
  /**
   * The theme's colour tokens that are not scheme roles, by path, resolved for the previewed mode and brand. Plain data:
   * the theme travels to the canvas worker in a message.
   */
  readonly tokens?: Readonly<Record<string, Rgba>>;
  /** The theme's `typography.<role>` tokens, resolved; the other roles keep Material 3's type scale. */
  readonly typography?: Readonly<Partial<Record<TypeRole, TypeSpec>>>;
  /**
   * Every typography token of the theme by path (`type.headline`), with the fields it sets: a text style bound to one
   * takes those fields over the default text style, as the runtime's `TextStyle` merge does.
   */
  readonly textTokens?: Readonly<Record<string, Partial<TypeSpec>>>;
  /** The URL of each image asset of the app by asset id, for the canvas to load (AST-002). */
  readonly assets?: Readonly<Record<string, string>>;
  /** Every dimension token of the theme by path (`space.md`), in logical pixels. */
  readonly dimensions?: Readonly<Record<string, number>>;
  /** True in a right-to-left screen, where `start` and `end` insets swap. */
  readonly rtl?: boolean;
}

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

/** An ARGB integer as the canvas's RGBA. */
export function argbToRgba(argb: number): Rgba {
  return ((argb << 8) | (argb >>> 24)) >>> 0;
}

/** The scheme's `role` as RGBA. */
export function roleColor(theme: PaintTheme, role: ColorRole): Rgba {
  return argbToRgba(theme.scheme[role]);
}

/** A `#RRGGBB` or `#RRGGBBAA` literal as RGBA, the format the runtime reads. */
export function parseHexColor(s: string): Rgba | undefined {
  if (!/^#([0-9A-Fa-f]{6}|[0-9A-Fa-f]{8})$/.test(s)) {
    return undefined;
  }
  return Number.parseInt(s.length === 7 ? `${s.slice(1)}ff` : s.slice(1), 16) >>> 0;
}

/** A colour prop's value: a literal or a theme token; an expression or anything else is unknown. */
export function resolveColor(value: unknown, theme: PaintTheme): Rgba | undefined {
  if (typeof value === "string") {
    return parseHexColor(value);
  }
  if (!isObj(value) || typeof value["$token"] !== "string") {
    return undefined;
  }
  const path = value["$token"];
  const role = path.startsWith("color.") ? (path.slice(6) as ColorRole) : undefined;
  if (role !== undefined && typeof theme.scheme[role] === "number") {
    return roleColor(theme, role);
  }
  return theme.tokens?.[path];
}

/** `rgba` with its alpha multiplied by `opacity`. */
export function withOpacity(rgba: Rgba, opacity: number): Rgba {
  if (opacity >= 1) {
    return rgba;
  }
  const a = Math.round((rgba & 0xff) * Math.max(0, opacity));
  return ((rgba & 0xffffff00) | a) >>> 0;
}

/** EdgeInsets (`all`, `horizontal`, `vertical`, the four sides, `start` and `end`) as left, top, right, bottom. */
export function edgeInsets(value: unknown, rtl = false): [number, number, number, number] {
  if (typeof value === "number") {
    return [value, value, value, value];
  }
  if (!isObj(value)) {
    return [0, 0, 0, 0];
  }
  const all = num(value["all"]) ?? 0;
  const h = num(value["horizontal"]) ?? all;
  const v = num(value["vertical"]) ?? all;
  const start = num(value["start"]);
  const end = num(value["end"]);
  const left = num(value["left"]) ?? (rtl ? end : start) ?? h;
  const right = num(value["right"]) ?? (rtl ? start : end) ?? h;
  return [left, num(value["top"]) ?? v, right, num(value["bottom"]) ?? v];
}

function radius(value: unknown): number {
  if (typeof value === "number") {
    return Math.max(0, value);
  }
  return isObj(value) ? Math.max(0, num(value["circular"]) ?? num(value["x"]) ?? 0) : 0;
}

/** BorderRadius (`all` or the four corners) as radii; undefined when there are none. */
export function borderRadius(value: unknown): Radii | undefined {
  if (typeof value === "number") {
    return value > 0 ? [value, value, value, value] : undefined;
  }
  if (!isObj(value)) {
    return undefined;
  }
  const all = num(value["all"]);
  const r: Radii = [
    radius(value["topLeft"] ?? all),
    radius(value["topRight"] ?? all),
    radius(value["bottomRight"] ?? all),
    radius(value["bottomLeft"] ?? all),
  ];
  return r.some((x) => x > 0) ? r : undefined;
}

function side(value: unknown, theme: PaintTheme): SidePaint | undefined {
  if (!isObj(value) || value["style"] === "none") {
    return undefined;
  }
  const width = num(value["width"]) ?? 1.0;
  const color = resolveColor(value["color"], theme) ?? 0x000000ff;
  return width > 0 ? { color, width } : undefined;
}

/** Border (`all` or one side per edge). */
export function border(value: unknown, theme: PaintTheme): BorderPaint | undefined {
  if (!isObj(value)) {
    return undefined;
  }
  const all = side(value["all"], theme);
  if (all !== undefined) {
    return { kind: "all", side: all };
  }
  const sides = {
    top: side(value["top"], theme),
    right: side(value["right"], theme),
    bottom: side(value["bottom"], theme),
    left: side(value["left"], theme),
  };
  const defined = Object.fromEntries(Object.entries(sides).filter(([, s]) => s !== undefined));
  return Object.keys(defined).length === 0 ? undefined : { kind: "sides", ...defined };
}

/** A list of BoxShadow values. */
export function boxShadows(value: unknown, theme: PaintTheme): ShadowPaint[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const list = value.flatMap((s): ShadowPaint[] => {
    if (!isObj(s)) {
      return [];
    }
    const offset = isObj(s["offset"]) ? s["offset"] : {};
    return [
      {
        color: resolveColor(s["color"], theme) ?? 0x00000080,
        dx: num(offset["dx"]) ?? 0,
        dy: num(offset["dy"]) ?? 0,
        blur: Math.max(0, num(s["blurRadius"]) ?? 0),
        spread: num(s["spreadRadius"]) ?? 0,
      },
    ];
  });
  return list.length === 0 ? undefined : list;
}

/** An Alignment (`x`, `y`, or `start` in place of `x`) as a point in `r`. */
function alignmentPoint(value: unknown, r: Rect, fallback: readonly [number, number], rtl: boolean): [number, number] {
  let [ax, ay] = fallback;
  if (isObj(value)) {
    const start = num(value["start"]);
    ax = num(value["x"]) ?? (start === undefined ? ax : rtl ? -start : start);
    ay = num(value["y"]) ?? ay;
  }
  return [r.x + ((ax + 1) / 2) * r.w, r.y + ((ay + 1) / 2) * r.h];
}

/** A Gradient over the box `r`. */
export function gradient(value: unknown, r: Rect, theme: PaintTheme): GradientPaint | undefined {
  if (!isObj(value) || !Array.isArray(value["colors"])) {
    return undefined;
  }
  const colors = value["colors"].map((c) => resolveColor(c, theme)).filter((c): c is Rgba => c !== undefined);
  if (colors.length < 2) {
    return undefined;
  }
  const rtl = theme.rtl === true;
  const kind = value["kind"] === "radial" || value["kind"] === "sweep" ? value["kind"] : "linear";
  const stops = Array.isArray(value["stops"]) ? value["stops"].filter((s): s is number => typeof s === "number") : [];
  const centre = alignmentPoint(value["center"], r, [0, 0], rtl);
  return {
    kind,
    colors,
    ...(stops.length === colors.length ? { stops } : {}),
    from: kind === "linear" ? alignmentPoint(value["begin"], r, [-1, 0], rtl) : centre,
    to: kind === "linear" ? alignmentPoint(value["end"], r, [1, 0], rtl) : centre,
    radius: (num(value["radius"]) ?? 0.5) * Math.min(r.w, r.h),
  };
}

/** `r` shrunk by insets left, top, right, bottom. */
export function deflate(r: Rect, [l, t, rt, b]: readonly [number, number, number, number]): Rect {
  return { x: r.x + l, y: r.y + t, w: Math.max(0, r.w - l - rt), h: Math.max(0, r.h - t - b) };
}

/** A BoxDecoration over `r`. */
export function decorationPaint(value: unknown, r: Rect, theme: PaintTheme): BoxPaint | undefined {
  if (!isObj(value)) {
    return undefined;
  }
  const fill = resolveColor(value["color"], theme);
  const grad = gradient(value["gradient"], r, theme);
  const radii = borderRadius(value["borderRadius"]);
  const outline = border(value["border"], theme);
  const shadows = boxShadows(value["boxShadow"], theme);
  const paint: BoxPaint = {
    ...r,
    ...(fill === undefined ? {} : { fill }),
    ...(grad === undefined ? {} : { gradient: grad }),
    ...(value["shape"] === "circle" ? { oval: true } : radii === undefined ? {} : { radii }),
    ...(outline === undefined ? {} : { border: outline }),
    ...(shadows === undefined ? {} : { shadows }),
  };
  return paint.fill === undefined &&
    paint.gradient === undefined &&
    paint.border === undefined &&
    paint.shadows === undefined
    ? undefined
    : paint;
}

/** A ShapeBorder over `r`: its radii or oval, and its outline. */
export function shapePaint(
  value: unknown,
  r: Rect,
  theme: PaintTheme,
  fallback: Radii,
): { radii?: Radii; oval?: boolean; border?: BorderPaint } {
  if (!isObj(value)) {
    return { radii: fallback };
  }
  const outline = side(value["side"], theme);
  const border: BorderPaint | undefined = outline === undefined ? undefined : { kind: "all", side: outline };
  const withBorder = border === undefined ? {} : { border };
  switch (value["kind"]) {
    case "circle":
      return { oval: true, ...withBorder };
    case "stadium": {
      const half = Math.min(r.w, r.h) / 2;
      return { radii: [half, half, half, half], ...withBorder };
    }
    default: {
      const radii = borderRadius(value["borderRadius"]);
      return { ...(radii === undefined ? {} : { radii }), ...withBorder };
    }
  }
}

/** Material's shadow for `elevation` in `color` (the scheme's shadow role by default). */
export function elevationShadows(elevation: number, color: Rgba): ShadowPaint[] {
  return shadowsForElevation(elevation).map((s) => ({
    color: ((color & 0xffffff00) | Math.round((s.alpha * (color & 0xff)) / 255)) >>> 0,
    dx: s.dx,
    dy: s.dy,
    blur: s.blur,
    spread: s.spread,
  }));
}
