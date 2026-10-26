// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/navigation_bar.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `NavigationBar` and `NavigationDestination` (Material 3 defaults): a row of
 * equal destinations, each an icon over the 64 by 32 indicator and a label the
 * label behaviour shows or hides, placed by `_NavigationDestinationLayoutDelegate`.
 */

import { BoxConstraints } from "../constraints.ts";
import { ContainerRenderBox } from "../container.ts";
import { Axis, CrossAxisAlignment, FlexFit, FlexParentData, MainAxisSize, RenderFlex } from "../flex.ts";
import { Alignment, EdgeInsets, Offset, type Size, type TextDirection } from "../geometry.ts";
import { RenderConstrainedBox } from "../proxy-box.ts";
import type { RenderBox } from "../render-box.ts";
import { RenderPadding, setChildOffset } from "../shifted-box.ts";
import { RenderStack } from "../stack.ts";

export type NavigationLabelBehavior = "alwaysShow" | "alwaysHide" | "onlyShowSelected";

/** `_kIndicatorWidth` and `_kIndicatorHeight`. */
export const kIndicatorWidth = 64;
export const kIndicatorHeight = 32;
/** `_kMaxLabelTextScaleFactor`. */
export const kMaxLabelTextScaleFactor = 1.3;
/** The default `NavigationBarThemeData.height` of Material 3. */
export const kNavigationBarHeight = 80;
/** The default `NavigationBarThemeData.labelPadding` of Material 3: 4 above the label. */
export const kLabelPaddingTop = 4;

/** The icon with the indicator behind it: a stack centred on the larger of the two. */
export function indicatorIcon(icon: RenderBox, width: number, height: number): RenderBox {
  return new RenderStack({
    alignment: Alignment.center,
    children: [new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width, height }) }), icon],
  });
}

/** The label of a destination: padded above, in the text style the caller built. */
export function destinationLabel(text: RenderBox): RenderBox {
  return new RenderPadding({ padding: EdgeInsets.only({ top: kLabelPaddingTop }), child: text });
}

/** `_NavigationDestinationLayoutDelegate` in a `CustomMultiChildLayout`. */
class RenderDestinationLayout extends ContainerRenderBox<RenderBox> {
  constructor(
    private readonly icon: RenderBox,
    private readonly label: RenderBox,
    /** The label animation's value: 1 when the label is shown, 0 when hidden. */
    private readonly shown: number,
  ) {
    super();
    this.add(icon);
    this.add(label);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  protected override performLayout(): void {
    this.size = this.constraints.biggest;
    const loose = BoxConstraints.loose(this.size);
    this.icon.layout(loose, { parentUsesSize: true });
    this.label.layout(loose, { parentUsesSize: true });
    const halfIcon = this.icon.size.height / 2;
    const halfLabel = this.label.size.height / 2;
    const iconY = this.size.height / 2 - (halfIcon + halfLabel * this.shown);
    setChildOffset(this.icon, new Offset(this.size.width / 2 - this.icon.size.width / 2, iconY));
    setChildOffset(
      this.label,
      new Offset(this.size.width / 2 - this.label.size.width / 2, iconY + this.icon.size.height),
    );
  }
}

/** One destination as `_NavigationDestinationBuilder` lays it out: a row holding the expanded layout. */
export function buildDestination(
  icon: RenderBox,
  label: RenderBox,
  shown: number,
  textDirection: TextDirection,
): RenderBox {
  const layout = new RenderDestinationLayout(icon, label, shown);
  const flexible = new FlexParentData();
  flexible.flex = 1;
  flexible.fit = FlexFit.tight;
  layout.parentData = flexible;
  return new RenderFlex({
    direction: Axis.horizontal,
    mainAxisSize: MainAxisSize.max,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection,
    children: [layout],
  });
}

export interface NavigationBarOptions {
  /** The destination rows, already built with [buildDestination]. */
  destinations: RenderBox[];
  height: number;
  textDirection: TextDirection;
}

/** `NavigationBar.build`: a `SizedBox` of the bar's height around a row of expanded destinations. */
export function buildNavigationBar(o: NavigationBarOptions): RenderBox {
  for (const destination of o.destinations) {
    const flexible = new FlexParentData();
    flexible.flex = 1;
    flexible.fit = FlexFit.tight;
    destination.parentData = flexible;
  }
  return new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({ height: o.height }),
    child: new RenderFlex({
      direction: Axis.horizontal,
      mainAxisSize: MainAxisSize.max,
      crossAxisAlignment: CrossAxisAlignment.center,
      textDirection: o.textDirection,
      children: o.destinations,
    }),
  });
}
