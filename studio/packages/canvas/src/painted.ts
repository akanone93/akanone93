// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Drawing what a screen's widgets paint (STU-005, ADR-0013): one box at a time, as Flutter's `BoxPainter` draws a
 * decoration — the box shadows first (blurred, offset, spread), then the fill or gradient, then the border inside the
 * box — clipped to the shapes of the widget's ancestors and faded by their opacity. Boxes that only need a fill and a
 * uniform stroke keep the batched path; these are the ones that cannot be batched.
 */

import type { Canvas, CanvasKit, Image as CkImage, Paint, ParagraphStyle, Shader, TextStyle } from "canvaskit-wasm";
import { color4f } from "./batch.ts";
import type { ClipSpec, ColorRgba, GradientSpec, ImageFitSpec, RectSpec, TextRunSpec, TextSpec } from "./protocol.ts";

/** Whether `spec` needs the painted path: anything a batched rounded rectangle cannot draw. */
export function isPainted(spec: RectSpec): boolean {
  return (
    spec.radii !== undefined ||
    spec.oval === true ||
    spec.gradient !== undefined ||
    spec.shadows !== undefined ||
    spec.sides !== undefined ||
    spec.opacity !== undefined ||
    spec.clips !== undefined ||
    spec.imageFit !== undefined ||
    spec.arc !== undefined
  );
}

/** The decoded image of a key, or null while it is missing. */
export type ImageLookup = (key: string) => CkImage | null;

/**
 * Flutter's `applyBoxFit` and `paintImage`: the part of an image of `iw` by `ih` drawn, and where in the box, for
 * `fit` and the alignment.
 */
export function fitImage(
  fit: ImageFitSpec["fit"],
  iw: number,
  ih: number,
  box: { x: number; y: number; w: number; h: number },
  alignX: number,
  alignY: number,
): { src: [number, number, number, number]; dst: [number, number, number, number] } {
  const [sw, sh, dw, dh] = boxFit(fit, iw, ih, box.w, box.h);
  const halfW = (box.w - dw) / 2;
  const halfH = (box.h - dh) / 2;
  const dx = box.x + halfW + alignX * halfW;
  const dy = box.y + halfH + alignY * halfH;
  const sx = (iw - sw) / 2 + (alignX * (iw - sw)) / 2;
  const sy = (ih - sh) / 2 + (alignY * (ih - sh)) / 2;
  return { src: [sx, sy, sx + sw, sy + sh], dst: [dx, dy, dx + dw, dy + dh] };
}

/** `applyBoxFit`: the source size taken from the image and the destination size it fills, `[sw, sh, dw, dh]`. */
type Fitted = readonly [sw: number, sh: number, dw: number, dh: number];

function boxFit(fit: ImageFitSpec["fit"], iw: number, ih: number, w: number, h: number): Fitted {
  switch (fit) {
    case "fill":
      return [iw, ih, w, h];
    case "contain":
      return w / h > iw / ih ? [iw, ih, (iw * h) / ih, h] : [iw, ih, w, (ih * w) / iw];
    case "cover":
      return w / h > iw / ih ? [iw, (iw * h) / w, w, h] : [(ih * w) / h, ih, w, h];
    case "fitWidth":
      return w / h > iw / ih ? [iw, (iw * h) / w, w, h] : [iw, ih, w, (ih * w) / iw];
    case "fitHeight":
      return w / h > iw / ih ? [iw, ih, (iw * h) / ih, h] : [(ih * w) / h, ih, w, h];
    case "none": {
      const sw = Math.min(iw, w);
      const sh = Math.min(ih, h);
      return [sw, sh, sw, sh];
    }
    default: {
      const [, , cw, ch] = boxFit("contain", iw, ih, w, h);
      return cw > iw || ch > ih ? [iw, ih, iw, ih] : [iw, ih, cw, ch];
    }
  }
}

/** The image of `spec`, fitted, clipped to the box's shape and faded with it. */
function drawImage(
  ck: CanvasKit,
  canvas: Canvas,
  spec: RectSpec,
  corners: Corners | "oval",
  paint: Paint,
  opacity: number,
  images: ImageLookup | undefined,
): number {
  const fill = spec.imageFit;
  const image = fill === undefined || spec.image === undefined ? null : (images?.(spec.image) ?? null);
  if (fill === undefined) {
    return 0;
  }
  if (image === null) {
    if (fill.placeholder === undefined) {
      return 0;
    }
    paint.setColor(color4f(ck, faded(fill.placeholder, opacity)));
    drawShape(canvas, spec, corners, paint);
    return 1;
  }
  const { src, dst } = fitImage(fill.fit, image.width(), image.height(), spec, fill.alignX, fill.alignY);
  canvas.save();
  canvas.clipRRect(rrect(spec.x, spec.y, spec.w, spec.h, corners), ck.ClipOp.Intersect, true);
  paint.setColor(color4f(ck, faded(0x000000ff, opacity)));
  canvas.drawImageRectOptions(
    image,
    ck.LTRBRect(...src),
    ck.LTRBRect(...dst),
    ck.FilterMode.Linear,
    ck.MipmapMode.None,
    paint,
  );
  canvas.restore();
  return 1;
}

/** Flutter's `Shadow.convertRadiusToSigma`: the blur radius of a box shadow as a Gaussian sigma. */
export function blurSigma(radius: number): number {
  return radius > 0 ? radius * 0.57735 + 0.5 : 0;
}

/** `rgba` with its alpha multiplied by `opacity`. */
export function faded(rgba: ColorRgba, opacity: number): ColorRgba {
  return opacity >= 1 ? rgba : ((rgba & 0xffffff00) | Math.round((rgba & 0xff) * Math.max(0, opacity))) >>> 0;
}

type Corners = readonly [number, number, number, number];

/** A rounded rectangle in CanvasKit's layout: the rectangle, then x and y radii clockwise from the top left. */
function rrect(x: number, y: number, w: number, h: number, corners: Corners | "oval"): Float32Array {
  const [tl, tr, br, bl] = corners === "oval" ? [w / 2, w / 2, w / 2, w / 2] : corners;
  const ry = (r: number) => (corners === "oval" ? h / 2 : r);
  return Float32Array.of(x, y, x + w, y + h, tl, ry(tl), tr, ry(tr), br, ry(br), bl, ry(bl));
}

function cornersOf(spec: { radii?: Corners; oval?: boolean; radius?: number }): Corners | "oval" {
  if (spec.oval === true) {
    return "oval";
  }
  if (spec.radii !== undefined) {
    return spec.radii;
  }
  const r = spec.radius ?? 0;
  return [r, r, r, r];
}

/** Grows (or, for a negative `by`, shrinks) every corner with the box, as a shadow's spread does. */
function grown(corners: Corners | "oval", by: number): Corners | "oval" {
  return corners === "oval" ? corners : (corners.map((r) => Math.max(0, r + by)) as unknown as Corners);
}

/** Clips `canvas` to `clips`, each in world units. */
export function clipTo(ck: CanvasKit, canvas: Canvas, clips: readonly ClipSpec[] | undefined): void {
  for (const c of clips ?? []) {
    canvas.clipRRect(rrect(c.x, c.y, c.w, c.h, cornersOf(c)), ck.ClipOp.Intersect, true);
  }
}

function gradientShader(ck: CanvasKit, g: GradientSpec, opacity: number): Shader {
  const colors = g.colors.map((c) => color4f(ck, faded(c, opacity)));
  const stops = g.stops === undefined ? null : [...g.stops];
  switch (g.kind) {
    case "radial":
      return ck.Shader.MakeRadialGradient(
        [g.from[0], g.from[1]],
        Math.max(0.0001, g.radius),
        colors,
        stops,
        ck.TileMode.Clamp,
      );
    case "sweep":
      return ck.Shader.MakeSweepGradient(g.from[0], g.from[1], colors, stops, ck.TileMode.Clamp);
    default:
      return ck.Shader.MakeLinearGradient([g.from[0], g.from[1]], [g.to[0], g.to[1]], colors, stops, ck.TileMode.Clamp);
  }
}

function drawShape(
  canvas: Canvas,
  spec: RectSpec,
  corners: Corners | "oval",
  paint: Paint,
  inset = 0,
  dx = 0,
  dy = 0,
): void {
  canvas.drawRRect(
    rrect(spec.x + inset + dx, spec.y + inset + dy, spec.w - 2 * inset, spec.h - 2 * inset, grown(corners, -inset)),
    paint,
  );
}

function drawShadows(
  ck: CanvasKit,
  canvas: Canvas,
  spec: RectSpec,
  corners: Corners | "oval",
  paint: Paint,
  opacity: number,
): void {
  for (const s of spec.shadows ?? []) {
    const filter = s.blur > 0 ? ck.MaskFilter.MakeBlur(ck.BlurStyle.Normal, blurSigma(s.blur), true) : null;
    paint.setColor(color4f(ck, faded(s.color, opacity)));
    paint.setMaskFilter(filter);
    drawShape(canvas, spec, corners, paint, -s.spread, s.dx, s.dy);
    paint.setMaskFilter(null);
    filter?.delete();
  }
}

function drawFill(
  ck: CanvasKit,
  canvas: Canvas,
  spec: RectSpec,
  corners: Corners | "oval",
  paint: Paint,
  opacity: number,
): void {
  if (spec.gradient !== undefined) {
    const shader = gradientShader(ck, spec.gradient, opacity);
    paint.setColor(color4f(ck, 0x000000ff));
    paint.setShader(shader);
    drawShape(canvas, spec, corners, paint);
    paint.setShader(null);
    shader.delete();
  } else if (spec.fill !== undefined && spec.arc !== undefined) {
    drawArc(ck, canvas, spec, spec.arc, paint, faded(spec.fill, opacity));
  } else if (spec.fill !== undefined) {
    paint.setColor(color4f(ck, faded(spec.fill, opacity)));
    drawShape(canvas, spec, corners, paint);
  }
}

/** An arc of the box's oval: a filled wedge, or a stroke of `arc.width` centred on the oval inset by half of it. */
function drawArc(
  ck: CanvasKit,
  canvas: Canvas,
  spec: RectSpec,
  arc: NonNullable<RectSpec["arc"]>,
  paint: Paint,
  color: ColorRgba,
): void {
  paint.setColor(color4f(ck, color));
  const inset = arc.width === undefined ? 0 : arc.width / 2;
  const oval = ck.LTRBRect(spec.x + inset, spec.y + inset, spec.x + spec.w - inset, spec.y + spec.h - inset);
  if (arc.width !== undefined) {
    paint.setStyle(ck.PaintStyle.Stroke);
    paint.setStrokeWidth(arc.width);
  }
  canvas.drawArc(oval, arc.start, arc.sweep, arc.width === undefined, paint);
  paint.setStyle(ck.PaintStyle.Fill);
}

/** The border inside the box: a uniform stroke along its shape, or a strip per side. */
function drawBorder(
  ck: CanvasKit,
  canvas: Canvas,
  spec: RectSpec,
  corners: Corners | "oval",
  paint: Paint,
  opacity: number,
): void {
  if (spec.stroke !== undefined && spec.stroke.width > 0) {
    paint.setStyle(ck.PaintStyle.Stroke);
    paint.setStrokeWidth(spec.stroke.width);
    paint.setColor(color4f(ck, faded(spec.stroke.color, opacity)));
    drawShape(canvas, spec, corners, paint, spec.stroke.width / 2);
    paint.setStyle(ck.PaintStyle.Fill);
  }
  const sides = spec.sides;
  if (sides === undefined) {
    return;
  }
  const strip = (side: { color: ColorRgba; width: number } | undefined, x: number, y: number, w: number, h: number) => {
    if (side !== undefined && side.width > 0) {
      paint.setColor(color4f(ck, faded(side.color, opacity)));
      canvas.drawRect(ck.LTRBRect(x, y, x + w, y + h), paint);
    }
  };
  strip(sides.top, spec.x, spec.y, spec.w, sides.top?.width ?? 0);
  strip(sides.bottom, spec.x, spec.y + spec.h - (sides.bottom?.width ?? 0), spec.w, sides.bottom?.width ?? 0);
  strip(sides.left, spec.x, spec.y, sides.left?.width ?? 0, spec.h);
  strip(sides.right, spec.x + spec.w - (sides.right?.width ?? 0), spec.y, sides.right?.width ?? 0, spec.h);
}

/**
 * Draws one painted box with `paint`, a scratch paint the caller owns, its image from `images`; returns the draw calls
 * made.
 */
export function drawPainted(ck: CanvasKit, canvas: Canvas, spec: RectSpec, paint: Paint, images?: ImageLookup): number {
  const opacity = spec.opacity ?? 1;
  const corners = cornersOf(spec);
  canvas.save();
  clipTo(ck, canvas, spec.clips);
  paint.setAntiAlias(true);
  paint.setStyle(ck.PaintStyle.Fill);
  drawShadows(ck, canvas, spec, corners, paint, opacity);
  drawFill(ck, canvas, spec, corners, paint, opacity);
  const drawn = drawImage(ck, canvas, spec, corners, paint, opacity, images);
  drawBorder(ck, canvas, spec, corners, paint, opacity);
  canvas.restore();
  return 1 + drawn + (spec.shadows?.length ?? 0);
}

/** Whether a text needs clipping or fading before it is drawn. */
export function textNeedsLayer(spec: TextSpec): boolean {
  return spec.clips !== undefined || (spec.opacity !== undefined && spec.opacity < 1);
}

const WEIGHTS = [
  "Thin",
  "ExtraLight",
  "Light",
  "Normal",
  "Medium",
  "SemiBold",
  "Bold",
  "ExtraBold",
  "ExtraBlack",
] as const;

const fontWeightName = (weight: TextSpec["weight"]): (typeof WEIGHTS)[number] => {
  const n = weight === "bold" ? 700 : (weight ?? 400);
  return WEIGHTS[Math.min(Math.max(Math.round(n / 100), 1), 9) - 1] ?? "Normal";
};

function decorationOf(ck: CanvasKit, d: TextSpec["decoration"]): number {
  switch (d) {
    case "underline":
      return ck.UnderlineDecoration;
    case "lineThrough":
      return ck.LineThroughDecoration;
    case "overline":
      return ck.OverlineDecoration;
    default:
      return ck.NoDecoration;
  }
}

function alignOf(ck: CanvasKit, a: TextSpec["align"]) {
  switch (a) {
    case "center":
      return ck.TextAlign.Center;
    case "end":
      return ck.TextAlign.End;
    case "justify":
      return ck.TextAlign.Justify;
    default:
      return ck.TextAlign.Start;
  }
}

/** Lines past `maxLines` are cut, with an ellipsis unless the text asks to clip or fade. */
function linesOf(spec: TextSpec): Pick<ParagraphStyle, "maxLines" | "ellipsis"> {
  if (spec.maxLines === undefined) {
    return {};
  }
  return spec.overflow === undefined || spec.overflow === "ellipsis"
    ? { maxLines: spec.maxLines, ellipsis: "…" }
    : { maxLines: spec.maxLines };
}

type TextFields = Pick<TextSpec, "size" | "color" | "weight" | "italic" | "letterSpacing" | "height" | "decoration">;

/** A text style of the fields a text or one of its runs sets. */
function textStyleOf(ck: CanvasKit, t: TextFields, opacity: number, fontFamilies: string[]): TextStyle {
  const color = color4f(ck, faded(t.color, opacity));
  return {
    color,
    fontSize: t.size,
    fontFamilies,
    fontStyle: {
      weight: ck.FontWeight[fontWeightName(t.weight)],
      slant: t.italic === true ? ck.FontSlant.Italic : ck.FontSlant.Upright,
    },
    ...(t.letterSpacing === undefined ? {} : { letterSpacing: t.letterSpacing }),
    ...(t.height === undefined ? {} : { heightMultiplier: t.height, halfLeading: true }),
    decoration: decorationOf(ck, t.decoration),
    decorationColor: color,
  };
}

/** The style of one run of a rich text: the text's style with what the run sets. */
export function runStyleOf(ck: CanvasKit, spec: TextSpec, run: TextRunSpec, fontFamilies: string[]): TextStyle {
  const families = run.family === undefined ? fontFamilies : [run.family, ...fontFamilies];
  const own = Object.fromEntries(
    Object.entries(run).filter(([k, v]) => k !== "text" && k !== "family" && v !== undefined),
  );
  return textStyleOf(ck, { ...spec, ...own } as TextFields, spec.opacity ?? 1, families);
}

/** A text item's paragraph style: alignment, lines and ellipsis, and its text style as it was measured (STU-006). */
export function paragraphStyleOf(ck: CanvasKit, spec: TextSpec, fontFamilies: string[]): ParagraphStyle {
  return {
    textAlign: alignOf(ck, spec.align),
    ...linesOf(spec),
    textStyle: textStyleOf(ck, spec, spec.opacity ?? 1, fontFamilies),
  };
}
