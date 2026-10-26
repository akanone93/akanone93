// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/cupertino/{button,switch,slider,nav_bar,bottom_tab_bar,page_scaffold,constants}.dart and packages/flutter/lib/src/widgets/navigation_toolbar.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Cupertino composites (STU-005, S4.4): each function builds the render tree
 * the Flutter widget of the same name builds, from the render objects already
 * ported. Painting, gestures, semantics, opacity animations and the debug
 * checks are not ported; none of them takes part in layout. The widgets read
 * nothing from a theme except the text styles in [cupertinoTextTheme], whose
 * sizes are Flutter's and whose family is the one the canvas lays text out
 * with (SF Pro is not available, so the device preview stays authoritative).
 */

import { BoxConstraints } from "./constraints.ts";
import { ContainerRenderBox } from "./container.ts";
import {
  Axis,
  CrossAxisAlignment,
  FlexFit,
  FlexParentData,
  MainAxisAlignment,
  MainAxisSize,
  RenderFlex,
} from "./flex.ts";
import {
  Alignment,
  AlignmentDirectional,
  clampDouble,
  EdgeInsets,
  EdgeInsetsDirectional,
  type EdgeInsetsGeometry,
  Offset,
  Size,
  TextDirection,
} from "./geometry.ts";
import { RenderListWheelViewport, type WheelRows } from "./list-wheel.ts";
import { Affine, type PaintTransformer } from "./matrix.ts";
import type { RenderObject } from "./object.ts";
import { RenderConstrainedBox, RenderDecoratedBox } from "./proxy-box.ts";
import { RenderLimitedBox } from "./proxy-boxes.ts";
import { BoxParentData, type RenderBox } from "./render-box.ts";
import { RenderAligningShiftedBox, RenderPadding, RenderPositionedBox, RenderShiftedBox } from "./shifted-box.ts";
import { RenderStack, StackFit, StackParentData } from "./stack.ts";
import type { TextStyle } from "./text-style.ts";

/** The layout-relevant fields of `CupertinoTextThemeData`, in the canvas family. */
export const cupertinoTextTheme = {
  textStyle: { fontSize: 17 },
  actionTextStyle: { fontSize: 17 },
  actionSmallTextStyle: { fontSize: 15 },
  tabLabelTextStyle: { fontSize: 10 },
  navTitleTextStyle: { fontSize: 17, fontWeight: 700 },
  navLargeTitleTextStyle: { fontSize: 34, fontWeight: 700 },
  navActionTextStyle: { fontSize: 17 },
  pickerTextStyle: { fontSize: 21 },
  dateTimePickerTextStyle: { fontSize: 21 },
} as const satisfies Record<string, TextStyle>;

/** `kMinInteractiveDimensionCupertino`. */
export const kMinInteractiveDimensionCupertino = 44;

export type CupertinoButtonSize = "small" | "medium" | "large";

/** `kCupertinoButtonPadding`. */
export function cupertinoButtonPadding(size: CupertinoButtonSize): EdgeInsets {
  switch (size) {
    case "small":
      return EdgeInsets.symmetric({ vertical: 6, horizontal: 12 });
    case "medium":
      return EdgeInsets.symmetric({ vertical: 10, horizontal: 15 });
    case "large":
      return EdgeInsets.symmetric({ vertical: 16, horizontal: 20 });
  }
}

/** `kCupertinoButtonMinSize`. */
export function cupertinoButtonMinSize(size: CupertinoButtonSize): number {
  return size === "small" ? 28 : size === "medium" ? 32 : 44;
}

function flexible(box: RenderBox, fit: FlexFit = FlexFit.tight): RenderBox {
  const parentData = new FlexParentData();
  parentData.flex = 1;
  parentData.fit = fit;
  box.parentData = parentData;
  return box;
}

/** `Expanded`. */
export function expanded(box: RenderBox): RenderBox {
  return flexible(box);
}

/** `CupertinoButton` / `CupertinoButton.filled` (`_CupertinoButtonState.build`): the minimum size is the tap target. */
export function cupertinoButton(options: {
  size: CupertinoButtonSize;
  padding?: EdgeInsetsGeometry | null;
  minWidth?: number | null;
  minHeight?: number | null;
  alignment?: Alignment;
  textDirection: TextDirection;
  child: RenderBox;
}): RenderBox {
  const aligned = new RenderPositionedBox({
    alignment: options.alignment ?? Alignment.center,
    textDirection: options.textDirection,
    widthFactor: 1,
    heightFactor: 1,
    child: options.child,
  });
  const padded = new RenderPadding({
    padding: options.padding ?? cupertinoButtonPadding(options.size),
    textDirection: options.textDirection,
    child: aligned,
  });
  const decorated = new RenderDecoratedBox({ decoration: {}, child: padded });
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({
      minWidth: options.minWidth ?? cupertinoButtonMinSize(options.size),
      minHeight: options.minHeight ?? cupertinoButtonMinSize(options.size),
    }),
    child: decorated,
  });
}

/** `CupertinoSwitch`: a 59 x 39 canvas (`_kSwitchSize`) inside the incoming constraints. */
export function cupertinoSwitch(): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 59, height: 39 }) });
}

/** `CupertinoSlider`: 176 wide (`_kSliderWidth`) and 2 * (thumb radius 14 + padding 8) high. */
export function cupertinoSlider(): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 176, height: 44 }) });
}

/** Where `NavigationToolbar` puts a child. */
export type ToolbarSlot = "leading" | "middle" | "trailing";

/** `NavigationToolbar`'s `_ToolbarLayout`: leading and trailing at the ends, the middle centred between them. */
export class RenderNavigationToolbar extends ContainerRenderBox<RenderBox> {
  private readonly _slots = new Map<ToolbarSlot, RenderBox>();
  private readonly _centerMiddle: boolean;
  private readonly _middleSpacing: number;
  private readonly _textDirection: TextDirection;

  constructor(options: {
    leading?: RenderBox | null;
    middle?: RenderBox | null;
    trailing?: RenderBox | null;
    centerMiddle?: boolean;
    middleSpacing?: number;
    textDirection: TextDirection;
  }) {
    super();
    this._centerMiddle = options.centerMiddle ?? true;
    this._middleSpacing = options.middleSpacing ?? 16;
    this._textDirection = options.textDirection;
    for (const [slot, box] of [
      ["leading", options.leading],
      ["middle", options.middle],
      ["trailing", options.trailing],
    ] as const) {
      if (box !== null && box !== undefined) {
        this._slots.set(slot, box);
        this.add(box);
      }
    }
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  protected override performLayout(): void {
    const size = this.constraints.biggest;
    this.size = size;
    const leadingWidth = this.layoutLeading(size);
    const trailingWidth = this.layoutTrailing(size);
    this.layoutMiddle(size, leadingWidth, trailingWidth);
  }

  private layoutLeading(size: Size): number {
    const leading = this._slots.get("leading");
    if (leading === undefined) {
      return 0;
    }
    leading.layout(new BoxConstraints({ maxWidth: size.width, minHeight: size.height, maxHeight: size.height }), {
      parentUsesSize: true,
    });
    const width = leading.size.width;
    place(leading, new Offset(this._textDirection === TextDirection.rtl ? size.width - width : 0, 0));
    return width;
  }

  private layoutTrailing(size: Size): number {
    const trailing = this._slots.get("trailing");
    if (trailing === undefined) {
      return 0;
    }
    trailing.layout(BoxConstraints.loose(size), { parentUsesSize: true });
    const width = trailing.size.width;
    const x = this._textDirection === TextDirection.rtl ? 0 : size.width - width;
    place(trailing, new Offset(x, (size.height - trailing.size.height) / 2));
    return width;
  }

  private layoutMiddle(size: Size, leadingWidth: number, trailingWidth: number): void {
    const middle = this._slots.get("middle");
    if (middle === undefined) {
      return;
    }
    const maxWidth = Math.max(size.width - leadingWidth - trailingWidth - this._middleSpacing * 2, 0);
    middle.layout(BoxConstraints.loose(size).copyWith({ maxWidth }), { parentUsesSize: true });
    const margin = leadingWidth + this._middleSpacing;
    let start = margin;
    if (this._centerMiddle) {
      start = (size.width - middle.size.width) / 2;
      if (start + middle.size.width > size.width - trailingWidth) {
        start = size.width - trailingWidth - middle.size.width - this._middleSpacing;
      } else if (start < margin) {
        start = margin;
      }
    }
    const x = this._textDirection === TextDirection.rtl ? size.width - middle.size.width - start : start;
    place(middle, new Offset(x, (size.height - middle.size.height) / 2));
  }
}

function place(child: RenderBox, offset: Offset): void {
  if (child.parentData instanceof BoxParentData) {
    child.parentData.offset = offset;
  }
}

/** `_RenderLargeTitle`: the large title, bottom-start aligned and scaled up to 10 % while the bar is pulled. */
export class RenderLargeTitle extends RenderShiftedBox implements PaintTransformer {
  private readonly _alignment: Alignment;
  private readonly _height: number;
  private _scale = 1;

  constructor(options: { textDirection: TextDirection; height: number; child: RenderBox | null }) {
    super(options.child);
    this._alignment = options.textDirection === TextDirection.rtl ? new Alignment(1, 1) : new Alignment(-1, 1);
    this._height = options.height;
  }

  private static titleScale(childSize: Size, constraints: BoxConstraints, height: number): number {
    const maxHeight = height - kNavBarBottomPadding;
    const scale = 1 + (0.03 * (constraints.maxHeight - maxHeight)) / maxHeight;
    const maxScale = childSize.width !== 0 ? clampDouble(constraints.maxWidth / childSize.width, 1, 1.1) : 1.1;
    return clampDouble(scale, 1, maxScale);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    return constraints.biggest;
  }

  protected override performLayout(): void {
    const size = this.constraints.biggest;
    this.size = size;
    const child = this.child;
    if (child === null) {
      return;
    }
    child.layout(this.constraints.widthConstraints().loosen(), { parentUsesSize: true });
    this._scale = RenderLargeTitle.titleScale(child.size, this.constraints, this._height);
    const free = new Offset(size.width - child.size.width * this._scale, size.height - child.size.height * this._scale);
    place(child, this._alignment.alongOffset(free));
  }

  paintTransformOf(child: RenderObject): Affine {
    const offset = child.parentData instanceof BoxParentData ? child.parentData.offset : Offset.zero;
    return Affine.translation(offset.dx, offset.dy).scale(this._scale, this._scale);
  }
}

/** `CupertinoPicker._kDefaultDiameterRatio`. */
export const kPickerDiameterRatio = 1.07;

/** `_kSqueeze`. */
export const kPickerSqueeze = 1.45;

/** `_kNavBarPersistentHeight`. */
export const kNavBarPersistentHeight = kMinInteractiveDimensionCupertino;
/** `_kNavBarLargeTitleHeightExtension`. */
export const kNavBarLargeTitleHeightExtension = 52;
/** `_kNavBarEdgePadding`. */
export const kNavBarEdgePadding = 16;
/** `_kNavBarBottomPadding`. */
export const kNavBarBottomPadding = 8;
const kMaxScaleFactor = 1.235;
const kMinScaleFactor = 0.9;
const kLargeTitleScaleDampingRatio = 3;

/** `_clampedTextScaler`: the factor the leading, middle and trailing widgets are scaled by. */
export function navBarClampedScale(factor: number): number {
  return clampDouble(factor, 1, kMaxScaleFactor);
}

/** `_dampScaleFactor` for the large title (the 52-pt title extent scaled by `factor`). */
export function navBarLargeTitleScale(factor: number): number {
  return factor < 1 ? Math.max(kMinScaleFactor, factor) : 1 + (factor - 1) / kLargeTitleScaleDampingRatio;
}

/** `_PersistentNavigationBar`: the 44-pt toolbar under the status bar. */
export function persistentNavigationBar(options: {
  leading: RenderBox | null;
  middle: RenderBox | null;
  trailing: RenderBox | null;
  safeTop: number;
  textDirection: TextDirection;
}): RenderBox {
  const leading = new RenderPositionedBox({
    widthFactor: 1,
    child: options.leading,
    textDirection: options.textDirection,
  });
  const toolbar = new RenderNavigationToolbar({
    leading,
    middle: options.middle,
    trailing: options.trailing,
    middleSpacing: 6,
    textDirection: options.textDirection,
  });
  const safe = new RenderPadding({ padding: EdgeInsets.only({ top: options.safeTop }), child: toolbar });
  return new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({ height: kNavBarPersistentHeight + options.safeTop }),
    child: safe,
  });
}

/** `CupertinoNavigationBar`'s `leading` / `trailing` edge padding. */
export function navBarEdge(child: RenderBox, edge: "start" | "end", textDirection: TextDirection): RenderBox {
  return new RenderPadding({
    padding: EdgeInsetsDirectional.only(edge === "start" ? { start: kNavBarEdgePadding } : { end: kNavBarEdgePadding }),
    textDirection,
    child,
  });
}

/** `CupertinoNavigationBar.build` (`largeTitle` set: the static large-title bar). */
export function cupertinoNavigationBar(options: {
  leading: RenderBox | null;
  middle: RenderBox | null;
  trailing: RenderBox | null;
  largeTitle: RenderBox | null;
  safeTop: number;
  textDirection: TextDirection;
}): RenderBox {
  const persistent = persistentNavigationBar(options);
  const persistentHeight = kNavBarPersistentHeight + options.safeTop;
  const children: RenderBox[] = [persistent];
  let maxHeight = persistentHeight;
  if (options.largeTitle !== null) {
    maxHeight = persistentHeight + kNavBarLargeTitleHeightExtension;
    const title = new RenderLargeTitle({
      textDirection: options.textDirection,
      height: kNavBarLargeTitleHeightExtension,
      child: options.largeTitle,
    });
    children.push(
      expanded(
        new RenderPadding({
          padding: EdgeInsetsDirectional.only({ start: kNavBarEdgePadding, bottom: kNavBarBottomPadding }),
          textDirection: options.textDirection,
          child: title,
        }),
      ),
    );
  }
  const column = new RenderFlex({
    direction: Axis.vertical,
    mainAxisSize: MainAxisSize.max,
    mainAxisAlignment: MainAxisAlignment.start,
    crossAxisAlignment: CrossAxisAlignment.center,
    textDirection: options.textDirection,
    children,
  });
  const constrained = new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ maxHeight }),
    child: column,
  });
  return new RenderDecoratedBox({ decoration: {}, child: constrained });
}

/** `CupertinoNavigationBar.preferredSize.height`. */
export function navBarPreferredHeight(large: boolean): number {
  return kNavBarPersistentHeight + (large ? kNavBarLargeTitleHeightExtension : 0);
}

/** `CupertinoTabBar.build`: icons centred above the labels, items sharing the width. */
export function cupertinoTabBar(options: {
  height: number;
  bottomPadding: number;
  items: readonly { icon: RenderBox; label: RenderBox | null }[];
  textDirection: TextDirection;
}): RenderBox {
  const items = options.items.map((item) => {
    const iconSlot = expanded(
      new RenderPositionedBox({ alignment: Alignment.center, textDirection: options.textDirection, child: item.icon }),
    );
    const column = new RenderFlex({
      direction: Axis.vertical,
      mainAxisSize: MainAxisSize.max,
      mainAxisAlignment: MainAxisAlignment.end,
      crossAxisAlignment: CrossAxisAlignment.center,
      textDirection: options.textDirection,
      children: item.label === null ? [iconSlot] : [iconSlot, item.label],
    });
    return expanded(new RenderPadding({ padding: EdgeInsets.only({ bottom: 4 }), child: column }));
  });
  const row = new RenderFlex({
    direction: Axis.horizontal,
    mainAxisSize: MainAxisSize.max,
    crossAxisAlignment: CrossAxisAlignment.end,
    textDirection: options.textDirection,
    children: items,
  });
  const padded = new RenderPadding({ padding: EdgeInsets.only({ bottom: options.bottomPadding }), child: row });
  const sized = new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({ height: options.height + options.bottomPadding }),
    child: padded,
  });
  return new RenderDecoratedBox({ decoration: {}, child: sized });
}

/** `CupertinoPageScaffold.build`: the content, with the navigation bar pinned over its top edge. */
export function cupertinoPageScaffold(options: {
  content: RenderBox;
  navigationBar: RenderBox | null;
  navigationBarHeight: number;
  /** Whether the bar's background is opaque (`shouldFullyObstruct`). */
  obstructing: boolean;
  safeTop: number;
  textDirection: TextDirection;
}): RenderBox {
  let padded: RenderBox;
  if (options.navigationBar !== null && options.obstructing) {
    padded = new RenderPadding({
      padding: EdgeInsets.only({ top: options.navigationBarHeight + options.safeTop }),
      child: options.content,
    });
  } else {
    padded = new RenderPadding({ padding: EdgeInsets.only({ bottom: 0 }), child: options.content });
  }
  const children: RenderBox[] = [padded];
  if (options.navigationBar !== null) {
    children.push(positioned(options.navigationBar, { top: 0, left: 0, right: 0 }));
  }
  children.push(
    positioned(new RenderConstrainedBox({ additionalConstraints: BoxConstraints.expand() }), {
      top: 0,
      left: 0,
      right: 0,
      height: options.safeTop,
    }),
  );
  const stack = new RenderStack({ children, textDirection: options.textDirection });
  return new RenderDecoratedBox({ decoration: {}, child: stack });
}

/** `Positioned`. */
export function positioned(
  box: RenderBox,
  sides: { top?: number; left?: number; right?: number; bottom?: number; height?: number; width?: number },
): RenderBox {
  const parentData = new StackParentData();
  parentData.top = sides.top ?? null;
  parentData.left = sides.left ?? null;
  parentData.right = sides.right ?? null;
  parentData.bottom = sides.bottom ?? null;
  parentData.height = sides.height ?? null;
  parentData.width = sides.width ?? null;
  box.parentData = parentData;
  return box;
}

/** `UnconstrainedBox(constrainedAxis: Axis.horizontal)`: the child keeps the width constraints and is free in height. */
export class RenderUnconstrainedHeight extends RenderAligningShiftedBox {
  constructor(options: { textDirection: TextDirection; child: RenderBox | null }) {
    super({ alignment: Alignment.center, textDirection: options.textDirection, child: options.child });
  }

  private childConstraints(constraints: BoxConstraints): BoxConstraints {
    return new BoxConstraints({ minWidth: constraints.minWidth, maxWidth: constraints.maxWidth });
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child;
    return child === null
      ? constraints.smallest
      : constraints.constrain(child.getDryLayout(this.childConstraints(constraints)));
  }

  protected override performLayout(): void {
    const child = this.child;
    if (child === null) {
      this.size = this.constraints.smallest;
      return;
    }
    child.layout(this.childConstraints(this.constraints), { parentUsesSize: true });
    this.size = this.constraints.constrain(child.size);
    this.alignChild();
  }
}

/** `_RenderSegmentedControl`: equally wide segments, the widest child sets the width. */
export class RenderSegmentedControl extends ContainerRenderBox<RenderBox> {
  private readonly _textDirection: TextDirection;

  constructor(options: { children: readonly RenderBox[]; textDirection: TextDirection }) {
    super();
    this._textDirection = options.textDirection;
    this.addAll(options.children);
  }

  private childSizeFor(constraints: BoxConstraints): Size {
    let height = 28;
    let width = constraints.minWidth / this.childCount;
    for (const child of this.getChildrenAsList()) {
      width = Math.max(width, child.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY));
    }
    width = Math.min(width, constraints.maxWidth / this.childCount);
    for (const child of this.getChildrenAsList()) {
      height = Math.max(height, child.getMaxIntrinsicHeight(width));
    }
    return new Size(width, height);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const childSize = this.childSizeFor(constraints);
    return constraints.constrain(new Size(childSize.width * this.childCount, childSize.height));
  }

  protected override performLayout(): void {
    const childSize = this.childSizeFor(this.constraints);
    const tight = BoxConstraints.tightFor({ width: childSize.width, height: childSize.height });
    const children = this.getChildrenAsList();
    for (const child of children) {
      child.layout(tight, { parentUsesSize: true });
    }
    const ordered = this._textDirection === TextDirection.rtl ? [...children].reverse() : children;
    let start = 0;
    for (const child of ordered) {
      place(child, new Offset(start, 0));
      start += child.size.width;
    }
    this.size = this.constraints.constrain(new Size(childSize.width * this.childCount, childSize.height));
  }
}

/** `CupertinoSegmentedControl.build`: the segments centred in their cells, 16 pt in from the sides. */
export function cupertinoSegmentedControl(options: {
  segments: readonly RenderBox[];
  padding?: EdgeInsetsGeometry | null;
  textDirection: TextDirection;
}): RenderBox {
  const cells = options.segments.map(
    (segment) =>
      new RenderPositionedBox({ alignment: Alignment.center, textDirection: options.textDirection, child: segment }),
  );
  const control = new RenderSegmentedControl({ children: cells, textDirection: options.textDirection });
  const unconstrained = new RenderUnconstrainedHeight({ textDirection: options.textDirection, child: control });
  return new RenderPadding({
    padding: options.padding ?? EdgeInsets.symmetric({ horizontal: 16 }),
    textDirection: options.textDirection,
    child: unconstrained,
  });
}

export type CupertinoListType = "base" | "notched";

function sizedBox(width: number | null, height: number | null, child: RenderBox | null = null): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width, height }), child });
}

/** `CupertinoListTile` / `CupertinoListTile.notched` (`_CupertinoListTileState.build`). */
export function cupertinoListTile(options: {
  type: CupertinoListType;
  title: RenderBox;
  subtitle: RenderBox | null;
  additionalInfo: RenderBox | null;
  leading: RenderBox | null;
  trailing: RenderBox | null;
  padding?: EdgeInsetsGeometry | null;
  textDirection: TextDirection;
}): RenderBox {
  const base = options.type === "base";
  const leadingSize = base ? 28 : 30;
  const leadingToTitle = base ? 16 : 12;
  const hasSubtitle = options.subtitle !== null;
  const hasLeading = options.leading !== null;
  let padding: EdgeInsetsGeometry;
  let minHeight: number;
  if (base) {
    padding = EdgeInsetsDirectional.only({ start: 20, end: 14 });
    minHeight = hasSubtitle ? leadingSize + 20 : leadingSize + 16;
  } else if (hasLeading) {
    padding = EdgeInsets.symmetric({ horizontal: 14 });
    minHeight = leadingSize + 24;
  } else {
    padding = EdgeInsetsDirectional.fromSTEB(28, 10, 14, 10);
    minHeight = leadingSize + 20;
  }
  const rowChildren: RenderBox[] = [];
  if (options.leading !== null) {
    rowChildren.push(
      sizedBox(
        leadingSize,
        leadingSize,
        new RenderPositionedBox({
          alignment: Alignment.center,
          textDirection: options.textDirection,
          child: options.leading,
        }),
      ),
      sizedBox(leadingToTitle, null),
    );
  } else {
    rowChildren.push(sizedBox(null, leadingSize));
  }
  const texts: RenderBox[] = [options.title];
  if (options.subtitle !== null) {
    texts.push(sizedBox(null, 3), options.subtitle);
  }
  rowChildren.push(
    expanded(
      new RenderFlex({
        direction: Axis.vertical,
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        crossAxisAlignment: CrossAxisAlignment.start,
        textDirection: options.textDirection,
        children: texts,
      }),
    ),
  );
  if (options.additionalInfo !== null) {
    rowChildren.push(options.additionalInfo);
    if (options.trailing !== null) {
      rowChildren.push(sizedBox(6, null));
    }
  }
  if (options.trailing !== null) {
    rowChildren.push(options.trailing);
  }
  const row = new RenderFlex({
    direction: Axis.horizontal,
    textDirection: options.textDirection,
    children: rowChildren,
  });
  const padded = new RenderPadding({
    padding: options.padding ?? padding,
    textDirection: options.textDirection,
    child: row,
  });
  return new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minWidth: Number.POSITIVE_INFINITY, minHeight }),
    child: new RenderDecoratedBox({ decoration: {}, child: padded }),
  });
}

/** A `Container(color:, height:)`: it fills the width it is offered. */
function divider(height: number, startMargin: number | null, textDirection: TextDirection): RenderBox {
  const line = new RenderDecoratedBox({
    decoration: {},
    child: new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ height }),
      child: new RenderLimitedBox({
        maxWidth: 0,
        maxHeight: 0,
        child: new RenderConstrainedBox({ additionalConstraints: BoxConstraints.expand() }),
      }),
    }),
  });
  return startMargin === null
    ? line
    : new RenderPadding({ padding: EdgeInsetsDirectional.only({ start: startMargin }), textDirection, child: line });
}

/** The margin around a section's rows. */
function rowsMargin(base: boolean, hasHeader: boolean): EdgeInsetsGeometry {
  if (base) {
    return EdgeInsets.only({ bottom: 8 });
  }
  return hasHeader ? EdgeInsetsDirectional.fromSTEB(20, 0, 20, 10) : EdgeInsetsDirectional.fromSTEB(20, 20, 20, 10);
}

/** A section header or footer: padded, at the start edge. */
function startAligned(child: RenderBox, margin: EdgeInsetsDirectional, textDirection: TextDirection): RenderBox {
  return new RenderPositionedBox({
    alignment: AlignmentDirectional.centerStart,
    textDirection,
    child: new RenderPadding({ padding: margin, textDirection, child }),
  });
}

/** The rows of a section with a short divider between them, and long ones around them in the base style. */
function rowsWithDividers(
  rows: readonly RenderBox[],
  base: boolean,
  short: () => RenderBox,
  long: () => RenderBox,
): RenderBox[] {
  const result: RenderBox[] = base ? [long()] : [];
  rows.forEach((row, index) => {
    result.push(row);
    if (index < rows.length - 1) {
      result.push(short());
    }
  });
  if (base) {
    result.push(long());
  }
  return result;
}

/** `CupertinoListSection` and `CupertinoListSection.insetGrouped` (`build`). */
export function cupertinoListSection(options: {
  type: CupertinoListType;
  header: RenderBox | null;
  footer: RenderBox | null;
  rows: readonly RenderBox[];
  hasLeading: boolean;
  topMargin?: number | null;
  devicePixelRatio: number;
  textDirection: TextDirection;
}): RenderBox {
  const base = options.type === "base";
  const dividerHeight = 1 / options.devicePixelRatio;
  const dividerMargin = base ? 20 : 14;
  const additional = base ? 44 : options.hasLeading ? 42 : 14;
  const direction = options.textDirection;
  const column: RenderBox[] = [];
  if (base) {
    column.push(sizedBox(null, options.topMargin ?? 22));
  }
  if (options.header !== null) {
    column.push(
      startAligned(
        options.header,
        base ? EdgeInsetsDirectional.fromSTEB(20, 0, 20, 6) : EdgeInsetsDirectional.fromSTEB(20, 16, 20, 6),
        direction,
      ),
    );
  }
  if (options.rows.length > 0) {
    const short = (): RenderBox => divider(dividerHeight, dividerMargin + additional, direction);
    const withDividers = rowsWithDividers(options.rows, base, short, () => divider(dividerHeight, null, direction));
    const group = new RenderDecoratedBox({
      decoration: {},
      child: new RenderFlex({ direction: Axis.vertical, textDirection: direction, children: withDividers }),
    });
    column.push(
      new RenderPadding({ padding: rowsMargin(base, options.header !== null), textDirection: direction, child: group }),
    );
  }
  if (options.footer !== null) {
    column.push(
      startAligned(
        options.footer,
        base ? EdgeInsetsDirectional.fromSTEB(20, 0, 20, 0) : EdgeInsetsDirectional.fromSTEB(20, 0, 20, 10),
        direction,
      ),
    );
  }
  return new RenderDecoratedBox({
    decoration: {},
    child: new RenderFlex({ direction: Axis.vertical, textDirection: direction, children: column }),
  });
}

/** `CupertinoTextField` (single line): the 7-pt padded editable, as high as one line of its text. */
export function cupertinoTextField(options: {
  line: RenderBox;
  padding?: EdgeInsetsGeometry | null;
  textDirection: TextDirection;
}): RenderBox {
  const editable = new RenderPositionedBox({
    alignment: Alignment.topLeft,
    textDirection: options.textDirection,
    heightFactor: 1,
    child: options.line,
  });
  const padded = new RenderPadding({
    padding: options.padding ?? EdgeInsets.all(7),
    textDirection: options.textDirection,
    child: editable,
  });
  const row = new RenderFlex({
    direction: Axis.horizontal,
    textDirection: options.textDirection,
    children: [expanded(padded)],
  });
  const aligned = new RenderPositionedBox({
    alignment: new Alignment(-1, 0),
    textDirection: options.textDirection,
    widthFactor: 1,
    heightFactor: 1,
    child: row,
  });
  return new RenderDecoratedBox({ decoration: {}, child: aligned });
}

/** `CupertinoTabScaffold.build`: the active tab fills the space, the tab bar sits at the bottom edge. */
export function cupertinoTabScaffold(options: {
  tab: RenderBox;
  tabBar: RenderBox;
  tabBarHeight: number;
  safeBottom: number;
  opaque: boolean;
  textDirection: TextDirection;
}): RenderBox {
  const switching = new RenderStack({
    fit: StackFit.expand,
    textDirection: options.textDirection,
    children: [options.tab],
  });
  const padded = new RenderPadding({
    padding: EdgeInsets.only({ bottom: options.opaque ? options.tabBarHeight + options.safeBottom : 0 }),
    child: switching,
  });
  const bar = new RenderPositionedBox({
    alignment: Alignment.bottomCenter,
    textDirection: options.textDirection,
    child: options.tabBar,
  });
  const stack = new RenderStack({ children: [padded, bar], textDirection: options.textDirection });
  return new RenderDecoratedBox({ decoration: {}, child: stack });
}

/**
 * `CupertinoPicker`: the wheel (a `ListWheelViewport` with the picker's
 * diameter ratio and squeeze) is a positioned fill, and so is the selection
 * overlay, so the picker takes all the room it is given.
 */
export function cupertinoPicker(options: {
  rows: WheelRows;
  itemExtent: number;
  selectedItem?: number;
  offAxisFraction?: number;
  squeeze?: number;
  diameterRatio?: number;
  textDirection: TextDirection;
}): RenderBox {
  const wheel = new RenderListWheelViewport({
    rows: options.rows,
    itemExtent: options.itemExtent,
    selectedItem: options.selectedItem ?? 0,
    offAxisFraction: options.offAxisFraction ?? 0,
    diameterRatio: options.diameterRatio ?? kPickerDiameterRatio,
    squeeze: options.squeeze ?? kPickerSqueeze,
  });
  const fill = positioned(wheel, { top: 0, left: 0, right: 0, bottom: 0 });
  return new RenderDecoratedBox({
    decoration: {},
    child: new RenderStack({ children: [fill], textDirection: options.textDirection }),
  });
}
