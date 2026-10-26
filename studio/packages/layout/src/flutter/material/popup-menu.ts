// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/popup_menu.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Material 3 popup menu as a static (fully open) state: a panel 112 to 280
 * wide in steps of 56, its items stacked under an 8 inset; each item at least
 * 48 high with a 12 horizontal inset and its child at the start.
 */

import { BoxConstraints } from "../constraints.ts";
import { Axis, CrossAxisAlignment, MainAxisSize, RenderFlex } from "../flex.ts";
import { AlignmentDirectional, EdgeInsets, type TextDirection } from "../geometry.ts";
import { RenderConstrainedBox } from "../proxy-box.ts";
import { RenderIntrinsicWidth } from "../proxy-boxes.ts";
import type { RenderBox } from "../render-box.ts";
import { RenderPadding, RenderPositionedBox } from "../shifted-box.ts";
import { RenderSingleChildScroll } from "../single-child-scroll.ts";
import { kMinInteractiveDimension } from "./theme.ts";

/** `_kMenuWidthStep` and the min and max widths it gives. */
const kMenuWidthStep = 56;
const kMenuMinWidth = 2 * kMenuWidthStep;
const kMenuMaxWidth = 5 * kMenuWidthStep;
/** `PopupMenuDivider.height`'s default. */
export const kPopupDividerHeight = 16;

/** `PopupMenuItem.build`: the child at the start of a box at least `height` high. */
export function buildPopupMenuItem(child: RenderBox, height: number | null, textDirection: TextDirection): RenderBox {
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minHeight: height ?? kMinInteractiveDimension }),
    child: new RenderPadding({
      padding: EdgeInsets.symmetric({ horizontal: 12 }),
      child: new RenderPositionedBox({
        alignment: AlignmentDirectional.centerStart,
        textDirection,
        child,
      }),
    }),
  });
}

/** A `PopupMenuDivider`: a box `height` high. */
export function buildPopupMenuDivider(height: number | null): RenderBox {
  return new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({ height: height ?? kPopupDividerHeight }),
  });
}

/** `_PopupMenu.build` once the route animation has completed. */
export function buildPopupMenu(items: RenderBox[], textDirection: TextDirection): RenderBox {
  const list = new RenderFlex({
    direction: Axis.vertical,
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: CrossAxisAlignment.stretch,
    textDirection,
    children: items,
  });
  const scroll = new RenderSingleChildScroll(
    new RenderPadding({ padding: EdgeInsets.symmetric({ vertical: 8 }), child: list }),
    Axis.vertical,
    textDirection,
  );
  return new RenderPositionedBox({
    alignment: AlignmentDirectional.topEnd,
    textDirection,
    widthFactor: 1,
    heightFactor: 1,
    child: new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({ minWidth: kMenuMinWidth, maxWidth: kMenuMaxWidth }),
      child: new RenderIntrinsicWidth({ stepWidth: kMenuWidthStep, child: scroll }),
    }),
  });
}
