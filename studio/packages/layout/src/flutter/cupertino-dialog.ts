// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/cupertino/dialog.dart and packages/flutter/lib/src/widgets/single_child_scroll_view.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * `CupertinoAlertDialog` and `CupertinoActionSheet` (STU-005, S4.4) as the
 * static canvas states: nothing is pressed and nothing is scrolled, so the
 * pressed backgrounds, the overscroll fill, the scrollbars, the focus halo
 * and the gesture detectors, none of which takes part in layout, are not
 * ported. The sections are laid out by the same render objects as Flutter's:
 * a vertical single-child viewport, the priority column that lets the
 * actions of a tall dialog keep a minimum height, and the actions layout
 * that puts two short actions side by side.
 */

import { BoxFit } from "./box-fit.ts";
import { BoxConstraints } from "./constraints.ts";
import {
  Axis,
  CrossAxisAlignment,
  FlexFit,
  FlexParentData,
  MainAxisAlignment,
  MainAxisSize,
  RenderFlex,
} from "./flex.ts";
import { Alignment, EdgeInsets, Offset, Size, TextDirection } from "./geometry.ts";
import { RenderConstrainedBox, RenderDecoratedBox } from "./proxy-box.ts";
import { RenderFittedBox, RenderLimitedBox } from "./proxy-boxes.ts";
import type { BoxParentData, RenderBox } from "./render-box.ts";
import { RenderPadding, RenderPositionedBox, RenderShiftedBox } from "./shifted-box.ts";
import { RenderStack } from "./stack.ts";

const kDividerThickness = 0.3;
export const kCupertinoDialogWidth = 270;
export const kAccessibilityCupertinoDialogWidth = 310;
export const kDialogEdgePadding = 20;
const kDialogMinButtonHeight = 45;
const kDialogMinButtonFontSize = 10;
const kDialogActionsSectionMinHeight = 67.8;
const kActionSheetEdgePadding = 8;
const kActionSheetCancelButtonPadding = 8;
const kActionSheetContentHorizontalPadding = 16;
const kActionSheetContentVerticalPadding = 13.5;
const kActionSheetActionsSectionMinHeight = 84;
const kActionSheetButtonHorizontalPadding = 10;
const kActionSheetButtonMinHeight = 57.17;
const kActionSheetButtonVerticalPaddingFactor = 0.4;
const kActionSheetButtonVerticalPaddingBase = 1.8;
const kMaxRegularTextScaleFactor = 1.4;

/** The text styles of the dialog (`_kCupertinoDialog*Style`, `_kActionSheet*Style`): sizes, weights and metrics. */
export const cupertinoDialogTextStyles = {
  title: { fontSize: 17, fontWeight: 600, height: 1.3, letterSpacing: -0.5 },
  content: { fontSize: 13, fontWeight: 400, height: 1.35, letterSpacing: -0.2 },
  action: { fontSize: 16.8, fontWeight: 400 },
  sheetContent: { fontSize: 13, fontWeight: 400 },
} as const;

/** `_isInAccessibilityMode`: the text scale factor is above `_kMaxRegularTextScaleFactor`. */
export function isInAccessibilityMode(textScaleFactor: number): boolean {
  const defaultFontSize = 14;
  return defaultFontSize * textScaleFactor > defaultFontSize * kMaxRegularTextScaleFactor;
}

/** `_buttonFontSize` of the action sheet: the font size of an action for a body size. */
export function actionSheetButtonFontSize(contextBodySize: number): number {
  if (contextBodySize <= 17) {
    return 21;
  }
  if (contextBodySize <= 19) {
    return 21 + (contextBodySize - 17);
  }
  if (contextBodySize <= 21) {
    return 23 + (contextBodySize - 19) / 2;
  }
  return contextBodySize <= 24 ? 24 : contextBodySize;
}

function place(child: RenderBox, offset: Offset): void {
  (child.parentData as BoxParentData).offset = offset;
}

/** `Flexible`: a loose flex child. */
function flexibleLoose(box: RenderBox): RenderBox {
  const parentData = new FlexParentData();
  parentData.flex = 1;
  parentData.fit = FlexFit.loose;
  box.parentData = parentData;
  return box;
}

function sizedBox(width: number | null, height: number | null, child: RenderBox | null = null): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width, height }), child });
}

function colored(child: RenderBox | null): RenderBox {
  return new RenderDecoratedBox({ decoration: {}, child });
}

/** `_Divider`: a 0.3 pt rule. */
function divider(): RenderBox {
  return new RenderLimitedBox({
    maxWidth: kDividerThickness,
    maxHeight: kDividerThickness,
    child: new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({ minWidth: kDividerThickness, minHeight: kDividerThickness }),
      child: colored(null),
    }),
  });
}

/** `LimitedBox(maxWidth: 0, child: SizedBox(width: infinity, height: 0))`: what an empty section is. */
function emptySection(): RenderBox {
  return new RenderLimitedBox({ maxWidth: 0, child: sizedBox(Number.POSITIVE_INFINITY, 0) });
}

/** `_RenderSingleChildViewport` (vertical): the child is free in height; the viewport is as tall as the child, within its constraints. */
export class RenderSingleChildViewport extends RenderShiftedBox {
  constructor(child: RenderBox | null) {
    super(child);
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return this.child?.getMinIntrinsicWidth(height) ?? 0;
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    return this.child?.getMaxIntrinsicWidth(height) ?? 0;
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return this.child?.getMinIntrinsicHeight(width) ?? 0;
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this.child?.getMaxIntrinsicHeight(width) ?? 0;
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child;
    return child === null
      ? constraints.smallest
      : constraints.constrain(child.getDryLayout(constraints.widthConstraints()));
  }

  protected override performLayout(): void {
    const child = this.child;
    if (child === null) {
      this.size = this.constraints.smallest;
      return;
    }
    child.layout(this.constraints.widthConstraints(), { parentUsesSize: true });
    this.size = this.constraints.constrain(child.size);
    place(child, Offset.zero);
  }
}

/** `_RenderAlertDialogActionsLayout`: three children (action, divider, action) that fit are laid out side by side. */
export class RenderAlertDialogActionsLayout extends RenderFlex {
  private readonly _dividerThickness: number;

  constructor(options: { children: readonly RenderBox[]; dividerThickness: number; textDirection: TextDirection }) {
    super({
      direction: Axis.vertical,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      textDirection: options.textDirection,
      children: options.children,
    });
    this._dividerThickness = options.dividerThickness;
  }

  private slotWidthFor(overallWidth: number): number {
    return (overallWidth - this._dividerThickness) / 2;
  }

  private forEachSlot(action: (slot: RenderBox) => void): void {
    let slot = this.firstChild;
    while (slot !== null) {
      action(slot);
      const divider = this.childAfter(slot);
      slot = divider === null ? null : this.childAfter(divider);
    }
  }

  private useHorizontalLayout(overallWidth: number): boolean {
    if (this.childCount !== 3) {
      return false;
    }
    const slotWidth = this.slotWidthFor(overallWidth);
    let fits = true;
    this.forEachSlot((slot) => {
      if (slot.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY) > slotWidth) {
        fits = false;
      }
    });
    return fits;
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    if (!this.useHorizontalLayout(width)) {
      return super.computeMinIntrinsicHeight(width);
    }
    const slotWidth = this.slotWidthFor(width);
    let height = 0;
    this.forEachSlot((slot) => {
      height = Math.max(height, slot.getMinIntrinsicHeight(slotWidth));
    });
    return height;
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    if (!this.useHorizontalLayout(width)) {
      return super.computeMaxIntrinsicHeight(width);
    }
    const slotWidth = this.slotWidthFor(width);
    let height = 0;
    this.forEachSlot((slot) => {
      height = Math.max(height, slot.getMaxIntrinsicHeight(slotWidth));
    });
    return height;
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const overallWidth = constraints.maxWidth;
    if (!this.useHorizontalLayout(overallWidth)) {
      return super.computeDryLayout(constraints);
    }
    return new Size(overallWidth, this.getMinIntrinsicHeight(overallWidth));
  }

  protected override performLayout(): void {
    const first = this.firstChild;
    if (first === null) {
      this.size = this.constraints.smallest;
      return;
    }
    const overallWidth = this.constraints.maxWidth;
    if (!this.useHorizontalLayout(overallWidth)) {
      super.performLayout();
      return;
    }
    const slotWidth = this.slotWidthFor(overallWidth);
    const height = this.getMinIntrinsicHeight(overallWidth);
    this.size = new Size(overallWidth, height);
    const ltr = this.textDirection === TextDirection.ltr;
    let slot: RenderBox = first;
    let x = ltr ? 0 : overallWidth - slotWidth;
    for (;;) {
      slot.layout(BoxConstraints.tight(new Size(slotWidth, height)), { parentUsesSize: true });
      place(slot, new Offset(x, 0));
      x += ltr ? slot.size.width : -slot.size.width;
      const rule = this.childAfter(slot);
      if (rule === null) {
        break;
      }
      rule.layout(BoxConstraints.tight(new Size(this._dividerThickness, height)));
      place(rule, new Offset(x, 0));
      x += ltr ? this._dividerThickness : -this._dividerThickness;
      const next = this.childAfter(rule);
      if (next === null) {
        break;
      }
      slot = next;
    }
  }
}

/** `_RenderPriorityColumn`: the top child takes what it needs, the bottom child keeps at least its minimum height. */
export class RenderPriorityColumn extends RenderFlex {
  private readonly _bottomMinHeight: (maxHeight: number) => number;

  constructor(options: { top: RenderBox; bottom: RenderBox; bottomMinHeight: (maxHeight: number) => number }) {
    super({
      direction: Axis.vertical,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [options.top, options.bottom],
    });
    this._bottomMinHeight = options.bottomMinHeight;
  }

  private get topChild(): RenderBox {
    return this.firstChild as RenderBox;
  }

  private get bottomChild(): RenderBox {
    return this.lastChild as RenderBox;
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return this.topChild.getMinIntrinsicHeight(width) + this.bottomChild.getMinIntrinsicHeight(width);
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this.topChild.getMaxIntrinsicHeight(width) + this.bottomChild.getMaxIntrinsicHeight(width);
  }

  private childrenHeights(width: number, maxHeight: number): { top: number; bottom: number } {
    const topIntrinsic = this.topChild.getMinIntrinsicHeight(width);
    const bottomIntrinsic = this.bottomChild.getMinIntrinsicHeight(width);
    if (topIntrinsic + bottomIntrinsic <= maxHeight) {
      return { top: topIntrinsic, bottom: bottomIntrinsic };
    }
    const bottomMin = Math.min(this._bottomMinHeight(maxHeight), bottomIntrinsic);
    if (maxHeight - topIntrinsic >= bottomMin) {
      return { top: topIntrinsic, bottom: maxHeight - topIntrinsic };
    }
    if (maxHeight >= bottomMin) {
      return { top: maxHeight - bottomMin, bottom: bottomMin };
    }
    return { top: 0, bottom: maxHeight };
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const { top, bottom } = this.childrenHeights(constraints.maxWidth, constraints.maxHeight);
    return new Size(constraints.maxWidth, top + bottom);
  }

  protected override performLayout(): void {
    const width = this.constraints.maxWidth;
    const { top, bottom } = this.childrenHeights(width, this.constraints.maxHeight);
    this.size = new Size(width, top + bottom);
    this.topChild.layout(BoxConstraints.tight(new Size(width, top)), { parentUsesSize: true });
    place(this.topChild, Offset.zero);
    this.bottomChild.layout(BoxConstraints.tight(new Size(width, bottom)), { parentUsesSize: true });
    place(this.bottomChild, new Offset(0, top));
  }
}

function column(
  children: readonly RenderBox[],
  options: { stretch: boolean; min?: boolean; end?: boolean },
): RenderBox {
  return new RenderFlex({
    direction: Axis.vertical,
    crossAxisAlignment: options.stretch ? CrossAxisAlignment.stretch : CrossAxisAlignment.center,
    mainAxisSize: options.min === true ? MainAxisSize.min : MainAxisSize.max,
    mainAxisAlignment: options.end === true ? MainAxisAlignment.end : MainAxisAlignment.start,
    children,
  });
}

/** `_CupertinoAlertContentSection`: title and message in padding, in a scrolling column. */
function contentSection(options: {
  title: RenderBox | null;
  message: RenderBox | null;
  titlePadding: EdgeInsets;
  messagePadding: EdgeInsets;
  betweenPadding?: EdgeInsets;
  textDirection: TextDirection;
}): RenderBox {
  const group: RenderBox[] = [];
  if (options.title !== null) {
    group.push(
      new RenderPadding({
        padding: options.titlePadding,
        textDirection: options.textDirection,
        child: options.title,
      }),
    );
  }
  if (options.message !== null) {
    group.push(
      new RenderPadding({
        padding: options.messagePadding,
        textDirection: options.textDirection,
        child: options.message,
      }),
    );
  }
  if (options.betweenPadding !== undefined && group.length > 1) {
    group.splice(
      1,
      0,
      new RenderPadding({ padding: options.betweenPadding, textDirection: options.textDirection, child: null }),
    );
  }
  return new RenderSingleChildViewport(column(group, { stretch: true }));
}

/** `_OverscrollBackground`: a `Stack` whose only laid-out child is the scrolled section. */
function overscroll(child: RenderBox, textDirection: TextDirection): RenderBox {
  return new RenderStack({ children: [child], textDirection });
}

/** `CupertinoDialogAction.build`: the label shrinks to fit (regular text sizes) or wraps (accessibility sizes). */
export function cupertinoDialogAction(options: {
  child: RenderBox;
  fontSize: number;
  textScaleFactor: number;
  textDirection: TextDirection;
}): RenderBox {
  const accessibility = isInAccessibilityMode(options.textScaleFactor);
  const padding = 8 * options.textScaleFactor;
  let content = options.child;
  if (!accessibility) {
    const dialogWidth = kCupertinoDialogWidth;
    const fontSizeRatio = (options.fontSize * options.textScaleFactor) / kDialogMinButtonFontSize;
    content = new RenderFittedBox({
      fit: BoxFit.scaleDown,
      textDirection: options.textDirection,
      child: new RenderConstrainedBox({
        additionalConstraints: new BoxConstraints({ maxWidth: fontSizeRatio * (dialogWidth - 2 * padding) }),
        child: options.child,
      }),
    });
  }
  const centered = new RenderPositionedBox({
    alignment: Alignment.center,
    textDirection: options.textDirection,
    child: content,
  });
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minHeight: kDialogMinButtonHeight }),
    child: new RenderPadding({
      padding: EdgeInsets.all(padding),
      textDirection: options.textDirection,
      child: centered,
    }),
  });
}

/** `CupertinoAlertDialog` (`_buildBody` and `build`), at the incoming text scale factor `textScaleFactor` (at least 1). */
export function cupertinoAlertDialog(options: {
  title: RenderBox | null;
  content: RenderBox | null;
  actions: readonly RenderBox[];
  textScaleFactor: number;
  textDirection: TextDirection;
}): RenderBox {
  const { textDirection } = options;
  const accessibility = isInAccessibilityMode(options.textScaleFactor);
  const hasContent = options.title !== null || options.content !== null;
  const content = hasContent
    ? colored(
        contentSection({
          title: options.title,
          message: options.content,
          titlePadding: EdgeInsets.only({
            left: kDialogEdgePadding,
            right: kDialogEdgePadding,
            bottom: options.content === null ? kDialogEdgePadding : 1,
            top: kDialogEdgePadding * options.textScaleFactor,
          }),
          messagePadding: EdgeInsets.only({
            left: kDialogEdgePadding,
            right: kDialogEdgePadding,
            bottom: kDialogEdgePadding * options.textScaleFactor,
            top: options.title === null ? kDialogEdgePadding : 1,
          }),
          textDirection,
        }),
      )
    : null;

  let body: RenderBox;
  if (options.actions.length === 0) {
    body = content ?? emptySection();
  } else {
    const withDividers: RenderBox[] = [];
    options.actions.forEach((action, index) => {
      if (index !== 0) {
        withDividers.push(divider());
      }
      withDividers.push(colored(action));
    });
    const actions = overscroll(
      new RenderSingleChildViewport(
        new RenderAlertDialogActionsLayout({
          children: withDividers,
          dividerThickness: kDividerThickness,
          textDirection,
        }),
      ),
      textDirection,
    );
    body =
      content === null
        ? actions
        : new RenderPriorityColumn({
            top: content,
            bottom: column([sizedBox(Number.POSITIVE_INFINITY, null, divider()), flexibleLoose(actions)], {
              stretch: true,
              min: true,
            }),
            bottomMinHeight: (maxHeight) =>
              (accessibility ? maxHeight / 2 : kDialogActionsSectionMinHeight) + kDividerThickness,
          });
  }
  const sized = sizedBox(accessibility ? kAccessibilityCupertinoDialogWidth : kCupertinoDialogWidth, null, body);
  const centered = new RenderPositionedBox({
    alignment: Alignment.center,
    textDirection,
    child: new RenderPadding({
      padding: EdgeInsets.symmetric({ vertical: kDialogEdgePadding }),
      textDirection,
      child: sized,
    }),
  });
  return new RenderPadding({
    padding: EdgeInsets.symmetric({ horizontal: 40, vertical: 24 }),
    textDirection,
    child: centered,
  });
}

/** `_ActionSheetActionContent` and `CupertinoActionSheetAction.build`: the label centred in the padded, 57.17 pt minimum button. */
export function cupertinoActionSheetAction(options: {
  child: RenderBox;
  textScaleFactor: number;
  textDirection: TextDirection;
}): RenderBox {
  const fontSize = actionSheetButtonFontSize(17 * options.textScaleFactor);
  const vertical = kActionSheetButtonVerticalPaddingBase + fontSize * kActionSheetButtonVerticalPaddingFactor;
  const centered = new RenderPositionedBox({
    alignment: Alignment.center,
    textDirection: options.textDirection,
    child: options.child,
  });
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minHeight: kActionSheetButtonMinHeight }),
    child: new RenderPadding({
      padding: EdgeInsets.fromLTRB(
        kActionSheetButtonHorizontalPadding,
        vertical,
        kActionSheetButtonHorizontalPadding,
        vertical,
      ),
      textDirection: options.textDirection,
      child: centered,
    }),
  });
}

/** The font size, before the text scale, of an action sheet action's text. */
export function actionSheetActionFontSize(textScaleFactor: number): number {
  return actionSheetButtonFontSize(17 * textScaleFactor) / textScaleFactor;
}

/** `_topPadding`: the gap above a sheet under a status bar of `viewPaddingTop`. */
export function actionSheetTopPadding(viewPaddingTop: number, landscape: boolean): number {
  if (landscape) {
    return kActionSheetEdgePadding;
  }
  const t = Math.min(Math.max((viewPaddingTop - 47) / (59 - 47), 0), 1);
  const ratio = 1 + (54 / 59 - 1) * t;
  return Math.max(Math.round(ratio * viewPaddingTop), kDialogEdgePadding);
}

/** `_ActionSheetMainSheet.build`: the content section over the scrolled, divided actions. */
function actionSheetMain(
  content: RenderBox | null,
  actions: readonly RenderBox[],
  textDirection: TextDirection,
): RenderBox {
  if (actions.length === 0) {
    return content ?? emptySection();
  }
  const rows: RenderBox[] = [];
  actions.forEach((action, index) => {
    if (index !== 0) {
      rows.push(divider());
    }
    rows.push(colored(action));
  });
  const scrolled = overscroll(colored(new RenderSingleChildViewport(column(rows, { stretch: true }))), textDirection);
  if (content === null) {
    return scrolled;
  }
  return new RenderPriorityColumn({
    top: content,
    bottom: column([sizedBox(Number.POSITIVE_INFINITY, null, divider()), flexibleLoose(scrolled)], {
      stretch: true,
      min: true,
    }),
    bottomMinHeight: () => kActionSheetActionsSectionMinHeight + kDividerThickness,
  });
}

/** `CupertinoActionSheet.build` with the main sheet (`_ActionSheetMainSheet`), the content section and the cancel button. */
export function cupertinoActionSheet(options: {
  title: RenderBox | null;
  message: RenderBox | null;
  actions: readonly RenderBox[];
  cancel: RenderBox | null;
  screen: Size;
  safeTop: number;
  safeBottom: number;
  textDirection: TextDirection;
}): RenderBox {
  const { textDirection } = options;
  const hasContent = options.title !== null || options.message !== null;
  const content = hasContent
    ? colored(
        contentSection({
          title: options.title,
          message: options.message,
          titlePadding: EdgeInsets.only({
            left: kActionSheetContentHorizontalPadding,
            right: kActionSheetContentHorizontalPadding,
            bottom: options.message === null ? kActionSheetContentVerticalPadding : 0,
            top: kActionSheetContentVerticalPadding,
          }),
          messagePadding: EdgeInsets.only({
            left: kActionSheetContentHorizontalPadding,
            right: kActionSheetContentHorizontalPadding,
            bottom: kActionSheetContentVerticalPadding,
            top: options.title === null ? kActionSheetContentVerticalPadding : 0,
          }),
          betweenPadding: EdgeInsets.only({ top: 4 }),
          textDirection,
        }),
      )
    : null;

  const main = actionSheetMain(content, options.actions, textDirection);
  const children: RenderBox[] = [flexibleLoose(main)];
  if (options.cancel !== null) {
    const cancelPadding = options.actions.length > 0 || hasContent ? kActionSheetCancelButtonPadding : 0;
    children.push(
      new RenderPadding({
        padding: EdgeInsets.only({ top: cancelPadding }),
        textDirection,
        child: colored(options.cancel),
      }),
    );
  }
  const landscape = options.screen.width > options.screen.height;
  const sheetWidth = landscape ? options.screen.height : options.screen.width;
  const sheet = sizedBox(
    sheetWidth - kActionSheetEdgePadding * 2,
    null,
    column(children, { stretch: true, min: true, end: true }),
  );
  const padded = new RenderPadding({
    padding: EdgeInsets.only({
      left: kActionSheetEdgePadding,
      right: kActionSheetEdgePadding,
      top: actionSheetTopPadding(options.safeTop, landscape),
    }),
    textDirection,
    child: sheet,
  });
  return new RenderPadding({
    padding: EdgeInsets.fromLTRB(0, options.safeTop, 0, Math.max(options.safeBottom, kActionSheetEdgePadding)),
    textDirection,
    child: padded,
  });
}
