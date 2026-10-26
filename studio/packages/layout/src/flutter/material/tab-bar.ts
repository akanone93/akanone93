// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/tabs.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Material 3 `TabBar` (primary) and `Tab`: each tab is a label (text, icon, or
 * both in a column) padded by `kTabLabelPadding`, with the indicator weight
 * below it. A fixed bar divides its width among the tabs; a scrollable bar
 * lays them out in a horizontal single-child scroll view at offset zero,
 * aligned by `TabAlignment`. The indicator and divider only paint, so they add
 * no box.
 */

import { BoxConstraints } from "../constraints.ts";
import {
  Axis,
  CrossAxisAlignment,
  FlexFit,
  FlexParentData,
  MainAxisAlignment,
  MainAxisSize,
  RenderFlex,
} from "../flex.ts";
import {
  Alignment,
  AlignmentDirectional,
  EdgeInsets,
  EdgeInsetsDirectional,
  Offset,
  type Size,
  TextDirection,
} from "../geometry.ts";
import { RenderConstrainedBox } from "../proxy-box.ts";
import type { RenderBox, TextBaseline } from "../render-box.ts";
import { RenderPadding, RenderPositionedBox, RenderShiftedBox, setChildOffset } from "../shifted-box.ts";
import { RenderStack } from "../stack.ts";

/** `_kTabHeight` and `_kTextAndIconTabHeight`. */
export const kTabHeight = 46;
export const kTextAndIconTabHeight = 72;
/** `_kStartOffset`: the start inset of a `TabAlignment.startOffset` scrollable bar. */
const kStartOffset = 52;
/** `TabBar.indicatorWeight`: the space below each tab for the indicator. */
const kIndicatorWeight = 2;
/** `_TabsPrimaryDefaultsM3.iconMargin`. */
const iconMarginBottom = 2;
/** `kTabLabelPadding`'s horizontal inset. */
const labelPaddingHorizontal = 16;
/** `(_kTextAndIconTabHeight - _kTabHeight) / 2`: what a text-only tab adds beside a text-and-icon one. */
const verticalAdjustment = (kTextAndIconTabHeight - kTabHeight) / 2;

export type TabAlignment = "start" | "startOffset" | "center" | "fill";

export interface TabOptions {
  /** The icon, if the tab has one. */
  icon: RenderBox | null;
  /** The label text (`Tab.text`), if the tab has one. */
  text: RenderBox | null;
  /** `Tab.height`. */
  height: number | null;
}

/** `Tab.preferredSize.height`. */
export function tabPreferredHeight(o: { hasIcon: boolean; hasText: boolean; height: number | null }): number {
  if (o.height !== null) {
    return o.height;
  }
  return o.hasIcon && o.hasText ? kTextAndIconTabHeight : kTabHeight;
}

const column = (children: RenderBox[]): RenderBox =>
  new RenderFlex({
    direction: Axis.vertical,
    mainAxisAlignment: MainAxisAlignment.center,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection: TextDirection.ltr,
    children,
  });

/** `Tab.build`: a fixed-height box around the label, centred and shrink-wrapped in width. */
export function buildTab(o: TabOptions): RenderBox {
  let label: RenderBox;
  if (o.icon !== null && o.text !== null) {
    label = column([
      new RenderPadding({ padding: EdgeInsets.only({ bottom: iconMarginBottom }), child: o.icon }),
      o.text,
    ]);
  } else {
    const only = o.icon ?? o.text;
    if (only === null) {
      throw new Error("a Tab needs an icon or a text");
    }
    label = only;
  }
  const height = tabPreferredHeight({ hasIcon: o.icon !== null, hasText: o.text !== null, height: o.height });
  return new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({ height }),
    child: new RenderPositionedBox({ alignment: Alignment.center, widthFactor: 1, child: label }),
  });
}

/** `_RenderSingleChildViewport` of a horizontal `SingleChildScrollView` at scroll offset zero. */
class RenderHorizontalScroll extends RenderShiftedBox {
  constructor(
    child: RenderBox,
    private readonly textDirection: TextDirection,
  ) {
    super(child);
  }

  private innerConstraints(constraints: BoxConstraints): BoxConstraints {
    return new BoxConstraints({
      minHeight: constraints.minHeight,
      maxHeight: constraints.maxHeight,
      minWidth: 0,
      maxWidth: Number.POSITIVE_INFINITY,
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
    // Offset zero on a reversed (right-to-left) axis puts the content's end at the viewport's end.
    const dx = this.textDirection === TextDirection.rtl ? this.size.width - child.size.width : 0;
    setChildOffset(child, new Offset(dx, 0));
  }
}

export interface TabBarOptions {
  /** The built tabs with their `Tab.preferredSize.height`. */
  tabs: { box: RenderBox; preferredHeight: number }[];
  isScrollable: boolean;
  tabAlignment: TabAlignment;
  textDirection: TextDirection;
}

/** `TabBar.build` for a non-empty primary bar of Material 3. */
export function buildTabBar(o: TabBarOptions): RenderBox {
  const fill = !o.isScrollable && o.tabAlignment === "fill";
  const hasTextAndIcon = o.tabs.some((t) => t.preferredHeight === kTextAndIconTabHeight);
  const wrapped = o.tabs.map(({ box, preferredHeight }) => {
    const vertical = preferredHeight === kTabHeight && hasTextAndIcon ? verticalAdjustment : 0;
    const label = new RenderPositionedBox({
      alignment: Alignment.center,
      heightFactor: 1,
      child: new RenderPadding({
        padding: EdgeInsets.symmetric({ horizontal: labelPaddingHorizontal, vertical }),
        child: box,
      }),
    });
    const inked = new RenderPadding({
      padding: EdgeInsets.only({ bottom: kIndicatorWeight }),
      child: new RenderStack({ textDirection: o.textDirection, children: [label] }),
    });
    if (fill) {
      const parentData = new FlexParentData();
      parentData.flex = 1;
      parentData.fit = FlexFit.tight;
      inked.parentData = parentData;
    }
    return inked;
  });
  const bar = new RenderFlex({
    direction: Axis.horizontal,
    mainAxisSize: o.tabAlignment === "fill" ? MainAxisSize.max : MainAxisSize.min,
    mainAxisAlignment: MainAxisAlignment.start,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection: o.textDirection,
    children: wrapped,
  });
  if (!o.isScrollable) {
    return bar;
  }
  const inset = o.tabAlignment === "startOffset" ? EdgeInsetsDirectional.only({ start: kStartOffset }) : null;
  const scroll = new RenderHorizontalScroll(
    inset === null ? bar : new RenderPadding({ padding: inset, textDirection: o.textDirection, child: bar }),
    o.textDirection,
  );
  return new RenderPositionedBox({
    alignment: o.tabAlignment === "center" ? Alignment.center : AlignmentDirectional.centerStart,
    textDirection: o.textDirection,
    heightFactor: 1,
    child: scroll,
  });
}
