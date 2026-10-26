// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * A 2D affine transform: the part of Flutter's `Matrix4` that box layout
 * uses for paint transforms (`applyPaintTransform`). Maps (x, y) to
 * (a·x + c·y + tx, b·x + d·y + ty), divided by p·x + q·y + w when the
 * transform has a perspective part (`ListWheelViewport`'s cylinder); p, q and
 * w default to the affine 0, 0, 1.
 */

import { Offset, Rect } from "./geometry.ts";
import type { RenderObject } from "./object.ts";

export class Affine {
  static readonly identity = new Affine(1, 0, 0, 1, 0, 0);

  constructor(
    readonly a: number,
    readonly b: number,
    readonly c: number,
    readonly d: number,
    readonly tx: number,
    readonly ty: number,
    readonly p = 0,
    readonly q = 0,
    readonly w = 1,
  ) {}

  static translation(dx: number, dy: number): Affine {
    return new Affine(1, 0, 0, 1, dx, dy);
  }

  static scaling(sx: number, sy: number): Affine {
    return new Affine(sx, 0, 0, sy, 0, 0);
  }

  /** A counter-clockwise rotation by `radians` (`Matrix4.rotationZ`). */
  static rotation(radians: number): Affine {
    const cos = Math.cos(radians);
    const sin = Math.sin(radians);
    return new Affine(cos, sin, -sin, cos, 0, 0);
  }

  /** `this × other`: `other` applies first (Matrix4.multiply). */
  multiply(other: Affine): Affine {
    return new Affine(
      this.a * other.a + this.c * other.b + this.tx * other.p,
      this.b * other.a + this.d * other.b + this.ty * other.p,
      this.a * other.c + this.c * other.d + this.tx * other.q,
      this.b * other.c + this.d * other.d + this.ty * other.q,
      this.a * other.tx + this.c * other.ty + this.tx * other.w,
      this.b * other.tx + this.d * other.ty + this.ty * other.w,
      this.p * other.a + this.q * other.b + this.w * other.p,
      this.p * other.c + this.q * other.d + this.w * other.q,
      this.p * other.tx + this.q * other.ty + this.w * other.w,
    );
  }

  /** `this × translation(dx, dy)`, as `Matrix4.translate`. */
  translate(dx: number, dy: number): Affine {
    return this.multiply(Affine.translation(dx, dy));
  }

  /** `this × scaling(sx, sy)`, as `Matrix4.scale`. */
  scale(sx: number, sy: number): Affine {
    return this.multiply(Affine.scaling(sx, sy));
  }

  /** Maps a point. */
  apply(point: Offset): Offset {
    const x = this.a * point.dx + this.c * point.dy + this.tx;
    const y = this.b * point.dx + this.d * point.dy + this.ty;
    const w = this.p * point.dx + this.q * point.dy + this.w;
    return w === 1 ? new Offset(x, y) : new Offset(x / w, y / w);
  }

  /** The bounding box of `rect` after the transform (`MatrixUtils.transformRect`). */
  transformRect(rect: Rect): Rect {
    const corners = [
      this.apply(new Offset(rect.left, rect.top)),
      this.apply(new Offset(rect.right, rect.top)),
      this.apply(new Offset(rect.left, rect.bottom)),
      this.apply(new Offset(rect.right, rect.bottom)),
    ];
    const xs = corners.map((p) => p.dx);
    const ys = corners.map((p) => p.dy);
    return Rect.fromLTRB(Math.min(...xs), Math.min(...ys), Math.max(...xs), Math.max(...ys));
  }
}

/**
 * A render object whose children are painted under a transform other than
 * their parent-data offset (`RenderObject.applyPaintTransform`). Global rects
 * of the children (the Studio canvas' hit rects) follow it.
 */
export interface PaintTransformer {
  /** The transform from `child`'s coordinates to this object's. */
  paintTransformOf(child: RenderObject): Affine;
}

/** Whether `object` places its children by a paint transform. */
export function isPaintTransformer(object: RenderObject): object is RenderObject & PaintTransformer {
  return "paintTransformOf" in object && typeof object.paintTransformOf === "function";
}
