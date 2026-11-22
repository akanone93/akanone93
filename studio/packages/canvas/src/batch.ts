// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Draw batching (ADR-0013 M-12): geometry is accumulated into one path per
 * layer and style — all links of one stroke style, all boxes of one fill —
 * and drawn with one call each. Builders and paints are created once per
 * style and reused; the path detached for drawing is deleted in the same
 * frame.
 */

import type { Canvas, CanvasKit, Paint, PathBuilder } from "canvaskit-wasm";
import type { ColorRgba, LinkSpec } from "./protocol.ts";
import type { Resources } from "./resources.ts";
import { dotRadius } from "./scene.ts";

/** Draw order of the batch layers. */
export const BatchLayer = {
  /** Contact shadows of boxes (RectSpec `shadow` on the CPU path), under every fill of their depth. */
  shadow: 0,
  frameFill: 1,
  frameStroke: 2,
  rectFill: 3,
  rectStroke: 4,
  /** The glow under focused links, blurred, so the link itself stays crisp on top of it. */
  linkGlow: 5,
  link: 6,
  bar: 7,
} as const;

/** A batch layer. */
export type BatchLayer = (typeof BatchLayer)[keyof typeof BatchLayer];

const LAYER_COUNT = 8;

/** `0xRRGGBBAA` as a CanvasKit colour. */
export function color4f(ck: CanvasKit, rgba: ColorRgba): Float32Array {
  return ck.Color4f(
    ((rgba >>> 24) & 255) / 255,
    ((rgba >>> 16) & 255) / 255,
    ((rgba >>> 8) & 255) / 255,
    (rgba & 255) / 255,
  );
}

interface Entry {
  readonly builder: PathBuilder;
  readonly paint: Paint;
  used: boolean;
  /** The stroke's width in world units, and the least it keeps on screen in CSS pixels (0: none). */
  readonly width: number;
  readonly minScreen: number;
  /** The width the paint holds now, set again only when the zoom changes it. */
  applied: number;
  /** Start dots of links (x, y, nx, ny, radius, least screen radius per dot), added at their size for the zoom. */
  readonly dots: number[];
}

/** How a link's stroke is drawn: the least width it keeps on screen, and the blur of a glow (0: none). */
interface LinkPaint {
  readonly minScreen: number;
  readonly blur: number;
}

/** The blur of an arrow's glow, in device pixels: soft, but not a haze. */
const GLOW_SIGMA = 3;

/** Per-layer, per-style path batches. */
export class Batches {
  private readonly layers: Map<string, Entry>[] = Array.from({ length: LAYER_COUNT }, () => new Map());
  private readonly rect4 = new Float32Array(4);
  private readonly rrect = new Float32Array(12);

  constructor(
    private readonly ck: CanvasKit,
    private readonly resources: Resources,
  ) {}

  /** The number of distinct batches (layer and style) created so far. */
  get batchCount(): number {
    return this.layers.reduce((sum, layer) => sum + layer.size, 0);
  }

  private entry(
    layer: BatchLayer,
    key: string,
    color: ColorRgba,
    strokeWidth: number,
    dash = 0,
    link?: LinkPaint,
  ): Entry {
    const map = this.layers[layer];
    const found = map?.get(key);
    if (found) {
      found.used = true;
      return found;
    }
    const paint = this.resources.own(new this.ck.Paint());
    paint.setAntiAlias(true);
    paint.setColor(color4f(this.ck, color));
    if (strokeWidth > 0) {
      paint.setStyle(this.ck.PaintStyle.Stroke);
      paint.setStrokeWidth(strokeWidth);
      if (dash > 0) {
        // Dashes of `dash` with gaps of three quarters of it: an outline that reads as dashed at every zoom.
        paint.setPathEffect(this.resources.own(this.ck.PathEffect.MakeDash([dash, dash * 0.75], 0)));
      }
    }
    if (link !== undefined) {
      // Round ends and joins: a curve's ends and an arrowhead's two strokes meet softly instead of in square corners.
      paint.setStrokeCap(this.ck.StrokeCap.Round);
      paint.setStrokeJoin(this.ck.StrokeJoin.Round);
      if (link.blur > 0) {
        paint.setMaskFilter(
          this.resources.own(this.ck.MaskFilter.MakeBlur(this.ck.BlurStyle.Normal, link.blur, false)),
        );
      }
    }
    const entry: Entry = {
      builder: this.resources.own(new this.ck.PathBuilder()),
      paint,
      used: true,
      width: strokeWidth,
      minScreen: link?.minScreen ?? 0,
      applied: strokeWidth,
      dots: [],
    };
    map?.set(key, entry);
    return entry;
  }

  /** Adds a (rounded) rectangle; `strokeWidth` 0 fills it, and `dash` draws its stroke dashed. */
  rect(
    layer: BatchLayer,
    key: string,
    color: ColorRgba,
    strokeWidth: number,
    x: number,
    y: number,
    w: number,
    h: number,
    radius: number,
    dash = 0,
  ): void {
    const { builder } = this.entry(layer, key, color, strokeWidth, dash);
    if (radius <= 0) {
      this.rect4.set([x, y, x + w, y + h]);
      builder.addRect(this.rect4);
      return;
    }
    const r = this.rrect;
    r[0] = x;
    r[1] = y;
    r[2] = x + w;
    r[3] = y + h;
    r.fill(radius, 4);
    builder.addRRect(r);
  }

  /**
   * Adds a rounded ring: the outer rounded rectangle less the inner one, wound the other way so the non-zero rule
   * leaves it unfilled. Only the ring's pixels are drawn, not the area it encloses.
   */
  ring(
    layer: BatchLayer,
    key: string,
    color: ColorRgba,
    outer: readonly [number, number, number, number, number],
    inner: readonly [number, number, number, number, number],
  ): void {
    const { builder } = this.entry(layer, key, color, 0);
    for (const [[x, y, w, h, radius], ccw] of [
      [outer, false],
      [inner, true],
    ] as const) {
      const r = this.rrect;
      r[0] = x;
      r[1] = y;
      r[2] = x + w;
      r[3] = y + h;
      r.fill(radius, 4);
      builder.addRRect(r, ccw);
    }
  }

  /**
   * Adds a link to its stroke style's batch; `straight` draws a segment (LOD). A stroke with a glow also adds the
   * path to the glow layer under it, a wider blurred stroke of the glow's colour.
   */
  link(key: string, spec: LinkSpec, straight: boolean): void {
    const stroke = spec.stroke;
    const main = this.entry(BatchLayer.link, key, stroke.color, stroke.width, 0, {
      minScreen: stroke.minScreenWidth ?? 0,
      blur: 0,
    });
    trace(main.builder, spec, straight);
    if (spec.dot !== undefined) {
      // A fill in the links' layer, one entry per colour: every arrow's start dot in one draw call, sized at the flush
      // for the zoom.
      const { x, y, nx, ny, radius, minScreenRadius } = spec.dot;
      this.entry(BatchLayer.link, `dot${stroke.color}`, stroke.color, 0).dots.push(
        x,
        y,
        nx,
        ny,
        radius,
        minScreenRadius ?? 0,
      );
    }
    if (stroke.glow !== undefined) {
      // The glow's width is all on screen: a hair in world units, widened to the glow's width at every zoom.
      const glow = this.entry(BatchLayer.linkGlow, `g${key}`, stroke.glow.color, 0.001, 0, {
        minScreen: stroke.glow.width,
        blur: GLOW_SIGMA,
      });
      trace(glow.builder, spec, straight);
    }
  }

  /**
   * Draws every used batch in layer order; returns the number of draw calls. `zoom` is the view's: a stroke with a
   * least on-screen width is widened to it when the view is zoomed out far enough.
   */
  flush(canvas: Canvas, zoom = 1): number {
    let calls = 0;
    for (const layer of this.layers) {
      for (const entry of layer.values()) {
        if (!entry.used) {
          continue;
        }
        if (entry.minScreen > 0 && zoom > 0) {
          const width = Math.max(entry.width, entry.minScreen / zoom);
          if (width !== entry.applied) {
            entry.paint.setStrokeWidth(width);
            entry.applied = width;
          }
        }
        addDots(entry, zoom);
        const path = entry.builder.detach();
        canvas.drawPath(path, entry.paint);
        this.resources.discard(path);
        entry.used = false;
        calls++;
      }
    }
    return calls;
  }
}

/** Adds the entry's start dots to its path at their size for `zoom`, each touching its side from outside. */
function addDots(entry: Entry, zoom: number): void {
  const d = entry.dots;
  for (let i = 0; i + 5 < d.length; i += 6) {
    const r = dotRadius(d[i + 4] as number, d[i + 5] as number, zoom);
    entry.builder.addCircle(
      (d[i] as number) + (d[i + 2] as number) * r,
      (d[i + 1] as number) + (d[i + 3] as number) * r,
      r,
    );
  }
  d.length = 0;
}

/**
 * Adds a link's path to `builder`: its cubic pieces, or, when it has no control points or is straightened (LOD),
 * segments between their ends.
 */
function trace(builder: PathBuilder, spec: LinkSpec, straight: boolean): void {
  builder.moveTo(spec.from.x, spec.from.y);
  const curved = !straight && spec.c1 !== undefined && spec.c2 !== undefined;
  if (curved && spec.c1 && spec.c2) {
    builder.cubicTo(spec.c1.x, spec.c1.y, spec.c2.x, spec.c2.y, spec.to.x, spec.to.y);
  } else {
    builder.lineTo(spec.to.x, spec.to.y);
  }
  for (const piece of spec.via ?? []) {
    if (curved) {
      builder.cubicTo(piece.c1.x, piece.c1.y, piece.c2.x, piece.c2.y, piece.to.x, piece.to.y);
    } else {
      builder.lineTo(piece.to.x, piece.to.y);
    }
  }
}
