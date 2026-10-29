// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/rendering/proxy_box.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/** Boxes that take their child's size: proxy, constrained and decorated boxes (layout only). */

import type { BoxConstraints } from "./constraints.ts";
import { Size } from "./geometry.ts";
import { type RenderBox, RenderBoxWithChild, type TextBaseline } from "./render-box.ts";

/**
 * A box that lays its child out under its own constraints and takes the
 * child's size (`RenderProxyBox` with `RenderProxyBoxMixin`). Without a
 * child it takes [computeSizeForNoChild].
 */
export class RenderProxyBox extends RenderBoxWithChild {
  constructor(child: RenderBox | null = null) {
    super();
    this.child = child;
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return this.child?.getMinIntrinsicWidth(height) ?? 0;
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    return this.child?.getMaxIntrinsicWidth(height) ?? 0;
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return this.child?.getMinIntrinsicHeight(width) ?? 0;
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this.child?.getMaxIntrinsicHeight(width) ?? 0;
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    return this.child?.getDistanceToActualBaseline(baseline) ?? super.computeDistanceToActualBaseline(baseline);
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    const result = this.child?.getDryBaseline(constraints, baseline) ?? null;
    return result ?? super.computeDryBaseline(constraints, baseline);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return this.child?.getDryLayout(constraints) ?? this.computeSizeForNoChild(constraints);
  }

  protected override performLayout(): void {
    const child = this.child;
    if (child !== null) {
      child.layout(this.constraints, { parentUsesSize: true });
      this.size = child.size;
    } else {
      this.size = this.computeSizeForNoChild(this.constraints);
    }
  }

  /** The size without a child: the smallest the constraints allow. */
  protected computeSizeForNoChild(constraints: BoxConstraints): Size {
    return constraints.smallest;
  }
}

/**
 * Imposes `additionalConstraints` on its child, within the incoming
 * constraints. Backs `SizedBox` and `ConstrainedBox`.
 */
export class RenderConstrainedBox extends RenderProxyBox {
  private _additionalConstraints: BoxConstraints;

  constructor(options: { additionalConstraints: BoxConstraints; child?: RenderBox | null }) {
    super(options.child ?? null);
    this._additionalConstraints = options.additionalConstraints;
  }

  get additionalConstraints(): BoxConstraints {
    return this._additionalConstraints;
  }

  set additionalConstraints(value: BoxConstraints) {
    if (this._additionalConstraints.equals(value)) {
      return;
    }
    this._additionalConstraints = value;
    this.markNeedsLayout();
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    const additional = this._additionalConstraints;
    if (additional.hasBoundedWidth && additional.hasTightWidth) {
      return additional.minWidth;
    }
    const width = super.computeMinIntrinsicWidth(height);
    if (!additional.hasInfiniteWidth) {
      return additional.constrainWidth(width);
    }
    return width;
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    const additional = this._additionalConstraints;
    if (additional.hasBoundedWidth && additional.hasTightWidth) {
      return additional.minWidth;
    }
    const width = super.computeMaxIntrinsicWidth(height);
    if (!additional.hasInfiniteWidth) {
      return additional.constrainWidth(width);
    }
    return width;
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    const additional = this._additionalConstraints;
    if (additional.hasBoundedHeight && additional.hasTightHeight) {
      return additional.minHeight;
    }
    const height = super.computeMinIntrinsicHeight(width);
    if (!additional.hasInfiniteHeight) {
      return additional.constrainHeight(height);
    }
    return height;
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    const additional = this._additionalConstraints;
    if (additional.hasBoundedHeight && additional.hasTightHeight) {
      return additional.minHeight;
    }
    const height = super.computeMaxIntrinsicHeight(width);
    if (!additional.hasInfiniteHeight) {
      return additional.constrainHeight(height);
    }
    return height;
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    return this.child?.getDryBaseline(this._additionalConstraints.enforce(constraints), baseline) ?? null;
  }

  protected override performLayout(): void {
    const constraints = this.constraints;
    const child = this.child;
    if (child !== null) {
      child.layout(this._additionalConstraints.enforce(constraints), { parentUsesSize: true });
      this.size = child.size;
    } else {
      this.size = this._additionalConstraints.enforce(constraints).constrain(Size.zero);
    }
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return (
      this.child?.getDryLayout(this._additionalConstraints.enforce(constraints)) ??
      this._additionalConstraints.enforce(constraints).constrain(Size.zero)
    );
  }
}

/** Whether a decoration paints behind or in front of the child. */
export enum DecorationPosition {
  background = "background",
  foreground = "foreground",
}

/**
 * A box that paints a decoration around its child. Layout is that of
 * [RenderProxyBox]: the decoration's padding is applied by a separate
 * [RenderPadding], as Flutter's `Container` does. `D` is the decoration
 * value, opaque to layout.
 */
export class RenderDecoratedBox<D = unknown> extends RenderProxyBox {
  /** The decoration; changing it affects painting only. */
  decoration: D;
  /** Where the decoration paints; changing it affects painting only. */
  position: DecorationPosition;

  constructor(options: { decoration: D; position?: DecorationPosition; child?: RenderBox | null }) {
    super(options.child ?? null);
    this.decoration = options.decoration;
    this.position = options.position ?? DecorationPosition.background;
  }
}
