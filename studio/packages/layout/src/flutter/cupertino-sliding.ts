// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/cupertino/sliding_segmented_control.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `CupertinoSlidingSegmentedControl` (STU-005, S4.4): segments separated by
 * one-point separators, each segment an `IndexedStack` of its content in the
 * normal weight and in the highlighted (semi-bold) weight so that selecting a
 * segment never changes its width. Thumb, gestures and animations do not take
 * part in layout and are not ported.
 */

import { BoxConstraints } from "./constraints.ts";
import { ContainerRenderBox } from "./container.ts";
import { RenderUnconstrainedHeight } from "./cupertino.ts";
import { Alignment, EdgeInsets, Offset, Size, TextDirection } from "./geometry.ts";
import { RenderConstrainedBox, RenderDecoratedBox } from "./proxy-box.ts";
import { BoxParentData, type RenderBox } from "./render-box.ts";
import { RenderPadding } from "./shifted-box.ts";
import { RenderIndexedStack } from "./stack.ts";

/** `_kHorizontalItemPadding`. */
const kItemPadding = EdgeInsets.symmetric({ vertical: 2, horizontal: 3 });
/** `_kSeparatorInset`. */
const kSeparatorInset = EdgeInsets.symmetric({ vertical: 5 });
const kSeparatorWidth = 1;
const kSegmentMinPadding = 10;
const kMinSegmentedControlHeight = 28;

/** `_kFontSize`, `_kFontWeight` and `_kHighlightedFontWeight`: the text style a segment's content sets. */
export const kSlidingSegmentFontSize = 13;
export const kSlidingSegmentFontWeight = 500;
export const kSlidingSegmentHighlightedFontWeight = 600;

function sum(values: readonly number[]): number {
  let total = 0;
  for (const value of values) {
    total += value;
  }
  return total;
}

/** `_RenderSegmentedControl` of the sliding control: children alternate segment, separator, segment. */
export class RenderSlidingSegments extends ContainerRenderBox<RenderBox> {
  private readonly _proportionalWidth: boolean;

  constructor(options: { children: readonly RenderBox[]; proportionalWidth: boolean }) {
    super();
    this._proportionalWidth = options.proportionalWidth;
    this.addAll(options.children);
  }

  private get segments(): RenderBox[] {
    return this.getChildrenAsList().filter((_, index) => index % 2 === 0);
  }

  private get segmentCount(): number {
    return Math.floor(this.childCount / 2) + 1;
  }

  private get totalSeparatorWidth(): number {
    return (kSeparatorInset.horizontal + kSeparatorWidth) * Math.floor(this.childCount / 2);
  }

  private maxChildWidth(constraints: BoxConstraints): number {
    let width = (constraints.minWidth - this.totalSeparatorWidth) / this.segmentCount;
    for (const child of this.segments) {
      width = Math.max(width, child.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY) + 2 * kSegmentMinPadding);
    }
    return Math.min(width, (constraints.maxWidth - this.totalSeparatorWidth) / this.segmentCount);
  }

  private maxChildHeight(width: number): number {
    let height = kMinSegmentedControlHeight;
    for (const child of this.segments) {
      height = Math.max(height, child.getMaxIntrinsicHeight(width));
    }
    return height;
  }

  private childWidths(constraints: BoxConstraints): number[] {
    if (!this._proportionalWidth) {
      return new Array<number>(this.segmentCount).fill(this.maxChildWidth(constraints));
    }
    const widths = this.segments.map(
      (child) => child.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY) + 2 * kSegmentMinPadding,
    );
    const total = sum(widths);
    const allowedMax = constraints.maxWidth - this.totalSeparatorWidth;
    const allowedMin = constraints.minWidth - this.totalSeparatorWidth;
    const scale = Math.min(Math.max(total, allowedMin), allowedMax) / total;
    return scale === 1 ? widths : widths.map((width) => width * scale);
  }

  private overallSize(constraints: BoxConstraints): Size {
    const height = this.maxChildHeight(constraints.maxWidth);
    return constraints.constrain(new Size(sum(this.childWidths(constraints)) + this.totalSeparatorWidth, height));
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return this.overallSize(constraints);
  }

  protected override performLayout(): void {
    const widths = this.childWidths(this.constraints);
    const height = this.maxChildHeight(Number.POSITIVE_INFINITY);
    const separatorConstraints = new BoxConstraints({ minHeight: height, maxHeight: height });
    let start = 0;
    this.getChildrenAsList().forEach((child, index) => {
      const width = widths[Math.floor(index / 2)] ?? 0;
      child.layout(index % 2 === 0 ? BoxConstraints.tight(new Size(width, height)) : separatorConstraints, {
        parentUsesSize: true,
      });
      if (child.parentData instanceof BoxParentData) {
        child.parentData.offset = new Offset(start, 0);
      }
      start += child.size.width;
    });
    this.size = this.overallSize(this.constraints);
  }
}

/** One segment: the content in the ambient weight over the content in the highlighted weight (both size the stack). */
export function cupertinoSlidingSegment(options: {
  content: RenderBox;
  highlightedContent: RenderBox;
  textDirection: TextDirection;
}): RenderBox {
  return new RenderIndexedStack({
    alignment: Alignment.center,
    textDirection: options.textDirection,
    children: [options.content, options.highlightedContent],
  });
}

function separator(): RenderBox {
  return new RenderPadding({
    padding: kSeparatorInset,
    child: new RenderDecoratedBox({
      decoration: {},
      child: new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: kSeparatorWidth }) }),
    }),
  });
}

/** `CupertinoSlidingSegmentedControl.build`: in right-to-left the segments and separators are laid out in reverse. */
export function cupertinoSlidingSegmentedControl(options: {
  segments: readonly RenderBox[];
  proportionalWidth: boolean;
  padding?: EdgeInsets | null;
  textDirection: TextDirection;
}): RenderBox {
  const children: RenderBox[] = [];
  options.segments.forEach((segment, index) => {
    if (index > 0) {
      children.push(separator());
    }
    children.push(segment);
  });
  if (options.textDirection === TextDirection.rtl) {
    children.reverse();
  }
  const control = new RenderSlidingSegments({ children, proportionalWidth: options.proportionalWidth });
  const padded = new RenderPadding({ padding: options.padding ?? kItemPadding, child: control });
  const decorated = new RenderDecoratedBox({ decoration: {}, child: padded });
  return new RenderUnconstrainedHeight({ textDirection: options.textDirection, child: decorated });
}
