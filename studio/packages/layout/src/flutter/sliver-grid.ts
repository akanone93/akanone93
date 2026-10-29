// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/sliver_grid.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Grids: the [SliverGridLayout] a [SliverGridDelegate] computes from the
 * sliver constraints, the two stock delegates (fixed cross axis count,
 * maximum cross axis extent) and [RenderSliverGrid]. Painting and hit
 * testing are not ported.
 */

import type { BoxConstraints } from "./constraints.ts";
import { precisionErrorTolerance } from "./geometry.ts";
import type { RenderObject } from "./object.ts";
import type { RenderBox } from "./render-box.ts";
import { axisDirectionIsReversed, type SliverConstraints, SliverGeometry } from "./sliver.ts";
import {
  type RenderSliverBoxChildManager,
  RenderSliverMultiBoxAdaptor,
  SliverMultiBoxAdaptorParentData,
} from "./sliver-list.ts";

/** Where and how large one grid tile is (`SliverGridGeometry`). */
export class SliverGridGeometry {
  constructor(
    readonly scrollOffset: number,
    readonly crossAxisOffset: number,
    readonly mainAxisExtent: number,
    readonly crossAxisExtent: number,
  ) {}

  /** The scroll offset where the tile ends. */
  get trailingScrollOffset(): number {
    return this.scrollOffset + this.mainAxisExtent;
  }

  /** The tight box constraints the tile's child is laid out with. */
  getBoxConstraints(constraints: SliverConstraints): BoxConstraints {
    return constraints.asBoxConstraints({
      minExtent: this.mainAxisExtent,
      maxExtent: this.mainAxisExtent,
      crossAxisExtent: this.crossAxisExtent,
    });
  }
}

/** The placement of the tiles of a grid (`SliverGridLayout`). */
export interface SliverGridLayout {
  getMinChildIndexForScrollOffset(scrollOffset: number): number;
  getMaxChildIndexForScrollOffset(scrollOffset: number): number;
  getGeometryForChildIndex(index: number): SliverGridGeometry;
  computeMaxScrollOffset(childCount: number): number;
}

/** A grid whose tiles all have the same size (`SliverGridRegularTileLayout`). */
export class SliverGridRegularTileLayout implements SliverGridLayout {
  readonly crossAxisCount: number;
  readonly mainAxisStride: number;
  readonly crossAxisStride: number;
  readonly childMainAxisExtent: number;
  readonly childCrossAxisExtent: number;
  readonly reverseCrossAxis: boolean;

  constructor(init: {
    crossAxisCount: number;
    mainAxisStride: number;
    crossAxisStride: number;
    childMainAxisExtent: number;
    childCrossAxisExtent: number;
    reverseCrossAxis: boolean;
  }) {
    this.crossAxisCount = init.crossAxisCount;
    this.mainAxisStride = init.mainAxisStride;
    this.crossAxisStride = init.crossAxisStride;
    this.childMainAxisExtent = init.childMainAxisExtent;
    this.childCrossAxisExtent = init.childCrossAxisExtent;
    this.reverseCrossAxis = init.reverseCrossAxis;
  }

  getMinChildIndexForScrollOffset(scrollOffset: number): number {
    return this.mainAxisStride > precisionErrorTolerance
      ? this.crossAxisCount * Math.trunc(scrollOffset / this.mainAxisStride)
      : 0;
  }

  getMaxChildIndexForScrollOffset(scrollOffset: number): number {
    if (this.mainAxisStride > 0) {
      const mainAxisCount = Math.ceil(scrollOffset / this.mainAxisStride);
      return Math.max(0, this.crossAxisCount * mainAxisCount - 1);
    }
    return 0;
  }

  private getOffsetFromStartInCrossAxis(crossAxisStart: number): number {
    if (this.reverseCrossAxis) {
      return (
        this.crossAxisCount * this.crossAxisStride -
        crossAxisStart -
        this.childCrossAxisExtent -
        (this.crossAxisStride - this.childCrossAxisExtent)
      );
    }
    return crossAxisStart;
  }

  getGeometryForChildIndex(index: number): SliverGridGeometry {
    const crossAxisStart = (index % this.crossAxisCount) * this.crossAxisStride;
    return new SliverGridGeometry(
      Math.trunc(index / this.crossAxisCount) * this.mainAxisStride,
      this.getOffsetFromStartInCrossAxis(crossAxisStart),
      this.childMainAxisExtent,
      this.childCrossAxisExtent,
    );
  }

  computeMaxScrollOffset(childCount: number): number {
    if (childCount === 0) {
      return 0;
    }
    const mainAxisCount = Math.trunc((childCount - 1) / this.crossAxisCount) + 1;
    const mainAxisSpacing = this.mainAxisStride - this.childMainAxisExtent;
    return this.mainAxisStride * mainAxisCount - mainAxisSpacing;
  }
}

/** Computes the layout of a grid from its constraints (`SliverGridDelegate`). */
export interface SliverGridDelegate {
  getLayout(constraints: SliverConstraints): SliverGridLayout;
}

/** The options shared by the stock delegates. */
export interface SliverGridSpacing {
  mainAxisSpacing?: number;
  crossAxisSpacing?: number;
  childAspectRatio?: number;
  mainAxisExtent?: number | null;
}

function regularLayout(
  constraints: SliverConstraints,
  crossAxisCount: number,
  spacing: SliverGridSpacing,
): SliverGridRegularTileLayout {
  const mainAxisSpacing = spacing.mainAxisSpacing ?? 0;
  const crossAxisSpacing = spacing.crossAxisSpacing ?? 0;
  const childAspectRatio = spacing.childAspectRatio ?? 1;
  const usableCrossAxisExtent = Math.max(0, constraints.crossAxisExtent - crossAxisSpacing * (crossAxisCount - 1));
  const childCrossAxisExtent = usableCrossAxisExtent / crossAxisCount;
  const childMainAxisExtent = spacing.mainAxisExtent ?? childCrossAxisExtent / childAspectRatio;
  return new SliverGridRegularTileLayout({
    crossAxisCount,
    mainAxisStride: childMainAxisExtent + mainAxisSpacing,
    crossAxisStride: childCrossAxisExtent + crossAxisSpacing,
    childMainAxisExtent,
    childCrossAxisExtent,
    reverseCrossAxis: axisDirectionIsReversed(constraints.crossAxisDirection),
  });
}

/** A grid with a fixed number of tiles across (`SliverGridDelegateWithFixedCrossAxisCount`). */
export class SliverGridDelegateWithFixedCrossAxisCount implements SliverGridDelegate {
  constructor(
    readonly crossAxisCount: number,
    readonly spacing: SliverGridSpacing = {},
  ) {}

  getLayout(constraints: SliverConstraints): SliverGridLayout {
    return regularLayout(constraints, this.crossAxisCount, this.spacing);
  }
}

/** A grid whose tiles are at most `maxCrossAxisExtent` wide (`SliverGridDelegateWithMaxCrossAxisExtent`). */
export class SliverGridDelegateWithMaxCrossAxisExtent implements SliverGridDelegate {
  constructor(
    readonly maxCrossAxisExtent: number,
    readonly spacing: SliverGridSpacing = {},
  ) {}

  getLayout(constraints: SliverConstraints): SliverGridLayout {
    const crossAxisSpacing = this.spacing.crossAxisSpacing ?? 0;
    const crossAxisCount = Math.max(
      1,
      Math.ceil(constraints.crossAxisExtent / (this.maxCrossAxisExtent + crossAxisSpacing)),
    );
    return regularLayout(constraints, crossAxisCount, this.spacing);
  }
}

/** Parent data of a grid tile (`SliverGridParentData`). */
export class SliverGridParentData extends SliverMultiBoxAdaptorParentData {
  /** The offset of the child from the sliver's start edge along the cross axis. */
  crossAxisOffset: number | null = null;
}

/** A sliver that places its children in a grid (`RenderSliverGrid`). */
export class RenderSliverGrid extends RenderSliverMultiBoxAdaptor {
  private _gridDelegate: SliverGridDelegate;

  constructor(options: { childManager: RenderSliverBoxChildManager; gridDelegate: SliverGridDelegate }) {
    super(options);
    this._gridDelegate = options.gridDelegate;
  }

  get gridDelegate(): SliverGridDelegate {
    return this._gridDelegate;
  }

  set gridDelegate(value: SliverGridDelegate) {
    this._gridDelegate = value;
    this.markNeedsLayout();
  }

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof SliverGridParentData)) {
      child.parentData = new SliverGridParentData();
    }
  }

  override childCrossAxisPosition(child: RenderObject): number {
    return (child.parentData as SliverGridParentData).crossAxisOffset ?? 0;
  }

  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a line-for-line port of Flutter's RenderSliverGrid.performLayout; splitting it would hide the correspondence
  protected override performLayout(): void {
    const constraints = this.constraints;
    this.childManager.didStartLayout();
    this.childManager.setDidUnderflow(false);
    const scrollOffset = constraints.scrollOffset + constraints.cacheOrigin;
    const remainingExtent = constraints.remainingCacheExtent;
    const targetEndScrollOffset = scrollOffset + remainingExtent;
    const layout = this._gridDelegate.getLayout(constraints);
    const firstIndex = layout.getMinChildIndexForScrollOffset(scrollOffset);
    const targetLastIndex = Number.isFinite(targetEndScrollOffset)
      ? layout.getMaxChildIndexForScrollOffset(targetEndScrollOffset)
      : null;

    if (this.firstChild !== null) {
      const leadingGarbage = this.calculateLeadingGarbage({ firstIndex });
      const trailingGarbage =
        targetLastIndex !== null ? this.calculateTrailingGarbage({ lastIndex: targetLastIndex }) : 0;
      this.collectGarbage(leadingGarbage, trailingGarbage);
    } else {
      this.collectGarbage(0, 0);
    }

    const firstChildGridGeometry = layout.getGeometryForChildIndex(firstIndex);
    if (this.firstChild === null) {
      if (!this.addInitialChild({ index: firstIndex, layoutOffset: firstChildGridGeometry.scrollOffset })) {
        const max = layout.computeMaxScrollOffset(this.childManager.childCount);
        this.geometry = new SliverGeometry({ scrollExtent: max, maxPaintExtent: max });
        this.childManager.didFinishLayout();
        return;
      }
    }

    const leadingScrollOffset = firstChildGridGeometry.scrollOffset;
    let trailingScrollOffset = firstChildGridGeometry.trailingScrollOffset;
    let trailingChildWithLayout: RenderBox | null = null;
    let reachedEnd = false;

    for (let index = this.indexOf(this.firstChild as RenderBox) - 1; index >= firstIndex; index -= 1) {
      const gridGeometry = layout.getGeometryForChildIndex(index);
      const child = this.insertAndLayoutLeadingChild(gridGeometry.getBoxConstraints(constraints)) as RenderBox;
      const parentData = child.parentData as SliverGridParentData;
      parentData.layoutOffset = gridGeometry.scrollOffset;
      parentData.crossAxisOffset = gridGeometry.crossAxisOffset;
      trailingChildWithLayout ??= child;
      trailingScrollOffset = Math.max(trailingScrollOffset, gridGeometry.trailingScrollOffset);
    }

    if (trailingChildWithLayout === null) {
      const first = this.firstChild as RenderBox;
      first.layout(firstChildGridGeometry.getBoxConstraints(constraints));
      const parentData = first.parentData as SliverGridParentData;
      parentData.layoutOffset = firstChildGridGeometry.scrollOffset;
      parentData.crossAxisOffset = firstChildGridGeometry.crossAxisOffset;
      trailingChildWithLayout = first;
    }

    for (
      let index = this.indexOf(trailingChildWithLayout) + 1;
      targetLastIndex === null || index <= targetLastIndex;
      index += 1
    ) {
      const gridGeometry = layout.getGeometryForChildIndex(index);
      const childConstraints = gridGeometry.getBoxConstraints(constraints);
      let child: RenderBox | null = this.childAfter(trailingChildWithLayout);
      if (child === null || this.indexOf(child) !== index) {
        child = this.insertAndLayoutChild(childConstraints, { after: trailingChildWithLayout });
        if (child === null) {
          reachedEnd = true;
          break;
        }
      } else {
        child.layout(childConstraints);
      }
      trailingChildWithLayout = child;
      const parentData = child.parentData as SliverGridParentData;
      parentData.layoutOffset = gridGeometry.scrollOffset;
      parentData.crossAxisOffset = gridGeometry.crossAxisOffset;
      trailingScrollOffset = Math.max(trailingScrollOffset, gridGeometry.trailingScrollOffset);
    }

    const lastIndex = this.indexOf(this.lastChild as RenderBox);
    const estimatedTotalExtent = reachedEnd
      ? trailingScrollOffset
      : this.childManager.estimateMaxScrollOffset(constraints, {
          firstIndex,
          lastIndex,
          leadingScrollOffset,
          trailingScrollOffset,
        });
    const paintExtent = this.calculatePaintOffset(constraints, {
      from: Math.min(constraints.scrollOffset, leadingScrollOffset),
      to: trailingScrollOffset,
    });
    const cacheExtent = this.calculateCacheOffset(constraints, { from: leadingScrollOffset, to: trailingScrollOffset });
    this.geometry = new SliverGeometry({
      scrollExtent: estimatedTotalExtent,
      paintExtent,
      maxPaintExtent: estimatedTotalExtent,
      cacheExtent,
      hasVisualOverflow:
        estimatedTotalExtent > paintExtent || constraints.scrollOffset > 0 || constraints.overlap !== 0,
    });
    if (estimatedTotalExtent === trailingScrollOffset) {
      this.childManager.setDidUnderflow(true);
    }
    this.childManager.didFinishLayout();
  }
}
