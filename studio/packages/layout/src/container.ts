// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/object.dart (ContainerRenderObjectMixin, ContainerParentDataMixin) and packages/flutter/lib/src/rendering/box.dart (ContainerBoxParentData, RenderBoxContainerDefaultsMixin)
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Boxes with a list of box children kept as a doubly linked list through
 * each child's parent data, the seam for flex, stack and wrap layouts.
 */

import type { PipelineOwner, RenderObject } from "./object.ts";
import { BaselineOffset, BoxParentData, RenderBox, type TextBaseline } from "./render-box.ts";

/** Box parent data with sibling links (`ContainerBoxParentData`). */
export class ContainerBoxParentData<C extends RenderBox = RenderBox> extends BoxParentData {
  previousSibling: C | null = null;
  nextSibling: C | null = null;
}

/**
 * A box with any number of box children (`ContainerRenderObjectMixin` with
 * `RenderBoxContainerDefaultsMixin`, layout parts). Subclasses that need
 * more per-child data override [setupParentData] with a subclass of
 * [ContainerBoxParentData].
 */
export abstract class ContainerRenderBox<C extends RenderBox = RenderBox> extends RenderBox {
  private _childCount = 0;
  private _firstChild: C | null = null;
  private _lastChild: C | null = null;

  override setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof ContainerBoxParentData)) {
      child.parentData = new ContainerBoxParentData<C>();
    }
  }

  /** The container parent data of `child`; throws if `child` is not a child of this box. */
  protected childParentData(child: C): ContainerBoxParentData<C> {
    const parentData = child.parentData;
    if (child.parent !== this || !(parentData instanceof ContainerBoxParentData)) {
      throw new Error(`${child.constructor.name} is not a child of ${this.constructor.name}`);
    }
    return parentData as ContainerBoxParentData<C>;
  }

  get childCount(): number {
    return this._childCount;
  }

  get firstChild(): C | null {
    return this._firstChild;
  }

  get lastChild(): C | null {
    return this._lastChild;
  }

  childBefore(child: C): C | null {
    return this.childParentData(child).previousSibling;
  }

  childAfter(child: C): C | null {
    return this.childParentData(child).nextSibling;
  }

  /** The children in order. */
  getChildrenAsList(): C[] {
    const result: C[] = [];
    let child = this._firstChild;
    while (child !== null) {
      result.push(child);
      child = this.childParentData(child).nextSibling;
    }
    return result;
  }

  private _insertIntoChildList(child: C, after: C | null): void {
    const childParentData = this.childParentData(child);
    this._childCount += 1;
    if (after === null) {
      childParentData.nextSibling = this._firstChild;
      if (this._firstChild !== null) {
        this.childParentData(this._firstChild).previousSibling = child;
      }
      this._firstChild = child;
      this._lastChild ??= child;
      return;
    }
    const afterParentData = this.childParentData(after);
    const next = afterParentData.nextSibling;
    if (next === null) {
      childParentData.previousSibling = after;
      afterParentData.nextSibling = child;
      this._lastChild = child;
      return;
    }
    childParentData.nextSibling = next;
    childParentData.previousSibling = after;
    afterParentData.nextSibling = child;
    this.childParentData(next).previousSibling = child;
  }

  /** Inserts `child` after `after`, or first when `after` is null. */
  insert(child: C, after: C | null = null): void {
    this.adoptChild(child);
    this._insertIntoChildList(child, after);
  }

  /** Appends `child`. */
  add(child: C): void {
    this.insert(child, this._lastChild);
  }

  /** Appends each of `children` in order. */
  addAll(children: readonly C[]): void {
    for (const child of children) {
      this.add(child);
    }
  }

  private _removeFromChildList(child: C): void {
    const childParentData = this.childParentData(child);
    const previous = childParentData.previousSibling;
    const next = childParentData.nextSibling;
    if (previous === null) {
      this._firstChild = next;
    } else {
      this.childParentData(previous).nextSibling = next;
    }
    if (next === null) {
      this._lastChild = previous;
    } else {
      this.childParentData(next).previousSibling = previous;
    }
    childParentData.previousSibling = null;
    childParentData.nextSibling = null;
    this._childCount -= 1;
  }

  /** Removes `child`. */
  remove(child: C): void {
    this._removeFromChildList(child);
    this.dropChild(child);
  }

  /** Removes every child. */
  removeAll(): void {
    let child = this._firstChild;
    while (child !== null) {
      const childParentData = this.childParentData(child);
      const next = childParentData.nextSibling;
      childParentData.previousSibling = null;
      childParentData.nextSibling = null;
      this.dropChild(child);
      child = next;
    }
    this._firstChild = null;
    this._lastChild = null;
    this._childCount = 0;
  }

  /** Moves `child` to just after `after` (first when null). */
  move(child: C, after: C | null = null): void {
    if (this.childParentData(child).previousSibling === after) {
      return;
    }
    this._removeFromChildList(child);
    this._insertIntoChildList(child, after);
    this.markNeedsLayout();
  }

  override attach(owner: PipelineOwner): void {
    super.attach(owner);
    for (const child of this.getChildrenAsList()) {
      child.attach(owner);
    }
  }

  override detach(): void {
    super.detach();
    for (const child of this.getChildrenAsList()) {
      child.detach();
    }
  }

  override visitChildren(visitor: (child: RenderObject) => void): void {
    for (const child of this.getChildrenAsList()) {
      visitor(child);
    }
  }

  /** The baseline of the first child that has one, offset by its position. */
  protected defaultComputeDistanceToFirstActualBaseline(baseline: TextBaseline): number | null {
    let child = this._firstChild;
    while (child !== null) {
      const childParentData = this.childParentData(child);
      const result = child.getDistanceToActualBaseline(baseline);
      if (result !== null) {
        return result + childParentData.offset.dy;
      }
      child = childParentData.nextSibling;
    }
    return null;
  }

  /** The smallest baseline among the children, each offset by its position. */
  protected defaultComputeDistanceToHighestActualBaseline(baseline: TextBaseline): number | null {
    let minBaseline = BaselineOffset.noBaseline;
    let child = this._firstChild;
    while (child !== null) {
      const childParentData = this.childParentData(child);
      const candidate = new BaselineOffset(child.getDistanceToActualBaseline(baseline)).plus(childParentData.offset.dy);
      minBaseline = minBaseline.minOf(candidate);
      child = childParentData.nextSibling;
    }
    return minBaseline.offset;
  }
}
