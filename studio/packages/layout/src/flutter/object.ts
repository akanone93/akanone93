// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/object.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * The layout half of Flutter's render tree: [RenderObject] with its
 * relayout-boundary rule and dirty propagation, and [PipelineOwner], which
 * lays out dirty relayout boundaries shallowest first. Painting, compositing,
 * semantics, hit testing and debug checks are not ported.
 */

import type { Constraints } from "./constraints.ts";

/** Data a parent stores on each child (object.dart `ParentData`). */
export class ParentData {
  /** Called when the child is removed from its parent. */
  detach(): void {}
}

/**
 * Owns a render tree and the list of relayout boundaries that need layout
 * (object.dart `PipelineOwner`, layout only). There is no global owner: the
 * caller creates one and assigns its [rootNode].
 */
export class PipelineOwner {
  private _rootNode: RenderObject | null = null;
  private _nodesNeedingLayout: RenderObject[] = [];
  private readonly _onNeedVisualUpdate: (() => void) | null;

  /** `onNeedVisualUpdate` is called whenever a node is scheduled for layout. */
  constructor(options: { onNeedVisualUpdate?: () => void } = {}) {
    this._onNeedVisualUpdate = options.onNeedVisualUpdate ?? null;
  }

  /** The root of the tree; setting it attaches the new root and detaches the old one. */
  get rootNode(): RenderObject | null {
    return this._rootNode;
  }

  set rootNode(value: RenderObject | null) {
    if (this._rootNode === value) {
      return;
    }
    this._rootNode?.detach();
    this._rootNode = value;
    this._rootNode?.attach(this);
  }

  /** The relayout boundaries waiting for [flushLayout], in scheduling order. */
  get nodesNeedingLayout(): readonly RenderObject[] {
    return this._nodesNeedingLayout;
  }

  /** Asks the embedder for a new frame. */
  requestVisualUpdate(): void {
    this._onNeedVisualUpdate?.();
  }

  /**
   * Adds a relayout boundary to the dirty list. Called by
   * [RenderObject.markNeedsLayout] and [RenderObject.scheduleInitialLayout].
   */
  scheduleLayoutFor(node: RenderObject): void {
    this._nodesNeedingLayout.push(node);
  }

  /**
   * Lays out every dirty relayout boundary, shallowest first, until none
   * remain. A node laid out by an ancestor in the same pass is skipped
   * because it is no longer dirty.
   */
  flushLayout(): void {
    while (this._nodesNeedingLayout.length > 0) {
      const dirtyNodes = this._nodesNeedingLayout;
      this._nodesNeedingLayout = [];
      dirtyNodes.sort((a, b) => a.depth - b.depth);
      for (const node of dirtyNodes) {
        if (node.needsLayout && node.owner === this) {
          node.layoutWithoutResize();
        }
      }
    }
  }
}

/**
 * A node in the render tree, as far as layout is concerned.
 *
 * Subclasses implement [performLayout] (and [performResize] when
 * [sizedByParent]); parents call [layout] on their children with
 * constraints. Changing anything that affects layout calls [markNeedsLayout],
 * which marks this node and its ancestors dirty up to the nearest relayout
 * boundary, which the owner lays out on the next [PipelineOwner.flushLayout].
 */
export abstract class RenderObject {
  /** Data the parent stores on this node, set by the parent's [setupParentData]. */
  parentData: ParentData | null = null;

  /**
   * The Plux node ID this render object renders, if any. Not part of
   * Flutter: it keys [globalRects] for the layout conformance suite.
   */
  debugId: string | null = null;

  private _parent: RenderObject | null = null;
  private _owner: PipelineOwner | null = null;
  private _depth = 0;
  private _needsLayout = true;
  private _isRelayoutBoundary: boolean | null = null;
  private _constraints: Constraints | null = null;

  /** The parent in the render tree, or null for the root or a detached node. */
  get parent(): RenderObject | null {
    return this._parent;
  }

  /** The owner of the tree this node is attached to, or null. */
  get owner(): PipelineOwner | null {
    return this._owner;
  }

  /** Whether this node is attached to an owner. */
  get attached(): boolean {
    return this._owner !== null;
  }

  /** Strictly greater than the parent's depth; the owner lays out shallow nodes first. */
  get depth(): number {
    return this._depth;
  }

  /** Whether this node must be laid out before its size can be used. */
  get needsLayout(): boolean {
    return this._needsLayout;
  }

  /**
   * Whether this node was a relayout boundary in its last layout; null
   * before the first layout or after being dropped by its parent.
   */
  get isRelayoutBoundary(): boolean | null {
    return this._isRelayoutBoundary;
  }

  /** Whether [layout] has been called with constraints. */
  get hasConstraints(): boolean {
    return this._constraints !== null;
  }

  /** The constraints of the most recent [layout]. Throws before the first layout. */
  get constraints(): Constraints {
    if (this._constraints === null) {
      throw new Error(`${this.constructor.name} has not been laid out`);
    }
    return this._constraints;
  }

  /**
   * Whether the constraints are the only input to this node's size. When
   * true, [performResize] computes the size and the node is a relayout
   * boundary.
   */
  get sizedByParent(): boolean {
    return false;
  }

  /** Gives `child` the parent data this node needs; override to use a subclass. */
  setupParentData(child: RenderObject): void {
    if (!(child.parentData instanceof ParentData)) {
      child.parentData = new ParentData();
    }
  }

  /** Calls `visitor` for each direct child. */
  visitChildren(_visitor: (child: RenderObject) => void): void {}

  /** Makes `child` a child of this node. Subclasses with children call this. */
  protected adoptChild(child: RenderObject): void {
    this.setupParentData(child);
    this.markNeedsLayout();
    child._parent = this;
    if (this._owner !== null) {
      child.attach(this._owner);
    }
    this.redepthChild(child);
  }

  /** Removes `child` from this node. Subclasses with children call this. */
  protected dropChild(child: RenderObject): void {
    if (!(child._isRelayoutBoundary ?? true)) {
      child._isRelayoutBoundary = null;
    }
    child.parentData?.detach();
    child.parentData = null;
    child._parent = null;
    if (this.attached) {
      child.detach();
    }
    this.markNeedsLayout();
  }

  /** Raises `child`'s depth above this node's, if needed. */
  protected redepthChild(child: RenderObject): void {
    if (child._depth <= this._depth) {
      child._depth = this._depth + 1;
      child.redepthChildren();
    }
  }

  /** Re-applies [redepthChild] to every child. */
  redepthChildren(): void {
    this.visitChildren((child) => {
      this.redepthChild(child);
    });
  }

  /**
   * Attaches this node to `owner`. A node that was laid out before and is
   * dirty is rescheduled with the new owner. Subclasses attach their children.
   */
  attach(owner: PipelineOwner): void {
    this._owner = owner;
    if (this._needsLayout && this._isRelayoutBoundary !== null) {
      // Never laid out at all: scheduleInitialLayout() handles it.
      this._needsLayout = false;
      this.markNeedsLayout();
    }
  }

  /** Detaches this node from its owner. Subclasses detach their children. */
  detach(): void {
    this._owner = null;
  }

  /**
   * Marks this node dirty. A relayout boundary with an owner is scheduled
   * with the owner; any other node marks its parent instead, so dirtiness
   * propagates up to the nearest relayout boundary.
   */
  markNeedsLayout(): void {
    if (this._needsLayout) {
      return;
    }
    this._needsLayout = true;
    if (this._owner !== null && (this._isRelayoutBoundary ?? false)) {
      this._owner.scheduleLayoutFor(this);
      this._owner.requestVisualUpdate();
    } else if (this._parent !== null) {
      this.markParentNeedsLayout();
    }
  }

  /** Marks this node and its parent dirty, regardless of relayout boundaries. */
  protected markParentNeedsLayout(): void {
    this._needsLayout = true;
    this._parent?.markNeedsLayout();
  }

  /** Marks this node and its parent dirty after [sizedByParent] changed. */
  markNeedsLayoutForSizedByParentChange(): void {
    this.markNeedsLayout();
    this.markParentNeedsLayout();
  }

  /** Schedules the first layout of the root node. Requires an owner. */
  scheduleInitialLayout(): void {
    if (this._owner === null) {
      throw new Error("scheduleInitialLayout needs an attached PipelineOwner");
    }
    this._isRelayoutBoundary = true;
    this._owner.scheduleLayoutFor(this);
  }

  /**
   * Re-runs [performLayout] with the previous constraints. Only the
   * [PipelineOwner] calls this, for dirty relayout boundaries.
   */
  layoutWithoutResize(): void {
    this.performLayout();
    this._needsLayout = false;
  }

  /**
   * Lays out this node under `constraints`.
   *
   * `parentUsesSize` declares that the parent's own layout reads this
   * node's size. The node is a relayout boundary — its relayout never
   * dirties the parent — when the parent does not use its size, when it is
   * [sizedByParent], when the constraints are tight, or when it is the root.
   * A clean node given equal constraints returns at once.
   */
  layout(constraints: Constraints, options: { parentUsesSize?: boolean } = {}): void {
    const parentUsesSize = options.parentUsesSize ?? false;
    this._isRelayoutBoundary = !parentUsesSize || this.sizedByParent || constraints.isTight || this._parent === null;
    if (!this._needsLayout && this._constraints !== null && constraints.equals(this._constraints)) {
      return;
    }
    this._constraints = constraints;
    if (this.sizedByParent) {
      this.performResize();
    }
    this.performLayout();
    this._needsLayout = false;
  }

  /** Computes the size from the constraints alone; called only when [sizedByParent]. */
  protected abstract performResize(): void;

  /** Lays out the children and, unless [sizedByParent], computes the size. */
  protected abstract performLayout(): void;
}
