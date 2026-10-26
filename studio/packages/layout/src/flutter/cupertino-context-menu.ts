// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/cupertino/context_menu.dart and context_menu_action.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `CupertinoContextMenu` (STU-005, S4.4) as the static canvas state, which is
 * the closed menu: the widget builds only its child (inside a mouse region, a
 * listener and a visibility wrapper, none of which changes layout) until a
 * long press opens the preview route, and the actions take no part in the
 * layout of the screen. `CupertinoContextMenuAction` is the row the open
 * sheet shows.
 */

import { BoxConstraints } from "./constraints.ts";
import { Axis, FlexFit, FlexParentData, MainAxisAlignment, RenderFlex } from "./flex.ts";
import { EdgeInsets, type TextDirection } from "./geometry.ts";
import { RenderConstrainedBox, RenderDecoratedBox, RenderProxyBox } from "./proxy-box.ts";
import type { RenderBox } from "./render-box.ts";
import { RenderPadding } from "./shifted-box.ts";

/** `_kButtonHeight`. */
export const kContextMenuActionHeight = 43;
/** The label style of an action (`_kActionSheetActionStyle`): 16 pt; a default action is semi-bold. */
export const cupertinoContextMenuActionFontSize = 16;

/** `CupertinoContextMenu.build` while closed: the child, unchanged in size. */
export function cupertinoContextMenu(child: RenderBox): RenderBox {
  return new RenderProxyBox(child);
}

/** `_CupertinoContextMenuActionState.build`: a 43-pt minimum row, its label flexible between 15.5 and 17.5 pt of padding. */
export function cupertinoContextMenuAction(options: { child: RenderBox; textDirection: TextDirection }): RenderBox {
  const parentData = new FlexParentData();
  parentData.flex = 1;
  parentData.fit = FlexFit.loose;
  options.child.parentData = parentData;
  const row = new RenderFlex({
    direction: Axis.horizontal,
    mainAxisAlignment: MainAxisAlignment.spaceBetween,
    textDirection: options.textDirection,
    children: [options.child],
  });
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minHeight: kContextMenuActionHeight }),
    child: new RenderDecoratedBox({
      decoration: {},
      child: new RenderPadding({
        padding: EdgeInsets.fromLTRB(15.5, 8, 17.5, 8),
        textDirection: options.textDirection,
        child: row,
      }),
    }),
  });
}
