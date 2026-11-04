// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Drawing what a screen's widgets paint (STU-005, ADR-0013): one box at a time, as Flutter's `BoxPainter` draws a
 * decoration — the box shadows first (blurred, offset, spread), then the fill or gradient, then the border inside the
 * box — clipped to the shapes of the widget's ancestors and faded by their opacity. Boxes that only need a fill and a
 * uniform stroke keep the batched path; these are the ones that cannot be batched.
 */

import type { Canvas, CanvasKit, Paint, Shader } from "canvaskit-wasm";
import { color4f } from "./batch.ts";
import type { ClipSpec, ColorRgba, GradientSpec, RectSpec, TextSpec } from "./protocol.ts";

/** Whether `spec` needs the painted path: anything a batched rounded rectangle cannot draw. */
export function isPainted(spec: RectSpec): boolean {
  return (
    spec.radii !== undefined ||
    spec.oval === true ||
    spec.gradient !== undefined ||
    spec.shadows !== undefined ||
    spec.sides !== undefined ||
    spec.opacity !== undefined ||
    spec.clips !== undefined
  );
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
  } else if (spec.fill !== undefined) {
    paint.setColor(color4f(ck, faded(spec.fill, opacity)));
    drawShape(canvas, spec, corners, paint);
  }
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

/** Draws one painted box with `paint`, a scratch paint the caller owns; returns the draw calls made. */
export function drawPainted(ck: CanvasKit, canvas: Canvas, spec: RectSpec, paint: Paint): number {
  const opacity = spec.opacity ?? 1;
  const corners = cornersOf(spec);
  canvas.save();
  clipTo(ck, canvas, spec.clips);
  paint.setAntiAlias(true);
  paint.setStyle(ck.PaintStyle.Fill);
  drawShadows(ck, canvas, spec, corners, paint, opacity);
  drawFill(ck, canvas, spec, corners, paint, opacity);
  drawBorder(ck, canvas, spec, corners, paint, opacity);
  canvas.restore();
  return 1 + (spec.shadows?.length ?? 0);
}

/** Whether a text needs clipping or fading before it is drawn. */
export function textNeedsLayer(spec: TextSpec): boolean {
  return spec.clips !== undefined || (spec.opacity !== undefined && spec.opacity < 1);
}
