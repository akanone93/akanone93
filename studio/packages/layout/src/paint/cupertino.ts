// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * What Cupertino widgets paint (STU-005, WGT-011), with iOS's colours as Flutter's `CupertinoColors` define them for a
 * light and a dark brightness, and the runtime's Cupertino theme (its primary colour is the scheme's `primary`, its
 * bars `surfaceContainer`). Under the **Cupertino** preview a Material control marked `adaptive` is drawn as its
 * Cupertino counterpart, centred in the box Material lays out.
 */

import type { Foreground } from "./controls.ts";
import type { PartsPaint, PlacedGlyph } from "./fields.ts";
import {
  type BoxPaint,
  type PaintTheme,
  type Rect,
  type Rgba,
  resolveColor,
  roleColor,
  type ShadowPaint,
} from "./paint.ts";
import type { ResolvedText } from "./text-style.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

/** A node as the Cupertino painters read it. */
export interface CupertinoNode {
  readonly id: string;
  readonly type?: string;
  readonly props?: Obj;
  readonly events?: unknown;
}

/** A `CupertinoDynamicColor`: its light and dark RGBA, picked by the theme's brightness. */
const dynamic = (theme: PaintTheme, light: Rgba, dark: Rgba): Rgba =>
  theme.scheme.brightness === "dark" ? dark : light;

const SYSTEM_GREEN = (t: PaintTheme) => dynamic(t, 0x34c759ff, 0x30d158ff);
const SECONDARY_FILL = (t: PaintTheme) => dynamic(t, 0x78788028, 0x78788052);
const SYSTEM_FILL = (t: PaintTheme) => dynamic(t, 0x78788033, 0x7878805c);
const TERTIARY_FILL = (t: PaintTheme) => dynamic(t, 0x7676801f, 0x7676803d);
const SEPARATOR = (t: PaintTheme) => dynamic(t, 0x0000004d, 0xffffff29);
const PLACEHOLDER = (t: PaintTheme) => dynamic(t, 0x3c3c434c, 0xebebf54c);
const INACTIVE_GRAY = 0x999999ff;
/** The shadow of a switch's or a slider's thumb. */
const THUMB_SHADOWS: readonly ShadowPaint[] = [
  { color: 0x00000026, dx: 0, dy: 3, blur: 8, spread: 0 },
  { color: 0x0000000f, dx: 0, dy: 3, blur: 1, spread: 0 },
];

/** iOS body text, as `CupertinoTextThemeData.textStyle`: 17 pixels, tracking −0.41. */
const BODY: ResolvedText = { size: 17, weight: 400, height: 1.29, letterSpacing: -0.41, italic: false };

const handles = (node: CupertinoNode, events: readonly string[]): boolean => {
  const e = isObj(node.events) ? node.events : {};
  return events.some((name) => e[name] !== undefined && e[name] !== null);
};

function centred(r: Rect, w: number, h: number): Rect {
  return { x: r.x + (r.w - w) / 2, y: r.y + (r.h - h) / 2, w, h };
}

const round = (b: Rect, radius: number) => ({ ...b, radii: [radius, radius, radius, radius] as const });

/** A Cupertino switch: a 51 by 31 track, green when on, and a 27 pixel white thumb at its end. */
function cupertinoSwitch(node: CupertinoNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const on = node.props?.["value"] === true;
  const track = centred(r, 51, 31);
  const fill = on
    ? (resolveColor(node.props?.["activeTrackColor"], theme) ?? SYSTEM_GREEN(theme))
    : (resolveColor(node.props?.["inactiveTrackColor"], theme) ?? SECONDARY_FILL(theme));
  const x = on ? track.x + track.w - 29 : track.x + 2;
  return [
    { ...round(track, 15.5), fill },
    { x, y: track.y + 2, w: 27, h: 27, oval: true, fill: 0xffffffff, shadows: THUMB_SHADOWS },
  ];
}

/** A Cupertino slider: a 4 pixel track inset by the thumb's radius, active in primary, and a 28 pixel white thumb. */
function cupertinoSlider(node: CupertinoNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const p = node.props ?? {};
  const min = num(p["min"]) ?? 0;
  const max = num(p["max"]) ?? 1;
  const t = max > min ? (Math.min(max, Math.max(min, num(p["value"]) ?? min)) - min) / (max - min) : 0;
  const left = r.x + 14;
  const width = Math.max(0, r.w - 28);
  const y = r.y + r.h / 2;
  const at = left + width * t;
  const active = resolveColor(p["activeColor"], theme) ?? roleColor(theme, "primary");
  return [
    { x: left, y: y - 2, w: width, h: 4, radii: [2, 2, 2, 2], fill: SYSTEM_FILL(theme) },
    { x: left, y: y - 2, w: at - left, h: 4, radii: [2, 2, 2, 2], fill: active },
    { x: at - 14, y: y - 14, w: 28, h: 28, oval: true, fill: 0xffffffff, shadows: THUMB_SHADOWS },
  ];
}

/** A Cupertino button: its label in primary, or in a filled one (a `color`) on that colour, corners of 8. */
function cupertinoButton(node: CupertinoNode, r: Rect, theme: PaintTheme): BoxPaint[] {
  const color = resolveColor(node.props?.["color"], theme);
  if (color === undefined) {
    return [];
  }
  const off = !handles(node, ["onPressed", "onLongPress"]);
  return [{ ...round(r, 8), fill: off ? TERTIARY_FILL(theme) : color }];
}

/** A Cupertino text field: on the background colour, a hairline border, corners of 5. */
function cupertinoTextField(node: CupertinoNode, r: Rect, theme: PaintTheme): PartsPaint {
  const value =
    typeof node.props?.["value"] === "string" && node.props["value"] !== "" ? node.props["value"] : undefined;
  const placeholder = typeof node.props?.["placeholder"] === "string" ? node.props["placeholder"] : undefined;
  const shown = value ?? placeholder;
  return {
    boxes: [
      {
        ...round(r, 5),
        fill: dynamic(theme, 0xffffffff, 0x000000ff),
        border: { kind: "all", side: { color: dynamic(theme, 0x00000033, 0xffffff33), width: 0.5 } },
      },
    ],
    texts:
      shown === undefined
        ? []
        : [
            {
              text: shown,
              x: r.x + 7,
              y: r.y + (r.h - BODY.size * BODY.height) / 2,
              w: Math.max(0, r.w - 14),
              style: BODY,
              color: value === undefined ? PLACEHOLDER(theme) : roleColor(theme, "onSurface"),
            },
          ],
    glyphs: [],
  };
}

/** A Cupertino activity indicator: the spinner's glyph in grey, as large as its radius allows. */
function activityIndicator(node: CupertinoNode, r: Rect, theme: PaintTheme): PartsPaint {
  const size = Math.min(r.w, r.h, 2 * (num(node.props?.["radius"]) ?? 10));
  const color = resolveColor(node.props?.["color"], theme) ?? dynamic(theme, INACTIVE_GRAY, 0xebebf599);
  const glyph: PlacedGlyph = {
    ...centred(r, size, size),
    name: "progress_activity",
    set: "material",
    size,
    color,
  };
  return { boxes: [], texts: [], glyphs: [glyph] };
}

/** A sliding segmented control: its track in the tertiary fill, corners of 9, the selected segment's white thumb. */
function slidingSegments(
  node: CupertinoNode,
  r: Rect,
  theme: PaintTheme,
  rectOf: (id: string) => Rect | undefined,
): BoxPaint[] {
  const values = Array.isArray(node.props?.["values"]) ? node.props["values"] : [];
  const selected = values.findIndex((v) => JSON.stringify(v) === JSON.stringify(node.props?.["groupValue"]));
  const segments = (node as { slots?: Obj }).slots?.["segments"];
  const list = Array.isArray(segments) ? segments.filter(isObj) : [];
  const segment = selected < 0 ? undefined : list[selected];
  const at = typeof segment?.["id"] === "string" ? rectOf(segment["id"]) : undefined;
  const track = { ...round(r, 9), fill: resolveColor(node.props?.["backgroundColor"], theme) ?? TERTIARY_FILL(theme) };
  if (at === undefined) {
    return [track];
  }
  const thumb = { x: at.x, y: r.y + 2, w: at.w, h: r.h - 4 };
  return [
    track,
    {
      ...round(thumb, 7),
      fill: resolveColor(node.props?.["thumbColor"], theme) ?? dynamic(theme, 0xffffffff, 0x636366ff),
      shadows: [{ color: 0x0000001f, dx: 0, dy: 3, blur: 8, spread: 0 }],
    },
  ];
}

/** A bar's surface — a navigation bar's with a hairline below, a tab bar's with one above — in `surfaceContainer`. */
function bar(node: CupertinoNode, r: Rect, theme: PaintTheme, edge: "top" | "bottom"): BoxPaint[] {
  const fill = resolveColor(node.props?.["backgroundColor"], theme) ?? roleColor(theme, "surfaceContainer");
  return [{ ...r, fill, border: { kind: "sides", [edge]: { color: SEPARATOR(theme), width: 0.5 } } }];
}

type Painter = (
  node: CupertinoNode,
  r: Rect,
  theme: PaintTheme,
  rectOf: (id: string) => Rect | undefined,
) => PartsPaint;
const boxesOnly =
  (f: (node: CupertinoNode, r: Rect, theme: PaintTheme) => BoxPaint[]): Painter =>
  (node, r, theme) => ({ boxes: f(node, r, theme), texts: [], glyphs: [] });

/** The painters of the Cupertino widgets, by type. */
export const cupertinoPainters: ReadonlyMap<string, Painter> = new Map<string, Painter>([
  ["CupertinoSwitch", boxesOnly(cupertinoSwitch)],
  ["CupertinoSlider", boxesOnly(cupertinoSlider)],
  ["CupertinoButton", boxesOnly(cupertinoButton)],
  ["CupertinoTextField", (n, r, t) => cupertinoTextField(n, r, t)],
  ["CupertinoActivityIndicator", (n, r, t) => activityIndicator(n, r, t)],
  [
    "CupertinoSlidingSegmentedControl",
    (n, r, t, rectOf) => ({ boxes: slidingSegments(n, r, t, rectOf), texts: [], glyphs: [] }),
  ],
  ["CupertinoNavigationBar", boxesOnly((n, r, t) => bar(n, r, t, "bottom"))],
  ["CupertinoTabBar", boxesOnly((n, r, t) => bar(n, r, t, "top"))],
]);

/** The Cupertino counterparts of adaptive Material controls (`Switch.adaptive` and the rest) on iOS. */
const ADAPTIVE: Readonly<Record<string, Painter>> = {
  Switch: boxesOnly(cupertinoSwitch),
  Slider: boxesOnly(cupertinoSlider),
  CircularProgressIndicator: (n, r, t) => activityIndicator({ ...n, props: { radius: 10 } }, r, t),
  Checkbox: boxesOnly((n, r, t) => {
    const on = n.props?.["value"] === true;
    const b = round(centred(r, 18, 18), 9);
    return on
      ? [{ ...b, fill: roleColor(t, "primary") }]
      : [{ ...b, border: { kind: "all", side: { color: INACTIVE_GRAY, width: 1 } } }];
  }),
  Radio: boxesOnly((n, r, t) => {
    const p = n.props ?? {};
    const on = p["value"] !== undefined && JSON.stringify(p["value"]) === JSON.stringify(p["groupValue"]);
    const ring = { ...centred(r, 18, 18), oval: true };
    return on
      ? [
          { ...ring, fill: roleColor(t, "primary") },
          { ...centred(r, 7, 7), oval: true, fill: 0xffffffff },
        ]
      : [{ ...ring, border: { kind: "all", side: { color: INACTIVE_GRAY, width: 1 } } }];
  }),
};

/** The Cupertino painter of `node` on `theme`'s platform: a Cupertino widget's, or an adaptive control's on iOS. */
export function cupertinoPainterOf(node: CupertinoNode, theme: PaintTheme): Painter | undefined {
  const type = node.type ?? "";
  if (theme.platform === "cupertino" && node.props?.["adaptive"] === true && ADAPTIVE[type] !== undefined) {
    return ADAPTIVE[type];
  }
  return cupertinoPainters.get(type);
}

/**
 * The colours a Cupertino widget gives its labels and icons: a button's in primary, or the contrasting colour when
 * filled; a navigation bar's icons in primary (its title keeps the label colour).
 */
export function cupertinoForeground(node: CupertinoNode, theme: PaintTheme): Foreground | undefined {
  switch (node.type) {
    case "CupertinoButton": {
      const color = !handles(node, ["onPressed", "onLongPress"])
        ? dynamic(theme, INACTIVE_GRAY, 0x757575ff)
        : roleColor(theme, resolveColor(node.props?.["color"], theme) === undefined ? "primary" : "onPrimary");
      return { text: color, icon: color, iconSize: 24 };
    }
    case "CupertinoNavigationBar":
      return { icon: roleColor(theme, "primary"), iconSize: 24 };
    default:
      return undefined;
  }
}

/** Whether an adaptive control is drawn the iOS way: its check mark is then white. */
export const adaptiveOnIos = (node: CupertinoNode, theme: PaintTheme): boolean =>
  theme.platform === "cupertino" && node.props?.["adaptive"] === true;
