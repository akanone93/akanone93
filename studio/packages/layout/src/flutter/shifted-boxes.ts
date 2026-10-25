// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/shifted_box.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/** Shifted boxes: fractionally sized overflow boxes and baseline-positioned boxes. */

import { BoxConstraints } from "./constraints.ts";
import { type AlignmentGeometry, Offset, Size, type TextDirection } from "./geometry.ts";
import { BoxParentData, type RenderBox, type TextBaseline } from "./render-box.ts";
import { RenderAligningShiftedBox, RenderShiftedBox } from "./shifted-box.ts";

/**
 * Sizes its child to a fraction of the incoming maximum, then takes the
 * child's size, constrained (`FractionallySizedBox`). The child may overflow.
 */
export class RenderFractionallySizedOverflowBox extends RenderAligningShiftedBox {
  private _widthFactor: number | null;
  private _heightFactor: number | null;

  constructor(
    options: {
      alignment?: AlignmentGeometry;
      textDirection?: TextDirection | null;
      widthFactor?: number | null;
      heightFactor?: number | null;
      child?: RenderBox | null;
    } = {},
  ) {
    super(options);
    this._widthFactor = options.widthFactor ?? null;
    this._heightFactor = options.heightFactor ?? null;
  }

  get widthFactor(): number | null {
    return this._widthFactor;
  }

  set widthFactor(value: number | null) {
    if (this._widthFactor !== value) {
      this._widthFactor = value;
      this.markNeedsLayout();
    }
  }

  get heightFactor(): number | null {
    return this._heightFactor;
  }

  set heightFactor(value: number | null) {
    if (this._heightFactor !== value) {
      this._heightFactor = value;
      this.markNeedsLayout();
    }
  }

  private _getInnerConstraints(constraints: BoxConstraints): BoxConstraints {
    let { minWidth, maxWidth, minHeight, maxHeight } = constraints;
    if (this._widthFactor !== null) {
      const width = maxWidth * this._widthFactor;
      minWidth = width;
      maxWidth = width;
    }
    if (this._heightFactor !== null) {
      const height = maxHeight * this._heightFactor;
      minHeight = height;
      maxHeight = height;
    }
    return new BoxConstraints({ minWidth, maxWidth, minHeight, maxHeight });
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    const child = this.child;
    const result =
      child === null
        ? super.computeMinIntrinsicWidth(height)
        : child.getMinIntrinsicWidth(height * (this._heightFactor ?? 1));
    return result / (this._widthFactor ?? 1);
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    const child = this.child;
    const result =
      child === null
        ? super.computeMaxIntrinsicWidth(height)
        : child.getMaxIntrinsicWidth(height * (this._heightFactor ?? 1));
    return result / (this._widthFactor ?? 1);
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    const child = this.child;
    const result =
      child === null
        ? super.computeMinIntrinsicHeight(width)
        : child.getMinIntrinsicHeight(width * (this._widthFactor ?? 1));
    return result / (this._heightFactor ?? 1);
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    const child = this.child;
    const result =
      child === null
        ? super.computeMaxIntrinsicHeight(width)
        : child.getMaxIntrinsicHeight(width * (this._widthFactor ?? 1));
    return result / (this._heightFactor ?? 1);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child;
    if (child !== null) {
      return constraints.constrain(child.getDryLayout(this._getInnerConstraints(constraints)));
    }
    return constraints.constrain(this._getInnerConstraints(constraints).constrain(Size.zero));
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    const child = this.child;
    if (child === null) {
      return null;
    }
    const childConstraints = this._getInnerConstraints(constraints);
    const result = child.getDryBaseline(childConstraints, baseline);
    if (result === null) {
      return null;
    }
    const childSize = child.getDryLayout(childConstraints);
    const size = this.getDryLayout(constraints);
    return result + this.resolvedAlignment.alongOffset(size.minus(childSize)).dy;
  }

  protected override performLayout(): void {
    const child = this.child;
    if (child !== null) {
      child.layout(this._getInnerConstraints(this.constraints), { parentUsesSize: true });
      this.size = this.constraints.constrain(child.size);
      this.alignChild();
    } else {
      this.size = this.constraints.constrain(this._getInnerConstraints(this.constraints).constrain(Size.zero));
    }
  }
}

/**
 * Positions its child so the child's `baselineType` baseline lies `baseline`
 * below the top (`Baseline`).
 */
export class RenderBaseline extends RenderShiftedBox {
  private _baselineDistance: number;
  private _baselineType: TextBaseline;

  constructor(options: { baseline: number; baselineType: TextBaseline; child?: RenderBox | null }) {
    super(options.child ?? null);
    this._baselineDistance = options.baseline;
    this._baselineType = options.baselineType;
  }

  get baseline(): number {
    return this._baselineDistance;
  }

  set baseline(value: number) {
    if (this._baselineDistance !== value) {
      this._baselineDistance = value;
      this.markNeedsLayout();
    }
  }

  get baselineType(): TextBaseline {
    return this._baselineType;
  }

  set baselineType(value: TextBaseline) {
    if (this._baselineType !== value) {
      this._baselineType = value;
      this.markNeedsLayout();
    }
  }

  private _sizes(constraints: BoxConstraints, dry: boolean): { size: Size; top: number } {
    const child = this.child;
    if (child === null) {
      return { size: constraints.smallest, top: 0 };
    }
    const childConstraints = constraints.loosen();
    let childSize: Size;
    let childBaseline: number | null;
    if (dry) {
      childSize = child.getDryLayout(childConstraints);
      childBaseline = child.getDryBaseline(childConstraints, this._baselineType);
    } else {
      child.layout(childConstraints, { parentUsesSize: true });
      childSize = child.size;
      childBaseline = child.getDistanceToBaseline(this._baselineType, { onlyReal: true });
    }
    const top = this._baselineDistance - (childBaseline ?? childSize.height);
    return { size: constraints.constrain(new Size(childSize.width, top + childSize.height)), top };
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return this._sizes(constraints, true).size;
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    const child = this.child;
    const first = child?.getDryBaseline(constraints.loosen(), baseline) ?? null;
    const second = child?.getDryBaseline(constraints.loosen(), this._baselineType) ?? null;
    if (first === null || second === null) {
      return null;
    }
    return this._baselineDistance + first - second;
  }

  protected override performLayout(): void {
    const { size, top } = this._sizes(this.constraints, false);
    this.size = size;
    const child = this.child;
    if (child !== null && child.parentData instanceof BoxParentData) {
      child.parentData.offset = new Offset(0, top);
    }
  }
}
