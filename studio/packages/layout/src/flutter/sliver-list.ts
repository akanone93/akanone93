// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/sliver_multi_box_adaptor.dart, sliver_list.dart, sliver_fixed_extent_list.dart and sliver_fill.dart (RenderSliverFillViewport)
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Slivers with many box children: [RenderSliverMultiBoxAdaptor], the list
 * ([RenderSliverList]), the fixed and varied extent lists
 * ([RenderSliverFixedExtentList], [RenderSliverVariedExtentList]) and the
 * page-sized list ([RenderSliverFillViewport]).
 *
 * Flutter creates a list's children lazily through widgets and elements. Here
 * a [PrebuiltChildManager] owns every child render object and inserts or
 * removes them as the sliver asks, so the set of live children after a layout
 * is the one Flutter keeps. Keep-alive children are not ported (no widget in
 * the scene vocabulary asks for it). Painting and hit testing are not ported.
 */

import { ChildList, type ContainerParentData } from "./child-list.ts";
import type { BoxConstraints } from "./constraints.ts";
import { Axis } from "./flex.ts";
import { type Offset, precisionErrorTolerance } from "./geometry.ts";
import type { PipelineOwner, RenderObject } from "./object.ts";
import type { RenderBox } from "./render-box.ts";
import {
  type ItemExtentBuilder,
  RenderSliver,
  type SliverConstraints,
  SliverGeometry,
  type SliverLayoutDimensions,
  SliverLogicalParentData,
} from "./sliver.ts";

/** What a multi-box sliver asks of whoever owns its children (`RenderSliverBoxChildManager`). */
export interface RenderSliverBoxChildManager {
  /** Makes the child at `index` a child of the sliver, after `after`; does nothing when there is no such child. */
  createChild(index: number, options: { after: RenderBox | null }): void;
  /** Removes `child` from the sliver. */
  removeChild(child: RenderBox): void;
  /** The scroll extent of all children, extrapolated from the ones laid out. */
  estimateMaxScrollOffset(
    constraints: SliverConstraints,
    options: { firstIndex: number; lastIndex: number; leadingScrollOffset: number; trailingScrollOffset: number },
  ): number;
  /** The number of children there are. */
  readonly childCount: number;
  /** The number of children there are, if known. */
  readonly estimatedChildCount: number | null;
  /** Called when the sliver adopts `child`. */
  didAdoptChild(child: RenderBox): void;
  /** Records whether the last layout ran out of children. */
  setDidUnderflow(value: boolean): void;
  didStartLayout(): void;
  didFinishLayout(): void;
}

/** Parent data of a multi-box sliver's children (`SliverMultiBoxAdaptorParentData`). */
export class SliverMultiBoxAdaptorParentData extends SliverLogicalParentData implements ContainerParentData<RenderBox> {
  previousSibling: RenderBox | null = null;
  nextSibling: RenderBox | null = null;
  /** The child's index in the list. */
  index: number | null = null;
}

/**
 * A child manager that holds every child render object up front and inserts
 * them on demand, the way Flutter's `SliverMultiBoxAdaptorElement` builds
 * them.
 */
export class PrebuiltChildManager implements RenderSliverBoxChildManager {
  private adaptor: RenderSliverMultiBoxAdaptor | null = null;
  private pendingIndex = 0;
  private _didUnderflow = false;

  constructor(readonly children: readonly RenderBox[]) {}

  /** Whether the last layout ran out of children before filling the viewport. */
  get didUnderflow(): boolean {
    return this._didUnderflow;
  }

  /** Connects the sliver this manager serves; called by the sliver's constructor. */
  attachTo(adaptor: RenderSliverMultiBoxAdaptor): void {
    this.adaptor = adaptor;
  }

  private get sliver(): RenderSliverMultiBoxAdaptor {
    if (this.adaptor === null) {
      throw new Error("PrebuiltChildManager is not attached to a sliver");
    }
    return this.adaptor;
  }

  get childCount(): number {
    return this.children.length;
  }

  get estimatedChildCount(): number | null {
    return this.children.length;
  }

  createChild(index: number, options: { after: RenderBox | null }): void {
    const child = this.children[index];
    if (index < 0 || child === undefined) {
      return;
    }
    this.pendingIndex = index;
    this.sliver.insert(child, options.after);
  }

  removeChild(child: RenderBox): void {
    this.sliver.remove(child);
  }

  didAdoptChild(child: RenderBox): void {
    (child.parentData as SliverMultiBoxAdaptorParentData).index = this.pendingIndex;
  }

  estimateMaxScrollOffset(
    _constraints: SliverConstraints,
    options: { firstIndex: number; lastIndex: number; leadingScrollOffset: number; trailingScrollOffset: number },
  ): number {
    const { firstIndex, lastIndex, leadingScrollOffset, trailingScrollOffset } = options;
    if (lastIndex === this.childCount - 1) {
      return trailingScrollOffset;
    }
    const reifiedCount = lastIndex - firstIndex + 1;
    const averageExtent = (trailingScrollOffset - leadingScrollOffset) / reifiedCount;
    const remainingCount = this.childCount - lastIndex - 1;
    return trailingScrollOffset + averageExtent * remainingCount;
  }

  setDidUnderflow(value: boolean): void {
    this._didUnderflow = value;
  }

  didStartLayout(): void {}

  didFinishLayout(): void {}
}

/** A sliver with a linear list of box children, created and destroyed as it scrolls (`RenderSliverMultiBoxAdaptor`). */
export abstract class RenderSliverMultiBoxAdaptor extends RenderSliver {
  protected readonly boxes: ChildList<RenderBox>;

  constructor(options: { childManager: RenderSliverBoxChildManager }) {
    super();
    this.childManager = options.childManager;
    this.boxes = new ChildList<RenderBox>({
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
    if (options.childManager instanceof PrebuiltChildManager) {
      options.childManager.attachTo(this);
    }
  }

  protected readonly childManager: RenderSliverBoxChildManager;

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof SliverMultiBoxAdaptorParentData)) {
      child.parentData = new SliverMultiBoxAdaptorParentData();
    }
  }

  protected override adoptChild(child: RenderObject): void {
    super.adoptChild(child);
    this.childManager.didAdoptChild(child as RenderBox);
  }

  get firstChild(): RenderBox | null {
    return this.boxes.firstChild;
  }

  get lastChild(): RenderBox | null {
    return this.boxes.lastChild;
  }

  childBefore(child: RenderBox): RenderBox | null {
    return this.boxes.childBefore(child);
  }

  childAfter(child: RenderBox): RenderBox | null {
    return this.boxes.childAfter(child);
  }

  /** Inserts `child` after `after` (first when null). */
  insert(child: RenderBox, after: RenderBox | null = null): void {
    this.boxes.insert(child, after);
  }

  /** Removes `child`. */
  remove(child: RenderBox): void {
    this.boxes.remove(child);
  }

  override attach(owner: PipelineOwner): void {
    super.attach(owner);
    for (const child of this.boxes.toArray()) {
      child.attach(owner);
    }
  }

  override detach(): void {
    super.detach();
    for (const child of this.boxes.toArray()) {
      child.detach();
    }
  }

  override visitChildren(visitor: (child: RenderObject) => void): void {
    for (const child of this.boxes.toArray()) {
      visitor(child);
    }
  }

  /** Asks the child manager to create the child at `index`. */
  private createChild(index: number, after: RenderBox | null): void {
    this.childManager.createChild(index, { after });
  }

  /** Creates the first child at `index` and positions it at `layoutOffset`; false when there is none. */
  protected addInitialChild(options: { index?: number; layoutOffset?: number } = {}): boolean {
    this.createChild(options.index ?? 0, null);
    const first = this.firstChild;
    if (first !== null) {
      (first.parentData as SliverMultiBoxAdaptorParentData).layoutOffset = options.layoutOffset ?? 0;
      return true;
    }
    this.childManager.setDidUnderflow(true);
    return false;
  }

  /** Inserts and lays out the child before the first; null when there is none. */
  protected insertAndLayoutLeadingChild(childConstraints: BoxConstraints, parentUsesSize = false): RenderBox | null {
    const first = this.firstChild as RenderBox;
    const index = this.indexOf(first) - 1;
    this.createChild(index, null);
    const newFirst = this.firstChild as RenderBox;
    if (this.indexOf(newFirst) === index) {
      newFirst.layout(childConstraints, { parentUsesSize });
      return newFirst;
    }
    this.childManager.setDidUnderflow(true);
    return null;
  }

  /** Inserts and lays out the child after `after`; null when there is none. */
  protected insertAndLayoutChild(
    childConstraints: BoxConstraints,
    options: { after: RenderBox | null; parentUsesSize?: boolean },
  ): RenderBox | null {
    const after = options.after as RenderBox;
    const index = this.indexOf(after) + 1;
    this.createChild(index, after);
    const child = this.childAfter(after);
    if (child !== null && this.indexOf(child) === index) {
      child.layout(childConstraints, { parentUsesSize: options.parentUsesSize ?? false });
      return child;
    }
    this.childManager.setDidUnderflow(true);
    return null;
  }

  /** How many children lie wholly before `firstIndex`. */
  protected calculateLeadingGarbage(options: { firstIndex: number }): number {
    let walker = this.firstChild;
    let leadingGarbage = 0;
    while (walker !== null && this.indexOf(walker) < options.firstIndex) {
      leadingGarbage += 1;
      walker = this.childAfter(walker);
    }
    return leadingGarbage;
  }

  /** How many children lie wholly after `lastIndex`. */
  protected calculateTrailingGarbage(options: { lastIndex: number }): number {
    let walker = this.lastChild;
    let trailingGarbage = 0;
    while (walker !== null && this.indexOf(walker) > options.lastIndex) {
      trailingGarbage += 1;
      walker = this.childBefore(walker);
    }
    return trailingGarbage;
  }

  /** Removes `leadingGarbage` children from the start and `trailingGarbage` from the end. */
  protected collectGarbage(leadingGarbage: number, trailingGarbage: number): void {
    for (let n = leadingGarbage; n > 0; n -= 1) {
      this.childManager.removeChild(this.firstChild as RenderBox);
    }
    for (let n = trailingGarbage; n > 0; n -= 1) {
      this.childManager.removeChild(this.lastChild as RenderBox);
    }
  }

  /** The index of `child` in the list. */
  indexOf(child: RenderBox): number {
    return (child.parentData as SliverMultiBoxAdaptorParentData).index as number;
  }

  /** The extent of `child` along the main axis. */
  paintExtentOf(child: RenderBox): number {
    return this.constraints.axis === Axis.horizontal ? child.size.width : child.size.height;
  }

  override childMainAxisPosition(child: RenderObject): number {
    return (this.childScrollOffset(child) ?? 0) - this.constraints.scrollOffset;
  }

  override childScrollOffset(child: RenderObject): number | null {
    return (child.parentData as SliverMultiBoxAdaptorParentData).layoutOffset;
  }

  override childPaintOffset(child: RenderObject): Offset {
    return this.boxChildPaintOffset(child as RenderBox);
  }
}

/** A list of children of any size, laid out one after the other (`RenderSliverList`). */
export class RenderSliverList extends RenderSliverMultiBoxAdaptor {
  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a line-for-line port of Flutter's RenderSliverList.performLayout; splitting it would hide the correspondence
  protected override performLayout(): void {
    const constraints = this.constraints;
    this.childManager.didStartLayout();
    this.childManager.setDidUnderflow(false);
    const scrollOffset = constraints.scrollOffset + constraints.cacheOrigin;
    const remainingExtent = constraints.remainingCacheExtent;
    const targetEndScrollOffset = scrollOffset + remainingExtent;
    const childConstraints = constraints.asBoxConstraints();
    let leadingGarbage = 0;
    let trailingGarbage = 0;
    let reachedEnd = false;

    if (this.firstChild === null) {
      if (!this.addInitialChild()) {
        this.geometry = SliverGeometry.zero;
        this.childManager.didFinishLayout();
        return;
      }
    }

    let leadingChildWithLayout: RenderBox | null = null;
    let trailingChildWithLayout: RenderBox | null = null;
    let earliestUsefulChild: RenderBox | null = this.firstChild;

    if (this.childScrollOffset(this.firstChild as RenderBox) === null) {
      let leadingChildrenWithoutLayoutOffset = 0;
      while (earliestUsefulChild !== null && this.childScrollOffset(earliestUsefulChild) === null) {
        earliestUsefulChild = this.childAfter(earliestUsefulChild);
        leadingChildrenWithoutLayoutOffset += 1;
      }
      this.collectGarbage(leadingChildrenWithoutLayoutOffset, 0);
      if (this.firstChild === null) {
        if (!this.addInitialChild()) {
          this.geometry = SliverGeometry.zero;
          this.childManager.didFinishLayout();
          return;
        }
      }
    }

    earliestUsefulChild = this.firstChild as RenderBox;
    for (
      let earliestScrollOffset = this.childScrollOffset(earliestUsefulChild) as number;
      earliestScrollOffset > scrollOffset;
      earliestScrollOffset = this.childScrollOffset(earliestUsefulChild) as number
    ) {
      const inserted = this.insertAndLayoutLeadingChild(childConstraints, true);
      if (inserted === null) {
        const first = this.firstChild as RenderBox;
        (first.parentData as SliverMultiBoxAdaptorParentData).layoutOffset = 0;
        if (scrollOffset === 0) {
          first.layout(childConstraints, { parentUsesSize: true });
          earliestUsefulChild = first;
          leadingChildWithLayout = earliestUsefulChild;
          trailingChildWithLayout ??= earliestUsefulChild;
          break;
        }
        this.geometry = new SliverGeometry({ scrollOffsetCorrection: -scrollOffset });
        return;
      }
      earliestUsefulChild = inserted;
      const firstChildScrollOffset = earliestScrollOffset - this.paintExtentOf(this.firstChild as RenderBox);
      if (firstChildScrollOffset < -precisionErrorTolerance) {
        this.geometry = new SliverGeometry({ scrollOffsetCorrection: -firstChildScrollOffset });
        ((this.firstChild as RenderBox).parentData as SliverMultiBoxAdaptorParentData).layoutOffset = 0;
        return;
      }
      (earliestUsefulChild.parentData as SliverMultiBoxAdaptorParentData).layoutOffset = firstChildScrollOffset;
      leadingChildWithLayout = earliestUsefulChild;
      trailingChildWithLayout ??= earliestUsefulChild;
    }

    if (scrollOffset < precisionErrorTolerance) {
      while (this.indexOf(this.firstChild as RenderBox) > 0) {
        const earliestScrollOffset = this.childScrollOffset(this.firstChild as RenderBox) as number;
        earliestUsefulChild = this.insertAndLayoutLeadingChild(childConstraints, true);
        const first = this.firstChild as RenderBox;
        const firstChildScrollOffset = earliestScrollOffset - this.paintExtentOf(first);
        (first.parentData as SliverMultiBoxAdaptorParentData).layoutOffset = 0;
        if (firstChildScrollOffset < -precisionErrorTolerance) {
          this.geometry = new SliverGeometry({ scrollOffsetCorrection: -firstChildScrollOffset });
          return;
        }
      }
    }

    if (leadingChildWithLayout === null) {
      (earliestUsefulChild as RenderBox).layout(childConstraints, { parentUsesSize: true });
      leadingChildWithLayout = earliestUsefulChild;
      trailingChildWithLayout = earliestUsefulChild;
    }

    let inLayoutRange = true;
    let child: RenderBox | null = earliestUsefulChild;
    let index = this.indexOf(child as RenderBox);
    let endScrollOffset =
      (this.childScrollOffset(child as RenderBox) as number) + this.paintExtentOf(child as RenderBox);

    // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: the local `advance` function of Flutter's RenderSliverList.performLayout
    const advance = (): boolean => {
      if (child === trailingChildWithLayout) {
        inLayoutRange = false;
      }
      let next = this.childAfter(child as RenderBox);
      if (next === null) {
        inLayoutRange = false;
      }
      index += 1;
      if (!inLayoutRange) {
        if (next === null || this.indexOf(next) !== index) {
          next = this.insertAndLayoutChild(childConstraints, { after: trailingChildWithLayout, parentUsesSize: true });
          if (next === null) {
            child = null;
            return false;
          }
        } else {
          next.layout(childConstraints, { parentUsesSize: true });
        }
        trailingChildWithLayout = next;
      }
      if (next === null) {
        child = null;
        return false;
      }
      child = next;
      (next.parentData as SliverMultiBoxAdaptorParentData).layoutOffset = endScrollOffset;
      endScrollOffset = (this.childScrollOffset(next) as number) + this.paintExtentOf(next);
      return true;
    };

    while (endScrollOffset < scrollOffset) {
      leadingGarbage += 1;
      if (!advance()) {
        this.collectGarbage(leadingGarbage - 1, 0);
        const last = this.lastChild as RenderBox;
        const extent = (this.childScrollOffset(last) as number) + this.paintExtentOf(last);
        this.geometry = new SliverGeometry({ scrollExtent: extent, maxPaintExtent: extent });
        return;
      }
    }

    while (endScrollOffset < targetEndScrollOffset) {
      if (!advance()) {
        reachedEnd = true;
        break;
      }
    }

    if (child !== null) {
      child = this.childAfter(child);
      while (child !== null) {
        trailingGarbage += 1;
        child = this.childAfter(child);
      }
    }

    this.collectGarbage(leadingGarbage, trailingGarbage);

    const first = this.firstChild as RenderBox;
    const estimatedMaxScrollOffset = reachedEnd
      ? endScrollOffset
      : this.childManager.estimateMaxScrollOffset(constraints, {
          firstIndex: this.indexOf(first),
          lastIndex: this.indexOf(this.lastChild as RenderBox),
          leadingScrollOffset: this.childScrollOffset(first) as number,
          trailingScrollOffset: endScrollOffset,
        });
    const paintExtent = this.calculatePaintOffset(constraints, {
      from: this.childScrollOffset(first) as number,
      to: endScrollOffset,
    });
    const cacheExtent = this.calculateCacheOffset(constraints, {
      from: this.childScrollOffset(first) as number,
      to: endScrollOffset,
    });
    const targetEndScrollOffsetForPaint = constraints.scrollOffset + constraints.remainingPaintExtent;
    this.geometry = new SliverGeometry({
      scrollExtent: estimatedMaxScrollOffset,
      paintExtent,
      cacheExtent,
      maxPaintExtent: estimatedMaxScrollOffset,
      hasVisualOverflow: endScrollOffset > targetEndScrollOffsetForPaint || constraints.scrollOffset > 0,
    });
    if (estimatedMaxScrollOffset === endScrollOffset) {
      this.childManager.setDidUnderflow(true);
    }
    this.childManager.didFinishLayout();
  }
}

/** The list of children that all share one main axis extent, or whose extents a builder knows (`RenderSliverFixedExtentBoxAdaptor`). */
export abstract class RenderSliverFixedExtentBoxAdaptor extends RenderSliverMultiBoxAdaptor {
  private currentLayoutDimensions: SliverLayoutDimensions | null = null;

  /** The main axis extent of every child, or null when [itemExtentBuilder] gives them. */
  abstract get itemExtent(): number | null;

  /** Gives the extent of each child, or null when [itemExtent] is used. */
  get itemExtentBuilder(): ItemExtentBuilder | null {
    return null;
  }

  /** The viewport dimensions of the layout in progress, or of the last one. */
  get layoutDimensions(): SliverLayoutDimensions {
    return (
      this.currentLayoutDimensions ?? {
        scrollOffset: this.constraints.scrollOffset,
        precedingScrollExtent: this.constraints.precedingScrollExtent,
        viewportMainAxisExtent: this.constraints.viewportMainAxisExtent,
        crossAxisExtent: this.constraints.crossAxisExtent,
      }
    );
  }

  /** The scroll offset at which the child at `index` starts. */
  indexToLayoutOffset(index: number): number {
    const builder = this.itemExtentBuilder;
    if (builder === null) {
      return (this.itemExtent as number) * index;
    }
    let offset = 0;
    for (let i = 0; i < index; i += 1) {
      const childCount = this.childManager.estimatedChildCount;
      if (childCount !== null && i > childCount - 1) {
        break;
      }
      const itemExtent = builder(i, this.layoutDimensions);
      if (itemExtent === null) {
        break;
      }
      offset += itemExtent;
    }
    return offset;
  }

  /** The smallest index of a child that is not wholly before `scrollOffset`. */
  getMinChildIndexForScrollOffset(scrollOffset: number): number {
    const builder = this.itemExtentBuilder;
    if (builder !== null) {
      return this.getChildIndexForScrollOffset(scrollOffset, builder);
    }
    const itemExtent = this.itemExtent as number;
    if (itemExtent > 0) {
      const actual = scrollOffset / itemExtent;
      const round = Math.round(actual);
      if (Math.abs(actual * itemExtent - round * itemExtent) < precisionErrorTolerance) {
        return round;
      }
      return Math.floor(actual);
    }
    return 0;
  }

  /** The largest index of a child that starts before `scrollOffset`. */
  getMaxChildIndexForScrollOffset(scrollOffset: number): number {
    const builder = this.itemExtentBuilder;
    if (builder !== null) {
      return this.getChildIndexForScrollOffset(scrollOffset, builder);
    }
    const itemExtent = this.itemExtent as number;
    if (itemExtent > 0) {
      const actual = scrollOffset / itemExtent - 1;
      const round = Math.round(actual);
      if (Math.abs(actual * itemExtent - round * itemExtent) < precisionErrorTolerance) {
        return Math.max(0, round);
      }
      return Math.max(0, Math.ceil(actual));
    }
    return 0;
  }

  /** The scroll extent of all children. */
  computeMaxScrollOffset(): number {
    const builder = this.itemExtentBuilder;
    if (builder === null) {
      return this.childManager.childCount * (this.itemExtent as number);
    }
    let offset = 0;
    for (let i = 0; i < this.childManager.childCount; i += 1) {
      const itemExtent = builder(i, this.layoutDimensions);
      if (itemExtent === null) {
        break;
      }
      offset += itemExtent;
    }
    return offset;
  }

  private getChildIndexForScrollOffset(scrollOffset: number, callback: ItemExtentBuilder): number {
    if (scrollOffset === 0) {
      return 0;
    }
    let position = 0;
    let index = 0;
    while (position < scrollOffset) {
      const childCount = this.childManager.estimatedChildCount;
      if (childCount !== null && index > childCount - 1) {
        break;
      }
      const itemExtent = callback(index, this.layoutDimensions);
      if (itemExtent === null) {
        break;
      }
      position += itemExtent;
      index += 1;
    }
    return index - 1;
  }

  private getChildConstraints(index: number): BoxConstraints {
    const builder = this.itemExtentBuilder;
    const extent = builder === null ? (this.itemExtent as number) : (builder(index, this.layoutDimensions) as number);
    return this.constraints.asBoxConstraints({ minExtent: extent, maxExtent: extent });
  }

  override paintExtentOf(child: RenderBox): number {
    const builder = this.itemExtentBuilder;
    if (builder === null) {
      return this.itemExtent as number;
    }
    return builder(this.indexOf(child), this.layoutDimensions) as number;
  }

  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a line-for-line port of Flutter's RenderSliverFixedExtentBoxAdaptor.performLayout; splitting it would hide the correspondence
  protected override performLayout(): void {
    const constraints = this.constraints;
    this.childManager.didStartLayout();
    this.childManager.setDidUnderflow(false);
    const scrollOffset = constraints.scrollOffset + constraints.cacheOrigin;
    const remainingExtent = constraints.remainingCacheExtent;
    const targetEndScrollOffset = scrollOffset + remainingExtent;
    this.currentLayoutDimensions = {
      scrollOffset: constraints.scrollOffset,
      precedingScrollExtent: constraints.precedingScrollExtent,
      viewportMainAxisExtent: constraints.viewportMainAxisExtent,
      crossAxisExtent: constraints.crossAxisExtent,
    };
    const firstIndex = this.getMinChildIndexForScrollOffset(scrollOffset);
    const targetLastIndex = Number.isFinite(targetEndScrollOffset)
      ? this.getMaxChildIndexForScrollOffset(targetEndScrollOffset)
      : null;

    if (this.firstChild !== null) {
      const leadingGarbage = this.calculateLeadingGarbage({ firstIndex });
      const trailingGarbage =
        targetLastIndex !== null ? this.calculateTrailingGarbage({ lastIndex: targetLastIndex }) : 0;
      this.collectGarbage(leadingGarbage, trailingGarbage);
    } else {
      this.collectGarbage(0, 0);
    }

    if (this.firstChild === null) {
      const layoutOffset = this.indexToLayoutOffset(firstIndex);
      if (!this.addInitialChild({ index: firstIndex, layoutOffset })) {
        const max = firstIndex <= 0 ? 0 : this.computeMaxScrollOffset();
        this.geometry = new SliverGeometry({ scrollExtent: max, maxPaintExtent: max });
        this.childManager.didFinishLayout();
        return;
      }
    }

    let trailingChildWithLayout: RenderBox | null = null;
    for (let index = this.indexOf(this.firstChild as RenderBox) - 1; index >= firstIndex; index -= 1) {
      const child = this.insertAndLayoutLeadingChild(this.getChildConstraints(index));
      if (child === null) {
        this.geometry = new SliverGeometry({ scrollOffsetCorrection: this.indexToLayoutOffset(index) });
        return;
      }
      (child.parentData as SliverMultiBoxAdaptorParentData).layoutOffset = this.indexToLayoutOffset(index);
      trailingChildWithLayout ??= child;
    }

    if (trailingChildWithLayout === null) {
      const first = this.firstChild as RenderBox;
      first.layout(this.getChildConstraints(this.indexOf(first)));
      (first.parentData as SliverMultiBoxAdaptorParentData).layoutOffset = this.indexToLayoutOffset(firstIndex);
      trailingChildWithLayout = first;
    }

    let estimatedMaxScrollOffset = Number.POSITIVE_INFINITY;
    for (
      let index = this.indexOf(trailingChildWithLayout) + 1;
      targetLastIndex === null || index <= targetLastIndex;
      index += 1
    ) {
      let child: RenderBox | null = this.childAfter(trailingChildWithLayout);
      if (child === null || this.indexOf(child) !== index) {
        child = this.insertAndLayoutChild(this.getChildConstraints(index), { after: trailingChildWithLayout });
        if (child === null) {
          estimatedMaxScrollOffset = this.indexToLayoutOffset(index);
          break;
        }
      } else {
        child.layout(this.getChildConstraints(index));
      }
      trailingChildWithLayout = child;
      const parentData = child.parentData as SliverMultiBoxAdaptorParentData;
      parentData.layoutOffset = this.indexToLayoutOffset(parentData.index as number);
    }

    const lastIndex = this.indexOf(this.lastChild as RenderBox);
    const leadingScrollOffset = this.indexToLayoutOffset(firstIndex);
    const trailingScrollOffset = this.indexToLayoutOffset(lastIndex + 1);
    estimatedMaxScrollOffset = Math.min(
      estimatedMaxScrollOffset,
      this.childManager.estimateMaxScrollOffset(constraints, {
        firstIndex,
        lastIndex,
        leadingScrollOffset,
        trailingScrollOffset,
      }),
    );
    const paintExtent = this.calculatePaintOffset(constraints, { from: leadingScrollOffset, to: trailingScrollOffset });
    const cacheExtent = this.calculateCacheOffset(constraints, { from: leadingScrollOffset, to: trailingScrollOffset });
    const targetEndScrollOffsetForPaint = constraints.scrollOffset + constraints.remainingPaintExtent;
    const targetLastIndexForPaint = Number.isFinite(targetEndScrollOffsetForPaint)
      ? this.getMaxChildIndexForScrollOffset(targetEndScrollOffsetForPaint)
      : null;
    this.geometry = new SliverGeometry({
      scrollExtent: estimatedMaxScrollOffset,
      paintExtent,
      cacheExtent,
      maxPaintExtent: estimatedMaxScrollOffset,
      hasVisualOverflow:
        (targetLastIndexForPaint !== null && lastIndex >= targetLastIndexForPaint) || constraints.scrollOffset > 0,
    });
    if (estimatedMaxScrollOffset === trailingScrollOffset) {
      this.childManager.setDidUnderflow(true);
    }
    this.childManager.didFinishLayout();
  }
}

/** A list whose children all have the main axis extent `itemExtent` (`RenderSliverFixedExtentList`). */
export class RenderSliverFixedExtentList extends RenderSliverFixedExtentBoxAdaptor {
  private _itemExtent: number;

  constructor(options: { childManager: RenderSliverBoxChildManager; itemExtent: number }) {
    super(options);
    this._itemExtent = options.itemExtent;
  }

  get itemExtent(): number {
    return this._itemExtent;
  }

  set itemExtent(value: number) {
    if (this._itemExtent !== value) {
      this._itemExtent = value;
      this.markNeedsLayout();
    }
  }
}

/** A list whose children's extents come from a builder (`RenderSliverVariedExtentList`). */
export class RenderSliverVariedExtentList extends RenderSliverFixedExtentBoxAdaptor {
  private _itemExtentBuilder: ItemExtentBuilder;

  constructor(options: { childManager: RenderSliverBoxChildManager; itemExtentBuilder: ItemExtentBuilder }) {
    super(options);
    this._itemExtentBuilder = options.itemExtentBuilder;
  }

  override get itemExtentBuilder(): ItemExtentBuilder {
    return this._itemExtentBuilder;
  }

  get itemExtent(): number | null {
    return null;
  }
}

/** A list whose children each fill `viewportFraction` of the viewport (`RenderSliverFillViewport`, the pages of a `PageView`). */
export class RenderSliverFillViewport extends RenderSliverFixedExtentBoxAdaptor {
  private _viewportFraction: number;

  constructor(options: { childManager: RenderSliverBoxChildManager; viewportFraction?: number }) {
    super(options);
    this._viewportFraction = options.viewportFraction ?? 1;
  }

  get itemExtent(): number {
    return this.constraints.viewportMainAxisExtent * this.viewportFraction;
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
}
