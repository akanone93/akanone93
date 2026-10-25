// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/app_bar.dart, packages/flutter/lib/src/widgets/navigation_toolbar.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `AppBar` (Material 3 defaults): the toolbar container, `NavigationToolbar`'s
 * leading/middle/trailing layout, the title box and the optional bottom.
 */

import { BoxConstraints } from "../constraints.ts";
import { ContainerRenderBox } from "../container.ts";
import {
  Axis,
  CrossAxisAlignment,
  FlexFit,
  FlexParentData,
  MainAxisAlignment,
  MainAxisSize,
  RenderFlex,
} from "../flex.ts";
import { Alignment, EdgeInsets, Offset, Size, TextDirection } from "../geometry.ts";
import { RenderConstrainedBox } from "../proxy-box.ts";
import type { RenderBox } from "../render-box.ts";
import {
  RenderAligningShiftedBox,
  RenderPadding,
  RenderPositionedBox,
  RenderShiftedBox,
  setChildOffset,
} from "../shifted-box.ts";
import { kToolbarHeight } from "./theme.ts";

/** `NavigationToolbar.kMiddleSpacing`. */
export const kMiddleSpacing = 16;
/** The width of the leading slot (`_kLeadingWidth`). */
export const kLeadingWidth = kToolbarHeight;
/** `_kMaxTitleTextScaleFactor`: the title's text scale is clamped to this. */
export const kMaxTitleTextScaleFactor = 1.34;

/** `_RenderAppBarTitleBox`: the title may be taller than the bar, centred in it. */
class RenderAppBarTitleBox extends RenderAligningShiftedBox {
  constructor(textDirection: TextDirection, child: RenderBox) {
    super({ alignment: Alignment.center, textDirection, child });
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child as RenderBox;
    const inner = new BoxConstraints({
      minWidth: constraints.minWidth,
      maxWidth: constraints.maxWidth,
      minHeight: constraints.minHeight,
    });
    return constraints.constrain(child.getDryLayout(inner));
  }

  protected override performLayout(): void {
    const child = this.child as RenderBox;
    const c = this.constraints;
    child.layout(new BoxConstraints({ minWidth: c.minWidth, maxWidth: c.maxWidth, minHeight: c.minHeight }), {
      parentUsesSize: true,
    });
    this.size = c.constrain(child.size);
    this.alignChild();
  }
}

type ToolbarSlot = "leading" | "middle" | "trailing";

/** `_ToolbarLayout` of `NavigationToolbar`. */
class RenderNavigationToolbar extends ContainerRenderBox<RenderBox> {
  private readonly slots = new Map<ToolbarSlot, RenderBox>();

  constructor(
    leading: RenderBox | null,
    middle: RenderBox | null,
    trailing: RenderBox | null,
    private readonly centerMiddle: boolean,
    private readonly middleSpacing: number,
    private readonly textDirection: TextDirection,
  ) {
    super();
    for (const [slot, box] of [
      ["leading", leading],
      ["middle", middle],
      ["trailing", trailing],
    ] as const) {
      if (box !== null) {
        this.slots.set(slot, box);
        this.add(box);
      }
    }
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a line-for-line port of NavigationToolbar's _ToolbarLayout.performLayout
  protected override performLayout(): void {
    const size = this.constraints.biggest;
    this.size = size;
    const rtl = this.textDirection === TextDirection.rtl;
    let leadingWidth = 0;
    let trailingWidth = 0;
    const leading = this.slots.get("leading");
    if (leading !== undefined) {
      leading.layout(new BoxConstraints({ maxWidth: size.width, minHeight: size.height, maxHeight: size.height }), {
        parentUsesSize: true,
      });
      leadingWidth = leading.size.width;
      setChildOffset(leading, new Offset(rtl ? size.width - leadingWidth : 0, 0));
    }
    const trailing = this.slots.get("trailing");
    if (trailing !== undefined) {
      trailing.layout(BoxConstraints.loose(size), { parentUsesSize: true });
      trailingWidth = trailing.size.width;
      setChildOffset(
        trailing,
        new Offset(rtl ? 0 : size.width - trailingWidth, (size.height - trailing.size.height) / 2),
      );
    }
    const middle = this.slots.get("middle");
    if (middle !== undefined) {
      const maxWidth = Math.max(size.width - leadingWidth - trailingWidth - this.middleSpacing * 2, 0);
      middle.layout(new BoxConstraints({ maxWidth, maxHeight: size.height }), { parentUsesSize: true });
      const margin = leadingWidth + this.middleSpacing;
      let start = margin;
      if (this.centerMiddle) {
        start = (size.width - middle.size.width) / 2;
        if (start + middle.size.width > size.width - trailingWidth) {
          start = size.width - trailingWidth - middle.size.width - this.middleSpacing;
        } else if (start < margin) {
          start = margin;
        }
      }
      setChildOffset(
        middle,
        new Offset(rtl ? size.width - middle.size.width - start : start, (size.height - middle.size.height) / 2),
      );
    }
  }
}

/** `_ToolbarContainerLayout` in a `CustomSingleChildLayout`. */
class RenderToolbarContainer extends RenderShiftedBox {
  constructor(
    private readonly toolbarHeight: number,
    child: RenderBox,
  ) {
    super(child);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.constrain(new Size(constraints.maxWidth, this.toolbarHeight));
  }

  protected override performLayout(): void {
    const c = this.constraints;
    this.size = c.constrain(new Size(c.maxWidth, this.toolbarHeight));
    const child = this.child as RenderBox;
    child.layout(c.tighten({ height: this.toolbarHeight }), { parentUsesSize: true });
    setChildOffset(child, new Offset(0, this.size.height - child.size.height));
  }
}

export interface AppBarOptions {
  /** The leading widget, already built; an `IconButton` is centred in the leading slot. */
  leading: RenderBox | null;
  leadingIsIconButton: boolean;
  title: RenderBox | null;
  actions: RenderBox[];
  bottom: RenderBox | null;
  toolbarHeight: number;
  centerTitle: boolean;
  titleSpacing: number;
  leadingWidth: number;
  textDirection: TextDirection;
}

/** The height `AppBar.preferredSize` gives: the toolbar plus the bottom. */
export function appBarPreferredHeight(toolbarHeight: number | undefined, bottomHeight: number): number {
  return (toolbarHeight ?? kToolbarHeight) + bottomHeight;
}

/** `AppBar.build`, from the leading slot to the `Align` at the top. */
export function buildAppBar(o: AppBarOptions): RenderBox {
  let leading: RenderBox | null = o.leading;
  if (leading !== null) {
    const inner = o.leadingIsIconButton
      ? new RenderPositionedBox({ alignment: Alignment.center, child: leading })
      : leading;
    leading = new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: o.leadingWidth }),
      child: inner,
    });
  }
  const title = o.title === null ? null : new RenderAppBarTitleBox(o.textDirection, o.title);
  const actions =
    o.actions.length === 0
      ? null
      : new RenderPadding({
          padding: EdgeInsets.zero,
          child: new RenderFlex({
            direction: Axis.horizontal,
            mainAxisAlignment: MainAxisAlignment.end,
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.center,
            textDirection: o.textDirection,
            children: o.actions,
          }),
        });
  const toolbar = new RenderNavigationToolbar(leading, title, actions, o.centerTitle, o.titleSpacing, o.textDirection);
  let appBar: RenderBox = new RenderToolbarContainer(o.toolbarHeight, toolbar);
  if (o.bottom !== null) {
    const flexible = new FlexParentData();
    flexible.flex = 1;
    flexible.fit = FlexFit.loose;
    const limited = new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({ maxHeight: o.toolbarHeight }),
      child: appBar,
    });
    limited.parentData = flexible;
    appBar = new RenderFlex({
      direction: Axis.vertical,
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      textDirection: o.textDirection,
      children: [limited, o.bottom],
    });
  }
  return new RenderPositionedBox({
    alignment: Alignment.topCenter,
    textDirection: o.textDirection,
    child: new RenderPadding({ padding: EdgeInsets.zero, child: appBar }),
  });
}
