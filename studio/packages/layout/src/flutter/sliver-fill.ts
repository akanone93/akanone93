// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/sliver_fill.dart (RenderSliverFillRemaining*)
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Slivers that fill the space the other slivers leave: with a scrollable
 * child ([RenderSliverFillRemainingWithScrollable]), with a box child that
 * takes its natural size when larger ([RenderSliverFillRemaining]), and the
 * same with overscroll filling ([RenderSliverFillRemainingAndOverscroll]).
 * Flutter asserts that the extent is finite; that check is not ported.
 */

import { Axis } from "./flex.ts";
import { RenderSliverSingleBoxAdapter, SliverGeometry } from "./sliver.ts";

/** Fills the remaining viewport with a child that scrolls on its own (`RenderSliverFillRemainingWithScrollable`). */
export class RenderSliverFillRemainingWithScrollable extends RenderSliverSingleBoxAdapter {
  protected override performLayout(): void {
    const constraints = this.constraints;
    const extent = constraints.remainingPaintExtent - Math.min(constraints.overlap, 0);
    const cacheExtent = this.calculateCacheOffset(constraints, { from: 0, to: constraints.viewportMainAxisExtent });
    const child = this.child;
    if (child !== null) {
      let maxExtent = extent;
      if (extent === 0 && cacheExtent > 0) {
        maxExtent = cacheExtent;
      }
      child.layout(constraints.asBoxConstraints({ minExtent: extent, maxExtent }));
    }
    const paintedChildSize = this.calculatePaintOffset(constraints, { from: 0, to: extent });
    this.geometry = new SliverGeometry({
      scrollExtent: constraints.viewportMainAxisExtent,
      paintExtent: paintedChildSize,
      maxPaintExtent: paintedChildSize,
      hasVisualOverflow: extent > constraints.remainingPaintExtent || constraints.scrollOffset > 0,
      cacheExtent,
    });
    if (child !== null) {
      this.setChildParentData(child, constraints, this.laidOutGeometry);
    }
  }
}

/** Fills the remaining viewport with a box, growing past it if the child is taller (`RenderSliverFillRemaining`). */
export class RenderSliverFillRemaining extends RenderSliverSingleBoxAdapter {
  protected override performLayout(): void {
    const constraints = this.constraints;
    let extent = constraints.viewportMainAxisExtent - constraints.precedingScrollExtent;
    const child = this.child;
    if (child !== null) {
      const childExtent =
        constraints.axis === Axis.horizontal
          ? child.getMaxIntrinsicWidth(constraints.crossAxisExtent)
          : child.getMaxIntrinsicHeight(constraints.crossAxisExtent);
      extent = Math.max(extent, childExtent);
      child.layout(constraints.asBoxConstraints({ minExtent: extent, maxExtent: extent }));
    }
    const paintedChildSize = this.calculatePaintOffset(constraints, { from: 0, to: extent });
    const cacheExtent = this.calculateCacheOffset(constraints, { from: 0, to: extent });
    this.geometry = new SliverGeometry({
      scrollExtent: extent,
      paintExtent: paintedChildSize,
      maxPaintExtent: paintedChildSize,
      hasVisualOverflow: extent > constraints.remainingPaintExtent || constraints.scrollOffset > 0,
      cacheExtent,
    });
    if (child !== null) {
      this.setChildParentData(child, constraints, this.laidOutGeometry);
    }
  }
}

/** Like [RenderSliverFillRemaining], and the child also fills the area overscroll exposes (`RenderSliverFillRemainingAndOverscroll`). */
export class RenderSliverFillRemainingAndOverscroll extends RenderSliverSingleBoxAdapter {
  protected override performLayout(): void {
    const constraints = this.constraints;
    let extent = constraints.viewportMainAxisExtent - constraints.precedingScrollExtent;
    let maxExtent = constraints.remainingPaintExtent - Math.min(constraints.overlap, 0);
    const child = this.child;
    if (child !== null) {
      const childExtent =
        constraints.axis === Axis.horizontal
          ? child.getMaxIntrinsicWidth(constraints.crossAxisExtent)
          : child.getMaxIntrinsicHeight(constraints.crossAxisExtent);
      extent = Math.max(extent, childExtent);
      maxExtent = Math.max(extent, maxExtent);
      child.layout(constraints.asBoxConstraints({ minExtent: extent, maxExtent }));
    }
    const cacheExtent = this.calculateCacheOffset(constraints, { from: 0, to: extent });
    this.geometry = new SliverGeometry({
      scrollExtent: extent,
      paintExtent: Math.min(maxExtent, constraints.remainingPaintExtent),
      maxPaintExtent: maxExtent,
      hasVisualOverflow: extent > constraints.remainingPaintExtent || constraints.scrollOffset > 0,
      cacheExtent,
    });
    if (child !== null) {
      this.setChildParentData(child, constraints, this.laidOutGeometry);
    }
  }
}
