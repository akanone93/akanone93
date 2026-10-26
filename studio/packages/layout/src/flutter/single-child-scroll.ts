// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/widgets/single_child_scroll_view.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `SingleChildScrollView` at scroll offset zero: the child gets an unbounded
 * main axis, the viewport is as large as the child within its constraints, and
 * the child starts at the viewport's leading edge (the right edge of a
 * horizontal view in a right-to-left context).
 */

import { BoxConstraints } from "./constraints.ts";
import { Axis } from "./flex.ts";
import { Offset, type Size, TextDirection } from "./geometry.ts";
import type { RenderBox, TextBaseline } from "./render-box.ts";
import { RenderShiftedBox, setChildOffset } from "./shifted-box.ts";

/** `_RenderSingleChildViewport`. */
export class RenderSingleChildScroll extends RenderShiftedBox {
  constructor(
    child: RenderBox,
    private readonly axis: Axis,
    private readonly textDirection: TextDirection,
  ) {
    super(child);
  }

  private innerConstraints(constraints: BoxConstraints): BoxConstraints {
    if (this.axis === Axis.horizontal) {
      return new BoxConstraints({
        minHeight: constraints.minHeight,
        maxHeight: constraints.maxHeight,
        minWidth: 0,
        maxWidth: Number.POSITIVE_INFINITY,
      });
    }
    return new BoxConstraints({
      minWidth: constraints.minWidth,
      maxWidth: constraints.maxWidth,
      minHeight: 0,
      maxHeight: Number.POSITIVE_INFINITY,
    });
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child as RenderBox;
    return constraints.constrain(child.getDryLayout(this.innerConstraints(constraints)));
  }

  protected override computeDryBaseline(constraints: BoxConstraints, baseline: TextBaseline): number | null {
    return (this.child as RenderBox).getDryBaseline(this.innerConstraints(constraints), baseline);
  }

  protected override performLayout(): void {
    const child = this.child as RenderBox;
    child.layout(this.innerConstraints(this.constraints), { parentUsesSize: true });
    this.size = this.constraints.constrain(child.size);
    // Offset zero on a reversed (right-to-left) horizontal axis puts the content's end at the viewport's end.
    const reversed = this.axis === Axis.horizontal && this.textDirection === TextDirection.rtl;
    setChildOffset(child, new Offset(reversed ? this.size.width - child.size.width : 0, 0));
  }
}
