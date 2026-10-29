// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/dialog.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Material 3 `AlertDialog` as a static state: the dialog panel (at least 280
 * wide, as wide as its widest part) centred in the space the route gives it,
 * inset by 40 by 24. The title, content and actions are padded by 24 scaled
 * down to a third as the text scale grows to 2, and the actions sit in an
 * `OverflowBar` aligned to the end.
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
import { Alignment, EdgeInsets, type TextDirection } from "../geometry.ts";
import { RenderOverflowBar } from "../overflow-bar.ts";
import { RenderConstrainedBox } from "../proxy-box.ts";
import { RenderIntrinsicWidth } from "../proxy-boxes.ts";
import type { RenderBox } from "../render-box.ts";
import { RenderPadding, RenderPositionedBox } from "../shifted-box.ts";

/** `_defaultInsetPadding`. */
const insetHorizontal = 40;
const insetVertical = 24;
/** `Dialog`'s default `BoxConstraints(minWidth: 280)`. */
const kDialogMinWidth = 280;
/** The spacing between actions: half of the button padding's 16. */
const actionsSpacing = 8;

/** `_scalePadding`: 1 at text scale 1 to 1/3 at scale 2 and beyond. */
export function dialogPaddingScale(textScale: number): number {
  const clamped = Math.min(2, Math.max(1, textScale));
  return 1 + (1 / 3 - 1) * (clamped - 1);
}

export interface AlertDialogOptions {
  title: RenderBox | null;
  content: RenderBox | null;
  actions: RenderBox[] | null;
  /** `MediaQuery.textScalerOf(context).scale(14) / 14`. */
  textScale: number;
  textDirection: TextDirection;
}

/** `AlertDialog.build` with the Material 3 defaults. */
export function buildAlertDialog(o: AlertDialogOptions): RenderBox {
  const scale = dialogPaddingScale(o.textScale);
  const parts: RenderBox[] = [];
  if (o.title !== null) {
    parts.push(
      new RenderPadding({
        padding: EdgeInsets.only({
          left: 24 * scale,
          top: 24 * scale,
          right: 24 * scale,
          bottom: o.content === null ? 20 : 0,
        }),
        child: o.title,
      }),
    );
  }
  if (o.content !== null) {
    const content = new RenderPadding({
      padding: EdgeInsets.only({
        left: 24 * scale,
        top: o.title === null ? 16 * scale : 16,
        right: 24 * scale,
        bottom: 24,
      }),
      child: o.content,
    });
    const parentData = new FlexParentData();
    parentData.flex = 1;
    parentData.fit = FlexFit.loose;
    content.parentData = parentData;
    parts.push(content);
  }
  if (o.actions !== null) {
    parts.push(
      new RenderPadding({
        padding: EdgeInsets.only({ left: 24, right: 24, bottom: 24 }),
        child: new RenderOverflowBar({
          children: o.actions,
          alignment: MainAxisAlignment.end,
          spacing: actionsSpacing,
          overflowAlignment: "end",
          textDirection: o.textDirection,
        }),
      }),
    );
  }
  const column = new RenderFlex({
    direction: Axis.vertical,
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: CrossAxisAlignment.stretch,
    textDirection: o.textDirection,
    children: parts,
  });
  return new RenderPadding({
    padding: EdgeInsets.symmetric({ horizontal: insetHorizontal, vertical: insetVertical }),
    child: new RenderPositionedBox({
      alignment: Alignment.center,
      child: new RenderConstrainedBox({
        additionalConstraints: new BoxConstraints({ minWidth: kDialogMinWidth }),
        child: new RenderIntrinsicWidth({ child: column }),
      }),
    }),
  });
}
