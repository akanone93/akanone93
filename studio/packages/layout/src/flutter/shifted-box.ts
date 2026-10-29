// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/shifted_box.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/** Boxes that position their child at an offset: padding and alignment. */

import type { BoxConstraints } from "./constraints.ts";
import {
  Alignment,
  type AlignmentGeometry,
  type EdgeInsets,
  type EdgeInsetsGeometry,
  Offset,
  Size,
  type TextDirection,
} from "./geometry.ts";
import { BoxParentData, type RenderBox, RenderBoxWithChild, type TextBaseline } from "./render-box.ts";

/**
 * A box with one child placed at its [BoxParentData.offset]. Intrinsics
 * pass through to the child; the baseline is the child's, shifted by the
 * child's offset.
 */
export abstract class RenderShiftedBox extends RenderBoxWithChild {
  constructor(child: RenderBox | null) {
    super();
    this.child = child;
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return this.child?.getMinIntrinsicWidth(height) ?? 0;
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    return this.child?.getMaxIntrinsicWidth(height) ?? 0;
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return this.child?.getMinIntrinsicHeight(width) ?? 0;
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this.child?.getMaxIntrinsicHeight(width) ?? 0;
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    const child = this.child;
    if (child === null) {
      return super.computeDistanceToActualBaseline(baseline);
    }
    const result = child.getDistanceToActualBaseline(baseline);
    return result === null ? null : result + RenderShiftedBox.offsetOf(child).dy;
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    // The base class applies no offset; subclasses add theirs.
    return this.child?.getDryBaseline(constraints, baseline) ?? null;
  }
}

/** Insets its child by `padding` (`Padding`). */
export class RenderPadding extends RenderShiftedBox {
  private _padding: EdgeInsetsGeometry;
  private _textDirection: TextDirection | null;
  private _resolvedPaddingCache: EdgeInsets | null = null;

  constructor(options: {
    padding: EdgeInsetsGeometry;
    textDirection?: TextDirection | null;
    child?: RenderBox | null;
  }) {
    super(options.child ?? null);
    this._padding = options.padding;
    this._textDirection = options.textDirection ?? null;
  }

  private get _resolvedPadding(): EdgeInsets {
    if (this._resolvedPaddingCache === null) {
      this._resolvedPaddingCache = this._padding.resolve(this._textDirection);
    }
    return this._resolvedPaddingCache;
  }

  private _markNeedResolution(): void {
    this._resolvedPaddingCache = null;
    this.markNeedsLayout();
  }

  get padding(): EdgeInsetsGeometry {
    return this._padding;
  }

  set padding(value: EdgeInsetsGeometry) {
    if (this._padding === value) {
      return;
    }
    this._padding = value;
    this._markNeedResolution();
  }

  get textDirection(): TextDirection | null {
    return this._textDirection;
  }

  set textDirection(value: TextDirection | null) {
    if (this._textDirection === value) {
      return;
    }
    this._textDirection = value;
    this._markNeedResolution();
  }

  // Each intrinsic relies on Infinity absorbing the padding arithmetic.
  protected override computeMinIntrinsicWidth(height: number): number {
    const padding = this._resolvedPadding;
    const child = this.child;
    if (child !== null) {
      return child.getMinIntrinsicWidth(Math.max(0, height - padding.vertical)) + padding.horizontal;
    }
    return padding.horizontal;
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    const padding = this._resolvedPadding;
    const child = this.child;
    if (child !== null) {
      return child.getMaxIntrinsicWidth(Math.max(0, height - padding.vertical)) + padding.horizontal;
    }
    return padding.horizontal;
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    const padding = this._resolvedPadding;
    const child = this.child;
    if (child !== null) {
      return child.getMinIntrinsicHeight(Math.max(0, width - padding.horizontal)) + padding.vertical;
    }
    return padding.vertical;
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    const padding = this._resolvedPadding;
    const child = this.child;
    if (child !== null) {
      return child.getMaxIntrinsicHeight(Math.max(0, width - padding.horizontal)) + padding.vertical;
    }
    return padding.vertical;
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const padding = this._resolvedPadding;
    const child = this.child;
    if (child === null) {
      return constraints.constrain(new Size(padding.horizontal, padding.vertical));
    }
    const innerConstraints = constraints.deflate(padding);
    const childSize = child.getDryLayout(innerConstraints);
    return constraints.constrain(new Size(padding.horizontal + childSize.width, padding.vertical + childSize.height));
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    const child = this.child;
    if (child === null) {
      return null;
    }
    const padding = this._resolvedPadding;
    const childBaseline = child.getDryBaseline(constraints.deflate(padding), baseline);
    return childBaseline === null ? null : childBaseline + padding.top;
  }

  protected override performLayout(): void {
    const constraints = this.constraints;
    const padding = this._resolvedPadding;
    const child = this.child;
    if (child === null) {
      this.size = constraints.constrain(new Size(padding.horizontal, padding.vertical));
      return;
    }
    child.layout(constraints.deflate(padding), { parentUsesSize: true });
    setChildOffset(child, new Offset(padding.left, padding.top));
    this.size = constraints.constrain(
      new Size(padding.horizontal + child.size.width, padding.vertical + child.size.height),
    );
  }
}

/** Sets a laid-out child's offset in its [BoxParentData]. */
export function setChildOffset(child: RenderBox, offset: Offset): void {
  if (child.parentData instanceof BoxParentData) {
    child.parentData.offset = offset;
  }
}

/** A shifted box that places its child by an [AlignmentGeometry]. */
export abstract class RenderAligningShiftedBox extends RenderShiftedBox {
  private _alignment: AlignmentGeometry;
  private _textDirection: TextDirection | null;
  private _resolvedAlignmentCache: Alignment | null = null;

  constructor(options: {
    alignment?: AlignmentGeometry;
    textDirection?: TextDirection | null;
    child?: RenderBox | null;
  }) {
    super(options.child ?? null);
    this._alignment = options.alignment ?? Alignment.center;
    this._textDirection = options.textDirection ?? null;
  }

  /** The alignment resolved against [textDirection], cached until either changes. */
  protected get resolvedAlignment(): Alignment {
    if (this._resolvedAlignmentCache === null) {
      this._resolvedAlignmentCache = this._alignment.resolve(this._textDirection);
    }
    return this._resolvedAlignmentCache;
  }

  private _markNeedResolution(): void {
    this._resolvedAlignmentCache = null;
    this.markNeedsLayout();
  }

  get alignment(): AlignmentGeometry {
    return this._alignment;
  }

  set alignment(value: AlignmentGeometry) {
    if (this._alignment === value) {
      return;
    }
    this._alignment = value;
    this._markNeedResolution();
  }

  get textDirection(): TextDirection | null {
    return this._textDirection;
  }

  set textDirection(value: TextDirection | null) {
    if (this._textDirection === value) {
      return;
    }
    this._textDirection = value;
    this._markNeedResolution();
  }

  /** Places the laid-out child within [size] at [resolvedAlignment]. */
  protected alignChild(): void {
    const child = this.child;
    if (child === null) {
      return;
    }
    setChildOffset(child, this.resolvedAlignment.alongOffset(this.size.minus(child.size)));
  }
}

/**
 * Aligns its child (`Align`, `Center`). With a `widthFactor` its width is
 * the child's times the factor; without one it expands to fill bounded
 * constraints and shrink-wraps unbounded ones. Height likewise.
 */
export class RenderPositionedBox extends RenderAligningShiftedBox {
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
    if (this._widthFactor === value) {
      return;
    }
    this._widthFactor = value;
    this.markNeedsLayout();
  }

  get heightFactor(): number | null {
    return this._heightFactor;
  }

  set heightFactor(value: number | null) {
    if (this._heightFactor === value) {
      return;
    }
    this._heightFactor = value;
    this.markNeedsLayout();
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return super.computeMinIntrinsicWidth(height) * (this._widthFactor ?? 1);
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    return super.computeMaxIntrinsicWidth(height) * (this._widthFactor ?? 1);
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return super.computeMinIntrinsicHeight(width) * (this._heightFactor ?? 1);
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return super.computeMaxIntrinsicHeight(width) * (this._heightFactor ?? 1);
  }

  /** The size for a child of `childSize` (or no child) under `constraints`. */
  private _sizeFor(constraints: BoxConstraints, childSize: Size | null): Size {
    const shrinkWrapWidth = this._widthFactor !== null || constraints.maxWidth === Number.POSITIVE_INFINITY;
    const shrinkWrapHeight = this._heightFactor !== null || constraints.maxHeight === Number.POSITIVE_INFINITY;
    if (childSize !== null) {
      return constraints.constrain(
        new Size(
          shrinkWrapWidth ? childSize.width * (this._widthFactor ?? 1) : Number.POSITIVE_INFINITY,
          shrinkWrapHeight ? childSize.height * (this._heightFactor ?? 1) : Number.POSITIVE_INFINITY,
        ),
      );
    }
    return constraints.constrain(
      new Size(shrinkWrapWidth ? 0 : Number.POSITIVE_INFINITY, shrinkWrapHeight ? 0 : Number.POSITIVE_INFINITY),
    );
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child;
    return this._sizeFor(constraints, child === null ? null : child.getDryLayout(constraints.loosen()));
  }

  protected override performLayout(): void {
    const constraints = this.constraints;
    const child = this.child;
    if (child !== null) {
      child.layout(constraints.loosen(), { parentUsesSize: true });
      this.size = this._sizeFor(constraints, child.size);
      this.alignChild();
    } else {
      this.size = this._sizeFor(constraints, null);
    }
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    const child = this.child;
    if (child === null) {
      return null;
    }
    const childConstraints = constraints.loosen();
    const result = child.getDryBaseline(childConstraints, baseline);
    if (result === null) {
      return null;
    }
    const childSize = child.getDryLayout(childConstraints);
    const size = this._sizeFor(constraints, childSize);
    return result + this.resolvedAlignment.alongOffset(size.minus(childSize)).dy;
  }
}
