// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { primitives } from "@plux/brand";
import type { ArrowStyle, ColorRgba, ScreenDocument, ScreenHeight, ScreenStyle } from "@plux/canvas";
import { defaultScreenTheme } from "@plux/layout";
import type { GhostStyle } from "./ghosts.ts";
import type { Node } from "./plugin-model.ts";
import type { Point } from "./positions.ts";

/** The device frame every screen is drawn in, in logical pixels. */
export const DEVICE = { width: defaultScreenTheme.width, height: 844 } as const;

/** Fraction of full opacity for content beyond the viewport (STU-056). */
const OVERFLOW_ALPHA = 0x50;

/** A CSS colour of the brand tokens as 0xRRGGBBAA. */
export function rgba(css: string, alphaOverride?: number): ColorRgba {
  const hex = /^#([0-9a-f]{6})$/i.exec(css);
  if (hex?.[1] !== undefined) {
    return ((Number.parseInt(hex[1], 16) << 8) | (alphaOverride ?? 0xff)) >>> 0;
  }
  const m = /^rgba\((\d+),(\d+),(\d+),([\d.]+)\)$/.exec(css);
  if (m === null) {
    throw new Error(`unsupported colour ${css}`);
  }
  const [r, g, b] = [m[1], m[2], m[3]].map(Number) as [number, number, number];
  const a = alphaOverride ?? Math.round(Number(m[4]) * 255);
  return ((r << 24) | (g << 16) | (b << 8) | a) >>> 0;
}

/** How the canvas worker draws laid-out screens: the brand colours and the device. */
export const screenStyle: ScreenStyle = {
  deviceHeight: DEVICE.height,
  theme: defaultScreenTheme,
  frame: rgba(primitives.white),
  frameStroke: rgba(primitives["navy.alpha16"]),
  box: rgba(primitives["porcelain.100"]),
  boxStroke: rgba(primitives["navy.alpha16"]),
  placeholder: rgba(primitives["violet.700"], 0x14),
  text: rgba(primitives["navy.900"]),
  overflowFill: rgba(primitives["violet.700"], 0x14),
  overflowBox: rgba(primitives["porcelain.100"], OVERFLOW_ALPHA),
  overflowText: rgba(primitives["navy.900"], OVERFLOW_ALPHA),
  overflowStroke: rgba(primitives["violet.700"], 0x60),
};

/** Ghost screens and native nodes (STU-052, STU-054): dashed navy outline on porcelain. */
export const ghostStyle: GhostStyle = {
  stroke: rgba(primitives["navy.900"], 0x90),
  fill: rgba(primitives["porcelain.100"]),
  nativeFill: rgba(primitives["violet.700"], 0x14),
  text: rgba(primitives["navy.900"]),
  pulse: rgba(primitives["violet.700"]),
};

/** Navigation arrows (STU-040 to STU-048) in the brand colours. */
export const arrowStyle: ArrowStyle = {
  stroke: { color: rgba(primitives["navy.900"], 0xa0), width: 1.5 },
  dimmed: { color: rgba(primitives["navy.900"], 0x30), width: 1.5 },
  focus: { color: rgba(primitives["violet.700"]), width: 2 },
  badgeFill: rgba(primitives["navy.900"]),
  badgeText: rgba(primitives.white),
  labelText: rgba(primitives["navy.900"]),
  z: 1,
};

export interface PlacedScreen {
  readonly id: string;
  readonly title: string;
  /** The page's route name and kind (SCH-022, SCH-025), shown above its frame (PC-11). */
  readonly route?: string;
  readonly kind?: PageKind;
  /** The page a dialog or sheet page is shown over: the screen that opens it (PC-13). */
  readonly opener?: string;
  /** Another page opens this screen, so its app bar shows a back button (PC-15); the entry has none. */
  readonly back?: boolean;
  /** The translation key of a translated title; the page shows it in the previewed locale. */
  readonly titleKey?: string;
  readonly at: Point;
  readonly root: Node | undefined;
}

/** A page's kind (SCH-022). */
export type PageKind = "screen" | "dialog" | "bottomSheet" | "fullscreenDialog";

/**
 * The documents the worker lays out (ADR-0013): each screen's tree and where it stands, with what its header shows
 * after the name (`meta`, PC-11).
 */
export function screenDocuments(
  screens: readonly PlacedScreen[],
  meta?: (s: PlacedScreen) => string | undefined,
): ScreenDocument[] {
  return screens.map((s) => {
    const line = meta?.(s);
    return {
      id: s.id,
      title: s.title,
      ...(line === undefined || line === "" ? {} : { meta: line }),
      x: s.at.x,
      y: s.at.y,
      root: s.root ?? null,
    };
  });
}

/** The size of the device frame a style draws screens in. */
export function deviceOf(style: ScreenStyle): { readonly width: number; readonly height: number } {
  return { width: style.theme.width, height: style.deviceHeight };
}

/** Each screen's laid-out content height, by screen id. */
export type Heights = ReadonlyMap<string, number>;

export const heightsById = (heights: readonly ScreenHeight[]): Heights =>
  new Map(heights.map((h) => [h.id, h.contentHeight]));

/** The bounds of everything drawn, overflow regions included, for fitting the first view. */
export function contentBounds(
  screens: readonly PlacedScreen[],
  heights: Heights,
  device: { readonly width: number; readonly height: number } = DEVICE,
): { l: number; t: number; r: number; b: number } {
  let l = 0;
  let t = 0;
  let r: number = device.width;
  let b: number = device.height;
  screens.forEach((s, i) => {
    const bottom = s.at.y + Math.max(device.height, heights.get(s.id) ?? 0);
    if (i === 0) {
      [l, t, r, b] = [s.at.x, s.at.y, s.at.x + device.width, bottom];
    } else {
      l = Math.min(l, s.at.x);
      t = Math.min(t, s.at.y);
      r = Math.max(r, s.at.x + device.width);
      b = Math.max(b, bottom);
    }
  });
  return { l, t, r, b };
}
