// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/viewport.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Viewports: [RenderViewport] lays its sliver children out along the scroll
 * axis around a center sliver and sizes itself to the incoming constraints;
 * [RenderShrinkWrappingViewport] sizes itself to its content instead.
 *
 * Painting, clipping, hit testing, semantics, `showOnScreen` and the reveal
 * helpers are not ported. The cache area is `scrollCacheExtent`; the legacy
 * `cacheExtent` and `cacheExtentStyle` parameters Flutter deprecated are
 * folded into it. `paintOrder` only matters for painting and is not ported.
 */

import type { ContainerParentData } from "./child-list.ts";
import { ChildList } from "./child-list.ts";
import type { BoxConstraints } from "./constraints.ts";
import { Axis } from "./flex.ts";
import { clampDouble, Offset, Size } from "./geometry.ts";
import type { PipelineOwner, RenderObject } from "./object.ts";
import { RenderBox } from "./render-box.ts";
import {
  AxisDirection,
  applyGrowthDirectionToAxisDirection,
  applyGrowthDirectionToScrollDirection,
  axisDirectionToAxis,
  type ChildPaintOffsetProvider,
  GrowthDirection,
  type RenderSliver,
  SliverConstraints,
  type SliverGeometry,
  SliverLogicalParentData,
  SliverPhysicalParentData,
  type ViewportOffset,
} from "./sliver.ts";

/** How a [ScrollCacheExtent] is measured (`CacheExtentStyle`). */
export enum CacheExtentStyle {
  pixel = "pixel",
  viewport = "viewport",
}

/** The size of the area beyond the viewport that is laid out (`ScrollCacheExtent`). */
export class ScrollCacheExtent {
  private constructor(
    readonly style: CacheExtentStyle,
    readonly value: number,
  ) {}

  /** A cache extent of `pixels` logical pixels. */
  static pixels(pixels: number): ScrollCacheExtent {
    return new ScrollCacheExtent(CacheExtentStyle.pixel, pixels);
  }

  /** A cache extent of `fraction` times the viewport's main axis extent. */
  static viewport(fraction: number): ScrollCacheExtent {
    return new ScrollCacheExtent(CacheExtentStyle.viewport, fraction);
  }

  /** The cache extent in logical pixels for a viewport of `mainAxisExtent`. */
  calculateCacheOffset(mainAxisExtent: number): number {
    return this.style === CacheExtentStyle.pixel ? this.value : this.value * mainAxisExtent;
  }
}

/** The cache extent a viewport uses when none is given (`RenderAbstractViewport.defaultCacheExtent`). */
export const defaultCacheExtent = 250;

/** Parent data of a sliver in a [RenderViewport] (`SliverPhysicalContainerParentData`). */
export class SliverPhysicalContainerParentData
  extends SliverPhysicalParentData
  implements ContainerParentData<RenderSliver>
{
  previousSibling: RenderSliver | null = null;
  nextSibling: RenderSliver | null = null;
}

/** Parent data of a sliver in a [RenderShrinkWrappingViewport] (`SliverLogicalContainerParentData`). */
export class SliverLogicalContainerParentData
  extends SliverLogicalParentData
  implements ContainerParentData<RenderSliver>
{
  previousSibling: RenderSliver | null = null;
  nextSibling: RenderSliver | null = null;
}

/** Constructor options shared by the viewports. */
export interface RenderViewportBaseOptions {
  axisDirection?: AxisDirection;
  crossAxisDirection: AxisDirection;
  offset: ViewportOffset;
  scrollCacheExtent?: ScrollCacheExtent;
  children?: readonly RenderSliver[];
}

/** The shared part of the viewports (`RenderViewportBase`). */
export abstract class RenderViewportBase extends RenderBox implements ChildPaintOffsetProvider {
  protected readonly slivers: ChildList<RenderSliver>;
  protected calculatedCacheExtent: number | null = null;
  private _axisDirection: AxisDirection;
  private _crossAxisDirection: AxisDirection;
  private _offset: ViewportOffset;
  private _scrollCacheExtent: ScrollCacheExtent;

  constructor(options: RenderViewportBaseOptions) {
    super();
    this._axisDirection = options.axisDirection ?? AxisDirection.down;
    this._crossAxisDirection = options.crossAxisDirection;
    this._offset = options.offset;
    this._scrollCacheExtent = options.scrollCacheExtent ?? ScrollCacheExtent.pixels(defaultCacheExtent);
    this.slivers = new ChildList<RenderSliver>({
      adoptChild: (child) => {
        this.adoptChild(child);
      },
      dropChild: (child) => {
        this.dropChild(child);
      },
      markNeedsLayout: () => {
        this.markNeedsLayout();
      },
    });
  }

  get axisDirection(): AxisDirection {
    return this._axisDirection;
  }

  set axisDirection(value: AxisDirection) {
    if (value !== this._axisDirection) {
      this._axisDirection = value;
      this.markNeedsLayout();
    }
  }

  get crossAxisDirection(): AxisDirection {
    return this._crossAxisDirection;
  }

  set crossAxisDirection(value: AxisDirection) {
    if (value !== this._crossAxisDirection) {
      this._crossAxisDirection = value;
      this.markNeedsLayout();
    }
  }

  /** The axis the viewport scrolls along. */
  get axis(): Axis {
    return axisDirectionToAxis(this._axisDirection);
  }

  get offset(): ViewportOffset {
    return this._offset;
  }

  set offset(value: ViewportOffset) {
    if (value !== this._offset) {
      this._offset = value;
      this.markNeedsLayout();
    }
  }

  get scrollCacheExtent(): ScrollCacheExtent {
    return this._scrollCacheExtent;
  }

  set scrollCacheExtent(value: ScrollCacheExtent) {
    this._scrollCacheExtent = value;
    this.markNeedsLayout();
  }

  get firstChild(): RenderSliver | null {
    return this.slivers.firstChild;
  }

  get lastChild(): RenderSliver | null {
    return this.slivers.lastChild;
  }

  get childCount(): number {
    return this.slivers.childCount;
  }

  childBefore(child: RenderSliver): RenderSliver | null {
    return this.slivers.childBefore(child);
  }

  childAfter(child: RenderSliver): RenderSliver | null {
    return this.slivers.childAfter(child);
  }

  /** Appends `child` to the slivers. */
  add(child: RenderSliver): void {
    this.slivers.add(child);
  }

  /** Removes `child` from the slivers. */
  remove(child: RenderSliver): void {
    this.slivers.remove(child);
  }

  override attach(owner: PipelineOwner): void {
    super.attach(owner);
    for (const child of this.slivers.toArray()) {
      child.attach(owner);
    }
  }

  override detach(): void {
    super.detach();
    for (const child of this.slivers.toArray()) {
      child.detach();
    }
  }

  override visitChildren(visitor: (child: RenderObject) => void): void {
    for (const child of this.slivers.toArray()) {
      visitor(child);
    }
  }

  /** Whether any sliver reported visual overflow in the last layout. */
  abstract get hasVisualOverflow(): boolean;

  /** The index of the first child relative to the center child. */
  abstract get indexOfFirstChild(): number;

  /** Collects scroll extents and overflow reported by `childLayoutGeometry`. */
  protected abstract updateOutOfBandData(growthDirection: GrowthDirection, childLayoutGeometry: SliverGeometry): void;

  /** Records where `child` starts along the main axis. */
  protected abstract updateChildLayoutOffset(
    child: RenderSliver,
    layoutOffset: number,
    growthDirection: GrowthDirection,
  ): void;

  /** The offset of `child`'s paint area from the viewport's top left corner. */
  abstract paintOffsetOf(child: RenderSliver): Offset;

  /** The scroll offset of `scrollOffsetWithinChild` within `child`, from the viewport's start. */
  abstract scrollOffsetOf(child: RenderSliver, scrollOffsetWithinChild: number): number;

  /** The total `maxScrollObstructionExtent` of the slivers before `child`. */
  abstract maxScrollObstructionExtentBefore(child: RenderSliver): number;

  childPaintOffset(child: RenderObject): Offset {
    return this.paintOffsetOf(child as RenderSliver);
  }

  /**
   * Lays out `child` and its followers (`advance`) one after the other and
   * returns a scroll offset correction when a sliver asks for one, else zero.
   */
  protected layoutChildSequence(options: {
    child: RenderSliver | null;
    scrollOffset: number;
    overlap: number;
    layoutOffset: number;
    remainingPaintExtent: number;
    mainAxisExtent: number;
    crossAxisExtent: number;
    growthDirection: GrowthDirection;
    advance: (child: RenderSliver) => RenderSliver | null;
    remainingCacheExtent: number;
    cacheOrigin: number;
  }): number {
    const { mainAxisExtent, crossAxisExtent, growthDirection, advance } = options;
    let { child, scrollOffset, layoutOffset, remainingCacheExtent, cacheOrigin } = options;
    const { remainingPaintExtent } = options;
    const initialLayoutOffset = layoutOffset;
    const adjustedUserScrollDirection = applyGrowthDirectionToScrollDirection(
      this.offset.userScrollDirection,
      growthDirection,
    );
    let maxPaintOffset = layoutOffset + options.overlap;
    let precedingScrollExtent = 0;
    while (child !== null) {
      const sliverScrollOffset = scrollOffset <= 0 ? 0 : scrollOffset;
      const correctedCacheOrigin = Math.max(cacheOrigin, -sliverScrollOffset);
      const cacheExtentCorrection = cacheOrigin - correctedCacheOrigin;
      child.layout(
        new SliverConstraints({
          axisDirection: this.axisDirection,
          growthDirection,
          userScrollDirection: adjustedUserScrollDirection,
          scrollOffset: sliverScrollOffset,
          precedingScrollExtent,
          overlap: maxPaintOffset - layoutOffset,
          remainingPaintExtent: Math.max(0, remainingPaintExtent - layoutOffset + initialLayoutOffset),
          crossAxisExtent,
          crossAxisDirection: this.crossAxisDirection,
          viewportMainAxisExtent: mainAxisExtent,
          remainingCacheExtent: Math.max(0, remainingCacheExtent + cacheExtentCorrection),
          cacheOrigin: correctedCacheOrigin,
        }),
        { parentUsesSize: true },
      );
      const childLayoutGeometry = child.geometry;
      if (childLayoutGeometry === null) {
        throw new Error(`${child.constructor.name} did not set its geometry`);
      }
      if (childLayoutGeometry.scrollOffsetCorrection !== null) {
        return childLayoutGeometry.scrollOffsetCorrection;
      }
      const effectiveLayoutOffset = layoutOffset + childLayoutGeometry.paintOrigin;
      if (childLayoutGeometry.visible || scrollOffset > 0) {
        this.updateChildLayoutOffset(child, effectiveLayoutOffset, growthDirection);
      } else {
        this.updateChildLayoutOffset(child, -scrollOffset + initialLayoutOffset, growthDirection);
      }
      maxPaintOffset = Math.max(effectiveLayoutOffset + childLayoutGeometry.paintExtent, maxPaintOffset);
      scrollOffset -= childLayoutGeometry.scrollExtent;
      precedingScrollExtent += childLayoutGeometry.scrollExtent;
      layoutOffset += childLayoutGeometry.layoutExtent;
      if (childLayoutGeometry.cacheExtent !== 0) {
        remainingCacheExtent -= childLayoutGeometry.cacheExtent - cacheExtentCorrection;
        cacheOrigin = Math.min(correctedCacheOrigin + childLayoutGeometry.cacheExtent, 0);
      }
      this.updateOutOfBandData(growthDirection, childLayoutGeometry);
      child = advance(child);
    }
    return 0;
  }

  /** The physical top left of `child`'s paint area for a main axis `layoutOffset`. */
  protected computeAbsolutePaintOffset(
    child: RenderSliver,
    layoutOffset: number,
    growthDirection: GrowthDirection,
  ): Offset {
    const paintExtent = child.geometry?.paintExtent ?? 0;
    switch (applyGrowthDirectionToAxisDirection(this.axisDirection, growthDirection)) {
      case AxisDirection.up:
        return new Offset(0, this.size.height - layoutOffset - paintExtent);
      case AxisDirection.left:
        return new Offset(this.size.width - layoutOffset - paintExtent, 0);
      case AxisDirection.right:
        return new Offset(layoutOffset, 0);
      case AxisDirection.down:
        return new Offset(0, layoutOffset);
    }
  }
}

/** Options of [RenderViewport]. */
export interface RenderViewportOptions extends RenderViewportBaseOptions {
  anchor?: number;
  center?: RenderSliver | null;
}

const maxLayoutCyclesPerChild = 10;

/** A viewport whose center sliver sits at `anchor` of the main axis (`RenderViewport`). */
export class RenderViewport extends RenderViewportBase {
  private _anchor: number;
  private _center: RenderSliver | null;
  private _minScrollExtent = 0;
  private _maxScrollExtent = 0;
  private _hasVisualOverflow = false;

  constructor(options: RenderViewportOptions) {
    super(options);
    this._anchor = options.anchor ?? 0;
    this._center = options.center ?? null;
    this.slivers.addAll(options.children ?? []);
    if (this._center === null && this.firstChild !== null) {
      this._center = this.firstChild;
    }
  }

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof SliverPhysicalContainerParentData)) {
      child.parentData = new SliverPhysicalContainerParentData();
    }
  }

  get anchor(): number {
    return this._anchor;
  }

  set anchor(value: number) {
    if (value !== this._anchor) {
      this._anchor = value;
      this.markNeedsLayout();
    }
  }

  get center(): RenderSliver | null {
    return this._center;
  }

  set center(value: RenderSliver | null) {
    if (value !== this._center) {
      this._center = value;
      this.markNeedsLayout();
    }
  }

  override get sizedByParent(): boolean {
    return true;
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  /** The scroll extents of the last layout. */
  get scrollExtents(): { min: number; max: number } {
    return { min: this._minScrollExtent, max: this._maxScrollExtent };
  }

  protected override performLayout(): void {
    this.offset.applyViewportDimension(this.axis === Axis.vertical ? this.size.height : this.size.width);
    const center = this._center;
    if (center === null) {
      this._minScrollExtent = 0;
      this._maxScrollExtent = 0;
      this._hasVisualOverflow = false;
      this.offset.applyContentDimensions(0, 0);
      return;
    }
    const [mainAxisExtent, crossAxisExtent] =
      this.axis === Axis.vertical ? [this.size.height, this.size.width] : [this.size.width, this.size.height];
    const centerOffsetAdjustment = center.centerOffsetAdjustment;
    const maxLayoutCycles = maxLayoutCyclesPerChild * this.childCount;
    let count = 0;
    let correction: number;
    do {
      correction = this.attemptLayout(mainAxisExtent, crossAxisExtent, this.offset.pixels + centerOffsetAdjustment);
      if (correction !== 0) {
        this.offset.correctBy(correction);
      } else if (
        this.offset.applyContentDimensions(
          Math.min(0, this._minScrollExtent + mainAxisExtent * this.anchor),
          Math.max(0, this._maxScrollExtent - mainAxisExtent * (1 - this.anchor)),
        )
      ) {
        break;
      }
      count += 1;
    } while (count < maxLayoutCycles);
    if (count >= maxLayoutCycles) {
      throw new Error("A RenderViewport exceeded its maximum number of layout cycles.");
    }
  }

  private attemptLayout(mainAxisExtent: number, crossAxisExtent: number, correctedOffset: number): number {
    const center = this._center;
    if (center === null) {
      return 0;
    }
    this._minScrollExtent = 0;
    this._maxScrollExtent = 0;
    this._hasVisualOverflow = false;
    const centerOffset = mainAxisExtent * this.anchor - correctedOffset;
    const reverseDirectionRemainingPaintExtent = clampDouble(centerOffset, 0, mainAxisExtent);
    const forwardDirectionRemainingPaintExtent = clampDouble(mainAxisExtent - centerOffset, 0, mainAxisExtent);
    const calculatedCacheExtent = this.scrollCacheExtent.calculateCacheOffset(mainAxisExtent);
    this.calculatedCacheExtent = calculatedCacheExtent;
    const fullCacheExtent = mainAxisExtent + 2 * calculatedCacheExtent;
    const centerCacheOffset = centerOffset + calculatedCacheExtent;
    const reverseDirectionRemainingCacheExtent = clampDouble(centerCacheOffset, 0, fullCacheExtent);
    const forwardDirectionRemainingCacheExtent = clampDouble(fullCacheExtent - centerCacheOffset, 0, fullCacheExtent);
    const leadingNegativeChild = this.childBefore(center);
    if (leadingNegativeChild !== null) {
      const result = this.layoutChildSequence({
        child: leadingNegativeChild,
        scrollOffset: Math.max(mainAxisExtent, centerOffset) - mainAxisExtent,
        overlap: 0,
        layoutOffset: forwardDirectionRemainingPaintExtent,
        remainingPaintExtent: reverseDirectionRemainingPaintExtent,
        mainAxisExtent,
        crossAxisExtent,
        growthDirection: GrowthDirection.reverse,
        advance: (child) => this.childBefore(child),
        remainingCacheExtent: reverseDirectionRemainingCacheExtent,
        cacheOrigin: clampDouble(mainAxisExtent - centerOffset, -calculatedCacheExtent, 0),
      });
      if (result !== 0) {
        return -result;
      }
    }
    return this.layoutChildSequence({
      child: center,
      scrollOffset: Math.max(0, -centerOffset),
      overlap: leadingNegativeChild === null ? Math.min(0, -centerOffset) : 0,
      layoutOffset: centerOffset >= mainAxisExtent ? centerOffset : reverseDirectionRemainingPaintExtent,
      remainingPaintExtent: forwardDirectionRemainingPaintExtent,
      mainAxisExtent,
      crossAxisExtent,
      growthDirection: GrowthDirection.forward,
      advance: (child) => this.childAfter(child),
      remainingCacheExtent: forwardDirectionRemainingCacheExtent,
      cacheOrigin: clampDouble(centerOffset, -calculatedCacheExtent, 0),
    });
  }

  get hasVisualOverflow(): boolean {
    return this._hasVisualOverflow;
  }

  protected updateOutOfBandData(growthDirection: GrowthDirection, childLayoutGeometry: SliverGeometry): void {
    if (growthDirection === GrowthDirection.forward) {
      this._maxScrollExtent += childLayoutGeometry.scrollExtent;
    } else {
      this._minScrollExtent -= childLayoutGeometry.scrollExtent;
    }
    if (childLayoutGeometry.hasVisualOverflow) {
      this._hasVisualOverflow = true;
    }
  }

  protected updateChildLayoutOffset(child: RenderSliver, layoutOffset: number, growthDirection: GrowthDirection): void {
    const parentData = child.parentData as SliverPhysicalParentData;
    parentData.paintOffset = this.computeAbsolutePaintOffset(child, layoutOffset, growthDirection);
  }

  paintOffsetOf(child: RenderSliver): Offset {
    return (child.parentData as SliverPhysicalParentData).paintOffset;
  }

  scrollOffsetOf(child: RenderSliver, scrollOffsetWithinChild: number): number {
    const center = this._center;
    if (center === null) {
      throw new Error("A RenderViewport without a center has no scroll offsets");
    }
    if (child.constraints.growthDirection === GrowthDirection.forward) {
      let scrollOffsetToChild = 0;
      let current: RenderSliver | null = center;
      while (current !== child && current !== null) {
        scrollOffsetToChild += current.geometry?.scrollExtent ?? 0;
        current = this.childAfter(current);
      }
      return scrollOffsetToChild + scrollOffsetWithinChild;
    }
    let scrollOffsetToChild = 0;
    let current = this.childBefore(center);
    while (current !== child && current !== null) {
      scrollOffsetToChild -= current.geometry?.scrollExtent ?? 0;
      current = this.childBefore(current);
    }
    return scrollOffsetToChild - scrollOffsetWithinChild;
  }

  maxScrollObstructionExtentBefore(child: RenderSliver): number {
    const center = this._center;
    if (center === null) {
      return 0;
    }
    let pinnedExtent = 0;
    if (child.constraints.growthDirection === GrowthDirection.forward) {
      let current: RenderSliver | null = center;
      while (current !== child && current !== null) {
        pinnedExtent += current.geometry?.maxScrollObstructionExtent ?? 0;
        current = this.childAfter(current);
      }
      return pinnedExtent;
    }
    let current = this.childBefore(center);
    while (current !== child && current !== null) {
      pinnedExtent += current.geometry?.maxScrollObstructionExtent ?? 0;
      current = this.childBefore(current);
    }
    return pinnedExtent;
  }

  get indexOfFirstChild(): number {
    const center = this._center;
    if (center === null) {
      return 0;
    }
    let count = 0;
    let child: RenderSliver | null = center;
    while (child !== this.firstChild && child !== null) {
      count -= 1;
      child = this.childBefore(child);
    }
    return count;
  }
}

/** A viewport that is as long as its slivers' `maxPaintExtent` allow (`RenderShrinkWrappingViewport`). */
export class RenderShrinkWrappingViewport extends RenderViewportBase {
  private _maxScrollExtent = 0;
  private _shrinkWrapExtent = 0;
  private _hasVisualOverflow = false;

  constructor(options: RenderViewportBaseOptions) {
    super(options);
    this.slivers.addAll(options.children ?? []);
  }

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof SliverLogicalContainerParentData)) {
      child.parentData = new SliverLogicalContainerParentData();
    }
  }

  protected override performLayout(): void {
    const constraints = this.constraints;
    if (this.firstChild === null) {
      this.size =
        this.axis === Axis.vertical
          ? new Size(constraints.maxWidth, constraints.minHeight)
          : new Size(constraints.minWidth, constraints.maxHeight);
      this.offset.applyViewportDimension(0);
      this._maxScrollExtent = 0;
      this._shrinkWrapExtent = 0;
      this._hasVisualOverflow = false;
      this.offset.applyContentDimensions(0, 0);
      return;
    }
    const [mainAxisExtent, crossAxisExtent] =
      this.axis === Axis.vertical
        ? [constraints.maxHeight, constraints.maxWidth]
        : [constraints.maxWidth, constraints.maxHeight];
    let effectiveExtent: number;
    for (;;) {
      const correction = this.attemptLayout(mainAxisExtent, crossAxisExtent, this.offset.pixels);
      if (correction !== 0) {
        this.offset.correctBy(correction);
        continue;
      }
      effectiveExtent =
        this.axis === Axis.vertical
          ? constraints.constrainHeight(this._shrinkWrapExtent)
          : constraints.constrainWidth(this._shrinkWrapExtent);
      const didAcceptViewportDimension = this.offset.applyViewportDimension(effectiveExtent);
      const didAcceptContentDimension = this.offset.applyContentDimensions(
        0,
        Math.max(0, this._maxScrollExtent - effectiveExtent),
      );
      if (didAcceptViewportDimension && didAcceptContentDimension) {
        break;
      }
    }
    this.size =
      this.axis === Axis.vertical
        ? constraints.constrainDimensions(crossAxisExtent, effectiveExtent)
        : constraints.constrainDimensions(effectiveExtent, crossAxisExtent);
  }

  private attemptLayout(mainAxisExtent: number, crossAxisExtent: number, correctedOffset: number): number {
    this._maxScrollExtent = 0;
    this._shrinkWrapExtent = 0;
    this._hasVisualOverflow = correctedOffset < 0;
    const calculatedCacheExtent = Number.isFinite(mainAxisExtent)
      ? this.scrollCacheExtent.calculateCacheOffset(mainAxisExtent)
      : 0;
    this.calculatedCacheExtent = calculatedCacheExtent;
    return this.layoutChildSequence({
      child: this.firstChild,
      scrollOffset: Math.max(0, correctedOffset),
      overlap: Math.min(0, correctedOffset),
      layoutOffset: Math.max(0, -correctedOffset),
      remainingPaintExtent: mainAxisExtent + Math.min(0, correctedOffset),
      mainAxisExtent,
      crossAxisExtent,
      growthDirection: GrowthDirection.forward,
      advance: (child) => this.childAfter(child),
      remainingCacheExtent: mainAxisExtent + 2 * calculatedCacheExtent,
      cacheOrigin: -calculatedCacheExtent,
    });
  }

  get hasVisualOverflow(): boolean {
    return this._hasVisualOverflow;
  }

  protected updateOutOfBandData(_growthDirection: GrowthDirection, childLayoutGeometry: SliverGeometry): void {
    this._maxScrollExtent += childLayoutGeometry.scrollExtent;
    if (childLayoutGeometry.hasVisualOverflow) {
      this._hasVisualOverflow = true;
    }
    this._shrinkWrapExtent += childLayoutGeometry.maxPaintExtent;
  }

  protected updateChildLayoutOffset(
    child: RenderSliver,
    layoutOffset: number,
    _growthDirection: GrowthDirection,
  ): void {
    (child.parentData as SliverLogicalParentData).layoutOffset = layoutOffset;
  }

  paintOffsetOf(child: RenderSliver): Offset {
    const layoutOffset = (child.parentData as SliverLogicalParentData).layoutOffset ?? 0;
    return this.computeAbsolutePaintOffset(child, layoutOffset, GrowthDirection.forward);
  }

  scrollOffsetOf(child: RenderSliver, scrollOffsetWithinChild: number): number {
    let scrollOffsetToChild = 0;
    let current = this.firstChild;
    while (current !== child && current !== null) {
      scrollOffsetToChild += current.geometry?.scrollExtent ?? 0;
      current = this.childAfter(current);
    }
    return scrollOffsetToChild + scrollOffsetWithinChild;
  }

  maxScrollObstructionExtentBefore(child: RenderSliver): number {
    let pinnedExtent = 0;
    let current = this.firstChild;
    while (current !== child && current !== null) {
      pinnedExtent += current.geometry?.maxScrollObstructionExtent ?? 0;
      current = this.childAfter(current);
    }
    return pinnedExtent;
  }

  get indexOfFirstChild(): number {
    return 0;
  }
}
