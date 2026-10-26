// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/widgets/overflow_bar.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `OverflowBar`: children in a row spaced `spacing` apart, aligned by
 * `alignment`; when they do not fit, one under the other `overflowSpacing`
 * apart, aligned by `overflowAlignment`.
 */

import type { BoxConstraints } from "./constraints.ts";
import { ContainerRenderBox } from "./container.ts";
import { MainAxisAlignment, VerticalDirection } from "./flex.ts";
import { Offset, Size, TextDirection } from "./geometry.ts";
import type { RenderBox, TextBaseline } from "./render-box.ts";
import { setChildOffset } from "./shifted-box.ts";

export type OverflowBarAlignment = "start" | "end" | "center";

export interface OverflowBarOptions {
  children: readonly RenderBox[];
  spacing?: number;
  /** How the row is aligned; null shrink-wraps it and starts at the start edge. */
  alignment?: MainAxisAlignment | null;
  overflowSpacing?: number;
  overflowAlignment?: OverflowBarAlignment;
  overflowDirection?: VerticalDirection;
  textDirection: TextDirection;
}

/** `_RenderOverflowBar`. */
export class RenderOverflowBar extends ContainerRenderBox<RenderBox> {
  private readonly spacing: number;
  private readonly alignment: MainAxisAlignment | null;
  private readonly overflowSpacing: number;
  private readonly overflowAlignment: OverflowBarAlignment;
  private readonly overflowDirection: VerticalDirection;
  private readonly textDirection: TextDirection;

  constructor(o: OverflowBarOptions) {
    super();
    this.spacing = o.spacing ?? 0;
    this.alignment = o.alignment ?? null;
    this.overflowSpacing = o.overflowSpacing ?? 0;
    this.overflowAlignment = o.overflowAlignment ?? "start";
    this.overflowDirection = o.overflowDirection ?? VerticalDirection.down;
    this.textDirection = o.textDirection;
    this.addAll(o.children);
  }

  private each(): RenderBox[] {
    const out: RenderBox[] = [];
    for (let c = this.firstChild; c !== null; c = this.childAfter(c)) {
      out.push(c);
    }
    return out;
  }

  private sumWidths(maxWidth: boolean): number {
    let width = 0;
    for (const child of this.each()) {
      width += maxWidth
        ? child.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY)
        : child.getMinIntrinsicWidth(Number.POSITIVE_INFINITY);
    }
    return width + this.spacing * (this.childCount - 1);
  }

  protected override computeMinIntrinsicWidth(_height: number): number {
    return this.childCount === 0 ? 0 : this.sumWidths(false);
  }

  protected override computeMaxIntrinsicWidth(_height: number): number {
    return this.childCount === 0 ? 0 : this.sumWidths(true);
  }

  private intrinsicHeight(width: number, maxHeight: boolean): number {
    if (this.childCount === 0) {
      return 0;
    }
    const heightOf = (c: RenderBox): number =>
      maxHeight ? c.getMaxIntrinsicHeight(width) : c.getMinIntrinsicHeight(width);
    const children = this.each();
    if (this.sumWidths(false) > width) {
      return children.reduce((h, c) => h + heightOf(c), 0) + this.overflowSpacing * (this.childCount - 1);
    }
    return children.reduce((h, c) => Math.max(h, heightOf(c)), 0);
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return this.intrinsicHeight(width, false);
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this.intrinsicHeight(width, true);
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    return this.defaultComputeDistanceToHighestActualBaseline(baseline);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    if (this.firstChild === null) {
      return constraints.smallest;
    }
    const childConstraints = constraints.loosen();
    let childrenWidth = 0;
    let maxChildHeight = 0;
    let y = 0;
    for (const child of this.each()) {
      const size = child.getDryLayout(childConstraints);
      childrenWidth += size.width;
      maxChildHeight = Math.max(maxChildHeight, size.height);
      y += size.height + this.overflowSpacing;
    }
    const actualWidth = childrenWidth + this.spacing * (this.childCount - 1);
    if (actualWidth > constraints.maxWidth) {
      return constraints.constrain(new Size(constraints.maxWidth, y - this.overflowSpacing));
    }
    return constraints.constrain(
      new Size(this.alignment === null ? actualWidth : constraints.maxWidth, maxChildHeight),
    );
  }

  protected override performLayout(): void {
    const constraints = this.constraints;
    if (this.firstChild === null) {
      this.size = constraints.smallest;
      return;
    }
    const childConstraints = constraints.loosen();
    let childrenWidth = 0;
    let maxChildHeight = 0;
    const children = this.each();
    for (const child of children) {
      child.layout(childConstraints, { parentUsesSize: true });
      childrenWidth += child.size.width;
      maxChildHeight = Math.max(maxChildHeight, child.size.height);
    }
    const rtl = this.textDirection === TextDirection.rtl;
    const actualWidth = childrenWidth + this.spacing * (this.childCount - 1);
    if (actualWidth > constraints.maxWidth) {
      this.layoutOverflowed(children, constraints, rtl);
    } else {
      this.layoutRow(children, constraints, rtl, { childrenWidth, actualWidth, maxChildHeight });
    }
  }

  private layoutOverflowed(children: RenderBox[], constraints: BoxConstraints, rtl: boolean): void {
    const ordered = this.overflowDirection === VerticalDirection.down ? children : [...children].reverse();
    let y = 0;
    for (const child of ordered) {
      let x: number;
      if (this.overflowAlignment === "center") {
        x = (constraints.maxWidth - child.size.width) / 2;
      } else if (this.overflowAlignment === "start") {
        x = rtl ? constraints.maxWidth - child.size.width : 0;
      } else {
        x = rtl ? 0 : constraints.maxWidth - child.size.width;
      }
      setChildOffset(child, new Offset(x, y));
      y += child.size.height + this.overflowSpacing;
    }
    this.size = constraints.constrain(new Size(constraints.maxWidth, y - this.overflowSpacing));
  }

  /** The first child's x and the gap between children for the row's `alignment`. */
  private rowStart(
    width: number,
    firstWidth: number,
    rtl: boolean,
    m: { childrenWidth: number; actualWidth: number },
  ): { x: number; spacing: number } {
    const count = this.childCount;
    const free = width - m.childrenWidth;
    const at = (lead: number): number => (rtl ? width - lead - firstWidth : lead);
    switch (this.alignment) {
      case MainAxisAlignment.center:
        return { x: at((width - m.actualWidth) / 2), spacing: this.spacing };
      case MainAxisAlignment.end:
        return { x: rtl ? m.actualWidth - firstWidth : width - m.actualWidth, spacing: this.spacing };
      case MainAxisAlignment.spaceBetween:
        return { x: at(0), spacing: free / (count - 1) };
      case MainAxisAlignment.spaceAround:
        return { x: at(free / count / 2), spacing: free / count };
      case MainAxisAlignment.spaceEvenly:
        return { x: at(free / (count + 1)), spacing: free / (count + 1) };
      default:
        return { x: at(0), spacing: this.spacing };
    }
  }

  private layoutRow(
    children: RenderBox[],
    constraints: BoxConstraints,
    rtl: boolean,
    m: { childrenWidth: number; actualWidth: number; maxChildHeight: number },
  ): void {
    const first = children[0] as RenderBox;
    this.size = constraints.constrain(
      new Size(this.alignment === null ? m.actualWidth : constraints.maxWidth, m.maxChildHeight),
    );
    const start = this.rowStart(this.size.width, first.size.width, rtl, m);
    let x = start.x;
    children.forEach((child, index) => {
      if (rtl && index > 0) {
        x -= child.size.width + start.spacing;
      }
      setChildOffset(child, new Offset(x, (m.maxChildHeight - child.size.height) / 2));
      if (!rtl) {
        x += child.size.width + start.spacing;
      }
    });
  }
}
