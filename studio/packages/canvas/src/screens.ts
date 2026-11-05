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
  type IconPaint,
  layoutScreen,
  type NodePaint,
  type ParagraphEngine,
  type Placed,
  type PlacedRun,
  TextDirection,
  type ViewPadding,
} from "@plux/layout";
import { chromeItems } from "./chrome.ts";
import type {
  ClipSpec,
  DeviceChromeSpec,
  IconNames,
  ItemSpec,
  RectSpec,
  SceneOp,
  ScreenDocument,
  ScreenHeight,
  ScreenMove,
  ScreenStyle,
  SidesSpec,
  TextRunSpec,
} from "./protocol.ts";
import { moveSpec } from "./scene.ts";

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

/** The style fields of a screen text's item: the style it was measured in, else the theme's. */
function textStyleOf(b: Placed, style: ScreenStyle) {
  const t = b.textStyle;
  if (t === undefined) {
    return { size: style.theme.fontSize, families: style.theme.fontFamilies };
  }
  return {
    size: t.size,
    families: t.family === undefined ? style.theme.fontFamilies : [t.family, ...style.theme.fontFamilies],
    ...(t.weight === 400 ? {} : { weight: t.weight }),
    ...(t.italic ? { italic: true } : {}),
    ...(t.letterSpacing === 0 ? {} : { letterSpacing: t.letterSpacing }),
    height: t.height,
    ...(t.decoration === undefined ? {} : { decoration: t.decoration }),
    ...(t.align === undefined ? {} : { align: t.align }),
    ...(t.maxLines === undefined ? {} : { maxLines: t.maxLines }),
    ...(t.overflow === undefined ? {} : { overflow: t.overflow }),
    ...(t.spans === undefined ? {} : { spans: t.spans.map(runOf) }),
  };
}

/** A rich text's run as the canvas draws it; a run without a colour takes the text's. */
function runOf(r: PlacedRun): TextRunSpec {
  return {
    text: r.text,
    size: r.size,
    weight: r.weight,
    italic: r.italic,
    letterSpacing: r.letterSpacing,
    height: r.height,
    ...(r.color === undefined ? {} : { color: r.color }),
    ...(r.decoration === undefined ? {} : { decoration: r.decoration }),
    ...(r.family === undefined ? {} : { family: r.family }),
  };
}

/** Icon names as glyphs: the code point and families of an icon, and whether right-to-left text mirrors it. */
interface IconGlyphs {
  glyph(
    icon: IconPaint,
  ): { readonly text: string; readonly families: readonly string[]; readonly mirrored: boolean } | undefined;
}

function iconGlyphs(icons: IconNames): IconGlyphs {
  const mirrored = new Set(icons.mirrored);
  return {
    glyph(icon) {
      const point = icons[icon.set][icon.name];
      return point === undefined
        ? undefined
        : {
            text: String.fromCodePoint(point),
            families: icons.families[icon.set],
            mirrored: mirrored.has(`${icon.set} ${icon.name}`),
          };
    },
  };
}

/**
 * An icon as Flutter's `Icon` draws it: its glyph at its size, a line one size high, centred in the icon's box; an
 * empty box over the bounds selects it.
 */
function iconItems(
  s: ScreenDocument,
  b: Placed,
  paint: NodePaint,
  icon: IconPaint,
  glyph: { readonly text: string; readonly families: readonly string[]; readonly mirrored: boolean },
  style: ScreenStyle,
): ItemSpec[] {
  const frame = b.y >= style.deviceHeight ? {} : { frame: frameId(s.id) };
  const z = PAINT_Z + paint.order;
  const faded = paint.opacity < 1 ? { opacity: paint.opacity } : {};
  return [
    { kind: "rect", id: `${s.id}/${b.id}`, x: s.x + b.x, y: s.y + b.y, w: b.w, h: b.h, radius: 0, z, ...frame },
    {
      kind: "text",
      id: `${s.id}/${b.id}${PAINT_SUFFIX}glyph`,
      x: s.x + b.x + (b.w - icon.size) / 2,
      y: s.y + b.y + (b.h - icon.size) / 2,
      w: icon.size,
      text: glyph.text,
      size: icon.size,
      height: 1,
      align: "center",
      color: icon.color,
      families: glyph.families,
      ...(glyph.mirrored && style.textDirection === TextDirection.rtl ? { mirror: true } : {}),
      ...faded,
      ...clipsOf(s, paint),
      z: z + 0.5,
      ...frame,
    },
  ];
}

/** The image key of an app asset: the host sets its bytes under it (`setImages`). */
export const assetImageKey = (asset: string): string => `asset:${asset}`;

/**
 * A box's image as the canvas draws it: an asset by its key, fitted, over the surface container while it loads; a
 * network image is the device's to load, so the box shows the placeholder only.
 */
function imageOf(box: BoxPaint, style: ScreenStyle): Partial<RectSpec> {
  const image = box.image;
  if (image === undefined) {
    return {};
  }
  const placeholder = box.fill === undefined ? { placeholder: style.box } : {};
  return {
    ...(image.asset === undefined ? {} : { image: assetImageKey(image.asset) }),
    imageFit: { fit: image.fit, alignX: image.alignX, alignY: image.alignY, ...placeholder },
  };
}

/** A widget's glyph over the boxes it paints (a checkbox's check over its box). */
function glyphItems(
  s: ScreenDocument,
  b: Placed,
  paint: NodePaint,
  icon: IconPaint,
  glyph: { readonly text: string; readonly families: readonly string[]; readonly mirrored: boolean },
  style: ScreenStyle,
): ItemSpec[] {
  const frame = b.y >= style.deviceHeight ? {} : { frame: frameId(s.id) };
  return [
    ...iconItems(s, b, paint, icon, glyph, style),
    ...paint.boxes.map((box, i) => paintedBox(s, b, paint, box, i, frame, style)),
  ];
}

/** One painted box of a widget as a scene item. */
function paintedBox(
  s: ScreenDocument,
  b: Placed,
  paint: NodePaint,
  box: BoxPaint,
  i: number,
  frame: object,
  style: ScreenStyle,
): ItemSpec {
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
    ...(box.arc === undefined ? {} : { arc: box.arc }),
    ...imageOf(box, style),
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
function paintedItems(
  s: ScreenDocument,
  b: Placed,
  paint: NodePaint,
  style: ScreenStyle,
  glyphs?: IconGlyphs,
): ItemSpec[] {
  if (paint.role === "hidden") {
    return [];
  }
  const glyph = paint.icon === undefined ? undefined : glyphs?.glyph(paint.icon);
  if (paint.icon !== undefined && glyph !== undefined) {
    return glyphItems(s, b, paint, paint.icon, glyph, style);
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
        ...textStyleOf(b, style),
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
    ...paint.boxes.map((box, i) => paintedBox(s, b, paint, box, i, frame, style)),
    ...webImageGlyphs(s, b, paint, style, glyphs, frame),
    ...partItems(s, b, paint, style, glyphs, frame),
  ];
}

/**
 * A text a component laid out itself (`<id>#p<n>`, a Layer 2 component's), drawn in the style it was measured in and
 * in its owner's colour for titles or the rest; undefined for any other box.
 */
function textPartItem(
  s: ScreenDocument,
  b: Placed,
  paints: ReadonlyMap<string, NodePaint>,
  style: ScreenStyle,
): ItemSpec | undefined {
  const cut = typeof b.id === "string" ? b.id.indexOf("#p") : -1;
  if (cut < 0 || b.kind !== "text" || b.text === undefined || b.textStyle === undefined) {
    return undefined;
  }
  const owner = paints.get(b.id.slice(0, cut));
  const colors = owner?.partColors;
  const color = colors === undefined ? style.text : b.textStyle.size >= 16 ? colors.title : colors.body;
  return {
    kind: "text",
    id: `${s.id}/${b.id}`,
    x: s.x + b.x,
    y: s.y + b.y,
    w: b.w,
    text: b.text,
    ...textStyleOf(b, style),
    color,
    ...(owner === undefined ? {} : { z: PAINT_Z + owner.order + 0.5, ...clipsOf(s, owner) }),
    ...(b.y >= style.deviceHeight ? {} : { frame: frameId(s.id) }),
    z: PAINT_Z + (owner?.order ?? 0) + 0.5,
  } as ItemSpec;
}

/** The texts and icons a widget places itself (a field's label, hint and icons), over its boxes. */
function partItems(
  s: ScreenDocument,
  b: Placed,
  paint: NodePaint,
  style: ScreenStyle,
  glyphs: IconGlyphs | undefined,
  frame: object,
): ItemSpec[] {
  const z = PAINT_Z + paint.order + 0.5;
  const fade = { ...(paint.opacity < 1 ? { opacity: paint.opacity } : {}), ...clipsOf(s, paint) };
  const texts = (paint.texts ?? []).map(
    (t, i): ItemSpec => ({
      kind: "text",
      id: `${s.id}/${b.id}${PAINT_SUFFIX}t${i}`,
      x: s.x + t.x,
      y: s.y + t.y,
      w: t.w,
      text: t.text,
      size: t.style.size,
      families: t.style.family === undefined ? style.theme.fontFamilies : [t.style.family, ...style.theme.fontFamilies],
      ...(t.style.weight === 400 ? {} : { weight: t.style.weight }),
      ...(t.style.letterSpacing === 0 ? {} : { letterSpacing: t.style.letterSpacing }),
      height: t.style.height,
      maxLines: 1,
      color: t.color,
      ...fade,
      z,
      ...frame,
    }),
  );
  const icons = (paint.glyphs ?? []).flatMap((g, i): ItemSpec[] => {
    const glyph = glyphs?.glyph(g);
    return glyph === undefined
      ? []
      : [
          {
            kind: "text",
            id: `${s.id}/${b.id}${PAINT_SUFFIX}g${i}`,
            x: s.x + g.x + (g.w - g.size) / 2,
            y: s.y + g.y + (g.h - g.size) / 2,
            w: g.size,
            text: glyph.text,
            size: g.size,
            height: 1,
            align: "center",
            color: g.color,
            families: glyph.families,
            ...fade,
            z,
            ...frame,
          },
        ];
  });
  const lines = (paint.lines ?? []).map(
    (l, i): ItemSpec => ({
      kind: "link",
      id: `${s.id}/${b.id}${PAINT_SUFFIX}l${i}`,
      from: { x: s.x + l.x1, y: s.y + l.y1 },
      to: { x: s.x + l.x2, y: s.y + l.y2 },
      stroke: { color: l.color, width: l.width },
      z,
    }),
  );
  return [...texts, ...icons, ...lines];
}

/** A web image's placeholder carries an image glyph: the canvas does not load it (the device does). */
function webImageGlyphs(
  s: ScreenDocument,
  b: Placed,
  paint: NodePaint,
  style: ScreenStyle,
  glyphs: IconGlyphs | undefined,
  frame: object,
): ItemSpec[] {
  return paint.boxes.flatMap((box, i): ItemSpec[] => {
    const size = Math.min(24, box.w * 0.5, box.h * 0.5);
    const glyph =
      box.image?.url === undefined || size < 8
        ? undefined
        : glyphs?.glyph({ name: "image", set: "material", size, color: style.text });
    if (glyph === undefined) {
      return [];
    }
    return [
      {
        kind: "text",
        id: `${s.id}/${b.id}${PAINT_SUFFIX}${i}web`,
        x: s.x + box.x + (box.w - size) / 2,
        y: s.y + box.y + (box.h - size) / 2,
        w: size,
        text: glyph.text,
        size,
        height: 1,
        align: "center",
        color: ((style.text & 0xffffff00) | 0x8a) >>> 0,
        families: glyph.families,
        ...clipsOf(s, paint),
        z: PAINT_Z + paint.order + 0.5,
        ...frame,
      },
    ];
  });
}

/** The corner radius of a device frame without chrome: a generic phone's. */
export const DEFAULT_FRAME_RADIUS = 32;

/** The header above a frame (PC-11): the screen's name at the start, its route and kind after it, muted. */
function headerItems(s: ScreenDocument, style: ScreenStyle): ItemSpec[] {
  const { width } = style.theme;
  if (style.title === undefined || s.title === "") {
    return [];
  }
  const meta = s.meta === undefined || s.meta === "" ? undefined : s.meta;
  // The name takes what the meta leaves, at least half the width; the meta ends at the frame's end.
  const metaWidth = meta === undefined ? 0 : Math.min(width * 0.5, meta.length * 7 + 8);
  const items: ItemSpec[] = [
    {
      kind: "text",
      id: titleId(s.id),
      x: s.x + 4,
      y: s.y - 30,
      w: width - 8 - metaWidth,
      text: s.title,
      size: 15,
      weight: "bold",
      color: style.title,
      maxLines: 1,
      families: style.theme.fontFamilies,
      z: 2,
    },
  ];
  if (meta !== undefined) {
    items.push({
      kind: "text",
      id: `${titleId(s.id)}${META_SUFFIX}`,
      x: s.x + width - 4 - metaWidth,
      y: s.y - 28,
      w: metaWidth,
      text: meta,
      size: 12,
      color: style.meta ?? style.title,
      align: "end",
      maxLines: 1,
      families: style.theme.fontFamilies,
      z: 2,
    });
  }
  return items;
}

/** The suffix of the scene id of a screen's route and kind after its name; a hit on it is a hit on the header. */
export const META_SUFFIX = "#meta";

/** The scene items of one laid-out screen. */
function screenItems(
  s: ScreenDocument,
  boxes: readonly Placed[],
  contentHeight: number,
  style: ScreenStyle,
  glyphs?: IconGlyphs,
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
      radius: style.chrome?.radius ?? DEFAULT_FRAME_RADIUS,
      fill: style.frame,
      stroke: { color: style.frameStroke, width: 2 },
      shadow: true,
      z: 1,
    },
    ...headerItems(s, style),
  ];
  if (style.chrome !== undefined) {
    items.push(
      ...chromeItems(
        s,
        style.chrome,
        frameId(s.id),
        { width, height: style.deviceHeight },
        boxes,
        style.frame,
        style.theme.fontFamilies,
        style.textDirection === TextDirection.rtl,
      ),
    );
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
  const paints = new Map(boxes.flatMap((b) => (b.paint === undefined ? [] : [[b.id, b.paint] as const])));
  const radius = style.chrome?.radius ?? DEFAULT_FRAME_RADIUS;
  const corners = { x: s.x, y: s.y, w: width, h: style.deviceHeight, r: radius };
  for (const b of boxes) {
    items.push(...boxItems(s, b, paints, style, glyphs).map((item) => inCorners(item, corners)));
  }
  return items;
}

/**
 * A page's box that reaches into a corner of its device frame is clipped to the frame's rounded corners, as the
 * display cuts it (an app bar's or an image's full-bleed fill); the others keep the batched path.
 */
function inCorners(item: ItemSpec, f: { x: number; y: number; w: number; h: number; r: number }): ItemSpec {
  if (item.kind !== "rect" || item.frame === undefined || f.r <= 0) {
    return item;
  }
  const left = item.x < f.x + f.r;
  const right = item.x + item.w > f.x + f.w - f.r;
  const top = item.y < f.y + f.r;
  const bottom = item.y + item.h > f.y + f.h - f.r;
  if (!((left || right) && (top || bottom))) {
    return item;
  }
  const clip: ClipSpec = { x: f.x, y: f.y, w: f.w, h: f.h, radii: [f.r, f.r, f.r, f.r] };
  return { ...item, clips: [clip, ...(item.clips ?? [])] };
}

/** The scene items of one laid-out box of a screen. */
function boxItems(
  s: ScreenDocument,
  b: Placed,
  paints: ReadonlyMap<string, NodePaint>,
  style: ScreenStyle,
  glyphs: IconGlyphs | undefined,
): ItemSpec[] {
  const part = style.paint === undefined ? undefined : textPartItem(s, b, paints, style);
  if (part !== undefined) {
    return [part];
  }
  if (b.paint !== undefined) {
    return paintedItems(s, b, b.paint, style, glyphs);
  }
  // On a painted screen a box no widget of the document owns — a part the scene built in a widget's place
  // (`Container`'s padding, a destination's icon) — is neither drawn nor selected; its widget draws it.
  if (style.paint !== undefined && (typeof b.id !== "string" || b.id.includes("#") || b.kind !== "text")) {
    return [];
  }
  const label = labelItem(s, b, style);
  return [boxItem(s, b, style), ...(label === null ? [] : [label])];
}

/** Lays `screens` out with `engine` and returns the ops that replace the scene with them. */
export function layoutScreens(
  screens: readonly ScreenDocument[],
  style: ScreenStyle,
  engine: ParagraphEngine,
): { ops: SceneOp[]; heights: ScreenHeight[] } {
  return new ScreenLayouts().replaceAll(screens, style, engine);
}

function layOut(s: ScreenDocument, style: ScreenStyle, engine: ParagraphEngine, glyphs?: IconGlyphs): Laid {
  try {
    return layOutContent(s, style, engine, glyphs);
  } catch {
    // A screen the layout cannot lay out keeps its frame and title; the others are drawn as ever.
    return { items: screenItems(s, [], 0, style, glyphs), height: 0 };
  }
}

/** A laid-out screen: its items, its content height and, live, its scroll views' extents. */
interface Laid {
  readonly items: ItemSpec[];
  readonly height: number;
  readonly scrolls?: readonly { readonly node: string; readonly max: number }[];
}

/** A screen's height entry, with its scroll extents when it has any. */
function heightOf(id: string, laid: Laid): ScreenHeight {
  return laid.scrolls === undefined || laid.scrolls.length === 0
    ? { id, contentHeight: laid.height }
    : { id, contentHeight: laid.height, scrolls: laid.scrolls };
}

function layOutContent(s: ScreenDocument, style: ScreenStyle, engine: ParagraphEngine, glyphs?: IconGlyphs): Laid {
  const { boxes, contentHeight, scrolls } = layoutScreen(s.root, {
    engine,
    theme: style.theme,
    title: s.title,
    deviceHeight: style.deviceHeight,
    ...(style.textScale === undefined ? {} : { textScale: style.textScale }),
    ...(style.textDirection === undefined ? {} : { textDirection: style.textDirection }),
    ...(style.paint === undefined ? {} : { paint: { ...style.paint, rtl: style.textDirection === TextDirection.rtl } }),
    ...(style.chrome === undefined ? {} : { viewPadding: viewPaddingOf(style.chrome) }),
    ...(style.live === true ? { live: true } : {}),
  });
  return {
    items: screenItems(s, boxes, contentHeight, style, glyphs),
    height: contentHeight,
    ...(scrolls === undefined ? {} : { scrolls }),
  };
}

/** The insets the device's system areas take from the page (`MediaQuery.padding`). */
function viewPaddingOf(chrome: DeviceChromeSpec): ViewPadding {
  return { top: chrome.top, right: chrome.right, bottom: chrome.bottom, left: chrome.left };
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
  private readonly glyphs: IconGlyphs | undefined;

  constructor(icons?: IconNames) {
    this.glyphs = icons === undefined ? undefined : iconGlyphs(icons);
  }
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
      const laid = layOut(s, style, engine, this.glyphs);
      const { items } = laid;
      this.laidOut++;
      heights.push(heightOf(s.id, laid));
      this.byScreen.set(s.id, new Map(items.map((item) => [item.id, item])));
      for (const item of items) {
        ops.push({ op: "add", item });
      }
    }
    return { ops, heights };
  }

  /**
   * Lays out only `screens`, drops `removed` and translates the items of the screens that only `moved`; the ops change
   * only the items that differ.
   */
  update(
    screens: readonly ScreenDocument[],
    removed: readonly string[],
    style: ScreenStyle,
    engine: ParagraphEngine,
    moved: readonly ScreenMove[] = [],
  ): { ops: SceneOp[]; heights: ScreenHeight[] } {
    const ops: SceneOp[] = [];
    const heights: ScreenHeight[] = [];
    for (const id of removed) {
      for (const itemId of this.byScreen.get(id)?.keys() ?? []) {
        ops.push({ op: "remove", id: itemId });
      }
      this.byScreen.delete(id);
    }
    for (const m of moved) {
      const items = this.byScreen.get(m.id);
      if (items === undefined || (m.dx === 0 && m.dy === 0)) {
        continue;
      }
      const after = new Map<string, ItemSpec>();
      for (const [id, item] of items) {
        after.set(id, moveSpec(item, m.dx, m.dy));
        ops.push({ op: "move", id, dx: m.dx, dy: m.dy });
      }
      this.byScreen.set(m.id, after);
    }
    const changes: SceneOp[] = [];
    for (const s of screens) {
      const laid = layOut(s, style, engine, this.glyphs);
      const { items } = laid;
      this.laidOut++;
      heights.push(heightOf(s.id, laid));
      const after = new Map(items.map((item) => [item.id, item]));
      diffItems(this.byScreen.get(s.id) ?? new Map<string, ItemSpec>(), after, ops, changes);
      this.byScreen.set(s.id, after);
    }
    ops.push(...changes);
    return { ops, heights };
  }
}
