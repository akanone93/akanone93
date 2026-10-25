// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/scaffold.dart, floating_action_button_location.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `Scaffold`'s layout (`_ScaffoldLayout`): the app bar at the top, the bottom
 * navigation bar at the bottom, the body between them, the persistent bottom
 * sheet and the floating action button at its `FloatingActionButtonLocation`.
 * Snack bars, material banners, drawers and insets are not part of a canvas
 * state, so they are not laid out.
 */

import { BoxConstraints } from "../constraints.ts";
import { ContainerRenderBox } from "../container.ts";
import { Offset, Size, TextDirection } from "../geometry.ts";
import type { RenderBox } from "../render-box.ts";
import { setChildOffset } from "../shifted-box.ts";

/** `kFloatingActionButtonMargin`. */
const kFloatingActionButtonMargin = 16;

/** The `FloatingActionButtonLocation`s the canvas offers. */
export type FabLocation =
  | "startTop"
  | "centerTop"
  | "endTop"
  | "startFloat"
  | "centerFloat"
  | "endFloat"
  | "startDocked"
  | "centerDocked"
  | "endDocked";

export interface ScaffoldSlots {
  appBar: RenderBox | null;
  body: RenderBox | null;
  bottomNavigationBar: RenderBox | null;
  bottomSheet: RenderBox | null;
  floatingActionButton: RenderBox | null;
}

export interface ScaffoldOptions extends ScaffoldSlots {
  fabLocation: FabLocation;
  textDirection: TextDirection;
}

/** `_ScaffoldLayout` in a `CustomMultiChildLayout`: as big as allowed. */
export class RenderScaffold extends ContainerRenderBox<RenderBox> {
  constructor(private readonly o: ScaffoldOptions) {
    super();
    for (const child of [o.body, o.appBar, o.bottomNavigationBar, o.bottomSheet, o.floatingActionButton]) {
      if (child !== null) {
        this.add(child);
      }
    }
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  protected override performLayout(): void {
    const size = this.constraints.biggest;
    this.size = size;
    const o = this.o;
    const loose = BoxConstraints.loose(size);
    const fullWidth = loose.tighten({ width: size.width });
    const bottom = size.height;
    let contentTop = 0;
    let bottomWidgetsHeight = 0;
    let appBarHeight = 0;
    if (o.appBar !== null) {
      o.appBar.layout(fullWidth, { parentUsesSize: true });
      appBarHeight = o.appBar.size.height;
      contentTop = appBarHeight;
      setChildOffset(o.appBar, Offset.zero);
    }
    if (o.bottomNavigationBar !== null) {
      o.bottomNavigationBar.layout(fullWidth, { parentUsesSize: true });
      bottomWidgetsHeight += o.bottomNavigationBar.size.height;
      setChildOffset(o.bottomNavigationBar, new Offset(0, Math.max(0, bottom - bottomWidgetsHeight)));
    }
    const contentBottom = Math.max(0, bottom - bottomWidgetsHeight);
    if (o.body !== null) {
      o.body.layout(
        new BoxConstraints({ maxWidth: fullWidth.maxWidth, maxHeight: Math.max(0, contentBottom - contentTop) }),
        { parentUsesSize: true },
      );
      setChildOffset(o.body, new Offset(0, contentTop));
    }
    let bottomSheetHeight = 0;
    if (o.bottomSheet !== null) {
      o.bottomSheet.layout(
        new BoxConstraints({ maxWidth: fullWidth.maxWidth, maxHeight: Math.max(0, contentBottom - contentTop) }),
        { parentUsesSize: true },
      );
      bottomSheetHeight = o.bottomSheet.size.height;
      setChildOffset(
        o.bottomSheet,
        new Offset((size.width - o.bottomSheet.size.width) / 2, contentBottom - bottomSheetHeight),
      );
    }
    const fab = o.floatingActionButton;
    if (fab !== null) {
      fab.layout(loose, { parentUsesSize: true });
      setChildOffset(fab, this.fabOffset(fab.size, size, contentBottom, appBarHeight, bottomSheetHeight));
    }
  }

  /** `StandardFabLocation.getOffset` for a scaffold without insets. */
  private fabOffset(fab: Size, size: Size, contentBottom: number, contentTop: number, sheet: number): Offset {
    const location = this.o.fabLocation;
    const rtl = this.o.textDirection === TextDirection.rtl;
    const left = kFloatingActionButtonMargin;
    const right = size.width - kFloatingActionButtonMargin - fab.width;
    const x = location.startsWith("center")
      ? (size.width - fab.width) / 2
      : location.startsWith("start")
        ? rtl
          ? right
          : left
        : rtl
          ? left
          : right;
    let y: number;
    if (location.endsWith("Top")) {
      y = contentTop > 0 ? contentTop - fab.height / 2 : 0;
    } else if (location.endsWith("Float")) {
      y = contentBottom - fab.height - kFloatingActionButtonMargin;
      if (sheet > 0) {
        y = Math.min(y, contentBottom - sheet - fab.height / 2);
      }
    } else {
      // No view padding or inset: the safe margin of `FabDockedOffsetY` is zero.
      y = contentBottom - fab.height / 2;
      if (sheet > 0) {
        y = Math.min(y, contentBottom - sheet - fab.height / 2);
      }
      y = Math.min(size.height - fab.height, y);
    }
    return new Offset(x, y);
  }
}
