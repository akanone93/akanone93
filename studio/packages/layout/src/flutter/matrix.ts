// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * A 2D affine transform: the part of Flutter's `Matrix4` that box layout
 * uses for paint transforms (`applyPaintTransform`). Maps (x, y) to
 * (a·x + c·y + tx, b·x + d·y + ty).
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
      this.a * other.a + this.c * other.b,
      this.b * other.a + this.d * other.b,
      this.a * other.c + this.c * other.d,
      this.b * other.c + this.d * other.d,
      this.a * other.tx + this.c * other.ty + this.tx,
      this.b * other.tx + this.d * other.ty + this.ty,
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
    return new Offset(this.a * point.dx + this.c * point.dy + this.tx, this.b * point.dx + this.d * point.dy + this.ty);
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
