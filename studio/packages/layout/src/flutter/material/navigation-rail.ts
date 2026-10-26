// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/navigation_rail.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `NavigationRail` (Material 3 defaults, not extended): destinations stacked in
 * a column, each an icon over the 56 by 32 indicator, with the label hidden,
 * shown only when selected, or always shown.
 */

import { BoxConstraints } from "../constraints.ts";
import { Axis, CrossAxisAlignment, FlexFit, FlexParentData, MainAxisSize, RenderFlex } from "../flex.ts";
import { Alignment, AlignmentDirectional, EdgeInsets, type TextDirection } from "../geometry.ts";
import { RenderConstrainedBox } from "../proxy-box.ts";
import type { RenderBox } from "../render-box.ts";
import { RenderPadding, RenderPositionedBox } from "../shifted-box.ts";
import { RenderStack } from "../stack.ts";

export type RailLabelType = "none" | "selected" | "all";

/** `_kCircularIndicatorDiameter`: the rail's indicator is 56 wide (`NavigationIndicator.height` is 32). */
export const kRailIndicatorWidth = 56;
export const kRailIndicatorHeight = 32;
const horizontalDestinationPadding = 8;
const verticalIconLabelSpacing = 4;
const verticalDestinationSpacing = 12;

/** The default `NavigationRailThemeData.minWidth` and `groupAlignment` of Material 3. */
export const kRailMinWidth = 80;
export const kRailGroupAlignment = -1;
/** The default `NavigationRailThemeData.minExtendedWidth`. */
export const kRailMinExtendedWidth = 256;

const sizedHeight = (height: number): RenderBox =>
  new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ height }) });

function column(children: RenderBox[], mainAxisSize: MainAxisSize, textDirection: TextDirection): RenderBox {
  return new RenderFlex({
    direction: Axis.vertical,
    mainAxisSize,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection,
    children,
  });
}

/** `_AddIndicator`: the icon over the indicator. */
function withIndicator(icon: RenderBox, textDirection: TextDirection): RenderBox {
  return new RenderStack({
    alignment: Alignment.center,
    textDirection,
    children: [
      new RenderConstrainedBox({
        additionalConstraints: BoxConstraints.tightFor({ width: kRailIndicatorWidth, height: kRailIndicatorHeight }),
      }),
      icon,
    ],
  });
}

export interface RailDestinationOptions {
  icon: RenderBox;
  label: RenderBox;
  selected: boolean;
  labelType: RailLabelType;
  minWidth: number;
  /** The destination's minimum width once the rail is extended (`minExtendedWidth`); null when it is not. */
  extendedWidth?: number | null;
  textDirection: TextDirection;
}

/** `_RailDestinationState.build` for an extended rail: the icon part and the label beside it. */
function extendedDestination(o: RailDestinationOptions, iconPart: RenderBox, minExtendedWidth: number): RenderBox {
  const label = new RenderPositionedBox({
    alignment: AlignmentDirectional.centerStart,
    textDirection: o.textDirection,
    heightFactor: 1,
    widthFactor: 1,
    child: o.label,
  });
  const flexible = new FlexParentData();
  flexible.flex = 1;
  flexible.fit = FlexFit.loose;
  label.parentData = flexible;
  const row = new RenderFlex({
    direction: Axis.horizontal,
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection: o.textDirection,
    children: [
      iconPart,
      label,
      new RenderConstrainedBox({
        additionalConstraints: BoxConstraints.tightFor({ width: horizontalDestinationPadding }),
      }),
    ],
  });
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minWidth: minExtendedWidth }),
    child: row,
  });
}

/** `_RailDestinationState.build`: the content of one destination. */
export function buildRailDestination(o: RailDestinationOptions): RenderBox {
  const indicated = withIndicator(o.icon, o.textDirection);
  if (o.labelType === "none") {
    const spacing = verticalDestinationSpacing / 2;
    const iconPart = column(
      [
        sizedHeight(spacing),
        new RenderConstrainedBox({
          additionalConstraints: BoxConstraints.tightFor({ width: o.minWidth }),
          child: new RenderPositionedBox({ alignment: Alignment.center, child: indicated }),
        }),
        sizedHeight(spacing),
      ],
      MainAxisSize.max,
      o.textDirection,
    );
    if (o.extendedWidth !== undefined && o.extendedWidth !== null) {
      return new RenderPadding({
        padding: EdgeInsets.all(0),
        child: extendedDestination(o, iconPart, o.extendedWidth),
      });
    }
    return new RenderPadding({
      padding: EdgeInsets.all(0),
      child: new RenderStack({ textDirection: o.textDirection, children: [iconPart] }),
    });
  }
  const appearing = o.labelType === "selected" ? (o.selected ? 1 : 0) : 1;
  const label =
    o.labelType === "selected"
      ? new RenderPositionedBox({
          alignment: Alignment.topCenter,
          heightFactor: appearing,
          widthFactor: 1,
          child: o.label,
        })
      : o.label;
  const body = column(
    [
      sizedHeight(0),
      indicated,
      sizedHeight(o.labelType === "selected" ? verticalIconLabelSpacing * appearing : verticalIconLabelSpacing),
      label,
      sizedHeight(verticalDestinationSpacing),
    ],
    MainAxisSize.min,
    o.textDirection,
  );
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minWidth: o.minWidth, minHeight: 0 }),
    child: new RenderPadding({
      padding: EdgeInsets.symmetric({ horizontal: horizontalDestinationPadding }),
      child: body,
    }),
  });
}

export interface NavigationRailOptions {
  destinations: RenderBox[];
  /** `NavigationRail.leading`, above the destinations. */
  leading?: RenderBox | null;
  /** `NavigationRail.trailing`, below the destinations. */
  trailing?: RenderBox | null;
  groupAlignment: number;
  textDirection: TextDirection;
}

/** `NavigationRail.build`: a spacer, then the destinations aligned by `groupAlignment` in the rest. */
export function buildNavigationRail(o: NavigationRailOptions): RenderBox {
  const group = column([...o.destinations, ...(o.trailing ? [o.trailing] : [])], MainAxisSize.min, o.textDirection);
  const aligned = new RenderPositionedBox({ alignment: new Alignment(0, o.groupAlignment), child: group });
  const flexible = new FlexParentData();
  flexible.flex = 1;
  flexible.fit = FlexFit.loose;
  aligned.parentData = flexible;
  const top = o.leading ? [o.leading, sizedHeight(8)] : [];
  return column([sizedHeight(8), ...top, aligned], MainAxisSize.max, o.textDirection);
}
