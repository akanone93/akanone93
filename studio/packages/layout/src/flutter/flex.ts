// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/flex.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Flex layout: children in a row or column, with flexible children sharing
 * the free main-axis space (`RenderFlex`, the render object of `Row`,
 * `Column` and `Flex`).
 *
 * Painting, clipping, hit testing and the debug overflow indicator are not
 * ported; the amount of main-axis overflow is exposed as [RenderFlex.overflow]
 * so the Studio canvas can draw it (STU-056).
 *
 * Flutter checks several preconditions only with `assert`, which is compiled
 * out of release builds. This port follows RELEASE behaviour:
 * - flexible children under an unbounded main axis are not flexed; they are
 *   laid out like inflexible children (debug Flutter reports a FlutterError);
 * - a negative `spacing` is not rejected (debug Flutter asserts `spacing >= 0`);
 * - a missing `textDirection` or `verticalDirection` that would be needed to
 *   order the children falls back to left-to-right / top-to-bottom.
 * The one precondition Flutter enforces in release builds — a
 * `CrossAxisAlignment.baseline` row without a `textBaseline` — throws.
 */

import { BoxConstraints } from "./constraints.ts";
import { ContainerBoxParentData, ContainerRenderBox } from "./container.ts";
import { Offset, precisionErrorTolerance, Size, TextDirection } from "./geometry.ts";
import type { RenderObject } from "./object.ts";
import { BaselineOffset, type RenderBox, type TextBaseline } from "./render-box.ts";

/** The two cardinal directions in two dimensions (painting/basic_types.dart `Axis`). */
export enum Axis {
  horizontal = "horizontal",
  vertical = "vertical",
}

/**
 * The direction in which boxes flow vertically (painting/basic_types.dart
 * `VerticalDirection`).
 */
export enum VerticalDirection {
  /** Boxes start at the bottom and are stacked upwards. */
  up = "up",
  /** Boxes start at the top and are stacked downwards. */
  down = "down",
}

/** How a flexible child is inscribed into the available space. */
export enum FlexFit {
  /** The child is forced to fill the available space (`Expanded`). */
  tight = "tight",
  /** The child can be at most as large as the available space (`Flexible`). */
  loose = "loose",
}

/** Parent data for use with [RenderFlex]. */
export class FlexParentData extends ContainerBoxParentData<RenderBox> {
  /** The flex factor; null or zero means the child is inflexible. */
  flex: number | null = null;
  /** How a flexible child is inscribed into the available space; null means [FlexFit.tight]. */
  fit: FlexFit | null = null;
}

/** How much space should be occupied in the main axis. */
export enum MainAxisSize {
  /** Minimise the free space along the main axis, subject to the incoming constraints. */
  min = "min",
  /** Maximise the free space along the main axis, subject to the incoming constraints. */
  max = "max",
}

/** How the children should be placed along the main axis in a flex layout. */
export enum MainAxisAlignment {
  start = "start",
  end = "end",
  center = "center",
  spaceBetween = "spaceBetween",
  spaceAround = "spaceAround",
  spaceEvenly = "spaceEvenly",
}

/** How the children should be placed along the cross axis in a flex layout. */
export enum CrossAxisAlignment {
  start = "start",
  end = "end",
  center = "center",
  stretch = "stretch",
  baseline = "baseline",
}

/** `MainAxisAlignment._distributeSpace`: the leading space and the space between children. */
function distributeSpace(
  alignment: MainAxisAlignment,
  freeSpace: number,
  itemCount: number,
  flipped: boolean,
  spacing: number,
): [leadingSpace: number, betweenSpace: number] {
  switch (alignment) {
    case MainAxisAlignment.start:
      return flipped ? [freeSpace, spacing] : [0, spacing];
    case MainAxisAlignment.end:
      return distributeSpace(MainAxisAlignment.start, freeSpace, itemCount, !flipped, spacing);
    case MainAxisAlignment.center:
      return [freeSpace / 2.0, spacing];
    case MainAxisAlignment.spaceBetween:
      if (itemCount < 2) {
        return distributeSpace(MainAxisAlignment.start, freeSpace, itemCount, flipped, spacing);
      }
      return [0, freeSpace / (itemCount - 1) + spacing];
    case MainAxisAlignment.spaceAround:
      if (itemCount === 0) {
        return distributeSpace(MainAxisAlignment.start, freeSpace, itemCount, flipped, spacing);
      }
      return [freeSpace / itemCount / 2, freeSpace / itemCount + spacing];
    case MainAxisAlignment.spaceEvenly:
      return [freeSpace / (itemCount + 1), freeSpace / (itemCount + 1) + spacing];
  }
}

/** `CrossAxisAlignment._getChildCrossAxisOffset`; not used for baseline-aligned children. */
function getChildCrossAxisOffset(alignment: CrossAxisAlignment, freeSpace: number, flipped: boolean): number {
  switch (alignment) {
    case CrossAxisAlignment.stretch:
    case CrossAxisAlignment.baseline:
      return 0;
    case CrossAxisAlignment.start:
      return flipped ? freeSpace : 0;
    case CrossAxisAlignment.center:
      return freeSpace / 2;
    case CrossAxisAlignment.end:
      return getChildCrossAxisOffset(CrossAxisAlignment.start, freeSpace, !flipped);
  }
}

/** A size in main/cross axis terms (`_AxisSize`): `width` is the main extent. */
class AxisSize {
  static readonly empty = new AxisSize(0, 0);

  constructor(
    readonly mainAxisExtent: number,
    readonly crossAxisExtent: number,
  ) {}

  static fromSize(size: Size, direction: Axis): AxisSize {
    return direction === Axis.horizontal
      ? new AxisSize(size.width, size.height)
      : new AxisSize(size.height, size.width);
  }

  toSize(direction: Axis): Size {
    return direction === Axis.horizontal
      ? new Size(this.mainAxisExtent, this.crossAxisExtent)
      : new Size(this.crossAxisExtent, this.mainAxisExtent);
  }

  applyConstraints(constraints: BoxConstraints, direction: Axis): AxisSize {
    const effective = direction === Axis.horizontal ? constraints : constraints.flipped;
    const constrained = effective.constrain(new Size(this.mainAxisExtent, this.crossAxisExtent));
    return new AxisSize(constrained.width, constrained.height);
  }

  plus(other: AxisSize): AxisSize {
    return new AxisSize(
      this.mainAxisExtent + other.mainAxisExtent,
      Math.max(this.crossAxisExtent, other.crossAxisExtent),
    );
  }
}

/** The ascent and descent of baseline-aligned children (`_AscentDescent`); null is none. */
type AscentDescent = readonly [ascent: number, descent: number] | null;

function ascentDescent(baselineOffset: number | null, crossSize: number): AscentDescent {
  return baselineOffset === null ? null : [baselineOffset, crossSize - baselineOffset];
}

function addAscentDescent(x: AscentDescent, y: AscentDescent): AscentDescent {
  if (x === null) {
    return y;
  }
  if (y === null) {
    return x;
  }
  return [Math.max(x[0], y[0]), Math.max(x[1], y[1])];
}

type ChildLayouter = (child: RenderBox, constraints: BoxConstraints) => Size;
type ChildBaselineGetter = (child: RenderBox, constraints: BoxConstraints, baseline: TextBaseline) => number | null;

/** `ChildLayoutHelper.layoutChild`. */
function layoutChild(child: RenderBox, constraints: BoxConstraints): Size {
  child.layout(constraints, { parentUsesSize: true });
  return child.size;
}

/** `ChildLayoutHelper.dryLayoutChild`. */
function dryLayoutChild(child: RenderBox, constraints: BoxConstraints): Size {
  return child.getDryLayout(constraints);
}

/** `ChildLayoutHelper.getBaseline`. */
function getBaseline(child: RenderBox, _constraints: BoxConstraints, baseline: TextBaseline): number | null {
  return child.getDistanceToBaseline(baseline, { onlyReal: true });
}

/** `ChildLayoutHelper.getDryBaseline`. */
function getDryBaseline(child: RenderBox, constraints: BoxConstraints, baseline: TextBaseline): number | null {
  return child.getDryBaseline(constraints, baseline);
}

/** The result of [RenderFlex._computeSizes] (`_LayoutSizes`). */
interface LayoutSizes {
  /** The final constrained size of the flex. */
  readonly axisSize: AxisSize;
  /** Free main-axis space; negative when the children overflow. */
  readonly mainAxisFreeSpace: number;
  /** Null unless baseline aligned with at least one child that has the baseline. */
  readonly baselineOffset: number | null;
  /** The space allocated per flex unit; null when no child flexes. */
  readonly spacePerFlex: number | null;
}

/** Options of the [RenderFlex] constructor, with Flutter's defaults. */
export interface RenderFlexOptions {
  children?: readonly RenderBox[];
  direction?: Axis;
  mainAxisSize?: MainAxisSize;
  mainAxisAlignment?: MainAxisAlignment;
  crossAxisAlignment?: CrossAxisAlignment;
  textDirection?: TextDirection | null;
  verticalDirection?: VerticalDirection;
  textBaseline?: TextBaseline | null;
  spacing?: number;
}

/** Displays its children in a one-dimensional array, with flexible children sharing free space. */
export class RenderFlex extends ContainerRenderBox<RenderBox> {
  private _direction: Axis;
  private _mainAxisAlignment: MainAxisAlignment;
  private _mainAxisSize: MainAxisSize;
  private _crossAxisAlignment: CrossAxisAlignment;
  private _textDirection: TextDirection | null;
  private _verticalDirection: VerticalDirection;
  private _textBaseline: TextBaseline | null;
  private _spacing: number;
  /** Set during layout to the main-axis overflow. */
  private _overflow = 0;

  constructor(options: RenderFlexOptions = {}) {
    super();
    this._direction = options.direction ?? Axis.horizontal;
    this._mainAxisAlignment = options.mainAxisAlignment ?? MainAxisAlignment.start;
    this._mainAxisSize = options.mainAxisSize ?? MainAxisSize.max;
    this._crossAxisAlignment = options.crossAxisAlignment ?? CrossAxisAlignment.center;
    this._textDirection = options.textDirection ?? null;
    this._verticalDirection = options.verticalDirection ?? VerticalDirection.down;
    this._textBaseline = options.textBaseline ?? null;
    this._spacing = options.spacing ?? 0;
    this.addAll(options.children ?? []);
  }

  /** The direction to use as the main axis. */
  get direction(): Axis {
    return this._direction;
  }

  set direction(value: Axis) {
    if (this._direction !== value) {
      this._direction = value;
      this.markNeedsLayout();
    }
  }

  /** How the children are placed along the main axis. */
  get mainAxisAlignment(): MainAxisAlignment {
    return this._mainAxisAlignment;
  }

  set mainAxisAlignment(value: MainAxisAlignment) {
    if (this._mainAxisAlignment !== value) {
      this._mainAxisAlignment = value;
      this.markNeedsLayout();
    }
  }

  /** How much space is occupied in the main axis. */
  get mainAxisSize(): MainAxisSize {
    return this._mainAxisSize;
  }

  set mainAxisSize(value: MainAxisSize) {
    if (this._mainAxisSize !== value) {
      this._mainAxisSize = value;
      this.markNeedsLayout();
    }
  }

  /** How the children are placed along the cross axis. */
  get crossAxisAlignment(): CrossAxisAlignment {
    return this._crossAxisAlignment;
  }

  set crossAxisAlignment(value: CrossAxisAlignment) {
    if (this._crossAxisAlignment !== value) {
      this._crossAxisAlignment = value;
      this.markNeedsLayout();
    }
  }

  /** The horizontal order of the children; null behaves as left-to-right. */
  get textDirection(): TextDirection | null {
    return this._textDirection;
  }

  set textDirection(value: TextDirection | null) {
    if (this._textDirection !== value) {
      this._textDirection = value;
      this.markNeedsLayout();
    }
  }

  /** The vertical order of the children. */
  get verticalDirection(): VerticalDirection {
    return this._verticalDirection;
  }

  set verticalDirection(value: VerticalDirection) {
    if (this._verticalDirection !== value) {
      this._verticalDirection = value;
      this.markNeedsLayout();
    }
  }

  /** The baseline to align children to under [CrossAxisAlignment.baseline]. */
  get textBaseline(): TextBaseline | null {
    return this._textBaseline;
  }

  set textBaseline(value: TextBaseline | null) {
    if (this._textBaseline !== value) {
      this._textBaseline = value;
      this.markNeedsLayout();
    }
  }

  /** The space between consecutive children along the main axis. */
  get spacing(): number {
    return this._spacing;
  }

  set spacing(value: number) {
    if (this._spacing !== value) {
      this._spacing = value;
      this.markNeedsLayout();
    }
  }

  /**
   * The amount by which the children overflow the main axis after layout;
   * 0 when they fit. Overflow within [precisionErrorTolerance] counts as
   * none, as in Flutter's `_hasOverflow`.
   */
  get overflow(): number {
    return this._overflow > precisionErrorTolerance ? this._overflow : 0;
  }

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof FlexParentData)) {
      child.parentData = new FlexParentData();
    }
  }

  private _getIntrinsicSize(
    sizingDirection: Axis,
    extent: number,
    childSize: (child: RenderBox, extent: number) => number,
  ): number {
    if (this._direction === sizingDirection) {
      // INTRINSIC MAIN SIZE: the smallest size that keeps the min/max-content
      // contributions of the flex items.
      let totalFlex = 0;
      let inflexibleSpace = this.spacing * (this.childCount - 1);
      let maxFlexFractionSoFar = 0;
      for (let child = this.firstChild; child !== null; child = this.childAfter(child)) {
        const flex = RenderFlex._getFlex(child);
        totalFlex += flex;
        if (flex > 0) {
          const flexFraction = childSize(child, extent) / flex;
          maxFlexFractionSoFar = Math.max(maxFlexFractionSoFar, flexFraction);
        } else {
          inflexibleSpace += childSize(child, extent);
        }
      }
      return maxFlexFractionSoFar * totalFlex + inflexibleSpace;
    }
    // INTRINSIC CROSS SIZE: the largest child cross size after the flexible
    // children are fit into the available space, with every child sized by
    // its max intrinsic main extent.
    const isHorizontal = this.direction === Axis.horizontal;
    const intrinsicLayoutChild = (child: RenderBox, constraints: BoxConstraints): Size => {
      const mainAxisSizeFromConstraints = isHorizontal ? constraints.maxWidth : constraints.maxHeight;
      const maxMainAxisSize = Number.isFinite(mainAxisSizeFromConstraints)
        ? mainAxisSizeFromConstraints
        : isHorizontal
          ? child.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY)
          : child.getMaxIntrinsicHeight(Number.POSITIVE_INFINITY);
      return isHorizontal
        ? new Size(maxMainAxisSize, childSize(child, maxMainAxisSize))
        : new Size(childSize(child, maxMainAxisSize), maxMainAxisSize);
    };
    return this._computeSizes(
      isHorizontal ? new BoxConstraints({ maxWidth: extent }) : new BoxConstraints({ maxHeight: extent }),
      intrinsicLayoutChild,
      getDryBaseline,
    ).axisSize.crossAxisExtent;
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return this._getIntrinsicSize(Axis.horizontal, height, (child, extent) => child.getMinIntrinsicWidth(extent));
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    return this._getIntrinsicSize(Axis.horizontal, height, (child, extent) => child.getMaxIntrinsicWidth(extent));
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return this._getIntrinsicSize(Axis.vertical, width, (child, extent) => child.getMinIntrinsicHeight(extent));
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this._getIntrinsicSize(Axis.vertical, width, (child, extent) => child.getMaxIntrinsicHeight(extent));
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    return this._direction === Axis.horizontal
      ? this.defaultComputeDistanceToHighestActualBaseline(baseline)
      : this.defaultComputeDistanceToFirstActualBaseline(baseline);
  }

  private static _flexParentData(child: RenderBox): FlexParentData {
    const parentData = child.parentData;
    if (!(parentData instanceof FlexParentData)) {
      throw new Error("RenderFlex: child has no FlexParentData; it is not a child of a RenderFlex");
    }
    return parentData;
  }

  private static _getFlex(child: RenderBox): number {
    return RenderFlex._flexParentData(child).flex ?? 0;
  }

  private static _getFit(child: RenderBox): FlexFit {
    return RenderFlex._flexParentData(child).fit ?? FlexFit.tight;
  }

  private get _isBaselineAligned(): boolean {
    return this.crossAxisAlignment === CrossAxisAlignment.baseline && this.direction === Axis.horizontal;
  }

  private _getCrossSize(size: Size): number {
    return this._direction === Axis.horizontal ? size.height : size.width;
  }

  private _getMainSize(size: Size): number {
    return this._direction === Axis.horizontal ? size.width : size.height;
  }

  /** Whether children are laid out right-to-left or bottom-to-top. False without children. */
  private get _flipMainAxis(): boolean {
    if (this.firstChild === null) {
      return false;
    }
    return this.direction === Axis.horizontal
      ? this.textDirection === TextDirection.rtl
      : this.verticalDirection === VerticalDirection.up;
  }

  /** Whether the cross axis runs right-to-left or bottom-to-top. False without children. */
  private get _flipCrossAxis(): boolean {
    if (this.firstChild === null) {
      return false;
    }
    return this.direction === Axis.vertical
      ? this.textDirection === TextDirection.rtl
      : this.verticalDirection === VerticalDirection.up;
  }

  private _constraintsForNonFlexChild(constraints: BoxConstraints): BoxConstraints {
    const fillCrossAxis = this.crossAxisAlignment === CrossAxisAlignment.stretch;
    if (this._direction === Axis.horizontal) {
      return fillCrossAxis
        ? BoxConstraints.tightFor({ height: constraints.maxHeight })
        : new BoxConstraints({ maxHeight: constraints.maxHeight });
    }
    return fillCrossAxis
      ? BoxConstraints.tightFor({ width: constraints.maxWidth })
      : new BoxConstraints({ maxWidth: constraints.maxWidth });
  }

  private _constraintsForFlexChild(
    child: RenderBox,
    constraints: BoxConstraints,
    maxChildExtent: number,
  ): BoxConstraints {
    const minChildExtent = RenderFlex._getFit(child) === FlexFit.tight ? maxChildExtent : 0;
    const fillCrossAxis = this.crossAxisAlignment === CrossAxisAlignment.stretch;
    if (this._direction === Axis.horizontal) {
      return new BoxConstraints({
        minWidth: minChildExtent,
        maxWidth: maxChildExtent,
        minHeight: fillCrossAxis ? constraints.maxHeight : 0,
        maxHeight: constraints.maxHeight,
      });
    }
    return new BoxConstraints({
      minWidth: fillCrossAxis ? constraints.maxWidth : 0,
      maxWidth: constraints.maxWidth,
      minHeight: minChildExtent,
      maxHeight: maxChildExtent,
    });
  }

  /** The constraints a child receives in a layout that produced `sizes`. */
  private _constraintsForChild(child: RenderBox, constraints: BoxConstraints, sizes: LayoutSizes): BoxConstraints {
    const spacePerFlex = sizes.spacePerFlex;
    const flex = RenderFlex._getFlex(child);
    return spacePerFlex !== null && flex > 0
      ? this._constraintsForFlexChild(child, constraints, flex * spacePerFlex)
      : this._constraintsForNonFlexChild(constraints);
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    const sizes = this._computeSizes(constraints, dryLayoutChild, getDryBaseline);
    if (this._isBaselineAligned) {
      return sizes.baselineOffset;
    }
    return this._direction === Axis.horizontal
      ? this._computeDryDistanceToHighestBaseline(constraints, baseline, sizes)
      : this._computeDryDistanceToFirstBaseline(constraints, baseline, sizes);
  }

  /**
   * The cross-axis offset of a child (shared by [performLayout] and the dry
   * baseline). In a baseline-aligned row a child without the baseline is
   * aligned to the top, regardless of verticalDirection: flipCrossAxis is
   * ignored, as in Flutter.
   */
  private _childCrossPosition(
    crossAxisExtent: number,
    child: RenderBox,
    childCrossSize: (child: RenderBox) => number,
    baselineOffset: number | null,
    childBaselineOffset: number | null,
    flipCrossAxis: boolean,
  ): number {
    if (baselineOffset !== null && childBaselineOffset !== null) {
      return baselineOffset - childBaselineOffset;
    }
    const baselineRow = this.crossAxisAlignment === CrossAxisAlignment.baseline && this.direction === Axis.horizontal;
    return getChildCrossAxisOffset(
      baselineRow ? CrossAxisAlignment.start : this.crossAxisAlignment,
      crossAxisExtent - childCrossSize(child),
      baselineRow ? false : flipCrossAxis,
    );
  }

  /** Dry equivalent of [defaultComputeDistanceToHighestActualBaseline]. */
  private _computeDryDistanceToHighestBaseline(
    constraints: BoxConstraints,
    baseline: TextBaseline,
    sizes: LayoutSizes,
  ): number | null {
    const flipMainAxis = this._flipMainAxis;
    const flipCrossAxis = this._flipCrossAxis;
    const nextChild = (child: RenderBox): RenderBox | null =>
      flipMainAxis ? this.childBefore(child) : this.childAfter(child);
    const topLeftChild = flipMainAxis ? this.lastChild : this.firstChild;
    const textBaseline = this.textBaseline;
    const baselineOffset = this._isBaselineAligned && textBaseline !== null ? sizes.baselineOffset : null;
    let minBaseline = BaselineOffset.noBaseline;
    for (let child = topLeftChild; child !== null; child = nextChild(child)) {
      const childConstraints = this._constraintsForChild(child, constraints, sizes);
      const childBaseline = child.getDryBaseline(childConstraints, baseline);
      if (childBaseline !== null) {
        const childBaselineOffset =
          this._isBaselineAligned && textBaseline !== null
            ? child.getDryBaseline(childConstraints, textBaseline)
            : null;
        const childCrossPosition = this._childCrossPosition(
          sizes.axisSize.crossAxisExtent,
          child,
          (c) => this._getCrossSize(c.getDryLayout(childConstraints)),
          baselineOffset,
          childBaselineOffset,
          flipCrossAxis,
        );
        minBaseline = minBaseline.minOf(new BaselineOffset(childBaseline).plus(childCrossPosition));
      }
    }
    return minBaseline.offset;
  }

  /** Dry equivalent of [defaultComputeDistanceToFirstActualBaseline]. */
  private _computeDryDistanceToFirstBaseline(
    constraints: BoxConstraints,
    baseline: TextBaseline,
    sizes: LayoutSizes,
  ): number | null {
    const remainingSpace = Math.max(0, sizes.mainAxisFreeSpace);
    const flipMainAxis = this._flipMainAxis;
    const [leadingSpace, betweenSpace] = distributeSpace(
      this.mainAxisAlignment,
      remainingSpace,
      this.childCount,
      flipMainAxis,
      this.spacing,
    );
    const mainPositions = new Map<RenderBox, number>();
    const nextChildPaintOrder = (child: RenderBox): RenderBox | null =>
      flipMainAxis ? this.childBefore(child) : this.childAfter(child);
    let pos = leadingSpace;
    for (
      let child = flipMainAxis ? this.lastChild : this.firstChild;
      child !== null;
      child = nextChildPaintOrder(child)
    ) {
      mainPositions.set(child, pos);
      const childSize = child.getDryLayout(this._constraintsForChild(child, constraints, sizes));
      pos += this._getMainSize(childSize) + betweenSpace;
    }
    for (let child = this.firstChild; child !== null; child = this.childAfter(child)) {
      const childBaseline = child.getDryBaseline(this._constraintsForChild(child, constraints, sizes), baseline);
      if (childBaseline !== null) {
        return childBaseline + (mainPositions.get(child) ?? leadingSpace);
      }
    }
    return null;
  }

  /**
   * The dry size. Debug Flutter returns [Size.zero] for flexible children
   * under an unbounded main axis; release Flutter, which this follows,
   * computes the sizes with those children treated as inflexible.
   */
  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return this._computeSizes(constraints, dryLayoutChild, getDryBaseline).axisSize.toSize(this.direction);
  }

  /** The baseline to align to; null unless baseline aligned. Flutter throws here in release builds too. */
  private _requiredTextBaseline(): TextBaseline | null {
    if (!this._isBaselineAligned) {
      return null;
    }
    if (this.textBaseline === null) {
      throw new Error(
        'To use CrossAxisAlignment.baseline, you must also specify which baseline to use using the "textBaseline" argument.',
      );
    }
    return this.textBaseline;
  }

  private _computeSizes(
    constraints: BoxConstraints,
    layoutChildFn: ChildLayouter,
    getBaselineFn: ChildBaselineGetter,
  ): LayoutSizes {
    // Determine used flex factor, size inflexible items, calculate free space.
    const maxMainSize = this._getMainSize(constraints.biggest);
    const canFlex = Number.isFinite(maxMainSize);
    const nonFlexChildConstraints = this._constraintsForNonFlexChild(constraints);
    // Null means the children are not baseline aligned.
    const textBaseline = this._requiredTextBaseline();

    // The first pass lays out non-flex children and computes total flex.
    let totalFlex = 0;
    let firstFlexChild: RenderBox | null = null;
    let accumulatedAscentDescent: AscentDescent = null;
    // Initially, accumulatedSize is the sum of the spaces between children in the main axis.
    let accumulatedSize = new AxisSize(this.spacing * (this.childCount - 1), 0);
    // Lays out one child and adds its size and baseline to the accumulators.
    const accumulate = (child: RenderBox, childConstraints: BoxConstraints): void => {
      const childSize = AxisSize.fromSize(layoutChildFn(child, childConstraints), this.direction);
      accumulatedSize = accumulatedSize.plus(childSize);
      // Baseline-aligned children contribute to the cross axis extent separately.
      const baselineOffset = textBaseline === null ? null : getBaselineFn(child, childConstraints, textBaseline);
      accumulatedAscentDescent = addAscentDescent(
        accumulatedAscentDescent,
        ascentDescent(baselineOffset, childSize.crossAxisExtent),
      );
    };
    for (let child = this.firstChild; child !== null; child = this.childAfter(child)) {
      const flex = canFlex ? RenderFlex._getFlex(child) : 0;
      if (flex > 0) {
        totalFlex += flex;
        firstFlexChild ??= child;
      } else {
        accumulate(child, nonFlexChildConstraints);
      }
    }

    // The second pass distributes free space to flexible children.
    const flexSpace = Math.max(0, maxMainSize - accumulatedSize.mainAxisExtent);
    const spacePerFlex = flexSpace / totalFlex;
    for (let child = firstFlexChild; child !== null && totalFlex > 0; child = this.childAfter(child)) {
      const flex = RenderFlex._getFlex(child);
      if (flex === 0) {
        continue;
      }
      totalFlex -= flex;
      const maxChildExtent = spacePerFlex * flex;
      const childConstraints = this._constraintsForFlexChild(child, constraints, maxChildExtent);
      accumulate(child, childConstraints);
    }

    // The overall height of baseline-aligned children contributes to the cross axis extent.
    accumulatedSize = accumulatedSize.plus(
      accumulatedAscentDescent === null
        ? AxisSize.empty
        : new AxisSize(0, accumulatedAscentDescent[0] + accumulatedAscentDescent[1]),
    );

    const idealMainSize =
      this.mainAxisSize === MainAxisSize.max && Number.isFinite(maxMainSize)
        ? maxMainSize
        : accumulatedSize.mainAxisExtent;

    const constrainedSize = new AxisSize(idealMainSize, accumulatedSize.crossAxisExtent).applyConstraints(
      constraints,
      this.direction,
    );
    return {
      axisSize: constrainedSize,
      mainAxisFreeSpace: constrainedSize.mainAxisExtent - accumulatedSize.mainAxisExtent,
      baselineOffset: accumulatedAscentDescent === null ? null : accumulatedAscentDescent[0],
      spacePerFlex: firstFlexChild === null ? null : spacePerFlex,
    };
  }

  /**
   * Lays out the children and positions them in visual order. Debug Flutter
   * throws for flexible children under an unbounded main axis; release
   * Flutter, which this follows, lays them out as inflexible children.
   */
  protected override performLayout(): void {
    const constraints = this.constraints;
    const sizes = this._computeSizes(constraints, layoutChild, getBaseline);

    const crossAxisExtent = sizes.axisSize.crossAxisExtent;
    this.size = sizes.axisSize.toSize(this.direction);
    this._overflow = Math.max(0, -sizes.mainAxisFreeSpace);

    const remainingSpace = Math.max(0, sizes.mainAxisFreeSpace);
    const flipMainAxis = this._flipMainAxis;
    const flipCrossAxis = this._flipCrossAxis;
    const [leadingSpace, betweenSpace] = distributeSpace(
      this.mainAxisAlignment,
      remainingSpace,
      this.childCount,
      flipMainAxis,
      this.spacing,
    );
    const nextChild = (child: RenderBox): RenderBox | null =>
      flipMainAxis ? this.childBefore(child) : this.childAfter(child);
    const topLeftChild = flipMainAxis ? this.lastChild : this.firstChild;
    const baselineOffset = sizes.baselineOffset;
    const textBaseline = this.textBaseline;

    // Position all children in visual order: from the top-left child towards
    // the child farthest from the origin.
    let childMainPosition = leadingSpace;
    for (let child = topLeftChild; child !== null; child = nextChild(child)) {
      const childBaselineOffset =
        baselineOffset !== null && textBaseline !== null
          ? child.getDistanceToBaseline(textBaseline, { onlyReal: true })
          : null;
      const childCrossPosition = this._childCrossPosition(
        crossAxisExtent,
        child,
        (c) => this._getCrossSize(c.size),
        baselineOffset,
        childBaselineOffset,
        flipCrossAxis,
      );
      RenderFlex._flexParentData(child).offset =
        this.direction === Axis.horizontal
          ? new Offset(childMainPosition, childCrossPosition)
          : new Offset(childCrossPosition, childMainPosition);
      childMainPosition += this._getMainSize(child.size) + betweenSpace;
    }
  }
}
