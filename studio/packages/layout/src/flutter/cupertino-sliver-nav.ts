// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/cupertino/nav_bar.dart (CupertinoSliverNavigationBar)
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `CupertinoSliverNavigationBar` (STU-005, S4.4): a pinned persistent header
 * whose collapsed extent is the 44-pt toolbar under the status bar and whose
 * expanded extent adds the large title. The bar's content is a stack of the
 * large title (below the toolbar, clipped) and the toolbar; the scroll
 * position only changes how much of the large title the header's height
 * shows, so the same tree serves the expanded and the collapsed state. The
 * search field, the bottom slot, route-derived titles and the transition
 * animations are not ported.
 */

import {
  expanded,
  kNavBarBottomPadding,
  kNavBarEdgePadding,
  kNavBarPersistentHeight,
  persistentNavigationBar,
  positioned,
  RenderLargeTitle,
} from "./cupertino.ts";
import { Axis, CrossAxisAlignment, MainAxisAlignment, MainAxisSize, RenderFlex } from "./flex.ts";
import { EdgeInsetsDirectional, type TextDirection } from "./geometry.ts";
import { RenderDecoratedBox } from "./proxy-box.ts";
import type { RenderBox } from "./render-box.ts";
import { RenderPadding } from "./shifted-box.ts";
import type { RenderSliver } from "./sliver.ts";
import { RenderSliverPinnedPersistentHeader } from "./sliver-persistent-header.ts";
import { RenderStack } from "./stack.ts";

/** The header's collapsed and expanded extents for a status-bar inset and a (scaled) large-title height. */
export function sliverNavBarExtents(safeTop: number, largeTitleHeight: number): { min: number; max: number } {
  const persistentHeight = kNavBarPersistentHeight + safeTop;
  return { min: persistentHeight, max: persistentHeight + largeTitleHeight };
}

/** `_LargeTitleNavigationBarSliverDelegate.build` (no bottom): the bar for any header height. */
export function cupertinoSliverNavigationBar(options: {
  leading: RenderBox | null;
  middle: RenderBox | null;
  trailing: RenderBox | null;
  largeTitle: RenderBox;
  largeTitleHeight: number;
  safeTop: number;
  textDirection: TextDirection;
}): RenderSliver {
  const { min, max } = sliverNavBarExtents(options.safeTop, options.largeTitleHeight);
  const title = new RenderLargeTitle({
    textDirection: options.textDirection,
    height: options.largeTitleHeight,
    child: options.largeTitle,
  });
  const titleArea = positioned(
    new RenderPadding({
      padding: EdgeInsetsDirectional.only({ start: kNavBarEdgePadding, bottom: kNavBarBottomPadding }),
      textDirection: options.textDirection,
      child: title,
    }),
    { top: min, left: 0, right: 0, bottom: 0 },
  );
  const toolbar = positioned(persistentNavigationBar(options), { left: 0, right: 0, top: 0 });
  const stack = new RenderStack({ children: [titleArea, toolbar], textDirection: options.textDirection });
  const column = new RenderFlex({
    direction: Axis.vertical,
    mainAxisSize: MainAxisSize.max,
    mainAxisAlignment: MainAxisAlignment.start,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection: options.textDirection,
    children: [expanded(stack)],
  });
  return new RenderSliverPinnedPersistentHeader({
    child: new RenderDecoratedBox({ decoration: {}, child: column }),
    minExtent: min,
    maxExtent: max,
  });
}
