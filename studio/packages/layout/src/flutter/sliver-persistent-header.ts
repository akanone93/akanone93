// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/sliver_persistent_header.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Persistent headers: slivers whose box child changes size between
 * `minExtent` and `maxExtent` as the list scrolls. The scrolling header
 * leaves with the content, the pinned header stays at the leading edge, the
 * floating header reappears as soon as the user scrolls back, and the
 * floating pinned header does both.
 *
 * Header stretching, snap animations, `showOnScreen` and the child rebuild
 * hook (`updateChild`: the child is fixed here) are not ported, and so is the
 * `vsync` they need. A floating header's effective scroll offset therefore
 * follows the scroll offset of its first layout (a header that has not
 * been laid out before, as on the canvas, shows as if scrolled to there).
 */

import { Axis } from "./flex.ts";
import { clampDouble, type Offset } from "./geometry.ts";
import type { RenderObject } from "./object.ts";
import type { RenderBox } from "./render-box.ts";
import { RenderSliverSingleBoxAdapter, ScrollDirection, SliverGeometry, SliverPhysicalParentData } from "./sliver.ts";

/** The extents and child shared by the persistent headers. */
export interface RenderSliverPersistentHeaderOptions {
  child?: RenderBox | null;
  minExtent: number;
  maxExtent: number;
}

/** The base of the persistent headers (`RenderSliverPersistentHeader`). */
export abstract class RenderSliverPersistentHeader extends RenderSliverSingleBoxAdapter {
  private _minExtent: number;
  private _maxExtent: number;

  constructor(options: RenderSliverPersistentHeaderOptions) {
    super({ child: options.child ?? null });
    this._minExtent = options.minExtent;
    this._maxExtent = options.maxExtent;
  }

  /** The largest extent of the header. */
  get maxExtent(): number {
    return this._maxExtent;
  }

  set maxExtent(value: number) {
    this._maxExtent = value;
    this.markNeedsLayout();
  }

  /** The smallest extent of the header, as it is pinned or floats. */
  get minExtent(): number {
    return this._minExtent;
  }

  set minExtent(value: number) {
    this._minExtent = value;
    this.markNeedsLayout();
  }

  /** The child's extent along the main axis. */
  protected get childExtent(): number {
    const child = this.child;
    if (child === null) {
      return 0;
    }
    return this.constraints.axis === Axis.vertical ? child.size.height : child.size.width;
  }

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof SliverPhysicalParentData)) {
      child.parentData = new SliverPhysicalParentData();
    }
  }

  /** Lays the child out for a header that has scrolled `scrollOffset` of its `maxExtent`. */
  protected layoutChild(scrollOffset: number, maxExtent: number, _overlapsContent = false): void {
    const shrinkOffset = Math.min(scrollOffset, maxExtent);
    this.child?.layout(
      this.constraints.asBoxConstraints({ maxExtent: Math.max(this.minExtent, maxExtent - shrinkOffset) }),
      { parentUsesSize: true },
    );
  }

  override childPaintOffset(child: RenderObject): Offset {
    return this.boxChildPaintOffset(child as RenderBox);
  }
}

/** A header that scrolls away with the content (`RenderSliverScrollingPersistentHeader`). */
export class RenderSliverScrollingPersistentHeader extends RenderSliverPersistentHeader {
  private childPosition: number | null = null;

  /** Sets the geometry and returns the child's position (`updateGeometry`). */
  protected updateGeometry(): number {
    const constraints = this.constraints;
    const maxExtent = this.maxExtent;
    const paintExtent = maxExtent - constraints.scrollOffset;
    const cacheExtent = this.calculateCacheOffset(constraints, { from: 0, to: maxExtent });
    this.geometry = new SliverGeometry({
      cacheExtent,
      scrollExtent: maxExtent,
      paintOrigin: Math.min(constraints.overlap, 0),
      paintExtent: clampDouble(paintExtent, 0, constraints.remainingPaintExtent),
      maxPaintExtent: maxExtent,
      hasVisualOverflow: true,
    });
    return Math.min(0, paintExtent - this.childExtent);
  }

  protected override performLayout(): void {
    this.layoutChild(this.constraints.scrollOffset, this.maxExtent);
    this.childPosition = this.updateGeometry();
  }

  override childMainAxisPosition(_child: RenderObject): number {
    return this.childPosition ?? 0;
  }
}

/** A header that stays at the leading edge once it has shrunk to `minExtent` (`RenderSliverPinnedPersistentHeader`). */
export class RenderSliverPinnedPersistentHeader extends RenderSliverPersistentHeader {
  protected override performLayout(): void {
    const constraints = this.constraints;
    const maxExtent = this.maxExtent;
    const overlapsContent = constraints.overlap > 0;
    this.layoutChild(constraints.scrollOffset, maxExtent, overlapsContent);
    const effectiveRemainingPaintExtent = Math.max(0, constraints.remainingPaintExtent - constraints.overlap);
    const layoutExtent = clampDouble(maxExtent - constraints.scrollOffset, 0, effectiveRemainingPaintExtent);
    this.geometry = new SliverGeometry({
      scrollExtent: maxExtent,
      paintOrigin: constraints.overlap,
      paintExtent: Math.min(this.childExtent, effectiveRemainingPaintExtent),
      layoutExtent,
      maxPaintExtent: maxExtent,
      maxScrollObstructionExtent: this.minExtent,
      cacheExtent: layoutExtent > 0 ? -constraints.cacheOrigin + layoutExtent : layoutExtent,
      hasVisualOverflow: true,
    });
  }

  override childMainAxisPosition(_child: RenderObject): number {
    return 0;
  }
}

/** A header that reappears as soon as the user scrolls back (`RenderSliverFloatingPersistentHeader`). */
export class RenderSliverFloatingPersistentHeader extends RenderSliverPersistentHeader {
  protected lastActualScrollOffset: number | null = null;
  protected effectiveScrollOffset = 0;
  protected lastStartedScrollDirection: ScrollDirection | null = null;
  protected childPosition: number | null = null;

  /** Sets the geometry and returns the child's position (`updateGeometry`). */
  protected updateGeometry(): number {
    const constraints = this.constraints;
    const maxExtent = this.maxExtent;
    const paintExtent = maxExtent - this.effectiveScrollOffset;
    const layoutExtent = maxExtent - constraints.scrollOffset;
    this.geometry = new SliverGeometry({
      scrollExtent: maxExtent,
      paintOrigin: Math.min(constraints.overlap, 0),
      paintExtent: clampDouble(paintExtent, 0, constraints.remainingPaintExtent),
      layoutExtent: clampDouble(layoutExtent, 0, constraints.remainingPaintExtent),
      maxPaintExtent: maxExtent,
      hasVisualOverflow: true,
    });
    return Math.min(0, paintExtent - this.childExtent);
  }

  protected override performLayout(): void {
    const constraints = this.constraints;
    const maxExtent = this.maxExtent;
    if (
      this.lastActualScrollOffset !== null &&
      (constraints.scrollOffset < this.lastActualScrollOffset || this.effectiveScrollOffset < maxExtent)
    ) {
      let delta = this.lastActualScrollOffset - constraints.scrollOffset;
      const allowFloatingExpansion =
        constraints.userScrollDirection === ScrollDirection.forward ||
        this.lastStartedScrollDirection === ScrollDirection.forward;
      if (allowFloatingExpansion) {
        if (this.effectiveScrollOffset > maxExtent) {
          this.effectiveScrollOffset = maxExtent;
        }
      } else if (delta > 0) {
        delta = 0;
      }
      this.effectiveScrollOffset = clampDouble(this.effectiveScrollOffset - delta, 0, constraints.scrollOffset);
    } else {
      this.effectiveScrollOffset = constraints.scrollOffset;
    }
    const overlapsContent = this.effectiveScrollOffset < constraints.scrollOffset;
    this.layoutChild(this.effectiveScrollOffset, maxExtent, overlapsContent);
    this.childPosition = this.updateGeometry();
    this.lastActualScrollOffset = constraints.scrollOffset;
  }

  override childMainAxisPosition(_child: RenderObject): number {
    return this.childPosition ?? 0;
  }
}

/** A floating header that never shrinks below `minExtent` (`RenderSliverFloatingPinnedPersistentHeader`). */
export class RenderSliverFloatingPinnedPersistentHeader extends RenderSliverFloatingPersistentHeader {
  protected override updateGeometry(): number {
    const constraints = this.constraints;
    const minExtent = this.minExtent;
    const minAllowedExtent =
      constraints.remainingPaintExtent > minExtent ? minExtent : constraints.remainingPaintExtent;
    const maxExtent = this.maxExtent;
    const paintExtent = maxExtent - this.effectiveScrollOffset;
    const clampedPaintExtent = clampDouble(paintExtent, minAllowedExtent, constraints.remainingPaintExtent);
    const layoutExtent = maxExtent - constraints.scrollOffset;
    this.geometry = new SliverGeometry({
      scrollExtent: maxExtent,
      paintOrigin: Math.min(constraints.overlap, 0),
      paintExtent: clampedPaintExtent,
      layoutExtent: clampDouble(layoutExtent, 0, clampedPaintExtent),
      maxPaintExtent: maxExtent,
      maxScrollObstructionExtent: minExtent,
      hasVisualOverflow: true,
    });
    return 0;
  }
}
