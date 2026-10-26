// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/chip.dart, filter_chip.dart, input_chip.dart, ink_decoration.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Material 3 chips (`Chip`, `FilterChip`, `InputChip`; all build `RawChip`):
 * `_RenderChip` places an avatar (or the checkmark slot of a selected chip),
 * the label and the delete icon inside the chip padding; the one-pixel outline
 * `Ink` pads is added around it, and the tap target pads the whole.
 */

import { BoxConstraints } from "../constraints.ts";
import { ContainerRenderBox } from "../container.ts";
import { Alignment, EdgeInsets, Offset, Size, TextDirection } from "../geometry.ts";
import { RenderConstrainedBox } from "../proxy-box.ts";
import type { BoxParentData, RenderBox, TextBaseline } from "../render-box.ts";
import { RenderPadding, RenderPositionedBox, setChildOffset } from "../shifted-box.ts";
import { kMinInteractiveDimension, MaterialTapTargetSize, type VisualDensity } from "./theme.ts";

/** `_kChipHeight`. */
const kChipHeight = 32;
/** The padding of `_ChipDefaultsM3`. */
const chipPadding = 8;
/** The width of the outline `Ink` pads the chip by. */
const outlineWidth = 1;

export interface ChipOptions {
  label: RenderBox;
  /** The avatar, shown whether or not the chip is selected. */
  avatar: RenderBox | null;
  /** The delete icon; a chip has one when `onDeleted` is set. */
  deleteIcon: RenderBox | null;
  selected: boolean;
  tapTargetSize: MaterialTapTargetSize;
  density: VisualDensity;
  /** `MediaQuery.textScalerOf(context).scale(14) / 14`. */
  textScale: number;
  textDirection: TextDirection;
}

/** The label padding of `_ChipDefaultsM3`: 8 at text scale 1, 4 from scale 2. */
function labelPaddingFor(textScale: number): number {
  const t = Math.min(1, Math.max(0, textScale - 1));
  return 8 + (4 - 8) * t;
}

/** A square the chip lays an avatar or delete icon out in, centring the icon. */
function slotBox(child: RenderBox): RenderBox {
  return new RenderPositionedBox({ alignment: Alignment.center, child });
}

/** `_RenderChip`. */
class RenderChip extends ContainerRenderBox<RenderBox> {
  private readonly avatar: RenderBox | null;
  private readonly label: RenderBox;
  private readonly deleteIcon: RenderBox | null;
  private readonly labelPadding: number;

  constructor(private readonly o: ChipOptions) {
    super();
    this.avatar = o.avatar === null ? null : slotBox(o.avatar);
    this.label = o.label;
    this.deleteIcon = o.deleteIcon === null ? null : slotBox(o.deleteIcon);
    this.labelPadding = labelPaddingFor(o.textScale);
    for (const child of [this.avatar, this.label, this.deleteIcon]) {
      if (child !== null) {
        this.add(child);
      }
    }
  }

  private get showAvatar(): boolean {
    return this.avatar !== null;
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return (
      2 * chipPadding +
      2 * this.labelPadding +
      (this.avatar?.getMinIntrinsicWidth(height) ?? 0) +
      this.label.getMinIntrinsicWidth(height) +
      (this.deleteIcon?.getMinIntrinsicWidth(height) ?? 0)
    );
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    return (
      2 * chipPadding +
      2 * this.labelPadding +
      (this.avatar?.getMaxIntrinsicWidth(height) ?? 0) +
      this.label.getMaxIntrinsicWidth(height) +
      (this.deleteIcon?.getMaxIntrinsicWidth(height) ?? 0)
    );
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return Math.max(kChipHeight, 2 * chipPadding + this.label.getMinIntrinsicHeight(width));
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this.getMinIntrinsicHeight(width);
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    const b = this.label.getDistanceToActualBaseline(baseline);
    return b === null ? null : b + (this.label.parentData as BoxParentData).offset.dy;
  }

  /** `_computeSizes`: the avatar, label and delete icon widths, the content height and the chip's size. */
  private computeSizes(constraints: BoxConstraints, place: boolean) {
    const content = constraints.loosen();
    const layoutChild = (box: RenderBox, c: BoxConstraints): Size => {
      if (place) {
        box.layout(c, { parentUsesSize: true });
        return box.size;
      }
      return box.getDryLayout(c);
    };
    const rawLabel = this.label.getDryLayout(content);
    const contentSize = Math.max(kChipHeight - 2 * chipPadding, rawLabel.height);
    const tight = BoxConstraints.tightFor({ width: contentSize, height: contentSize });
    if (this.avatar !== null) {
      layoutChild(this.avatar, tight);
    }
    // A selected chip shows the checkmark in the avatar's place; an unselected one without avatar has none.
    const avatarWidth = this.showAvatar || this.o.selected ? contentSize : 0;
    if (this.deleteIcon !== null) {
      layoutChild(this.deleteIcon, tight);
    }
    const deleteWidth = this.deleteIcon === null ? 0 : contentSize;
    const freeSpace = content.maxWidth - avatarWidth - deleteWidth - 2 * this.labelPadding - 2 * chipPadding;
    const maxLabelWidth = Math.max(0, freeSpace);
    const labelConstraints = new BoxConstraints({
      minHeight: rawLabel.height,
      maxHeight: contentSize,
      maxWidth: Number.isFinite(maxLabelWidth) ? maxLabelWidth : rawLabel.width,
    });
    const labelBox = layoutChild(this.label, labelConstraints);
    const labelWidth = labelBox.width + 2 * this.labelPadding;
    const densityDy = this.o.density.baseSizeAdjustmentY / 2;
    const overall = new Size(avatarWidth + labelWidth + deleteWidth, contentSize + densityDy);
    const padded = new Size(overall.width + 2 * chipPadding, overall.height + 2 * chipPadding);
    return {
      size: constraints.constrain(padded),
      contentSize,
      densityDy,
      avatarWidth,
      labelHeight: labelBox.height,
      labelWidth,
      deleteWidth,
    };
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return this.computeSizes(constraints, false).size;
  }

  protected override performLayout(): void {
    const s = this.computeSizes(this.constraints, true);
    this.size = s.size;
    const rtl = this.o.textDirection === TextDirection.rtl;
    const centred = (boxWidth: number, boxHeight: number, x: number): Offset =>
      new Offset(rtl ? x - boxWidth : x, (s.contentSize - boxHeight + s.densityDy) / 2);
    const overallWidth = s.avatarWidth + s.labelWidth + s.deleteWidth;
    let start = rtl ? overallWidth : 0;
    const advance = (width: number): number => {
      const at = start;
      start += rtl ? -width : width;
      return at;
    };
    const origin = new Offset(chipPadding, chipPadding);
    const place = (box: RenderBox, offset: Offset): void => {
      setChildOffset(box, new Offset(origin.dx + offset.dx, origin.dy + offset.dy));
    };
    if (this.avatar !== null) {
      place(this.avatar, centred(this.avatar.size.width, this.avatar.size.height, advance(s.avatarWidth)));
    } else {
      advance(s.avatarWidth);
    }
    const labelOffset = centred(s.labelWidth, s.labelHeight, advance(s.labelWidth));
    place(
      this.label,
      new Offset(labelOffset.dx + this.labelPadding, labelOffset.dy + (s.labelHeight - this.label.size.height) / 2),
    );
    if (this.deleteIcon !== null) {
      place(this.deleteIcon, centred(this.deleteIcon.size.width, this.deleteIcon.size.height, advance(s.deleteWidth)));
    }
  }
}

/** The widget chain `RawChip.build` produces around the label. */
export function buildChip(o: ChipOptions): RenderBox {
  const chip = new RenderPadding({
    padding: EdgeInsets.all(outlineWidth),
    child: new RenderChip(o),
  });
  const padded = o.tapTargetSize === MaterialTapTargetSize.padded;
  const min = padded
    ? new BoxConstraints({
        minWidth: kMinInteractiveDimension + o.density.baseSizeAdjustmentX,
        minHeight: kMinInteractiveDimension + o.density.baseSizeAdjustmentY,
      })
    : new BoxConstraints();
  return new RenderConstrainedBox({
    additionalConstraints: min,
    child: new RenderPositionedBox({ alignment: Alignment.center, widthFactor: 1, heightFactor: 1, child: chip }),
  });
}
