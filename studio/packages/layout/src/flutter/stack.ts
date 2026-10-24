// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/stack.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Stack layout: children layered over each other, non-positioned children
 * aligned by [RenderStack.alignment] and positioned children placed by their
 * [StackParentData] edges (`RenderStack`, the render object of `Stack`, and
 * `RenderIndexedStack`, the render object of `IndexedStack`).
 *
 * Painting, clipping (`clipBehavior` never affects layout), hit testing and
 * semantics are not ported; whether a positioned child lies outside the
 * stack is exposed as [RenderStack.hasVisualOverflow].
 *
 * `Positioned.directional` / `PositionedDirectional` resolve `start`/`end`
 * to `left`/`right` in the widget layer; [StackParentData.setDirectional]
 * does the same here.
 *
 * Flutter checks several preconditions only with `assert`, which is compiled
 * out of release builds. This port follows RELEASE behaviour:
 * - a stack laid out under unbounded constraints with only positioned
 *   children takes an infinite size (debug Flutter asserts a finite size);
 * - a positioned child with both `left`, `right` and `width` (or `top`,
 *   `bottom` and `height`) uses `left`/`right` (`top`/`bottom`) and ignores
 *   the extent (debug Flutter asserts that at most two are set);
 * - a negative positioned extent is clamped to zero, as in Flutter.
 * Resolving an `AlignmentDirectional` without a `textDirection` throws, as
 * the null check does in release Flutter.
 */

import { BoxConstraints } from "./constraints.ts";
import { ContainerBoxParentData, ContainerRenderBox } from "./container.ts";
import {
  type Alignment,
  AlignmentDirectional,
  type AlignmentGeometry,
  Offset,
  Size,
  TextDirection,
} from "./geometry.ts";
import type { RenderObject } from "./object.ts";
import { BaselineOffset, type RenderBox, type TextBaseline } from "./render-box.ts";

/** How to size the non-positioned children of a stack (rendering/stack.dart `StackFit`). */
export enum StackFit {
  loose = "loose",
  expand = "expand",
  passthrough = "passthrough",
}

/** Parent data for children of a [RenderStack] (rendering/stack.dart `StackParentData`). */
export class StackParentData extends ContainerBoxParentData<RenderBox> {
  top: number | null = null;
  right: number | null = null;
  bottom: number | null = null;
  left: number | null = null;
  width: number | null = null;
  height: number | null = null;

  /** Whether any of the edges or extents is set. */
  get isPositioned(): boolean {
    return (
      this.top !== null ||
      this.right !== null ||
      this.bottom !== null ||
      this.left !== null ||
      this.width !== null ||
      this.height !== null
    );
  }

  /** Sets `left`/`right` from `start`/`end` under `textDirection` (`Positioned.directional`). */
  setDirectional(textDirection: TextDirection, start: number | null, end: number | null): void {
    if (textDirection === TextDirection.rtl) {
      this.left = end;
      this.right = start;
    } else {
      this.left = start;
      this.right = end;
    }
  }

  /** The tight-for constraints of a positioned child in a stack of `stackSize`. */
  positionedChildConstraints(stackSize: Size): BoxConstraints {
    const width = this.left !== null && this.right !== null ? stackSize.width - this.right - this.left : this.width;
    const height = this.top !== null && this.bottom !== null ? stackSize.height - this.bottom - this.top : this.height;
    return BoxConstraints.tightFor({
      width: width === null ? null : Math.max(0, width),
      height: height === null ? null : Math.max(0, height),
    });
  }
}

type ChildLayouter = (child: RenderBox, constraints: BoxConstraints) => Size;

/** `ChildLayoutHelper.layoutChild`. */
function layoutChild(child: RenderBox, constraints: BoxConstraints): Size {
  child.layout(constraints, { parentUsesSize: true });
  return child.size;
}

/** `ChildLayoutHelper.dryLayoutChild`. */
function dryLayoutChild(child: RenderBox, constraints: BoxConstraints): Size {
  return child.getDryLayout(constraints);
}

/** `size - childSize as Offset`. */
function remaining(size: Size, childSize: Size): Offset {
  return new Offset(size.width - childSize.width, size.height - childSize.height);
}

function stackParentData(child: RenderBox): StackParentData {
  const parentData = child.parentData;
  if (!(parentData instanceof StackParentData)) {
    throw new Error("RenderStack: child has no StackParentData; it is not a child of a RenderStack");
  }
  return parentData;
}

export interface RenderStackOptions {
  children?: RenderBox[];
  alignment?: AlignmentGeometry;
  textDirection?: TextDirection | null;
  fit?: StackFit;
}

/** Lays its children out on top of each other. */
export class RenderStack extends ContainerRenderBox<RenderBox> {
  private _alignment: AlignmentGeometry;
  private _textDirection: TextDirection | null;
  private _fit: StackFit;
  private _hasVisualOverflow = false;

  constructor(options: RenderStackOptions = {}) {
    super();
    this._alignment = options.alignment ?? AlignmentDirectional.topStart;
    this._textDirection = options.textDirection ?? null;
    this._fit = options.fit ?? StackFit.loose;
    this.addAll(options.children ?? []);
  }

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof StackParentData)) {
      child.parentData = new StackParentData();
    }
  }

  /** How to align the non-positioned and partially-positioned children. */
  get alignment(): AlignmentGeometry {
    return this._alignment;
  }

  set alignment(value: AlignmentGeometry) {
    if (this._alignment !== value) {
      this._alignment = value;
      this.markNeedsLayout();
    }
  }

  /** The text direction that resolves a directional [alignment]. */
  get textDirection(): TextDirection | null {
    return this._textDirection;
  }

  set textDirection(value: TextDirection | null) {
    if (this._textDirection !== value) {
      this._textDirection = value;
      this.markNeedsLayout();
    }
  }

  /** How to size the non-positioned children. */
  get fit(): StackFit {
    return this._fit;
  }

  set fit(value: StackFit) {
    if (this._fit !== value) {
      this._fit = value;
      this.markNeedsLayout();
    }
  }

  /** Whether a positioned child extended beyond the stack in the last layout. */
  get hasVisualOverflow(): boolean {
    return this._hasVisualOverflow;
  }

  protected get resolvedAlignment(): Alignment {
    return this._alignment.resolve(this._textDirection);
  }

  protected nonPositionedConstraints(constraints: BoxConstraints): BoxConstraints {
    switch (this._fit) {
      case StackFit.loose:
        return constraints.loosen();
      case StackFit.expand:
        return BoxConstraints.tight(constraints.biggest);
      case StackFit.passthrough:
        return constraints;
    }
  }

  /** The largest `mainChildSizeGetter` of the non-positioned children. */
  static getIntrinsicDimension(
    firstChild: RenderBox | null,
    mainChildSizeGetter: (child: RenderBox) => number,
  ): number {
    let extent = 0;
    let child = firstChild;
    while (child !== null) {
      const childParentData = stackParentData(child);
      if (!childParentData.isPositioned) {
        extent = Math.max(extent, mainChildSizeGetter(child));
      }
      child = childParentData.nextSibling;
    }
    return extent;
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return RenderStack.getIntrinsicDimension(this.firstChild, (child) => child.getMinIntrinsicWidth(height));
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    return RenderStack.getIntrinsicDimension(this.firstChild, (child) => child.getMaxIntrinsicWidth(height));
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return RenderStack.getIntrinsicDimension(this.firstChild, (child) => child.getMinIntrinsicHeight(width));
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return RenderStack.getIntrinsicDimension(this.firstChild, (child) => child.getMaxIntrinsicHeight(width));
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    return this.defaultComputeDistanceToHighestActualBaseline(baseline);
  }

  /**
   * Lays out a positioned `child` in a stack of `size`, sets its offset and
   * returns whether it extends beyond the stack.
   */
  static layoutPositionedChild(
    child: RenderBox,
    childParentData: StackParentData,
    size: Size,
    alignment: Alignment,
  ): boolean {
    child.layout(childParentData.positionedChildConstraints(size), { parentUsesSize: true });
    const childSize = child.size;
    const x =
      childParentData.left !== null
        ? childParentData.left
        : childParentData.right !== null
          ? size.width - childParentData.right - childSize.width
          : alignment.alongOffset(remaining(size, childSize)).dx;
    const y =
      childParentData.top !== null
        ? childParentData.top
        : childParentData.bottom !== null
          ? size.height - childParentData.bottom - childSize.height
          : alignment.alongOffset(remaining(size, childSize)).dy;
    childParentData.offset = new Offset(x, y);
    return x < 0 || x + childSize.width > size.width || y < 0 || y + childSize.height > size.height;
  }

  protected static baselineForChild(
    child: RenderBox,
    stackSize: Size,
    nonPositionedChildConstraints: BoxConstraints,
    alignment: Alignment,
    baseline: TextBaseline,
  ): number | null {
    const childParentData = stackParentData(child);
    const childConstraints = childParentData.isPositioned
      ? childParentData.positionedChildConstraints(stackSize)
      : nonPositionedChildConstraints;
    const baselineOffset = child.getDryBaseline(childConstraints, baseline);
    if (baselineOffset === null) {
      return null;
    }
    const y =
      childParentData.top !== null
        ? childParentData.top
        : childParentData.bottom !== null
          ? stackSize.height - childParentData.bottom - child.getDryLayout(childConstraints).height
          : alignment.alongOffset(remaining(stackSize, child.getDryLayout(childConstraints))).dy;
    return baselineOffset + y;
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    const nonPositioned = this.nonPositionedConstraints(constraints);
    const alignment = this.resolvedAlignment;
    const size = this.getDryLayout(constraints);
    let baselineOffset = BaselineOffset.noBaseline;
    for (let child = this.firstChild; child !== null; child = this.childAfter(child)) {
      baselineOffset = baselineOffset.minOf(
        new BaselineOffset(RenderStack.baselineForChild(child, size, nonPositioned, alignment, baseline)),
      );
    }
    return baselineOffset.offset;
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return this._computeSize(constraints, dryLayoutChild);
  }

  private _computeSize(constraints: BoxConstraints, layout: ChildLayouter): Size {
    if (this.childCount === 0) {
      return constraints.biggest.isFinite ? constraints.biggest : constraints.smallest;
    }
    let hasNonPositionedChildren = false;
    let width = constraints.minWidth;
    let height = constraints.minHeight;
    const nonPositioned = this.nonPositionedConstraints(constraints);
    let child = this.firstChild;
    while (child !== null) {
      const childParentData = stackParentData(child);
      if (!childParentData.isPositioned) {
        hasNonPositionedChildren = true;
        const childSize = layout(child, nonPositioned);
        width = Math.max(width, childSize.width);
        height = Math.max(height, childSize.height);
      }
      child = childParentData.nextSibling;
    }
    return hasNonPositionedChildren ? new Size(width, height) : constraints.biggest;
  }

  protected override performLayout(): void {
    const constraints = this.constraints;
    this._hasVisualOverflow = false;
    this.size = this._computeSize(constraints, layoutChild);
    const size = this.size;
    const resolvedAlignment = this.resolvedAlignment;
    let child = this.firstChild;
    while (child !== null) {
      const childParentData = stackParentData(child);
      if (!childParentData.isPositioned) {
        childParentData.offset = resolvedAlignment.alongOffset(remaining(size, child.size));
      } else {
        this._hasVisualOverflow =
          RenderStack.layoutPositionedChild(child, childParentData, size, resolvedAlignment) || this._hasVisualOverflow;
      }
      child = childParentData.nextSibling;
    }
  }
}

export interface RenderIndexedStackOptions extends RenderStackOptions {
  index?: number | null;
}

/**
 * A [RenderStack] that shows only the child at [index]. Every child is still
 * laid out and sizes the stack; only baselines (and, in Flutter, painting and
 * hit testing) consider the displayed child alone.
 */
export class RenderIndexedStack extends RenderStack {
  private _index: number | null;

  constructor(options: RenderIndexedStackOptions = {}) {
    super(options);
    this._index = options.index === undefined ? 0 : options.index;
  }

  /** The index of the displayed child, or null for none. */
  get index(): number | null {
    return this._index;
  }

  set index(value: number | null) {
    if (this._index !== value) {
      this._index = value;
      this.markNeedsLayout();
    }
  }

  /** The displayed child; null when [index] is null or past the last child. */
  get displayedChild(): RenderBox | null {
    const index = this._index;
    if (index === null) {
      return null;
    }
    let child = this.firstChild;
    for (let i = 0; i < index && child !== null; i += 1) {
      child = this.childAfter(child);
    }
    return child;
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    const displayedChild = this.displayedChild;
    if (displayedChild === null) {
      return null;
    }
    return new BaselineOffset(displayedChild.getDistanceToActualBaseline(baseline)).plus(
      stackParentData(displayedChild).offset.dy,
    ).offset;
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    const displayedChild = this.displayedChild;
    if (displayedChild === null) {
      return null;
    }
    return RenderStack.baselineForChild(
      displayedChild,
      this.getDryLayout(constraints),
      this.nonPositionedConstraints(constraints),
      this.resolvedAlignment,
      baseline,
    );
  }
}
