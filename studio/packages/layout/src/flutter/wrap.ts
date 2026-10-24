// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/wrap.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Wrap layout: children in runs along the main axis, starting a new run
 * when the next child does not fit (`RenderWrap`, the render object of
 * `Wrap`).
 *
 * Painting, clipping (`clipBehavior` never affects layout) and hit testing
 * are not ported; whether the children overflow the wrap is exposed as
 * [RenderWrap.hasVisualOverflow].
 *
 * Quirks of Flutter kept on purpose: dry layout (and so the intrinsics on
 * the cross axis) breaks runs without the `precisionErrorTolerance` that
 * real layout uses, and the dry baseline positions children with the
 * children's size as the free space, exactly as Flutter does.
 *
 * Flutter checks several preconditions only with `assert`, which is compiled
 * out of release builds. This port follows RELEASE behaviour: a missing
 * `textDirection` that debug Flutter requires to order the children or
 * resolve `start`/`end` falls back to left-to-right.
 */

import { BoxConstraints } from "./constraints.ts";
import { ContainerBoxParentData, ContainerRenderBox } from "./container.ts";
import { Axis, VerticalDirection } from "./flex.ts";
import { Offset, precisionErrorTolerance, Size, TextDirection } from "./geometry.ts";
import type { RenderObject } from "./object.ts";
import { BaselineOffset, type RenderBox, type TextBaseline } from "./render-box.ts";

/** How runs and the children of a run are placed along an axis (rendering/wrap.dart `WrapAlignment`). */
export enum WrapAlignment {
  start = "start",
  end = "end",
  center = "center",
  spaceBetween = "spaceBetween",
  spaceAround = "spaceAround",
  spaceEvenly = "spaceEvenly",
}

/** How the children of a run are aligned on the cross axis (rendering/wrap.dart `WrapCrossAlignment`). */
export enum WrapCrossAlignment {
  start = "start",
  end = "end",
  center = "center",
}

/** `WrapAlignment._distributeSpace`: leading space and space between items. */
function distributeSpace(
  alignment: WrapAlignment,
  freeSpace: number,
  itemSpacing: number,
  itemCount: number,
  flipped: boolean,
): [number, number] {
  switch (alignment) {
    case WrapAlignment.start:
      return [flipped ? freeSpace : 0, itemSpacing];
    case WrapAlignment.end:
      return distributeSpace(WrapAlignment.start, freeSpace, itemSpacing, itemCount, !flipped);
    case WrapAlignment.center:
      return [freeSpace / 2, itemSpacing];
    case WrapAlignment.spaceBetween:
      if (itemCount < 2) {
        return distributeSpace(WrapAlignment.start, freeSpace, itemSpacing, itemCount, flipped);
      }
      return [0, freeSpace / (itemCount - 1) + itemSpacing];
    case WrapAlignment.spaceAround:
      return [freeSpace / itemCount / 2, freeSpace / itemCount + itemSpacing];
    case WrapAlignment.spaceEvenly:
      return [freeSpace / (itemCount + 1), freeSpace / (itemCount + 1) + itemSpacing];
  }
}

function crossFlipped(alignment: WrapCrossAlignment): WrapCrossAlignment {
  switch (alignment) {
    case WrapCrossAlignment.start:
      return WrapCrossAlignment.end;
    case WrapCrossAlignment.end:
      return WrapCrossAlignment.start;
    case WrapCrossAlignment.center:
      return WrapCrossAlignment.center;
  }
}

function crossFactor(alignment: WrapCrossAlignment): number {
  switch (alignment) {
    case WrapCrossAlignment.start:
      return 0;
    case WrapCrossAlignment.end:
      return 1;
    case WrapCrossAlignment.center:
      return 0.5;
  }
}

/** `_AxisSize`: a size in (main, cross) coordinates. */
class AxisSize {
  static readonly empty = new AxisSize(0, 0);

  constructor(
    readonly main: number,
    readonly cross: number,
  ) {}

  static fromSize(size: Size, direction: Axis): AxisSize {
    return direction === Axis.horizontal
      ? new AxisSize(size.width, size.height)
      : new AxisSize(size.height, size.width);
  }

  toSize(direction: Axis): Size {
    return direction === Axis.horizontal ? new Size(this.main, this.cross) : new Size(this.cross, this.main);
  }

  applyConstraints(constraints: BoxConstraints, direction: Axis): AxisSize {
    const effective = direction === Axis.horizontal ? constraints : constraints.flipped;
    const size = effective.constrain(new Size(this.main, this.cross));
    return new AxisSize(size.width, size.height);
  }

  get flipped(): AxisSize {
    return new AxisSize(this.cross, this.main);
  }

  /** Main extents add, cross extents take the larger. */
  plus(other: AxisSize): AxisSize {
    return new AxisSize(this.main + other.main, Math.max(this.cross, other.cross));
  }

  minus(other: AxisSize): AxisSize {
    return new AxisSize(this.main - other.main, this.cross - other.cross);
  }
}

class RunMetrics {
  childCount = 1;

  constructor(
    public leadingChild: RenderBox,
    public axisSize: AxisSize,
  ) {}

  /** Adds `child` to this run, or returns a new run when it does not fit. */
  tryAddingNewChild(
    child: RenderBox,
    childSize: AxisSize,
    flipMainAxis: boolean,
    spacing: number,
    maxMainExtent: number,
  ): RunMetrics | null {
    const needsNewRun = this.axisSize.main + childSize.main + spacing - maxMainExtent > precisionErrorTolerance;
    if (needsNewRun) {
      return new RunMetrics(child, childSize);
    }
    this.axisSize = this.axisSize.plus(childSize.plus(new AxisSize(spacing, 0)));
    this.childCount += 1;
    if (flipMainAxis) {
      this.leadingChild = child;
    }
    return null;
  }
}

type ChildLayouter = (child: RenderBox, constraints: BoxConstraints) => Size;

function layoutChild(child: RenderBox, constraints: BoxConstraints): Size {
  child.layout(constraints, { parentUsesSize: true });
  return child.size;
}

function dryLayoutChild(child: RenderBox, constraints: BoxConstraints): Size {
  return child.getDryLayout(constraints);
}

/** Parent data for children of a [RenderWrap]. */
export class WrapParentData extends ContainerBoxParentData<RenderBox> {}

export interface RenderWrapOptions {
  children?: RenderBox[];
  direction?: Axis;
  alignment?: WrapAlignment;
  spacing?: number;
  runAlignment?: WrapAlignment;
  runSpacing?: number;
  crossAxisAlignment?: WrapCrossAlignment;
  textDirection?: TextDirection | null;
  verticalDirection?: VerticalDirection;
}

/** Displays its children in multiple horizontal or vertical runs. */
export class RenderWrap extends ContainerRenderBox<RenderBox> {
  private _direction: Axis;
  private _alignment: WrapAlignment;
  private _spacing: number;
  private _runAlignment: WrapAlignment;
  private _runSpacing: number;
  private _crossAxisAlignment: WrapCrossAlignment;
  private _textDirection: TextDirection | null;
  private _verticalDirection: VerticalDirection;
  private _hasVisualOverflow = false;

  constructor(options: RenderWrapOptions = {}) {
    super();
    this._direction = options.direction ?? Axis.horizontal;
    this._alignment = options.alignment ?? WrapAlignment.start;
    this._spacing = options.spacing ?? 0;
    this._runAlignment = options.runAlignment ?? WrapAlignment.start;
    this._runSpacing = options.runSpacing ?? 0;
    this._crossAxisAlignment = options.crossAxisAlignment ?? WrapCrossAlignment.start;
    this._textDirection = options.textDirection ?? null;
    this._verticalDirection = options.verticalDirection ?? VerticalDirection.down;
    this.addAll(options.children ?? []);
  }

  /** The main axis. */
  get direction(): Axis {
    return this._direction;
  }

  set direction(value: Axis) {
    if (this._direction !== value) {
      this._direction = value;
      this.markNeedsLayout();
    }
  }

  /** How the children of a run are placed on the main axis. */
  get alignment(): WrapAlignment {
    return this._alignment;
  }

  set alignment(value: WrapAlignment) {
    if (this._alignment !== value) {
      this._alignment = value;
      this.markNeedsLayout();
    }
  }

  /** The main-axis gap between children of a run. */
  get spacing(): number {
    return this._spacing;
  }

  set spacing(value: number) {
    if (this._spacing !== value) {
      this._spacing = value;
      this.markNeedsLayout();
    }
  }

  /** How the runs are placed on the cross axis. */
  get runAlignment(): WrapAlignment {
    return this._runAlignment;
  }

  set runAlignment(value: WrapAlignment) {
    if (this._runAlignment !== value) {
      this._runAlignment = value;
      this.markNeedsLayout();
    }
  }

  /** The cross-axis gap between runs. */
  get runSpacing(): number {
    return this._runSpacing;
  }

  set runSpacing(value: number) {
    if (this._runSpacing !== value) {
      this._runSpacing = value;
      this.markNeedsLayout();
    }
  }

  /** How the children of a run are aligned on the cross axis. */
  get crossAxisAlignment(): WrapCrossAlignment {
    return this._crossAxisAlignment;
  }

  set crossAxisAlignment(value: WrapCrossAlignment) {
    if (this._crossAxisAlignment !== value) {
      this._crossAxisAlignment = value;
      this.markNeedsLayout();
    }
  }

  /** The horizontal order of children and runs. */
  get textDirection(): TextDirection | null {
    return this._textDirection;
  }

  set textDirection(value: TextDirection | null) {
    if (this._textDirection !== value) {
      this._textDirection = value;
      this.markNeedsLayout();
    }
  }

  /** The vertical order of children and runs. */
  get verticalDirection(): VerticalDirection {
    return this._verticalDirection;
  }

  set verticalDirection(value: VerticalDirection) {
    if (this._verticalDirection !== value) {
      this._verticalDirection = value;
      this.markNeedsLayout();
    }
  }

  /** Whether the children overflowed the wrap in the last layout. */
  get hasVisualOverflow(): boolean {
    return this._hasVisualOverflow;
  }

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof WrapParentData)) {
      child.parentData = new WrapParentData();
    }
  }

  private _sumOrMax(extent: (child: RenderBox) => number, sum: boolean): number {
    let result = 0;
    for (let child = this.firstChild; child !== null; child = this.childAfter(child)) {
      result = sum ? result + extent(child) : Math.max(result, extent(child));
    }
    return result;
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return this._direction === Axis.horizontal
      ? this._sumOrMax((c) => c.getMinIntrinsicWidth(Number.POSITIVE_INFINITY), false)
      : this.getDryLayout(new BoxConstraints({ maxHeight: height })).width;
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    return this._direction === Axis.horizontal
      ? this._sumOrMax((c) => c.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY), true)
      : this.getDryLayout(new BoxConstraints({ maxHeight: height })).width;
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return this._direction === Axis.horizontal
      ? this.getDryLayout(new BoxConstraints({ maxWidth: width })).height
      : this._sumOrMax((c) => c.getMinIntrinsicHeight(Number.POSITIVE_INFINITY), false);
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this._direction === Axis.horizontal
      ? this.getDryLayout(new BoxConstraints({ maxWidth: width })).height
      : this._sumOrMax((c) => c.getMaxIntrinsicHeight(Number.POSITIVE_INFINITY), true);
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    return this.defaultComputeDistanceToHighestActualBaseline(baseline);
  }

  private _childConstraints(constraints: BoxConstraints): [BoxConstraints, number] {
    return this._direction === Axis.horizontal
      ? [new BoxConstraints({ maxWidth: constraints.maxWidth }), constraints.maxWidth]
      : [new BoxConstraints({ maxHeight: constraints.maxHeight }), constraints.maxHeight];
  }

  private _offset(main: number, cross: number): Offset {
    return this._direction === Axis.horizontal ? new Offset(main, cross) : new Offset(cross, main);
  }

  /** `_areAxesFlipped`: whether the main and the cross axis run backwards. */
  private get _axesFlipped(): [boolean, boolean] {
    const flipHorizontal = (this._textDirection ?? TextDirection.ltr) === TextDirection.rtl;
    const flipVertical = this._verticalDirection === VerticalDirection.up;
    return this._direction === Axis.horizontal ? [flipHorizontal, flipVertical] : [flipVertical, flipHorizontal];
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    if (this.firstChild === null) {
      return null;
    }
    const [childConstraints] = this._childConstraints(constraints);
    const [childrenAxisSize, runMetrics] = this._computeRuns(constraints, dryLayoutChild);
    const containerAxisSize = childrenAxisSize.applyConstraints(constraints, this._direction);
    let baselineOffset = BaselineOffset.noBaseline;
    this._positionChildren(
      runMetrics,
      childrenAxisSize,
      containerAxisSize,
      (offset, child) => {
        baselineOffset = baselineOffset.minOf(
          new BaselineOffset(child.getDryBaseline(childConstraints, baseline)).plus(offset.dy),
        );
      },
      (child) => child.getDryLayout(childConstraints),
    );
    return baselineOffset.offset;
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const [childConstraints, mainAxisLimit] = this._childConstraints(constraints);
    let mainAxisExtent = 0;
    let crossAxisExtent = 0;
    let runMainAxisExtent = 0;
    let runCrossAxisExtent = 0;
    let childCount = 0;
    for (let child = this.firstChild; child !== null; child = this.childAfter(child)) {
      const childSize = AxisSize.fromSize(dryLayoutChild(child, childConstraints), this._direction);
      // There must be at least one child before we move on to the next run.
      if (childCount > 0 && runMainAxisExtent + childSize.main + this._spacing > mainAxisLimit) {
        mainAxisExtent = Math.max(mainAxisExtent, runMainAxisExtent);
        crossAxisExtent += runCrossAxisExtent + this._runSpacing;
        runMainAxisExtent = 0;
        runCrossAxisExtent = 0;
        childCount = 0;
      }
      runMainAxisExtent += childSize.main;
      runCrossAxisExtent = Math.max(runCrossAxisExtent, childSize.cross);
      if (childCount > 0) {
        runMainAxisExtent += this._spacing;
      }
      childCount += 1;
    }
    crossAxisExtent += runCrossAxisExtent;
    mainAxisExtent = Math.max(mainAxisExtent, runMainAxisExtent);
    return constraints.constrain(new AxisSize(mainAxisExtent, crossAxisExtent).toSize(this._direction));
  }

  protected override performLayout(): void {
    const constraints = this.constraints;
    if (this.firstChild === null) {
      this.size = constraints.smallest;
      this._hasVisualOverflow = false;
      return;
    }
    const [childrenAxisSize, runMetrics] = this._computeRuns(constraints, layoutChild);
    const containerAxisSize = childrenAxisSize.applyConstraints(constraints, this._direction);
    this.size = containerAxisSize.toSize(this._direction);
    const freeAxisSize = containerAxisSize.minus(childrenAxisSize);
    this._hasVisualOverflow = freeAxisSize.main < 0 || freeAxisSize.cross < 0;
    this._positionChildren(
      runMetrics,
      freeAxisSize,
      containerAxisSize,
      (offset, child) => {
        (child.parentData as WrapParentData).offset = offset;
      },
      (child) => child.size,
    );
  }

  private _computeRuns(constraints: BoxConstraints, layout: ChildLayouter): [AxisSize, RunMetrics[]] {
    const [childConstraints, mainAxisLimit] = this._childConstraints(constraints);
    const [flipMainAxis] = this._axesFlipped;
    const runMetrics: RunMetrics[] = [];
    let currentRun: RunMetrics | null = null;
    let childrenAxisSize = AxisSize.empty;
    for (let child = this.firstChild; child !== null; child = this.childAfter(child)) {
      const childSize = AxisSize.fromSize(layout(child, childConstraints), this._direction);
      const newRun: RunMetrics | null =
        currentRun === null
          ? new RunMetrics(child, childSize)
          : currentRun.tryAddingNewChild(child, childSize, flipMainAxis, this._spacing, mainAxisLimit);
      if (newRun !== null) {
        runMetrics.push(newRun);
        childrenAxisSize = childrenAxisSize.plus(currentRun?.axisSize.flipped ?? AxisSize.empty);
        currentRun = newRun;
      }
    }
    const totalRunSpacing = this._runSpacing * (runMetrics.length - 1);
    // `currentRun` is set: the caller guarantees at least one child.
    const lastRun = currentRun as RunMetrics;
    childrenAxisSize = childrenAxisSize.plus(new AxisSize(totalRunSpacing, 0).plus(lastRun.axisSize.flipped));
    return [childrenAxisSize.flipped, runMetrics];
  }

  private _positionChildren(
    runMetrics: RunMetrics[],
    freeAxisSize: AxisSize,
    containerAxisSize: AxisSize,
    positionChild: (offset: Offset, child: RenderBox) => void,
    getChildSize: (child: RenderBox) => Size,
  ): void {
    const crossAxisFreeSpace = Math.max(0, freeAxisSize.cross);
    const [flipMainAxis, flipCrossAxis] = this._axesFlipped;
    const effectiveCrossAlignment = flipCrossAxis ? crossFlipped(this._crossAxisAlignment) : this._crossAxisAlignment;
    const [runLeadingSpace, runBetweenSpace] = distributeSpace(
      this._runAlignment,
      crossAxisFreeSpace,
      this._runSpacing,
      runMetrics.length,
      flipCrossAxis,
    );
    const nextChild = (child: RenderBox): RenderBox | null =>
      flipMainAxis ? this.childBefore(child) : this.childAfter(child);
    let runCrossAxisOffset = runLeadingSpace;
    const runs = flipCrossAxis ? [...runMetrics].reverse() : runMetrics;
    for (const run of runs) {
      const runCrossAxisExtent = run.axisSize.cross;
      const mainAxisFreeSpace = Math.max(0, containerAxisSize.main - run.axisSize.main);
      const [childLeadingSpace, childBetweenSpace] = distributeSpace(
        this._alignment,
        mainAxisFreeSpace,
        this._spacing,
        run.childCount,
        flipMainAxis,
      );
      let childMainAxisOffset = childLeadingSpace;
      let remainingChildCount = run.childCount;
      for (
        let child: RenderBox | null = run.leadingChild;
        child !== null && remainingChildCount > 0;
        child = nextChild(child), remainingChildCount -= 1
      ) {
        const childSize = AxisSize.fromSize(getChildSize(child), this._direction);
        const childCrossAxisOffset = crossFactor(effectiveCrossAlignment) * (runCrossAxisExtent - childSize.cross);
        positionChild(this._offset(childMainAxisOffset, runCrossAxisOffset + childCrossAxisOffset), child);
        childMainAxisOffset += childSize.main + childBetweenSpace;
      }
      runCrossAxisOffset += runCrossAxisExtent + runBetweenSpace;
    }
  }
}
