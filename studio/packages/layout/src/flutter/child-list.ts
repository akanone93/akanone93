// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/object.dart (ContainerParentDataMixin and ContainerRenderObjectMixin, the child list)
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * The doubly linked child list of Flutter's `ContainerRenderObjectMixin`,
 * for hosts whose children are not boxes (a viewport's slivers) or whose
 * own class cannot extend [ContainerRenderBox] (a sliver's box children).
 * The host passes [adoptChild], [dropChild] and [markNeedsLayout] because
 * those are protected on [RenderObject].
 */

import type { ParentData, RenderObject } from "./object.ts";

/** The sibling links a child's parent data carries (`ContainerParentDataMixin`). */
export interface ContainerParentData<C extends RenderObject> {
  previousSibling: C | null;
  nextSibling: C | null;
}

/** What a [ChildList] needs from its host render object. */
export interface ChildListHost {
  adoptChild(child: RenderObject): void;
  dropChild(child: RenderObject): void;
  markNeedsLayout(): void;
}

function links<C extends RenderObject>(child: C): ContainerParentData<C> {
  const parentData = child.parentData as (ParentData & Partial<ContainerParentData<C>>) | null;
  if (parentData === null || !("previousSibling" in parentData) || !("nextSibling" in parentData)) {
    throw new Error(`${child.constructor.name} has no container parent data`);
  }
  return parentData as unknown as ContainerParentData<C>;
}

/** A host's children in order, kept as links through each child's parent data. */
export class ChildList<C extends RenderObject> {
  private _count = 0;
  private _first: C | null = null;
  private _last: C | null = null;

  constructor(private readonly host: ChildListHost) {}

  get childCount(): number {
    return this._count;
  }

  get firstChild(): C | null {
    return this._first;
  }

  get lastChild(): C | null {
    return this._last;
  }

  childBefore(child: C): C | null {
    return links(child).previousSibling;
  }

  childAfter(child: C): C | null {
    return links(child).nextSibling;
  }

  toArray(): C[] {
    const result: C[] = [];
    for (let child = this._first; child !== null; child = links(child).nextSibling) {
      result.push(child);
    }
    return result;
  }

  private insertIntoList(child: C, after: C | null): void {
    const childLinks = links(child);
    this._count += 1;
    if (after === null) {
      childLinks.nextSibling = this._first;
      if (this._first !== null) {
        links(this._first).previousSibling = child;
      }
      this._first = child;
      this._last ??= child;
      return;
    }
    const afterLinks = links(after);
    const next = afterLinks.nextSibling;
    if (next === null) {
      childLinks.previousSibling = after;
      afterLinks.nextSibling = child;
      this._last = child;
      return;
    }
    childLinks.nextSibling = next;
    childLinks.previousSibling = after;
    afterLinks.nextSibling = child;
    links(next).previousSibling = child;
  }

  private removeFromList(child: C): void {
    const childLinks = links(child);
    const previous = childLinks.previousSibling;
    const next = childLinks.nextSibling;
    if (previous === null) {
      this._first = next;
    } else {
      links(previous).nextSibling = next;
    }
    if (next === null) {
      this._last = previous;
    } else {
      links(next).previousSibling = previous;
    }
    childLinks.previousSibling = null;
    childLinks.nextSibling = null;
    this._count -= 1;
  }

  /** Inserts `child` after `after`, or first when `after` is null. */
  insert(child: C, after: C | null = null): void {
    this.host.adoptChild(child);
    this.insertIntoList(child, after);
  }

  add(child: C): void {
    this.insert(child, this._last);
  }

  addAll(children: readonly C[]): void {
    for (const child of children) {
      this.add(child);
    }
  }

  remove(child: C): void {
    this.removeFromList(child);
    this.host.dropChild(child);
  }

  removeAll(): void {
    let child = this._first;
    while (child !== null) {
      const childLinks = links(child);
      const next = childLinks.nextSibling;
      childLinks.previousSibling = null;
      childLinks.nextSibling = null;
      this.host.dropChild(child);
      child = next;
    }
    this._first = null;
    this._last = null;
    this._count = 0;
  }

  move(child: C, after: C | null = null): void {
    if (links(child).previousSibling === after) {
      return;
    }
    this.removeFromList(child);
    this.insertIntoList(child, after);
    this.host.markNeedsLayout();
  }
}
