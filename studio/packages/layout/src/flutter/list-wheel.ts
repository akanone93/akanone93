// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/list_wheel_viewport.dart and packages/flutter/lib/src/painting/matrix_utils.dart (createCylindricalProjectionTransform)
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `RenderListWheelViewport` at rest on its selected item (scroll offset
 * `selectedItem * itemExtent`): which rows exist, how they are laid out and
 * the cylinder transform each is painted under, which the canvas' hit rects follow. Scrolling, clipping,
 * opacity and the magnifier do not take part in layout and are not ported.
 */

import type { BoxConstraints } from "./constraints.ts";
import { ContainerRenderBox } from "./container.ts";
import { Offset, type Size } from "./geometry.ts";
import { Affine, type PaintTransformer } from "./matrix.ts";
import type { RenderObject } from "./object.ts";
import type { RenderBox } from "./render-box.ts";
import { BoxParentData } from "./render-box.ts";

/** `RenderListWheelViewport.defaultPerspective`. */
export const kListWheelPerspective = 0.003;

/** `MatrixUtils.createCylindricalProjectionTransform` (vertical): the 4x4 matrix, row-major. */
function cylindricalProjection(radius: number, angle: number, perspective: number): number[][] {
  const mul = (x: number[][], y: number[][]): number[][] =>
    x.map((row) => y[0]?.map((_, j) => row.reduce((sum, value, k) => sum + value * (y[k]?.[j] ?? 0), 0)) ?? []);
  const projection = [
    [1, 0, 0, 0],
    [0, 1, 0, 0],
    [0, 0, 1, -radius],
    [0, 0, -perspective, perspective * radius + 1],
  ];
  const cos = Math.cos(angle);
  const sin = Math.sin(angle);
  const rotation = [
    [1, 0, 0, 0],
    [0, cos, -sin, 0],
    [0, sin, cos, 0],
    [0, 0, 0, 1],
  ];
  const translation = [
    [1, 0, 0, 0],
    [0, 1, 0, 0],
    [0, 0, 1, radius],
    [0, 0, 0, 1],
  ];
  return mul(projection, mul(rotation, translation));
}

/** The part of a 4x4 matrix that maps z = 0 points to the screen plane (rows and columns 0, 1, 3). */
function toPlane(m: number[][]): Affine {
  const at = (row: number, col: number): number => m[row]?.[col] ?? 0;
  return new Affine(at(0, 0), at(1, 0), at(0, 1), at(1, 1), at(0, 3), at(1, 3), at(3, 0), at(3, 1), at(3, 3));
}

/** The rows a wheel can show: `ListWheelChildDelegate.build` returning null is `exists` returning false. */
export interface WheelRows {
  /** `RenderListWheelChildManager.childExistsAt`. */
  exists(index: number): boolean;
  /** The row at `index`; called once per index while it stays built. */
  build(index: number): RenderBox;
}

/** `ListWheelChildListDelegate`: the rows 0 to n - 1, each used once. */
export function listRows(rows: readonly RenderBox[]): WheelRows {
  return { exists: (index) => index >= 0 && index < rows.length, build: (index) => rows[index] as RenderBox };
}

export class RenderListWheelViewport extends ContainerRenderBox<RenderBox> implements PaintTransformer {
  private readonly _itemExtent: number;
  private readonly _diameterRatio: number;
  private readonly _perspective: number;
  private readonly _squeeze: number;
  private readonly _offAxisFraction: number;
  private readonly _pixels: number;
  private readonly _rows: WheelRows;
  private readonly _built = new Map<RenderObject, number>();
  private readonly _cache = new Map<number, RenderBox>();

  constructor(options: {
    rows: WheelRows;
    itemExtent: number;
    selectedItem?: number;
    diameterRatio?: number;
    perspective?: number;
    squeeze?: number;
    offAxisFraction?: number;
  }) {
    super();
    this._rows = options.rows;
    this._itemExtent = options.itemExtent;
    this._pixels = (options.selectedItem ?? 0) * options.itemExtent;
    this._diameterRatio = options.diameterRatio ?? 2;
    this._perspective = options.perspective ?? kListWheelPerspective;
    this._squeeze = options.squeeze ?? 1;
    this._offAxisFraction = options.offAxisFraction ?? 0;
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  /** The index range Flutter builds: those within the squeezed viewport around the centred item, trimmed to rows that exist. */
  private builtRange(height: number): [number, number] {
    const visibleHeight = height * this._squeeze;
    const first = this._pixels + this._itemExtent / 2 - visibleHeight / 2;
    const last = first + visibleHeight;
    let firstIndex = Math.floor(first / this._itemExtent);
    let lastIndex = Math.floor(last / this._itemExtent);
    if (lastIndex * this._itemExtent === last) {
      lastIndex--;
    }
    while (!this._rows.exists(firstIndex) && firstIndex <= lastIndex) {
      firstIndex++;
    }
    while (!this._rows.exists(lastIndex) && firstIndex <= lastIndex) {
      lastIndex--;
    }
    return [firstIndex, lastIndex];
  }

  protected override performLayout(): void {
    const size = this.constraints.biggest;
    this.size = size;
    const [first, last] = this.builtRange(size.height);
    const childConstraints = this.constraints.copyWith({
      minHeight: this._itemExtent,
      maxHeight: this._itemExtent,
      minWidth: 0,
    });
    this.removeAll();
    this._built.clear();
    for (let index = first; index <= last; index++) {
      let child = this._cache.get(index);
      if (child === undefined) {
        child = this._rows.build(index);
        this._cache.set(index, child);
      }
      this.add(child);
      child.layout(childConstraints, { parentUsesSize: true });
      if (child.parentData instanceof BoxParentData) {
        child.parentData.offset = new Offset(size.width / 2 - child.size.width / 2, index * this._itemExtent);
      }
      this._built.set(child, index);
    }
  }

  /** `ListWheelParentData.transform`: the cylinder transform of the row's centre-origin paint. */
  paintTransformOf(child: RenderObject): Affine {
    const index = this._built.get(child);
    if (index === undefined || !(child.parentData instanceof BoxParentData)) {
      return Affine.identity;
    }
    const { width, height } = this.size;
    const maxVisibleRadian = this._diameterRatio < 1 ? Math.PI / 2 : Math.asin(1 / this._diameterRatio);
    const topScrollMargin = -height / 2 + this._itemExtent / 2;
    const layoutOffset = child.parentData.offset;
    const untransformedY = layoutOffset.dy - topScrollMargin - this._pixels;
    const fractionalY = (untransformedY + this._itemExtent / 2) / height;
    const angle = (-(fractionalY - 0.5) * 2 * maxVisibleRadian) / this._squeeze;
    if (angle > Math.PI / 2 || angle < -Math.PI / 2 || Number.isNaN(angle)) {
      return Affine.identity;
    }
    const cylinder = toPlane(cylindricalProjection((height * this._diameterRatio) / 2, angle, this._perspective));
    const cx = (width / 2) * (-this._offAxisFraction * 2 + 1);
    const cy = height / 2;
    return Affine.translation(cx, cy)
      .multiply(cylinder)
      .multiply(Affine.translation(-cx, -cy))
      .multiply(Affine.translation(layoutOffset.dx, -topScrollMargin));
  }
}
