// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { primitives } from "@plux/brand";
import type { ColorRgba, ItemSpec, SceneOp } from "@plux/canvas";
import type { Point } from "./positions.ts";
import { DEVICE, type Placed, type ScreenLayout } from "./screen-layout.ts";

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

const colors = {
  frame: rgba(primitives.white),
  frameStroke: rgba(primitives["navy.alpha16"]),
  box: rgba(primitives["porcelain.100"]),
  boxStroke: rgba(primitives["navy.alpha16"]),
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
  readonly layout: ScreenLayout;
}

export const frameId = (screen: string) => `screen:${screen}`;
export const overflowId = (screen: string) => `overflow:${screen}`;

function boxItem(s: PlacedScreen, b: Placed): ItemSpec {
  const fid = frameId(s.id);
  const beyond = b.y >= DEVICE.height;
  const common = {
    id: `${s.id}/${b.id}`,
    x: s.at.x + b.x,
    y: s.at.y + b.y,
    w: b.w,
    z: 2,
    ...(beyond ? {} : { frame: fid }),
  };
  if (b.kind === "text" && b.text !== undefined) {
    return { kind: "text", ...common, text: b.text, size: 16, color: beyond ? colors.overflowText : colors.text };
  }
  return {
    kind: "rect",
    ...common,
    h: b.h,
    radius: 8,
    fill: beyond ? colors.overflowBox : colors.box,
    stroke: { color: beyond ? colors.overflowStroke : colors.boxStroke, width: 1 },
    ...(b.text === undefined ? {} : { label: b.text }),
  };
}

/**
 * Scene items of one screen: its device frame, the boxes inside it, and, when the content is taller
 * than the viewport, a translucent region below the frame holding the rest (STU-056).
 */
export function screenItems(s: PlacedScreen): ItemSpec[] {
  const fid = frameId(s.id);
  const { x, y } = s.at;
  const items: ItemSpec[] = [
    {
      kind: "frame",
      id: fid,
      name: s.title,
      x,
      y,
      w: DEVICE.width,
      h: DEVICE.height,
      radius: 32,
      fill: colors.frame,
      stroke: { color: colors.frameStroke, width: 2 },
      shadow: true,
      z: 1,
    },
  ];
  const overflow = Math.max(0, s.layout.contentHeight - DEVICE.height);
  if (overflow > 0) {
    items.push({
      kind: "rect",
      id: overflowId(s.id),
      x,
      y: y + DEVICE.height,
      w: DEVICE.width,
      h: overflow,
      radius: 0,
      fill: colors.overflowFill,
      stroke: { color: colors.overflowStroke, width: 1 },
      z: 1,
    });
  }
  items.push(...s.layout.boxes.map((b) => boxItem(s, b)));
  return items;
}

/** The ops that replace the whole scene with `screens`. */
export function sceneOps(screens: readonly PlacedScreen[]): SceneOp[] {
  return [{ op: "clear" }, ...screens.flatMap(screenItems).map((item) => ({ op: "add", item }) as const)];
}

/** The bounds of everything drawn, for fitting the first view. */
export function contentBounds(screens: readonly PlacedScreen[]): { l: number; t: number; r: number; b: number } {
  let l = 0;
  let t = 0;
  let r: number = DEVICE.width;
  let b: number = DEVICE.height;
  screens.forEach((s, i) => {
    const bottom = s.at.y + Math.max(DEVICE.height, s.layout.contentHeight);
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
