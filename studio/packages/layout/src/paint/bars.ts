// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * What Material 3's bars and surfaces paint (STU-005): app bars, navigation bars, rails and drawers, tab bars and
 * bottom app bars, with the defaults of Flutter's `_*DefaultsM3`. A bar's destinations and tabs are the parts the scene
 * laid out (`<id>.<i>.icon`, `<id>.<i>#label`); the selected one takes its indicator and its colours.
 */

import type { ColorRole } from "../material-color/scheme.ts";
import type { PaintText, PartsPaint, PlacedGlyph } from "./fields.ts";
import { type BoxPaint, type PaintTheme, type Rect, type Rgba, resolveColor, roleColor } from "./paint.ts";
import { M3_TYPE_SCALE, type ResolvedText, type TypeRole, typeScaleOf } from "./text-style.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

/** A node as the bar painters read it. */
export interface BarNode {
  readonly id: string;
  readonly type?: string;
  readonly props?: Obj;
}

type RectOf = (id: string) => Rect | undefined;

const styleOf = (theme: PaintTheme, role: TypeRole): ResolvedText => ({
  ...(typeScaleOf(theme)[role] ?? M3_TYPE_SCALE[role]),
  italic: false,
});

/** A surface filling the bar in its `backgroundColor`, else the role Material 3 gives it. */
function surface(node: BarNode, r: Rect, theme: PaintTheme, role: ColorRole, extra: Partial<BoxPaint> = {}): BoxPaint {
  return { ...r, fill: resolveColor(node.props?.["backgroundColor"], theme) ?? roleColor(theme, role), ...extra };
}

/** The destinations or tabs of a bar: its `destinations` or `tabs`, each with a label and its icons. */
function itemsOf(node: BarNode, key: "destinations" | "tabs"): Obj[] {
  const list = node.props?.[key];
  return Array.isArray(list) ? list.filter(isObj) : [];
}

const glyphAt = (at: Rect, icon: unknown, color: Rgba): PlacedGlyph[] =>
  isObj(icon) && typeof icon["name"] === "string"
    ? [
        {
          ...at,
          name: icon["name"],
          set: icon["set"] === "cupertino" ? "cupertino" : "material",
          size: Math.min(24, at.w, at.h),
          color,
        },
      ]
    : [];

/**
 * A navigation bar or rail: its surface; each destination's icon (the selected one's selected icon over its indicator
 * pill in `secondaryContainer`) and label.
 */
function destinationsPaint(node: BarNode, r: Rect, theme: PaintTheme, rectOf: RectOf, rail: boolean): PartsPaint {
  const selected = num(node.props?.["selectedIndex"]) ?? 0;
  const boxes: BoxPaint[] = [surface(node, r, theme, rail ? "surface" : "surfaceContainer")];
  const texts: PaintText[] = [];
  const glyphs: PlacedGlyph[] = [];
  itemsOf(node, "destinations").forEach((d, i) => {
    const part = destinationPaint(node, d, i, i === selected, theme, rectOf, rail);
    boxes.push(...part.boxes);
    texts.push(...part.texts);
    glyphs.push(...part.glyphs);
  });
  return { boxes, texts, glyphs };
}

/** The selected destination's indicator: a pill in `secondaryContainer` centred on its icon. */
function indicator(icon: Rect, rail: boolean, theme: PaintTheme): BoxPaint {
  const [w, h] = rail ? [56, 32] : [64, 32];
  return {
    x: icon.x + icon.w / 2 - w / 2,
    y: icon.y + icon.h / 2 - h / 2,
    w,
    h,
    radii: [h / 2, h / 2, h / 2, h / 2],
    fill: roleColor(theme, "secondaryContainer"),
  };
}

/** One destination: its indicator when selected, its icon, its label. */
function destinationPaint(
  node: BarNode,
  d: Obj,
  i: number,
  on: boolean,
  theme: PaintTheme,
  rectOf: RectOf,
  rail: boolean,
): PartsPaint {
  const boxes: BoxPaint[] = [];
  const texts: PaintText[] = [];
  const glyphs: PlacedGlyph[] = [];
  const icon = rectOf(`${node.id}.${i}.${on && d["selectedIcon"] !== undefined ? "selectedIcon" : "icon"}`);
  if (icon !== undefined) {
    boxes.push(...(on ? [indicator(icon, rail, theme)] : []));
    const color = roleColor(theme, on ? "onSecondaryContainer" : "onSurfaceVariant");
    glyphs.push(...glyphAt(icon, on ? (d["selectedIcon"] ?? d["icon"]) : d["icon"], color));
  }
  const at = rectOf(`${node.id}.${i}#label`);
  if (at !== undefined && at.h > 0 && typeof d["label"] === "string") {
    const color = roleColor(theme, on ? "onSurface" : "onSurfaceVariant");
    texts.push({ text: d["label"], x: at.x, y: at.y, w: at.w + 1, style: styleOf(theme, "labelMedium"), color });
  }
  return { boxes, texts, glyphs };
}

/**
 * A primary tab bar: a divider in `outlineVariant` along its bottom; each tab's label in `titleSmall` and icon, the
 * selected one in `primary` over a 3 pixel indicator as wide as its label.
 */
function tabBarPaint(node: BarNode, r: Rect, theme: PaintTheme, rectOf: RectOf): PartsPaint {
  const selected = num(node.props?.["selectedIndex"]) ?? 0;
  const boxes: BoxPaint[] = [{ x: r.x, y: r.y + r.h - 1, w: r.w, h: 1, fill: roleColor(theme, "outlineVariant") }];
  const texts: PaintText[] = [];
  const glyphs: PlacedGlyph[] = [];
  const title = styleOf(theme, "titleSmall");
  itemsOf(node, "tabs").forEach((t, i) => {
    const on = i === selected;
    const color = roleColor(theme, on ? "primary" : "onSurfaceVariant");
    const label = rectOf(`${node.id}.${i}#label`);
    const icon = rectOf(`${node.id}.${i}.icon`);
    if (label !== undefined && typeof t["label"] === "string") {
      texts.push({ text: t["label"], x: label.x, y: label.y, w: label.w + 1, style: title, color });
    }
    if (icon !== undefined) {
      glyphs.push(...glyphAt(icon, t["icon"], color));
    }
    const under = label ?? icon;
    if (on && under !== undefined) {
      boxes.push({
        x: under.x,
        y: r.y + r.h - 3,
        w: under.w,
        h: 3,
        radii: [3, 3, 0, 0],
        fill: roleColor(theme, "primary"),
      });
    }
  });
  return { boxes, texts, glyphs };
}

/** The painters of bars that place their own parts, by widget type. */
export const barPainters: ReadonlyMap<
  string,
  (node: BarNode, r: Rect, theme: PaintTheme, rectOf: RectOf) => PartsPaint
> = new Map([
  ["NavigationBar", (n, r, t, rectOf) => destinationsPaint(n, r, t, rectOf, false)],
  ["NavigationRail", (n, r, t, rectOf) => destinationsPaint(n, r, t, rectOf, true)],
  ["TabBar", tabBarPaint],
]);

/** The surfaces of the bars and drawers that place no parts of their own, by widget type. */
export function barSurface(node: BarNode, r: Rect, theme: PaintTheme): BoxPaint[] | undefined {
  switch (node.type) {
    case "AppBar":
      return [surface(node, r, theme, "surface")];
    case "BottomAppBar":
      return [surface(node, r, theme, "surfaceContainer")];
    case "Drawer":
    case "NavigationDrawer":
      return [surface(node, r, theme, "surfaceContainerLow", { radii: [0, 16, 16, 0] })];
    default:
      return undefined;
  }
}
