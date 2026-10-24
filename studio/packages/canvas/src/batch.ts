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

/** Draw order of the batch layers. */
export const BatchLayer = {
  frameFill: 0,
  frameStroke: 1,
  rectFill: 2,
  rectStroke: 3,
  link: 4,
  bar: 5,
} as const;

/** A batch layer. */
export type BatchLayer = (typeof BatchLayer)[keyof typeof BatchLayer];

const LAYER_COUNT = 6;

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
}

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

  private entry(layer: BatchLayer, key: string, color: ColorRgba, strokeWidth: number): Entry {
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
    }
    const entry: Entry = { builder: this.resources.own(new this.ck.PathBuilder()), paint, used: true };
    map?.set(key, entry);
    return entry;
  }

  /** Adds a (rounded) rectangle; `strokeWidth` 0 fills it. */
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
  ): void {
    const { builder } = this.entry(layer, key, color, strokeWidth);
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

  /** Adds a link to its stroke style's batch; `straight` draws a segment (LOD). */
  link(key: string, spec: LinkSpec, straight: boolean): void {
    const { builder } = this.entry(BatchLayer.link, key, spec.stroke.color, spec.stroke.width);
    builder.moveTo(spec.from.x, spec.from.y);
    if (!straight && spec.c1 && spec.c2) {
      builder.cubicTo(spec.c1.x, spec.c1.y, spec.c2.x, spec.c2.y, spec.to.x, spec.to.y);
    } else {
      builder.lineTo(spec.to.x, spec.to.y);
    }
  }

  /** Draws every used batch in layer order; returns the number of draw calls. */
  flush(canvas: Canvas): number {
    let calls = 0;
    for (const layer of this.layers) {
      for (const entry of layer.values()) {
        if (!entry.used) {
          continue;
        }
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
