// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/cupertino/date_picker.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `CupertinoDatePicker` (STU-005, S4.4) in its `time`, `date` and
 * `dateAndTime` modes: the wheel columns (`CupertinoPicker`s with the date
 * picker's squeeze) under `_DatePickerLayoutDelegate`, each column's rows
 * positioned by the mode's item-positioning builder. Which rows exist is the
 * caller's: the wheel is laid out at its first row, so a column's rows start
 * at the selected one (a looping column continues round to the rows before
 * it). Scrolling, the selection overlay, the magnifier and validity styling do
 * not take part in layout and are not ported.
 */

import { BoxConstraints } from "./constraints.ts";
import { ContainerRenderBox } from "./container.ts";
import { cupertinoPicker } from "./cupertino.ts";
import { Alignment, type AlignmentGeometry, EdgeInsets, Offset, Size, TextDirection } from "./geometry.ts";
import { RenderConstrainedBox } from "./proxy-box.ts";
import { BoxParentData, type RenderBox } from "./render-box.ts";
import { RenderPadding, RenderPositionedBox } from "./shifted-box.ts";

/** `_kDatePickerPadSize`. */
export const kDatePickerPadSize = 12;
/** `_kPickerWidth`. */
export const kDatePickerWidth = 320;
/** `_kSqueeze`. */
export const kDatePickerSqueeze = 1.25;
/** `_kItemExtent`. */
export const kDatePickerItemExtent = 32;

/** `CustomMultiChildLayout(delegate: _DatePickerLayoutDelegate)`: the columns side by side, the outer two taking the spare width. */
export class RenderDatePickerLayout extends ContainerRenderBox<RenderBox> {
  private readonly _columnWidths: readonly number[];
  private readonly _textDirectionFactor: number;
  private readonly _maxWidth: number;

  constructor(options: {
    children: readonly RenderBox[];
    columnWidths: readonly number[];
    textDirectionFactor: number;
    maxWidth: number;
  }) {
    super();
    this._columnWidths = options.columnWidths;
    this._textDirectionFactor = options.textDirectionFactor;
    this._maxWidth = options.maxWidth;
    this.addAll(options.children);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  protected override performLayout(): void {
    const size = this.constraints.biggest;
    this.size = size;
    const count = this._columnWidths.length;
    const children = this.getChildrenAsList();
    let remaining = this._maxWidth < size.width ? this._maxWidth : size.width;
    let offset = (size.width - remaining) / 2;
    for (const width of this._columnWidths) {
      remaining -= width + kDatePickerPadSize * 2;
    }
    for (let i = 0; i < count; i++) {
      const index = this._textDirectionFactor === 1 ? i : count - i - 1;
      let childWidth = (this._columnWidths[index] ?? 0) + kDatePickerPadSize * 2;
      if (index === 0 || index === count - 1) {
        childWidth += remaining / 2;
      }
      const child = children[index];
      if (child === undefined) {
        continue;
      }
      child.layout(BoxConstraints.tight(new Size(Math.max(0, childWidth), size.height)));
      if (child.parentData instanceof BoxParentData) {
        child.parentData.offset = new Offset(offset, 0);
      }
      offset += childWidth;
    }
  }
}

function flipped(padding: EdgeInsets): EdgeInsets {
  return EdgeInsets.fromLTRB(padding.right, padding.top, padding.left, padding.bottom);
}

function aligned(alignment: AlignmentGeometry, direction: TextDirection, child: RenderBox): RenderBox {
  return new RenderPositionedBox({ alignment, textDirection: direction, child });
}

/** The item-positioning builder of one column of the `time` / `dateAndTime` states. */
function positionDateTimeRow(
  row: RenderBox,
  column: { width: number; first: boolean; last: boolean },
  direction: TextDirection,
): RenderBox {
  const factor = direction === TextDirection.ltr ? 1 : -1;
  const centerLeft = factor === 1 ? Alignment.centerLeft : Alignment.centerRight;
  const centerRight = factor === 1 ? Alignment.centerRight : Alignment.centerLeft;
  let padding = EdgeInsets.only({ right: kDatePickerPadSize });
  if (column.last) {
    padding = flipped(padding);
  }
  if (factor === -1) {
    padding = flipped(padding);
  }
  const child =
    column.first || column.last
      ? new RenderConstrainedBox({
          additionalConstraints: new BoxConstraints({ maxWidth: column.width + kDatePickerPadSize }),
          child: row,
        })
      : row;
  return new RenderPadding({
    padding,
    textDirection: direction,
    child: aligned(column.last ? centerLeft : centerRight, direction, child),
  });
}

/** The item-positioning builder of one column of the `date` state. */
function positionDateRow(
  row: RenderBox,
  column: { width: number; first: boolean; last: boolean },
  direction: TextDirection,
): RenderBox {
  const factor = direction === TextDirection.ltr ? 1 : -1;
  const centerLeft = factor === 1 ? Alignment.centerLeft : Alignment.centerRight;
  const centerRight = factor === 1 ? Alignment.centerRight : Alignment.centerLeft;
  const padding =
    factor === 1 ? EdgeInsets.only({ right: kDatePickerPadSize }) : EdgeInsets.only({ left: kDatePickerPadSize });
  const sized = new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({ width: column.width + kDatePickerPadSize }),
    child: aligned(column.first ? centerLeft : centerRight, direction, row),
  });
  return new RenderPadding({
    padding: column.first ? EdgeInsets.zero : padding,
    textDirection: direction,
    child: aligned(column.last ? centerLeft : centerRight, direction, sized),
  });
}

/**
 * `CupertinoDatePicker.build`. `pickers` are the columns' rows in the order
 * Flutter builds the pickers, `columnWidths` the measured widths in the order
 * of the columns (the two differ in a right-to-left time picker, as in
 * Flutter).
 */
export function cupertinoDatePicker(options: {
  kind: "dateTime" | "date";
  pickers: readonly (readonly RenderBox[])[];
  columnWidths: readonly number[];
  itemExtent: number;
  textDirection: TextDirection;
}): RenderBox {
  const direction = options.textDirection;
  const count = options.columnWidths.length;
  let total = 4 * kDatePickerPadSize;
  const columns = options.pickers.map((rows, index) => {
    const width = options.columnWidths[index] ?? 0;
    total += width + 2 * kDatePickerPadSize;
    const column = { width, first: index === 0, last: index === count - 1 };
    const position = options.kind === "dateTime" ? positionDateTimeRow : positionDateRow;
    return cupertinoPicker({
      rows: rows.map((row) => position(row, column, direction)),
      itemExtent: options.itemExtent,
      squeeze: kDatePickerSqueeze,
      diameterRatio: 1.07,
      textDirection: direction,
    });
  });
  return new RenderDatePickerLayout({
    children: columns,
    columnWidths: options.columnWidths,
    textDirectionFactor: direction === TextDirection.ltr ? 1 : -1,
    maxWidth: Math.max(total, kDatePickerWidth),
  });
}
