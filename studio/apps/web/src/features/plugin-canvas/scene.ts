// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { primitives } from "@plux/brand";
import type { ColorRgba, ScreenDocument, ScreenHeight, ScreenStyle } from "@plux/canvas";
import { defaultScreenTheme } from "@plux/layout";
import type { Node } from "./plugin-model.ts";
import type { Point } from "./positions.ts";

/** The device frame every screen is drawn in, in logical pixels. */
export const DEVICE = { width: defaultScreenTheme.width, height: 844 } as const;

/** Fraction of full opacity for content beyond the viewport (STU-056). */
const OVERFLOW_ALPHA = 0x50;

/** A CSS colour of the brand tokens as 0xRRGGBBAA. */
function rgba(css: string, alphaOverride?: number): ColorRgba {
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

export interface PlacedScreen {
  readonly id: string;
  readonly title: string;
  readonly at: Point;
  readonly root: Node | undefined;
}

/** The documents the worker lays out (ADR-0013): each screen's tree and where it stands. */
export function screenDocuments(screens: readonly PlacedScreen[]): ScreenDocument[] {
  return screens.map((s) => ({ id: s.id, title: s.title, x: s.at.x, y: s.at.y, root: s.root ?? null }));
}

/** Each screen's laid-out content height, by screen id. */
export type Heights = ReadonlyMap<string, number>;

export const heightsById = (heights: readonly ScreenHeight[]): Heights =>
  new Map(heights.map((h) => [h.id, h.contentHeight]));

/** The bounds of everything drawn, overflow regions included, for fitting the first view. */
export function contentBounds(
  screens: readonly PlacedScreen[],
  heights: Heights,
): { l: number; t: number; r: number; b: number } {
  let l = 0;
  let t = 0;
  let r: number = DEVICE.width;
  let b: number = DEVICE.height;
  screens.forEach((s, i) => {
    const bottom = s.at.y + Math.max(DEVICE.height, heights.get(s.id) ?? 0);
    if (i === 0) {
      [l, t, r, b] = [s.at.x, s.at.y, s.at.x + DEVICE.width, bottom];
    } else {
      l = Math.min(l, s.at.x);
      t = Math.min(t, s.at.y);
      r = Math.max(r, s.at.x + DEVICE.width);
      b = Math.max(b, bottom);
    }
  });
  return { l, t, r, b };
}
