// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/bottom_sheet.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Material 3 modal `BottomSheet` as a static (fully shown) state: the sheet is
 * at most 640 wide and centred, its content sits below the 48 high drag-handle
 * area when shown, and the modal layout gives it the route's full width, at
 * most 9/16 of the height unless scroll-controlled, at the bottom.
 */

import { BoxConstraints } from "../constraints.ts";
import { Alignment, EdgeInsets, Offset, type Size, type TextDirection } from "../geometry.ts";
import { RenderConstrainedBox } from "../proxy-box.ts";
import type { RenderBox } from "../render-box.ts";
import { RenderPadding, RenderPositionedBox, RenderShiftedBox, setChildOffset } from "../shifted-box.ts";
import { RenderStack } from "../stack.ts";
import { kMinInteractiveDimension } from "./theme.ts";

/** `_BottomSheetDefaultsM3.constraints`. */
const kSheetMaxWidth = 640;
/** `scrollControlDisabledMaxHeightRatio`'s default. */
export const kScrollControlDisabledMaxHeightRatio = 9 / 16;

/** `_RenderBottomSheetLayoutWithSizeListener` at animation value 1. */
class RenderModalSheetLayout extends RenderShiftedBox {
  constructor(
    child: RenderBox,
    private readonly isScrollControlled: boolean,
    private readonly maxHeightRatio: number,
  ) {
    super(child);
  }

  protected override computeMinIntrinsicWidth(): number {
    return 0;
  }

  protected override computeMaxIntrinsicWidth(): number {
    return 0;
  }

  protected override computeMinIntrinsicHeight(): number {
    return 0;
  }

  protected override computeMaxIntrinsicHeight(): number {
    return 0;
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  private childConstraints(constraints: BoxConstraints): BoxConstraints {
    return new BoxConstraints({
      minWidth: constraints.maxWidth,
      maxWidth: constraints.maxWidth,
      maxHeight: this.isScrollControlled ? constraints.maxHeight : constraints.maxHeight * this.maxHeightRatio,
    });
  }

  protected override performLayout(): void {
    this.size = this.constraints.biggest;
    const child = this.child as RenderBox;
    child.layout(this.childConstraints(this.constraints), { parentUsesSize: true });
    setChildOffset(child, new Offset(0, this.size.height - child.size.height));
  }
}

export interface BottomSheetOptions {
  child: RenderBox;
  showDragHandle: boolean;
  isScrollControlled: boolean;
  textDirection: TextDirection;
}

/** `BottomSheet.build`: the sheet, at most 640 wide and centred, as high as its content. */
export function buildBottomSheet(o: BottomSheetOptions): RenderBox {
  const content = o.showDragHandle
    ? new RenderStack({
        alignment: Alignment.topCenter,
        textDirection: o.textDirection,
        children: [
          new RenderConstrainedBox({
            additionalConstraints: BoxConstraints.tightFor({
              width: kMinInteractiveDimension,
              height: kMinInteractiveDimension,
            }),
          }),
          new RenderPadding({ padding: EdgeInsets.only({ top: kMinInteractiveDimension }), child: o.child }),
        ],
      })
    : o.child;
  return new RenderPositionedBox({
    alignment: Alignment.bottomCenter,
    heightFactor: 1,
    child: new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({ maxWidth: kSheetMaxWidth }),
      child: content,
    }),
  });
}

/** `_ModalBottomSheet.build` around `BottomSheet.build`, settled. */
export function buildModalBottomSheet(o: BottomSheetOptions): RenderBox {
  return new RenderModalSheetLayout(buildBottomSheet(o), o.isScrollControlled, kScrollControlDisabledMaxHeightRatio);
}
