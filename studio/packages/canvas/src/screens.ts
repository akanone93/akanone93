// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Screens laid out in the canvas worker (ADR-0013 Threading, Text): each
 * screen document goes through `@plux/layout` with the SkParagraph engine, and
 * the laid-out boxes become scene items — the device frame, the boxes inside
 * it, and a translucent region below the frame for content taller than the
 * device (STU-056).
 */

import {
  type BorderPaint,
  type BoxPaint,
  layoutScreen,
  type NodePaint,
  type ParagraphEngine,
  type Placed,
  TextDirection,
} from "@plux/layout";
import type { ClipSpec, ItemSpec, SceneOp, ScreenDocument, ScreenHeight, ScreenStyle, SidesSpec } from "./protocol.ts";

/** The scene id of a screen's device frame. */
export const frameId = (screen: string) => `screen:${screen}`;
/** The scene id of a screen's name above its frame. */
export const titleId = (screen: string) => `title:${screen}`;
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
      families: style.theme.fontFamilies,
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

/** The suffix of the scene id of a labelled box's label; a hit on the label is a hit on its box. */
export const LABEL_SUFFIX = "#label";

/**
 * The label of a labelled box (a button's text, an input's hint, a placeholder's widget type), centred in it on one
 * line at the theme's control text size; a box too small to read a label in gets none.
 */
function labelItem(s: ScreenDocument, b: Placed, style: ScreenStyle): ItemSpec | null {
  const size = Math.round(style.theme.fontSize * 0.875);
  if (b.kind === "text" || b.text === undefined || b.text === "" || b.h < size * 1.5 || b.w < size * 2) {
    return null;
  }
  const beyond = b.y >= style.deviceHeight;
  const pad = Math.min(12, b.w / 8);
  return {
    kind: "text",
    id: `${s.id}/${b.id}${LABEL_SUFFIX}`,
    x: s.x + b.x + pad,
    y: s.y + b.y + (b.h - size * 1.36) / 2,
    w: b.w - 2 * pad,
    text: b.text,
    size,
    color: beyond ? style.overflowText : style.text,
    align: "center",
    maxLines: 1,
    families: style.theme.fontFamilies,
    z: 3,
    ...(beyond ? {} : { frame: frameId(s.id) }),
  };
}

/** The suffix of the scene ids of the boxes a widget paints; a hit on one is a hit on the widget. */
export const PAINT_SUFFIX = "#paint";

/** The depth a painted widget's items are drawn at: its paint order above the frame; foreground decorations above all. */
const PAINT_Z = 2;
const FOREGROUND_Z = 5000;

function clipsOf(s: ScreenDocument, paint: NodePaint): { clips?: ClipSpec[] } {
  return paint.clips.length === 0
    ? {}
    : {
        clips: paint.clips.map((c) => ({
          x: s.x + c.x,
          y: s.y + c.y,
          w: c.w,
          h: c.h,
          ...(c.radii === undefined ? {} : { radii: c.radii }),
          ...(c.oval === true ? { oval: true } : {}),
        })),
      };
}

function sidesOf(border: Extract<BorderPaint, { kind: "sides" }>): SidesSpec {
  const { top, right, bottom, left } = border;
  return {
    ...(top === undefined ? {} : { top }),
    ...(right === undefined ? {} : { right }),
    ...(bottom === undefined ? {} : { bottom }),
    ...(left === undefined ? {} : { left }),
  };
}

/** One painted box of a widget as a scene item. */
function paintedBox(s: ScreenDocument, b: Placed, paint: NodePaint, box: BoxPaint, i: number, frame: object): ItemSpec {
  const border = box.border;
  return {
    kind: "rect",
    id: `${s.id}/${b.id}${PAINT_SUFFIX}${i}`,
    x: s.x + box.x,
    y: s.y + box.y,
    w: box.w,
    h: box.h,
    radius: 0,
    ...(box.radii === undefined ? {} : { radii: box.radii }),
    ...(box.oval === true ? { oval: true } : {}),
    ...(box.fill === undefined ? {} : { fill: box.fill }),
    ...(box.gradient === undefined
      ? {}
      : {
          gradient: {
            ...box.gradient,
            from: [s.x + box.gradient.from[0], s.y + box.gradient.from[1]],
            to: [s.x + box.gradient.to[0], s.y + box.gradient.to[1]],
          },
        }),
    ...(box.shadows === undefined ? {} : { shadows: box.shadows }),
    ...(border === undefined ? {} : border.kind === "all" ? { stroke: border.side } : { sides: sidesOf(border) }),
    ...(paint.opacity < 1 ? { opacity: paint.opacity } : {}),
    ...clipsOf(s, paint),
    z: box.foreground === true ? FOREGROUND_Z + paint.order : PAINT_Z + paint.order,
    ...frame,
  };
}

/**
 * The scene items of a widget laid out with a paint theme (STU-005): the boxes it paints, its text in its colour, a
 * neutral box for a widget not painted yet, and an empty box over its bounds for selection and hit testing.
 */
function paintedItems(s: ScreenDocument, b: Placed, paint: NodePaint, style: ScreenStyle): ItemSpec[] {
  if (paint.role === "hidden") {
    return [];
  }
  const beyond = b.y >= style.deviceHeight;
  const frame = beyond ? {} : { frame: frameId(s.id) };
  const z = PAINT_Z + paint.order;
  const own = { id: `${s.id}/${b.id}`, x: s.x + b.x, y: s.y + b.y, w: b.w, z, ...frame };
  if (paint.role === "text" && b.text !== undefined) {
    return [
      {
        kind: "text",
        ...own,
        text: b.text,
        size: style.theme.fontSize,
        families: style.theme.fontFamilies,
        color: paint.textColor ?? style.text,
        ...(paint.opacity < 1 ? { opacity: paint.opacity } : {}),
        ...clipsOf(s, paint),
      },
    ];
  }
  if (paint.role === "placeholder") {
    const placeholder = boxItem(s, b, style);
    const label = labelItem(s, b, style);
    const faded = { z, ...(paint.opacity < 1 ? { opacity: paint.opacity } : {}), ...clipsOf(s, paint) };
    return [
      { ...placeholder, ...faded } as ItemSpec,
      ...(label === null ? [] : [{ ...label, ...faded, z: z + 0.5 } as ItemSpec]),
    ];
  }
  return [
    { kind: "rect", ...own, h: b.h, radius: 0 },
    ...paint.boxes.map((box, i) => paintedBox(s, b, paint, box, i, frame)),
  ];
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
  if (style.title !== undefined && s.title !== "") {
    items.push({
      kind: "text",
      id: titleId(s.id),
      x: s.x + 4,
      y: s.y - 30,
      w: width - 8,
      text: s.title,
      size: 15,
      weight: "bold",
      color: style.title,
      maxLines: 1,
      families: style.theme.fontFamilies,
      z: 2,
    });
  }
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
  for (const b of boxes) {
    if (b.paint !== undefined) {
      items.push(...paintedItems(s, b, b.paint, style));
      continue;
    }
    items.push(boxItem(s, b, style));
    const label = labelItem(s, b, style);
    if (label !== null) {
      items.push(label);
    }
  }
  return items;
}

/** Lays `screens` out with `engine` and returns the ops that replace the scene with them. */
export function layoutScreens(
  screens: readonly ScreenDocument[],
  style: ScreenStyle,
  engine: ParagraphEngine,
): { ops: SceneOp[]; heights: ScreenHeight[] } {
  return new ScreenLayouts().replaceAll(screens, style, engine);
}

function layOut(s: ScreenDocument, style: ScreenStyle, engine: ParagraphEngine): { items: ItemSpec[]; height: number } {
  const { boxes, contentHeight } = layoutScreen(s.root, {
    engine,
    theme: style.theme,
    title: s.title,
    deviceHeight: style.deviceHeight,
    ...(style.textScale === undefined ? {} : { textScale: style.textScale }),
    ...(style.textDirection === undefined ? {} : { textDirection: style.textDirection }),
    ...(style.paint === undefined ? {} : { paint: { ...style.paint, rtl: style.textDirection === TextDirection.rtl } }),
  });
  return { items: screenItems(s, boxes, contentHeight, style), height: contentHeight };
}

/** Two item specs draw the same: they are plain data from `screenItems`, so their JSON compares them. */
const sameItem = (a: ItemSpec, b: ItemSpec): boolean => JSON.stringify(a) === JSON.stringify(b);

/** Pushes removals of the items gone from `before` to `removals`, and adds and replacements to `changes`. */
function diffItems(
  before: ReadonlyMap<string, ItemSpec>,
  after: ReadonlyMap<string, ItemSpec>,
  removals: SceneOp[],
  changes: SceneOp[],
): void {
  for (const id of before.keys()) {
    if (!after.has(id)) {
      removals.push({ op: "remove", id });
    }
  }
  for (const item of after.values()) {
    const old = before.get(item.id);
    if (old === undefined) {
      changes.push({ op: "add", item });
    } else if (!sameItem(old, item)) {
      changes.push({ op: "replace", item });
    }
  }
}

/**
 * The laid-out screens the worker holds, by screen (STU-121): a full
 * replacement lays every screen out, an update lays out only the screens it
 * names and answers with the ops that turn their old items into the new ones —
 * unchanged items are not touched, so only the tiles under a change redraw.
 */
export class ScreenLayouts {
  private readonly byScreen = new Map<string, ReadonlyMap<string, ItemSpec>>();
  /** How many screens were laid out since construction; tests read it. */
  laidOut = 0;

  /** Lays every screen out; the ops clear the scene first. */
  replaceAll(
    screens: readonly ScreenDocument[],
    style: ScreenStyle,
    engine: ParagraphEngine,
  ): { ops: SceneOp[]; heights: ScreenHeight[] } {
    this.byScreen.clear();
    const ops: SceneOp[] = [{ op: "clear" }];
    const heights: ScreenHeight[] = [];
    for (const s of screens) {
      const { items, height } = layOut(s, style, engine);
      this.laidOut++;
      heights.push({ id: s.id, contentHeight: height });
      this.byScreen.set(s.id, new Map(items.map((item) => [item.id, item])));
      for (const item of items) {
        ops.push({ op: "add", item });
      }
    }
    return { ops, heights };
  }

  /** Lays out only `screens` and drops `removed`; the ops change only the items that differ. */
  update(
    screens: readonly ScreenDocument[],
    removed: readonly string[],
    style: ScreenStyle,
    engine: ParagraphEngine,
  ): { ops: SceneOp[]; heights: ScreenHeight[] } {
    const ops: SceneOp[] = [];
    const heights: ScreenHeight[] = [];
    for (const id of removed) {
      for (const itemId of this.byScreen.get(id)?.keys() ?? []) {
        ops.push({ op: "remove", id: itemId });
      }
      this.byScreen.delete(id);
    }
    const changes: SceneOp[] = [];
    for (const s of screens) {
      const { items, height } = layOut(s, style, engine);
      this.laidOut++;
      heights.push({ id: s.id, contentHeight: height });
      const after = new Map(items.map((item) => [item.id, item]));
      diffItems(this.byScreen.get(s.id) ?? new Map<string, ItemSpec>(), after, ops, changes);
      this.byScreen.set(s.id, after);
    }
    ops.push(...changes);
    return { ops, heights };
  }
}
