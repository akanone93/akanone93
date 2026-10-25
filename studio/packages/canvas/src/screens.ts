// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Screens laid out in the canvas worker (ADR-0013 Threading, Text): each
 * screen document goes through `@plux/layout` with the SkParagraph engine, and
 * the laid-out boxes become scene items — the device frame, the boxes inside
 * it, and a translucent region below the frame for content taller than the
 * device (STU-056).
 */

import { layoutScreen, type ParagraphEngine, type Placed } from "@plux/layout";
import type { ItemSpec, SceneOp, ScreenDocument, ScreenHeight, ScreenStyle } from "./protocol.ts";

/** The scene id of a screen's device frame. */
export const frameId = (screen: string) => `screen:${screen}`;
/** The scene id of a screen's overflow region (STU-056). */
export const overflowId = (screen: string) => `overflow:${screen}`;

function boxItem(s: ScreenDocument, b: Placed, style: ScreenStyle): ItemSpec {
  const beyond = b.y >= style.deviceHeight;
  const common = {
    id: `${s.id}/${b.id}`,
    x: s.x + b.x,
    y: s.y + b.y,
    w: b.w,
    z: 2,
    ...(beyond ? {} : { frame: frameId(s.id) }),
  };
  if (b.kind === "text" && b.text !== undefined) {
    return {
      kind: "text",
      ...common,
      text: b.text,
      size: style.theme.fontSize,
      color: beyond ? style.overflowText : style.text,
    };
  }
  return {
    kind: "rect",
    ...common,
    h: b.h,
    radius: 8,
    fill: beyond ? style.overflowBox : b.kind === "placeholder" ? style.placeholder : style.box,
    stroke: { color: beyond ? style.overflowStroke : style.boxStroke, width: 1 },
    ...(b.text === undefined ? {} : { label: b.text }),
  };
}

/** The scene items of one laid-out screen. */
function screenItems(
  s: ScreenDocument,
  boxes: readonly Placed[],
  contentHeight: number,
  style: ScreenStyle,
): ItemSpec[] {
  const { width } = style.theme;
  const items: ItemSpec[] = [
    {
      kind: "frame",
      id: frameId(s.id),
      name: s.title,
      x: s.x,
      y: s.y,
      w: width,
      h: style.deviceHeight,
      radius: 32,
      fill: style.frame,
      stroke: { color: style.frameStroke, width: 2 },
      shadow: true,
      z: 1,
    },
  ];
  const overflow = Math.max(0, contentHeight - style.deviceHeight);
  if (overflow > 0) {
    items.push({
      kind: "rect",
      id: overflowId(s.id),
      x: s.x,
      y: s.y + style.deviceHeight,
      w: width,
      h: overflow,
      radius: 0,
      fill: style.overflowFill,
      stroke: { color: style.overflowStroke, width: 1 },
      z: 1,
    });
  }
  items.push(...boxes.map((b) => boxItem(s, b, style)));
  return items;
}

/** Lays `screens` out with `engine` and returns the ops that replace the scene with them. */
export function layoutScreens(
  screens: readonly ScreenDocument[],
  style: ScreenStyle,
  engine: ParagraphEngine,
): { ops: SceneOp[]; heights: ScreenHeight[] } {
  const ops: SceneOp[] = [{ op: "clear" }];
  const heights: ScreenHeight[] = [];
  for (const s of screens) {
    const { boxes, contentHeight } = layoutScreen(s.root, { engine, theme: style.theme, title: s.title });
    heights.push({ id: s.id, contentHeight });
    for (const item of screenItems(s, boxes, contentHeight, style)) {
      ops.push({ op: "add", item });
    }
  }
  return { ops, heights };
}
