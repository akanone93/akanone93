// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/sliver_padding.dart and packages/flutter/lib/src/widgets/sliver_fill.dart (_RenderSliverFractionalPadding)
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Padding around a sliver: [RenderSliverPadding] (the render object of
 * `SliverPadding`) and [RenderSliverFractionalPadding], the padding
 * `SliverFillViewport` puts around its pages so the first and last page
 * center in the viewport. Painting and hit testing are not ported.
 */

import { Axis } from "./flex.ts";
import { EdgeInsets, type EdgeInsetsGeometry, Offset, type TextDirection } from "./geometry.ts";
import type { PipelineOwner, RenderObject } from "./object.ts";
import {
  AxisDirection,
  applyGrowthDirectionToAxisDirection,
  RenderSliver,
  type SliverConstraints,
  SliverGeometry,
  SliverPhysicalParentData,
} from "./sliver.ts";

/** A sliver that insets its sliver child by an [EdgeInsets] (`RenderSliverEdgeInsetsPadding`). */
export abstract class RenderSliverEdgeInsetsPadding extends RenderSliver {
  private _child: RenderSliver | null = null;

  /** The padding to apply, resolved to physical sides. */
  abstract get resolvedPadding(): EdgeInsets;

  get child(): RenderSliver | null {
    return this._child;
  }

  set child(value: RenderSliver | null) {
    if (this._child !== null) {
      this.dropChild(this._child);
    }
    this._child = value;
    if (value !== null) {
      this.adoptChild(value);
    }
  }

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof SliverPhysicalParentData)) {
      child.parentData = new SliverPhysicalParentData();
    }
  }

  override attach(owner: PipelineOwner): void {
    super.attach(owner);
    this._child?.attach(owner);
  }

  override detach(): void {
    super.detach();
    this._child?.detach();
  }

  override visitChildren(visitor: (child: RenderObject) => void): void {
    if (this._child !== null) {
      visitor(this._child);
    }
  }

  /** The padding before the child along the main axis, at the sliver's growth direction. */
  get beforePadding(): number {
    const padding = this.resolvedPadding;
    switch (applyGrowthDirectionToAxisDirection(this.constraints.axisDirection, this.constraints.growthDirection)) {
      case AxisDirection.up:
        return padding.bottom;
      case AxisDirection.right:
        return padding.left;
      case AxisDirection.left:
        return padding.right;
      default:
        return padding.top;
    }
  }

  /** The padding after the child along the main axis. */
  get afterPadding(): number {
    const padding = this.resolvedPadding;
    switch (applyGrowthDirectionToAxisDirection(this.constraints.axisDirection, this.constraints.growthDirection)) {
      case AxisDirection.up:
        return padding.top;
      case AxisDirection.right:
        return padding.right;
      case AxisDirection.left:
        return padding.left;
      default:
        return padding.bottom;
    }
  }

  /** The total padding along the main axis. */
  get mainAxisPadding(): number {
    const padding = this.resolvedPadding;
    return this.constraints.axis === Axis.horizontal ? padding.horizontal : padding.vertical;
  }

  /** The total padding along the cross axis. */
  get crossAxisPadding(): number {
    const padding = this.resolvedPadding;
    return this.constraints.axis === Axis.horizontal ? padding.vertical : padding.horizontal;
  }

  protected override performLayout(): void {
    const constraints: SliverConstraints = this.constraints;
    const paintOffset = (range: { from: number; to: number }): number => this.calculatePaintOffset(constraints, range);
    const cacheOffset = (range: { from: number; to: number }): number => this.calculateCacheOffset(constraints, range);
    const resolvedPadding = this.resolvedPadding;
    const beforePadding = this.beforePadding;
    const mainAxisPadding = this.mainAxisPadding;
    const crossAxisPadding = this.crossAxisPadding;
    const child = this.child;
    if (child === null) {
      const paintExtent = paintOffset({ from: 0, to: mainAxisPadding });
      const cacheExtent = cacheOffset({ from: 0, to: mainAxisPadding });
      this.geometry = new SliverGeometry({
        scrollExtent: mainAxisPadding,
        paintExtent: Math.min(paintExtent, constraints.remainingPaintExtent),
        maxPaintExtent: mainAxisPadding,
        cacheExtent,
      });
      return;
    }
    const beforePaddingPaintExtent = paintOffset({ from: 0, to: beforePadding });
    let overlap = constraints.overlap;
    if (overlap > 0) {
      overlap = Math.max(0, constraints.overlap - beforePaddingPaintExtent);
    }
    child.layout(
      constraints.copyWith({
        scrollOffset: Math.max(0, constraints.scrollOffset - beforePadding),
        cacheOrigin: Math.min(0, constraints.cacheOrigin + beforePadding),
        overlap,
        remainingPaintExtent: constraints.remainingPaintExtent - paintOffset({ from: 0, to: beforePadding }),
        remainingCacheExtent: constraints.remainingCacheExtent - cacheOffset({ from: 0, to: beforePadding }),
        crossAxisExtent: Math.max(0, constraints.crossAxisExtent - crossAxisPadding),
        precedingScrollExtent: beforePadding + constraints.precedingScrollExtent,
      }),
      { parentUsesSize: true },
    );
    const childLayoutGeometry = child.geometry as SliverGeometry;
    if (childLayoutGeometry.scrollOffsetCorrection !== null) {
      this.geometry = new SliverGeometry({ scrollOffsetCorrection: childLayoutGeometry.scrollOffsetCorrection });
      return;
    }
    const scrollExtent = childLayoutGeometry.scrollExtent;
    const beforePaddingCacheExtent = cacheOffset({ from: 0, to: beforePadding });
    const afterPaddingCacheExtent = cacheOffset({
      from: beforePadding + scrollExtent,
      to: mainAxisPadding + scrollExtent,
    });
    const afterPaddingPaintExtent = paintOffset({
      from: beforePadding + scrollExtent,
      to: mainAxisPadding + scrollExtent,
    });
    const mainAxisPaddingCacheExtent = beforePaddingCacheExtent + afterPaddingCacheExtent;
    const mainAxisPaddingPaintExtent = beforePaddingPaintExtent + afterPaddingPaintExtent;
    const paintExtent = Math.min(
      beforePaddingPaintExtent +
        Math.max(childLayoutGeometry.paintExtent, childLayoutGeometry.layoutExtent + afterPaddingPaintExtent),
      constraints.remainingPaintExtent,
    );
    this.geometry = new SliverGeometry({
      paintOrigin: childLayoutGeometry.paintOrigin,
      scrollExtent: mainAxisPadding + scrollExtent,
      paintExtent,
      layoutExtent: Math.min(mainAxisPaddingPaintExtent + childLayoutGeometry.layoutExtent, paintExtent),
      cacheExtent: Math.min(
        mainAxisPaddingCacheExtent + childLayoutGeometry.cacheExtent,
        constraints.remainingCacheExtent,
      ),
      maxPaintExtent: mainAxisPadding + childLayoutGeometry.maxPaintExtent,
      hitTestExtent: Math.max(
        mainAxisPaddingPaintExtent + childLayoutGeometry.paintExtent,
        beforePaddingPaintExtent + childLayoutGeometry.hitTestExtent,
      ),
      hasVisualOverflow: childLayoutGeometry.hasVisualOverflow,
    });
    let calculatedOffset: number;
    switch (applyGrowthDirectionToAxisDirection(constraints.axisDirection, constraints.growthDirection)) {
      case AxisDirection.up:
        calculatedOffset = paintOffset({
          from: resolvedPadding.bottom + scrollExtent,
          to: resolvedPadding.vertical + scrollExtent,
        });
        break;
      case AxisDirection.left:
        calculatedOffset = paintOffset({
          from: resolvedPadding.right + scrollExtent,
          to: resolvedPadding.horizontal + scrollExtent,
        });
        break;
      case AxisDirection.right:
        calculatedOffset = paintOffset({ from: 0, to: resolvedPadding.left });
        break;
      case AxisDirection.down:
        calculatedOffset = paintOffset({ from: 0, to: resolvedPadding.top });
        break;
    }
    (child.parentData as SliverPhysicalParentData).paintOffset =
      constraints.axis === Axis.horizontal
        ? new Offset(calculatedOffset, resolvedPadding.top)
        : new Offset(resolvedPadding.left, calculatedOffset);
  }

  override childMainAxisPosition(_child: RenderObject): number {
    return this.calculatePaintOffset(this.constraints, { from: 0, to: this.beforePadding });
  }

  override childCrossAxisPosition(_child: RenderObject): number {
    return this.constraints.axis === Axis.horizontal ? this.resolvedPadding.top : this.resolvedPadding.left;
  }

  override childScrollOffset(_child: RenderObject): number | null {
    return this.beforePadding;
  }
}

/** Padding around a sliver (`RenderSliverPadding`). */
export class RenderSliverPadding extends RenderSliverEdgeInsetsPadding {
  private _padding: EdgeInsetsGeometry;
  private _textDirection: TextDirection | null;

  constructor(options: {
    padding: EdgeInsetsGeometry;
    textDirection?: TextDirection | null;
    child?: RenderSliver | null;
  }) {
    super();
    this._padding = options.padding;
    this._textDirection = options.textDirection ?? null;
    this.child = options.child ?? null;
  }

  get resolvedPadding(): EdgeInsets {
    return this._padding.resolve(this._textDirection);
  }

  get padding(): EdgeInsetsGeometry {
    return this._padding;
  }

  set padding(value: EdgeInsetsGeometry) {
    this._padding = value;
    this.markNeedsLayout();
  }

  get textDirection(): TextDirection | null {
    return this._textDirection;
  }

  set textDirection(value: TextDirection | null) {
    this._textDirection = value;
    this.markNeedsLayout();
  }
}

/** Padding of `viewportFraction` of the viewport's main axis extent on both ends (`_RenderSliverFractionalPadding`). */
export class RenderSliverFractionalPadding extends RenderSliverEdgeInsetsPadding {
  private _viewportFraction: number;

  constructor(options: { viewportFraction?: number; child?: RenderSliver | null } = {}) {
    super();
    this._viewportFraction = options.viewportFraction ?? 0;
    this.child = options.child ?? null;
  }

  get viewportFraction(): number {
    return this._viewportFraction;
  }

  set viewportFraction(value: number) {
    if (this._viewportFraction !== value) {
      this._viewportFraction = value;
      this.markNeedsLayout();
    }
  }

  get resolvedPadding(): EdgeInsets {
    const paddingValue = this.constraints.viewportMainAxisExtent * this._viewportFraction;
    return this.constraints.axis === Axis.horizontal
      ? EdgeInsets.symmetric({ horizontal: paddingValue })
      : EdgeInsets.symmetric({ vertical: paddingValue });
  }
}
