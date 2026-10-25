// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/sliver.dart, packages/flutter/lib/src/rendering/viewport_offset.dart and packages/flutter/lib/src/painting/basic_types.dart (AxisDirection)
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * The sliver protocol: [SliverConstraints] in, [SliverGeometry] out, the
 * [RenderSliver] base class with its paint and cache extent helpers, the box
 * adapters ([RenderSliverToBoxAdapter]) and the [ViewportOffset] a viewport
 * reads its scroll position from.
 *
 * Painting, hit testing, semantics, debug assertions and the debug paint
 * overlay are not ported. A viewport's scroll position is a [ViewportOffset]
 * whose pixels are fixed for a layout (the Studio canvas scrolls by choosing
 * the offset, STU-068); no ballistic animation, physics or notifications.
 * Like the rest of this port, the checks Flutter makes only with `assert`
 * are left out, as in a release build.
 */

import { BoxConstraints, type Constraints } from "./constraints.ts";
import { Axis } from "./flex.ts";
import { clampDouble, Offset, Rect, TextDirection } from "./geometry.ts";
import { ParentData, type PipelineOwner, RenderObject } from "./object.ts";
import type { RenderBox } from "./render-box.ts";

const infinity = Number.POSITIVE_INFINITY;

/** A direction along an axis, e.g. which way a scroll view's content flows (`AxisDirection`). */
export enum AxisDirection {
  up = "up",
  right = "right",
  down = "down",
  left = "left",
}

/** The axis an [AxisDirection] lies along. */
export function axisDirectionToAxis(axisDirection: AxisDirection): Axis {
  return axisDirection === AxisDirection.up || axisDirection === AxisDirection.down ? Axis.vertical : Axis.horizontal;
}

/** The [AxisDirection] reading direction `textDirection` flows in. */
export function textDirectionToAxisDirection(textDirection: TextDirection): AxisDirection {
  return textDirection === TextDirection.rtl ? AxisDirection.left : AxisDirection.right;
}

/** The opposite [AxisDirection]. */
export function flipAxisDirection(axisDirection: AxisDirection): AxisDirection {
  switch (axisDirection) {
    case AxisDirection.up:
      return AxisDirection.down;
    case AxisDirection.right:
      return AxisDirection.left;
    case AxisDirection.down:
      return AxisDirection.up;
    case AxisDirection.left:
      return AxisDirection.right;
  }
}

/** Whether `axisDirection` runs against the axis' natural order (up and left). */
export function axisDirectionIsReversed(axisDirection: AxisDirection): boolean {
  return axisDirection === AxisDirection.up || axisDirection === AxisDirection.left;
}

/** The axis direction that is perpendicular to `axis` (`flipAxis`). */
export function flipAxis(axis: Axis): Axis {
  return axis === Axis.horizontal ? Axis.vertical : Axis.horizontal;
}

/** Whether a sliver's content grows along or against its [AxisDirection] (`GrowthDirection`). */
export enum GrowthDirection {
  forward = "forward",
  reverse = "reverse",
}

/** The direction the user is scrolling (`ScrollDirection`). */
export enum ScrollDirection {
  idle = "idle",
  forward = "forward",
  reverse = "reverse",
}

/** `axisDirection` flipped when `growthDirection` is reverse. */
export function applyGrowthDirectionToAxisDirection(
  axisDirection: AxisDirection,
  growthDirection: GrowthDirection,
): AxisDirection {
  return growthDirection === GrowthDirection.forward ? axisDirection : flipAxisDirection(axisDirection);
}

/** `scrollDirection` flipped when `growthDirection` is reverse. */
export function applyGrowthDirectionToScrollDirection(
  scrollDirection: ScrollDirection,
  growthDirection: GrowthDirection,
): ScrollDirection {
  if (growthDirection === GrowthDirection.forward) {
    return scrollDirection;
  }
  switch (scrollDirection) {
    case ScrollDirection.idle:
      return ScrollDirection.idle;
    case ScrollDirection.forward:
      return ScrollDirection.reverse;
    case ScrollDirection.reverse:
      return ScrollDirection.forward;
  }
}

/** Describes the viewport a sliver is laid out in (`SliverLayoutDimensions`). */
export interface SliverLayoutDimensions {
  readonly scrollOffset: number;
  readonly precedingScrollExtent: number;
  readonly viewportMainAxisExtent: number;
  readonly crossAxisExtent: number;
}

/** Computes the extent of the item at `index`, or null when there is none (`ItemExtentBuilder`). */
export type ItemExtentBuilder = (index: number, dimensions: SliverLayoutDimensions) => number | null;

/** The fields of [SliverConstraints]. */
export interface SliverConstraintsInit {
  axisDirection: AxisDirection;
  growthDirection: GrowthDirection;
  userScrollDirection: ScrollDirection;
  scrollOffset: number;
  precedingScrollExtent: number;
  overlap: number;
  remainingPaintExtent: number;
  crossAxisExtent: number;
  crossAxisDirection: AxisDirection;
  viewportMainAxisExtent: number;
  remainingCacheExtent: number;
  cacheOrigin: number;
}

/** Immutable layout constraints for a sliver (`SliverConstraints`). */
export class SliverConstraints implements Constraints, SliverConstraintsInit {
  readonly axisDirection: AxisDirection;
  readonly growthDirection: GrowthDirection;
  readonly userScrollDirection: ScrollDirection;
  readonly scrollOffset: number;
  readonly precedingScrollExtent: number;
  readonly overlap: number;
  readonly remainingPaintExtent: number;
  readonly crossAxisExtent: number;
  readonly crossAxisDirection: AxisDirection;
  readonly viewportMainAxisExtent: number;
  readonly remainingCacheExtent: number;
  readonly cacheOrigin: number;

  constructor(init: SliverConstraintsInit) {
    this.axisDirection = init.axisDirection;
    this.growthDirection = init.growthDirection;
    this.userScrollDirection = init.userScrollDirection;
    this.scrollOffset = init.scrollOffset;
    this.precedingScrollExtent = init.precedingScrollExtent;
    this.overlap = init.overlap;
    this.remainingPaintExtent = init.remainingPaintExtent;
    this.crossAxisExtent = init.crossAxisExtent;
    this.crossAxisDirection = init.crossAxisDirection;
    this.viewportMainAxisExtent = init.viewportMainAxisExtent;
    this.remainingCacheExtent = init.remainingCacheExtent;
    this.cacheOrigin = init.cacheOrigin;
  }

  /** A copy with the given fields replaced. */
  copyWith(changes: Partial<SliverConstraintsInit>): SliverConstraints {
    return new SliverConstraints({ ...this, ...changes });
  }

  /** The axis the sliver scrolls along. */
  get axis(): Axis {
    return axisDirectionToAxis(this.axisDirection);
  }

  /** [growthDirection] as seen along the physical axis: flipped when the axis direction is up or left. */
  get normalizedGrowthDirection(): GrowthDirection {
    if (axisDirectionIsReversed(this.axisDirection)) {
      return this.growthDirection === GrowthDirection.forward ? GrowthDirection.reverse : GrowthDirection.forward;
    }
    return this.growthDirection;
  }

  get isTight(): boolean {
    return false;
  }

  get isNormalized(): boolean {
    return (
      this.scrollOffset >= 0 &&
      this.crossAxisExtent >= 0 &&
      axisDirectionToAxis(this.axisDirection) !== axisDirectionToAxis(this.crossAxisDirection) &&
      this.viewportMainAxisExtent >= 0 &&
      this.remainingPaintExtent >= 0
    );
  }

  /** Box constraints for a box child: the cross axis is tight, the main axis lies in `[minExtent, maxExtent]`. */
  asBoxConstraints(options: { minExtent?: number; maxExtent?: number; crossAxisExtent?: number } = {}): BoxConstraints {
    const minExtent = options.minExtent ?? 0;
    const maxExtent = options.maxExtent ?? infinity;
    const crossAxisExtent = options.crossAxisExtent ?? this.crossAxisExtent;
    if (this.axis === Axis.horizontal) {
      return new BoxConstraints({
        minHeight: crossAxisExtent,
        maxHeight: crossAxisExtent,
        minWidth: minExtent,
        maxWidth: maxExtent,
      });
    }
    return new BoxConstraints({
      minWidth: crossAxisExtent,
      maxWidth: crossAxisExtent,
      minHeight: minExtent,
      maxHeight: maxExtent,
    });
  }

  equals(other: Constraints): boolean {
    return (
      other instanceof SliverConstraints &&
      other.axisDirection === this.axisDirection &&
      other.growthDirection === this.growthDirection &&
      other.userScrollDirection === this.userScrollDirection &&
      other.scrollOffset === this.scrollOffset &&
      other.precedingScrollExtent === this.precedingScrollExtent &&
      other.overlap === this.overlap &&
      other.remainingPaintExtent === this.remainingPaintExtent &&
      other.crossAxisExtent === this.crossAxisExtent &&
      other.crossAxisDirection === this.crossAxisDirection &&
      other.viewportMainAxisExtent === this.viewportMainAxisExtent &&
      other.remainingCacheExtent === this.remainingCacheExtent &&
      other.cacheOrigin === this.cacheOrigin
    );
  }
}

/** The fields of [SliverGeometry]; the omitted extents default as in Flutter. */
export interface SliverGeometryInit {
  scrollExtent?: number;
  paintExtent?: number;
  paintOrigin?: number;
  layoutExtent?: number;
  maxPaintExtent?: number;
  maxScrollObstructionExtent?: number;
  crossAxisExtent?: number | null;
  hitTestExtent?: number;
  visible?: boolean;
  hasVisualOverflow?: boolean;
  scrollOffsetCorrection?: number | null;
  cacheExtent?: number;
}

/** A sliver's answer to its constraints (`SliverGeometry`). */
export class SliverGeometry {
  static readonly zero = new SliverGeometry();

  readonly scrollExtent: number;
  readonly paintExtent: number;
  readonly paintOrigin: number;
  readonly layoutExtent: number;
  readonly maxPaintExtent: number;
  readonly maxScrollObstructionExtent: number;
  readonly crossAxisExtent: number | null;
  readonly hitTestExtent: number;
  readonly visible: boolean;
  readonly hasVisualOverflow: boolean;
  readonly scrollOffsetCorrection: number | null;
  readonly cacheExtent: number;

  constructor(init: SliverGeometryInit = {}) {
    this.scrollExtent = init.scrollExtent ?? 0;
    this.paintExtent = init.paintExtent ?? 0;
    this.paintOrigin = init.paintOrigin ?? 0;
    this.layoutExtent = init.layoutExtent ?? this.paintExtent;
    this.maxPaintExtent = init.maxPaintExtent ?? 0;
    this.maxScrollObstructionExtent = init.maxScrollObstructionExtent ?? 0;
    this.crossAxisExtent = init.crossAxisExtent ?? null;
    this.hitTestExtent = init.hitTestExtent ?? this.paintExtent;
    this.visible = init.visible ?? this.paintExtent > 0;
    this.hasVisualOverflow = init.hasVisualOverflow ?? false;
    this.scrollOffsetCorrection = init.scrollOffsetCorrection ?? null;
    this.cacheExtent = init.cacheExtent ?? this.layoutExtent;
  }
}

/** Parent data of a sliver positioned by a logical layout offset (`SliverLogicalParentData`). */
export class SliverLogicalParentData extends ParentData {
  /** The position of the child relative to the zero scroll offset. */
  layoutOffset: number | null = null;
}

/** Parent data of a sliver positioned by a physical paint offset (`SliverPhysicalParentData`). */
export class SliverPhysicalParentData extends ParentData {
  /** The position of the child relative to the parent's top left corner. */
  paintOffset: Offset = Offset.zero;
}

/** Implemented by render objects that place their children by something other than [BoxParentData]. */
export interface ChildPaintOffsetProvider {
  /** The offset of `child`'s top left corner from this object's, in the paint coordinate system. */
  childPaintOffset(child: RenderObject): Offset;
}

/** Whether `object` places its children itself. */
export function providesChildPaintOffset(object: RenderObject): object is RenderObject & ChildPaintOffsetProvider {
  return (
    "childPaintOffset" in object && typeof (object as { childPaintOffset?: unknown }).childPaintOffset === "function"
  );
}

/**
 * Base class for the render objects that implement the sliver protocol
 * (`RenderSliver`): given [SliverConstraints] they set a [SliverGeometry].
 */
export abstract class RenderSliver extends RenderObject implements ChildPaintOffsetProvider {
  private _geometry: SliverGeometry | null = null;

  override get constraints(): SliverConstraints {
    const constraints = super.constraints;
    if (!(constraints instanceof SliverConstraints)) {
      throw new Error(`${this.constructor.name} was laid out without SliverConstraints`);
    }
    return constraints;
  }

  /** The geometry of the most recent layout; null before the first. */
  get geometry(): SliverGeometry | null {
    return this._geometry;
  }

  set geometry(value: SliverGeometry | null) {
    this._geometry = value;
  }

  /** The geometry, which exists once the sliver has been laid out. */
  protected get laidOutGeometry(): SliverGeometry {
    if (this._geometry === null) {
      throw new Error(`${this.constructor.name} has no geometry: it has not been laid out`);
    }
    return this._geometry;
  }

  /** The area the sliver paints into, relative to its own origin (`paintBounds`). */
  get paintBounds(): Rect {
    const geometry = this.laidOutGeometry;
    return this.constraints.axis === Axis.horizontal
      ? Rect.fromLTWH(0, 0, geometry.paintExtent, this.constraints.crossAxisExtent)
      : Rect.fromLTWH(0, 0, this.constraints.crossAxisExtent, geometry.paintExtent);
  }

  protected override performResize(): void {}

  /** How much a viewport shifts the center sliver's scroll offset (zero unless a sliver says otherwise). */
  get centerOffsetAdjustment(): number {
    return 0;
  }

  /** The part of `[from, to]` (in scroll offsets within this sliver) that is painted. */
  calculatePaintOffset(constraints: SliverConstraints, range: { from: number; to: number }): number {
    const a = constraints.scrollOffset;
    const b = constraints.scrollOffset + constraints.remainingPaintExtent;
    return clampDouble(
      clampDouble(range.to, a, b) - clampDouble(range.from, a, b),
      0,
      constraints.remainingPaintExtent,
    );
  }

  /** The part of `[from, to]` that lies in the cache area. */
  calculateCacheOffset(constraints: SliverConstraints, range: { from: number; to: number }): number {
    const a = constraints.scrollOffset + constraints.cacheOrigin;
    const b = constraints.scrollOffset + constraints.remainingCacheExtent;
    return clampDouble(
      clampDouble(range.to, a, b) - clampDouble(range.from, a, b),
      0,
      constraints.remainingCacheExtent,
    );
  }

  /** The distance from the sliver's leading edge to the child's, along the main axis. */
  childMainAxisPosition(_child: RenderObject): number {
    return 0;
  }

  /** The distance from the sliver's left or top edge to the child's, along the cross axis. */
  childCrossAxisPosition(_child: RenderObject): number {
    return 0;
  }

  /** The scroll offset, within this sliver, at which `child` starts. */
  childScrollOffset(_child: RenderObject): number | null {
    return 0;
  }

  /** Where `child`, a box, is placed in this sliver (`applyPaintTransformForBoxChild`). */
  protected boxChildPaintOffset(child: RenderBox): Offset {
    const constraints = this.constraints;
    const rightWayUp = this.getRightWayUp(constraints);
    let delta = this.childMainAxisPosition(child);
    const crossAxisDelta = this.childCrossAxisPosition(child);
    if (constraints.axis === Axis.horizontal) {
      if (!rightWayUp) {
        delta = this.laidOutGeometry.paintExtent - child.size.width - delta;
      }
      return new Offset(delta, crossAxisDelta);
    }
    if (!rightWayUp) {
      delta = this.laidOutGeometry.paintExtent - child.size.height - delta;
    }
    return new Offset(crossAxisDelta, delta);
  }

  private getRightWayUp(constraints: SliverConstraints): boolean {
    const reversed = axisDirectionIsReversed(constraints.axisDirection);
    return constraints.growthDirection === GrowthDirection.forward ? !reversed : reversed;
  }

  childPaintOffset(child: RenderObject): Offset {
    const parentData = child.parentData;
    return parentData instanceof SliverPhysicalParentData ? parentData.paintOffset : Offset.zero;
  }
}

/** A sliver with one box child, which it positions with a [SliverPhysicalParentData] (`RenderSliverSingleBoxAdapter`). */
export abstract class RenderSliverSingleBoxAdapter extends RenderSliver {
  private _child: RenderBox | null = null;

  constructor(options: { child?: RenderBox | null } = {}) {
    super();
    this.child = options.child ?? null;
  }

  get child(): RenderBox | null {
    return this._child;
  }

  set child(value: RenderBox | null) {
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

  /** Positions `child` for the geometry this sliver just computed. */
  protected setChildParentData(child: RenderObject, constraints: SliverConstraints, geometry: SliverGeometry): void {
    const parentData = child.parentData as SliverPhysicalParentData;
    switch (applyGrowthDirectionToAxisDirection(constraints.axisDirection, constraints.growthDirection)) {
      case AxisDirection.up:
        parentData.paintOffset = new Offset(0, geometry.paintExtent + constraints.scrollOffset - geometry.scrollExtent);
        break;
      case AxisDirection.left:
        parentData.paintOffset = new Offset(geometry.paintExtent + constraints.scrollOffset - geometry.scrollExtent, 0);
        break;
      case AxisDirection.right:
        parentData.paintOffset = new Offset(-constraints.scrollOffset, 0);
        break;
      case AxisDirection.down:
        parentData.paintOffset = new Offset(0, -constraints.scrollOffset);
        break;
    }
  }

  override childMainAxisPosition(_child: RenderObject): number {
    return -this.constraints.scrollOffset;
  }
}

/** A sliver that holds a single box (`RenderSliverToBoxAdapter`, the render object of `SliverToBoxAdapter`). */
export class RenderSliverToBoxAdapter extends RenderSliverSingleBoxAdapter {
  protected override performLayout(): void {
    const child = this.child;
    if (child === null) {
      this.geometry = SliverGeometry.zero;
      return;
    }
    const constraints = this.constraints;
    child.layout(constraints.asBoxConstraints(), { parentUsesSize: true });
    const childExtent = constraints.axis === Axis.horizontal ? child.size.width : child.size.height;
    const paintedChildSize = this.calculatePaintOffset(constraints, { from: 0, to: childExtent });
    const cacheExtent = this.calculateCacheOffset(constraints, { from: 0, to: childExtent });
    this.geometry = new SliverGeometry({
      scrollExtent: childExtent,
      paintExtent: paintedChildSize,
      cacheExtent,
      maxPaintExtent: childExtent,
      hitTestExtent: paintedChildSize,
      hasVisualOverflow: childExtent > constraints.remainingPaintExtent || constraints.scrollOffset > 0,
    });
    this.setChildParentData(child, constraints, this.laidOutGeometry);
  }
}

/** Where a [RenderViewport] reads its scroll position from (`ViewportOffset`). */
export interface ViewportOffset {
  /** The current scroll position in pixels. */
  readonly pixels: number;
  /** The direction the user is scrolling. */
  readonly userScrollDirection: ScrollDirection;
  /** Called with the viewport's main axis extent; false if the offset changed `pixels` and wants another layout pass. */
  applyViewportDimension(viewportDimension: number): boolean;
  /** Called with the scrollable range; false if the offset wants another layout pass. */
  applyContentDimensions(minScrollExtent: number, maxScrollExtent: number): boolean;
  /** Shifts `pixels` by `correction` after a sliver changed its mind about its extent. */
  correctBy(correction: number): void;
}

/** A [ViewportOffset] that stays where it is put (`ViewportOffset.fixed`). */
export class FixedViewportOffset implements ViewportOffset {
  private _pixels: number;

  constructor(
    pixels: number,
    readonly userScrollDirection: ScrollDirection = ScrollDirection.idle,
  ) {
    this._pixels = pixels;
  }

  get pixels(): number {
    return this._pixels;
  }

  applyViewportDimension(_viewportDimension: number): boolean {
    return true;
  }

  applyContentDimensions(_minScrollExtent: number, _maxScrollExtent: number): boolean {
    return true;
  }

  correctBy(correction: number): void {
    this._pixels += correction;
  }
}

/**
 * The scroll position of a `PageView` (`_PagePosition` of `PageController`):
 * `pixels` follow the page, which stays put when the viewport is resized.
 */
export class PageViewportOffset implements ViewportOffset {
  readonly userScrollDirection = ScrollDirection.idle;
  private _pixels: number | null = null;
  private viewportDimension: number | null = null;
  private cachedPage: number | null = null;

  constructor(
    private readonly initialPage: number,
    readonly viewportFraction: number,
  ) {}

  get pixels(): number {
    return this._pixels ?? 0;
  }

  private get initialPageOffset(): number {
    return Math.max(0, (this.viewportDimension ?? 0) * ((this.viewportFraction - 1) / 2));
  }

  /** The page shown at `pixels` for a viewport of `viewportDimension` (`getPageFromPixels`). */
  getPageFromPixels(pixels: number, viewportDimension: number): number {
    const actual = Math.max(0, pixels - this.initialPageOffset) / (viewportDimension * this.viewportFraction);
    const round = Math.round(actual);
    return Math.abs(actual - round) < 1e-10 ? round : actual;
  }

  /** The scroll offset at which `page` is shown (`getPixelsFromPage`). */
  getPixelsFromPage(page: number): number {
    return page * (this.viewportDimension ?? 0) * this.viewportFraction + this.initialPageOffset;
  }

  applyViewportDimension(viewportDimension: number): boolean {
    const oldViewportDimension = this.viewportDimension;
    if (viewportDimension === oldViewportDimension) {
      return true;
    }
    this.viewportDimension = viewportDimension;
    const oldPixels = this._pixels;
    let page: number;
    if (oldPixels === null) {
      page = this.initialPage;
    } else if (oldViewportDimension === 0) {
      page = this.cachedPage ?? this.initialPage;
    } else {
      page = this.getPageFromPixels(oldPixels, oldViewportDimension ?? viewportDimension);
    }
    const newPixels = this.getPixelsFromPage(page);
    this.cachedPage = viewportDimension === 0 ? page : null;
    if (newPixels !== oldPixels) {
      this._pixels = newPixels;
      return false;
    }
    return true;
  }

  applyContentDimensions(_minScrollExtent: number, _maxScrollExtent: number): boolean {
    return true;
  }

  correctBy(correction: number): void {
    this._pixels = this.pixels + correction;
  }
}
