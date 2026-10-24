// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/box.dart and packages/flutter/lib/src/rendering/object.dart (RenderObjectWithChildMixin)
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * The box protocol: [RenderBox] with its size, intrinsic dimensions, dry
 * layout and baselines, each cached per node until [RenderBox.markNeedsLayout].
 */

import { BoxConstraints } from "./constraints.ts";
import { Offset, Size } from "./geometry.ts";
import { ParentData, type PipelineOwner, RenderObject } from "./object.ts";

/** The horizontal line glyphs sit on (dart:ui `TextBaseline`). */
export enum TextBaseline {
  alphabetic = "alphabetic",
  ideographic = "ideographic",
}

/**
 * A nullable baseline distance with Flutter's `BaselineOffset` arithmetic:
 * adding to "no baseline" stays "no baseline", and [minOf] ignores it.
 */
export class BaselineOffset {
  static readonly noBaseline = new BaselineOffset(null);

  constructor(readonly offset: number | null) {}

  /** `this + offset`, keeping "no baseline". */
  plus(offset: number): BaselineOffset {
    return this.offset === null ? this : new BaselineOffset(this.offset + offset);
  }

  /** The smaller of two baselines, where "no baseline" loses to any value. */
  minOf(other: BaselineOffset): BaselineOffset {
    if (this.offset === null) {
      return other;
    }
    if (other.offset === null) {
      return this;
    }
    return this.offset <= other.offset ? this : other;
  }
}

/** Parent data holding the child's offset from the parent's origin (box.dart `BoxParentData`). */
export class BoxParentData extends ParentData {
  offset: Offset = Offset.zero;
}

type IntrinsicDimension = "minWidth" | "maxWidth" | "minHeight" | "maxHeight";

/** Per-node memo of intrinsics, dry layouts and baselines (box.dart `_LayoutCacheStorage`). */
class LayoutCacheStorage {
  readonly intrinsicDimensions = new Map<string, number>();
  readonly dryLayoutSizes = new Map<string, Size>();
  /** Shared by dry and actual baselines, keyed by baseline and constraints, as in Flutter. */
  readonly baselines = new Map<string, number | null>();

  /** Empties every cache; returns whether anything was cached. */
  clear(): boolean {
    const hasCache = this.intrinsicDimensions.size > 0 || this.dryLayoutSizes.size > 0 || this.baselines.size > 0;
    if (hasCache) {
      this.intrinsicDimensions.clear();
      this.dryLayoutSizes.clear();
      this.baselines.clear();
    }
    return hasCache;
  }
}

function memoize<T>(cache: Map<string, T>, key: string, compute: () => T): T {
  if (cache.has(key)) {
    return cache.get(key) as T;
  }
  const value = compute();
  cache.set(key, value);
  return value;
}

/**
 * A render object in a 2D Cartesian box: its size satisfies the
 * [BoxConstraints] its parent gave it, and each child's position is the
 * [BoxParentData.offset] the parent sets.
 *
 * Subclasses override [performLayout] (or [sizedByParent] with
 * [computeDryLayout]), the `compute*Intrinsic*` methods, [computeDryLayout],
 * [computeDistanceToActualBaseline] and [computeDryBaseline]. Callers use the
 * cached `get*` counterparts.
 */
export abstract class RenderBox extends RenderObject {
  private _size: Size | null = null;
  private readonly _layoutCacheStorage = new LayoutCacheStorage();

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof BoxParentData)) {
      child.parentData = new BoxParentData();
    }
  }

  override get constraints(): BoxConstraints {
    const constraints = super.constraints;
    if (!(constraints instanceof BoxConstraints)) {
      throw new Error(`${this.constructor.name} was laid out without BoxConstraints`);
    }
    return constraints;
  }

  /** Whether a size has been set by layout. */
  get hasSize(): boolean {
    return this._size !== null;
  }

  /** The size from the last layout. Throws before the first layout. */
  get size(): Size {
    if (this._size === null) {
      throw new Error(`${this.constructor.name} has no size: it has not been laid out`);
    }
    return this._size;
  }

  /** Set only by this box's own [performLayout] or [performResize]. */
  set size(value: Size) {
    this._size = value;
  }

  /** The child's offset when its parent data is [BoxParentData], else zero. */
  static offsetOf(child: RenderObject): Offset {
    return child.parentData instanceof BoxParentData ? child.parentData.offset : Offset.zero;
  }

  private _intrinsic(dimension: IntrinsicDimension, input: number, compute: (input: number) => number): number {
    return memoize(this._layoutCacheStorage.intrinsicDimensions, `${dimension}:${input}`, () => compute(input));
  }

  /** The smallest width at which this box paints correctly given `height`. */
  getMinIntrinsicWidth(height: number): number {
    return this._intrinsic("minWidth", height, (h) => this.computeMinIntrinsicWidth(h));
  }

  /** The width beyond which more width does not reduce this box's height, given `height`. */
  getMaxIntrinsicWidth(height: number): number {
    return this._intrinsic("maxWidth", height, (h) => this.computeMaxIntrinsicWidth(h));
  }

  /** The smallest height at which this box paints correctly given `width`. */
  getMinIntrinsicHeight(width: number): number {
    return this._intrinsic("minHeight", width, (w) => this.computeMinIntrinsicHeight(w));
  }

  /** The height beyond which more height is wasted, given `width`. */
  getMaxIntrinsicHeight(width: number): number {
    return this._intrinsic("maxHeight", width, (w) => this.computeMaxIntrinsicHeight(w));
  }

  protected computeMinIntrinsicWidth(_height: number): number {
    return 0;
  }

  protected computeMaxIntrinsicWidth(_height: number): number {
    return 0;
  }

  protected computeMinIntrinsicHeight(_width: number): number {
    return 0;
  }

  protected computeMaxIntrinsicHeight(_width: number): number {
    return 0;
  }

  /** The size this box would have under `constraints`, without laying it out. */
  getDryLayout(constraints: BoxConstraints): Size {
    return memoize(this._layoutCacheStorage.dryLayoutSizes, constraints.cacheKey, () =>
      this.computeDryLayout(constraints),
    );
  }

  /**
   * Computes [getDryLayout]. Must not lay out children or change state. The
   * default (a box that cannot compute a dry layout) returns [Size.zero], as
   * Flutter does in release builds.
   */
  protected computeDryLayout(_constraints: BoxConstraints): Size {
    return Size.zero;
  }

  /** The baseline this box would have under `constraints`, without laying it out. */
  getDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    return memoize(this._layoutCacheStorage.baselines, `${baseline}|${constraints.cacheKey}`, () =>
      this.computeDryBaseline(constraints, baseline),
    );
  }

  /** Computes [getDryBaseline]; the default has no baseline. */
  protected computeDryBaseline(_constraints: BoxConstraints, _baseline: TextBaseline): number | null {
    return null;
  }

  /**
   * The distance from the top of the box to its first `baseline`, after
   * layout. Without a real baseline this is the box's height, unless
   * `onlyReal` is set, when it is null.
   */
  getDistanceToBaseline(baseline: TextBaseline, options: { onlyReal?: boolean } = {}): number | null {
    const result = this.getDistanceToActualBaseline(baseline);
    if (result === null && !(options.onlyReal ?? false)) {
      return this.size.height;
    }
    return result;
  }

  /** The cached [computeDistanceToActualBaseline] under the current constraints. */
  getDistanceToActualBaseline(baseline: TextBaseline): number | null {
    return memoize(this._layoutCacheStorage.baselines, `${baseline}|${this.constraints.cacheKey}`, () =>
      this.computeDistanceToActualBaseline(baseline),
    );
  }

  /** Computes the actual baseline after layout; the default has none. */
  protected computeDistanceToActualBaseline(_baseline: TextBaseline): number | null {
    return null;
  }

  /**
   * Clears the layout caches. If anything was cached, an ancestor depended
   * on this box's intrinsics, dry layout or baseline, so the parent is
   * dirtied regardless of relayout boundaries.
   */
  override markNeedsLayout(): void {
    if (this._layoutCacheStorage.clear() && this.parent !== null) {
      this.markParentNeedsLayout();
      return;
    }
    super.markNeedsLayout();
  }

  /** The default for [sizedByParent] boxes: the dry layout is the size. */
  protected override performResize(): void {
    this.size = this.computeDryLayout(this.constraints);
  }

  /** The default does nothing, for [sizedByParent] boxes without children. */
  protected override performLayout(): void {}
}

/**
 * A box with at most one box child (object.dart `RenderObjectWithChildMixin`
 * applied to [RenderBox]).
 */
export abstract class RenderBoxWithChild extends RenderBox {
  private _child: RenderBox | null = null;

  /** The only child; assigning drops the old child and adopts the new one. */
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
}
