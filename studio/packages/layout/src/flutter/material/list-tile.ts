// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/list_tile.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `ListTile` (Material 3 defaults): `_RenderListTile`, which places leading,
 * title, subtitle and trailing, inside the content padding `SafeArea` applies.
 */

import { BoxConstraints } from "../constraints.ts";
import { EdgeInsetsDirectional, Offset, Size, TextDirection } from "../geometry.ts";
import { BoxParentData, type RenderBox, TextBaseline } from "../render-box.ts";
import { ContainerRenderBox } from "../container.ts";
import { RenderPadding, setChildOffset } from "../shifted-box.ts";
import type { VisualDensity } from "./theme.ts";

/** `ListTileTitleAlignment`. */
export enum ListTileTitleAlignment {
  threeLine = "threeLine",
  titleHeight = "titleHeight",
  top = "top",
  center = "center",
  bottom = "bottom",
}

export interface ListTileOptions {
  leading: RenderBox | null;
  title: RenderBox;
  subtitle: RenderBox | null;
  trailing: RenderBox | null;
  isDense: boolean;
  isThreeLine: boolean;
  visualDensity: VisualDensity;
  textDirection: TextDirection;
  titleAlignment?: ListTileTitleAlignment;
  minVerticalPadding?: number;
  minLeadingWidth?: number;
  horizontalTitleGap?: number;
  minTileHeight?: number | null;
}

type Slot = "leading" | "title" | "subtitle" | "trailing";

/** `_RenderListTile`. */
export class RenderListTile extends ContainerRenderBox<RenderBox> {
  private readonly slots = new Map<Slot, RenderBox>();

  constructor(private readonly o: ListTileOptions) {
    super();
    for (const [slot, box] of [
      ["leading", o.leading],
      ["title", o.title],
      ["subtitle", o.subtitle],
      ["trailing", o.trailing],
    ] as const) {
      if (box !== null) {
        this.slots.set(slot, box);
        this.add(box);
      }
    }
  }

  private get gap(): number {
    return (this.o.horizontalTitleGap ?? 16) + this.o.visualDensity.horizontal * 2;
  }

  private get minVerticalPadding(): number {
    return this.o.minVerticalPadding ?? 8;
  }

  private get minLeadingWidth(): number {
    return this.o.minLeadingWidth ?? 24;
  }

  private get targetTileHeight(): number {
    if (this.o.minTileHeight !== undefined && this.o.minTileHeight !== null) {
      return this.o.minTileHeight;
    }
    const dense = this.o.isDense;
    const subtitle = this.slots.has("subtitle");
    const base = this.o.isThreeLine ? (dense ? 76 : 88) : subtitle ? (dense ? 64 : 72) : dense ? 48 : 56;
    return this.o.visualDensity.baseSizeAdjustmentY + base;
  }

  private yOffsetFor(childHeight: number, tileHeight: number, isLeading: boolean): number {
    const a = this.o.titleAlignment ?? ListTileTitleAlignment.threeLine;
    const resolved =
      a === ListTileTitleAlignment.threeLine
        ? this.o.isThreeLine
          ? ListTileTitleAlignment.top
          : ListTileTitleAlignment.center
        : a;
    switch (resolved) {
      case ListTileTitleAlignment.titleHeight:
        if (tileHeight > 72) {
          return 16;
        }
        return isLeading ? Math.min((tileHeight - childHeight) / 2, 16) : (tileHeight - childHeight) / 2;
      case ListTileTitleAlignment.top:
        return this.minVerticalPadding;
      case ListTileTitleAlignment.bottom:
        return tileHeight - childHeight - this.minVerticalPadding;
      default:
        return (tileHeight - childHeight) / 2;
    }
  }

  private layoutSizes(constraints: BoxConstraints, place: boolean): Size {
    const loose = constraints.loosen();
    const tileWidth = loose.maxWidth;
    const iconConstraints = loose.enforce(
      new BoxConstraints({ maxHeight: (this.o.isDense ? 48 : 56) + this.o.visualDensity.baseSizeAdjustmentY }),
    );
    const size = (box: RenderBox, c: BoxConstraints): Size => {
      if (place) {
        box.layout(c, { parentUsesSize: true });
        return box.size;
      }
      return box.getDryLayout(c);
    };
    const baseline = (box: RenderBox, c: BoxConstraints, b: TextBaseline): number | null =>
      place ? box.getDistanceToBaseline(b, { onlyReal: true }) : box.getDryBaseline(c, b);
    const leading = this.slots.get("leading") ?? null;
    const trailing = this.slots.get("trailing") ?? null;
    const subtitle = this.slots.get("subtitle") ?? null;
    const title = this.slots.get("title") as RenderBox;
    const leadingSize = leading === null ? null : size(leading, iconConstraints);
    const trailingSize = trailing === null ? null : size(trailing, iconConstraints);
    const titleStart = leadingSize === null ? 0 : Math.max(this.minLeadingWidth, leadingSize.width) + this.gap;
    const adjustedTrailingWidth = trailingSize === null ? 0 : Math.max(trailingSize.width + this.gap, 32);
    const textConstraints = loose.tighten({ width: tileWidth - titleStart - adjustedTrailingWidth });
    const titleHeight = size(title, textConstraints).height;
    const isLtr = this.o.textDirection === TextDirection.ltr;
    const dense = this.o.isDense;
    const target = this.targetTileHeight;
    let titleY: number;
    let tileHeight: number;
    if (subtitle === null) {
      tileHeight = Math.max(target, titleHeight + 2 * this.minVerticalPadding);
      titleY = (tileHeight - titleHeight) / 2;
    } else {
      const subtitleHeight = size(subtitle, textConstraints).height;
      const titleBaseline = baseline(title, textConstraints, TextBaseline.alphabetic) ?? titleHeight;
      const subtitleBaseline = baseline(subtitle, textConstraints, TextBaseline.alphabetic) ?? subtitleHeight;
      const targetTitleY = (this.o.isThreeLine ? (dense ? 22 : 28) : dense ? 28 : 32) - titleBaseline;
      const targetSubtitleY =
        (this.o.isThreeLine ? (dense ? 42 : 48) : dense ? 48 : 52) +
        this.o.visualDensity.vertical * 2 -
        subtitleBaseline;
      const halfOverlap = Math.max(targetTitleY + titleHeight - targetSubtitleY, 0) / 2;
      const idealTitleY = targetTitleY - halfOverlap;
      const idealSubtitleY = targetSubtitleY + halfOverlap;
      const compact =
        idealTitleY < this.minVerticalPadding || idealSubtitleY + subtitleHeight + this.minVerticalPadding > target;
      if (place) {
        setChildOffset(
          subtitle,
          new Offset(
            isLtr ? titleStart : adjustedTrailingWidth,
            compact ? this.minVerticalPadding + titleHeight : idealSubtitleY,
          ),
        );
      }
      tileHeight = compact ? 2 * this.minVerticalPadding + titleHeight + subtitleHeight : target;
      titleY = compact ? this.minVerticalPadding : idealTitleY;
    }
    if (place) {
      setChildOffset(title, new Offset(isLtr ? titleStart : adjustedTrailingWidth, titleY));
      if (leading !== null && leadingSize !== null) {
        setChildOffset(
          leading,
          new Offset(isLtr ? 0 : tileWidth - leadingSize.width, this.yOffsetFor(leadingSize.height, tileHeight, true)),
        );
      }
      if (trailing !== null && trailingSize !== null) {
        setChildOffset(
          trailing,
          new Offset(isLtr ? tileWidth - trailingSize.width : 0, this.yOffsetFor(trailingSize.height, tileHeight, false)),
        );
      }
    }
    return new Size(tileWidth, tileHeight);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.constrain(this.layoutSizes(constraints, false));
  }

  protected override performLayout(): void {
    this.size = this.constraints.constrain(this.layoutSizes(this.constraints, true));
  }

  protected override computeDistanceToActualBaseline(baseline: TextBaseline): number | null {
    const title = this.slots.get("title") as RenderBox;
    const b = title.getDistanceToActualBaseline(baseline);
    return b === null ? null : b + (title.parentData as BoxParentData).offset.dy;
  }
}

/** The tile inside the content padding `SafeArea` applies (`ListTile.build`). */
export function buildListTile(o: ListTileOptions): RenderBox {
  return new RenderPadding({
    padding: EdgeInsetsDirectional.only({ start: 16, end: 24 }),
    textDirection: o.textDirection,
    child: new RenderListTile(o),
  });
}
