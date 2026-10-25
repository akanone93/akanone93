// SPDX-FileCopyrightText: 2014 The Flutter Authors
// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: BSD-3-Clause AND AGPL-3.0-only

// Ported from Flutter 3.47.5 (framework 6a19cca56475dbfba1478ee68d7bd0c2ef891da1): packages/flutter/lib/src/material/button_style_button.dart, elevated_button.dart, filled_button.dart, outlined_button.dart, text_button.dart, icon_button.dart, floating_action_button.dart, button.dart
// Copyright 2014 The Flutter Authors. All rights reserved. Use of this source code is governed by a BSD-style license (BSD-3-Clause).

/**
 * Material 3 buttons: the layout `ButtonStyleButton` builds around its label
 * (tap-target padding, density-adjusted minimum size, scaled padding) and the
 * floating action button's size constraints.
 */

import { BoxConstraints } from "../constraints.ts";
import { Axis, CrossAxisAlignment, FlexFit, FlexParentData, MainAxisSize, RenderFlex } from "../flex.ts";
import { Alignment, EdgeInsetsDirectional, Size, type TextDirection } from "../geometry.ts";
import { RenderConstrainedBox } from "../proxy-box.ts";
import type { RenderBox } from "../render-box.ts";
import {
  RenderAligningShiftedBox,
  RenderPadding,
  RenderPositionedBox,
  RenderShiftedBox,
  setChildOffset,
} from "../shifted-box.ts";
import {
  kMinInteractiveDimension,
  MaterialTapTargetSize,
  type Steb,
  scaledPadding,
  VisualDensity,
} from "./theme.ts";

export type ButtonKind = "elevated" | "filled" | "filledTonal" | "outlined" | "text";

/** `_RenderInputPadding`: grows its child to at least `minSize`, centred. */
export class RenderInputPadding extends RenderShiftedBox {
  constructor(
    private readonly minSize: Size,
    child: RenderBox | null,
  ) {
    super(child);
  }

  protected override computeMinIntrinsicWidth(height: number): number {
    return this.child === null ? 0 : Math.max(this.child.getMinIntrinsicWidth(height), this.minSize.width);
  }

  protected override computeMaxIntrinsicWidth(height: number): number {
    return this.child === null ? 0 : Math.max(this.child.getMaxIntrinsicWidth(height), this.minSize.width);
  }

  protected override computeMinIntrinsicHeight(width: number): number {
    return this.child === null ? 0 : Math.max(this.child.getMinIntrinsicHeight(width), this.minSize.height);
  }

  protected override computeMaxIntrinsicHeight(width: number): number {
    return this.child === null ? 0 : Math.max(this.child.getMaxIntrinsicHeight(width), this.minSize.height);
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child;
    if (child === null) {
      return Size.zero;
    }
    const s = child.getDryLayout(constraints);
    return constraints.constrain(new Size(Math.max(s.width, this.minSize.width), Math.max(s.height, this.minSize.height)));
  }

  protected override performLayout(): void {
    const child = this.child;
    if (child === null) {
      this.size = Size.zero;
      return;
    }
    const constraints = this.constraints;
    child.layout(constraints, { parentUsesSize: true });
    this.size = constraints.constrain(
      new Size(Math.max(child.size.width, this.minSize.width), Math.max(child.size.height, this.minSize.height)),
    );
    setChildOffset(child, Alignment.center.alongOffset(this.size.minus(child.size)));
  }
}

/** The tap target a button is padded to under `tapTargetSize` and `density`. */
export function tapTargetMinSize(tapTargetSize: MaterialTapTargetSize, density: VisualDensity): Size {
  return tapTargetSize === MaterialTapTargetSize.padded
    ? new Size(
        kMinInteractiveDimension + density.baseSizeAdjustmentX,
        kMinInteractiveDimension + density.baseSizeAdjustmentY,
      )
    : Size.zero;
}

/** The label's role-independent options of [buildButton]. */
export interface ButtonOptions {
  kind: ButtonKind | "icon";
  /** The label, or for an icon button the icon. */
  label: RenderBox;
  /** The leading icon of a `.icon` button. */
  icon?: RenderBox | null;
  tapTargetSize: MaterialTapTargetSize;
  density: VisualDensity;
  textDirection: TextDirection;
  /** `MediaQuery.textScalerOf(context).scale(14) / 14`. */
  textScale: number;
}

function stebPadding(kind: ButtonOptions["kind"], hasIcon: boolean, scale: number): Steb {
  switch (kind) {
    case "icon":
      return [8, 8, 8, 8];
    case "text":
      return hasIcon
        ? scaledPadding([12, 8, 16, 8], [4, 0, 4, 0], [4, 0, 4, 0], scale)
        : scaledPadding([12, 8, 12, 8], [8, 0, 8, 0], [4, 0, 4, 0], scale);
    default:
      return hasIcon
        ? scaledPadding([16, 0, 24, 0], [8, 0, 12, 0], [4, 0, 6, 0], scale)
        : scaledPadding([24, 0, 24, 0], [12, 0, 12, 0], [6, 0, 6, 0], scale);
  }
}

/** The widget chain `ButtonStyleButton.build` produces, down to the label. */
export function buildButton(o: ButtonOptions): RenderBox {
  const hasIcon = o.icon !== null && o.icon !== undefined;
  const scale = Math.min(2, Math.max(1, o.textScale)) - 1;
  let content: RenderBox = o.label;
  if (hasIcon) {
    const flexible = new FlexParentData();
    flexible.flex = 1;
    flexible.fit = FlexFit.loose;
    o.label.parentData = flexible;
    content = new RenderFlex({
      direction: Axis.horizontal,
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.center,
      textDirection: o.textDirection,
      spacing: 8 + (4 - 8) * scale,
      children: [o.icon as RenderBox, o.label],
    });
  }
  const [s, t, e, b] = stebPadding(o.kind, hasIcon, o.textScale);
  const dx = Math.max(0, o.density.baseSizeAdjustmentX);
  const dy = o.density.baseSizeAdjustmentY;
  const padded = new RenderPadding({
    padding: EdgeInsetsDirectional.fromSTEB(
      Math.max(0, s + dx),
      Math.max(0, t + dy),
      Math.max(0, e + dx),
      Math.max(0, b + dy),
    ),
    textDirection: o.textDirection,
    child: new RenderPositionedBox({
      alignment: Alignment.center,
      widthFactor: 1,
      heightFactor: 1,
      child: content,
    }),
  });
  const min = o.kind === "icon" ? 40 : 64;
  const constrained = new RenderConstrainedBox({
    additionalConstraints: o.density.effectiveConstraints(
      new BoxConstraints({ minWidth: min, minHeight: 40 }),
    ),
    child: padded,
  });
  return new RenderInputPadding(tapTargetMinSize(o.tapTargetSize, o.density), constrained);
}

export type FabKind = "regular" | "small" | "large" | "extended";

/** `_ChildOverflowBox`: sizes to the child within the constraints, centring it, lets it overflow. */
class RenderChildOverflowBox extends RenderAligningShiftedBox {
  constructor(textDirection: TextDirection, child: RenderBox) {
    super({ alignment: Alignment.center, textDirection, child });
  }

  protected override computeMinIntrinsicWidth(_height: number): number {
    return 0;
  }

  protected override computeMinIntrinsicHeight(_width: number): number {
    return 0;
  }

  protected override computeDryLayout(constraints: BoxConstraints): Size {
    const child = this.child;
    if (child === null) {
      return constraints.biggest;
    }
    const s = child.getDryLayout(new BoxConstraints());
    return new Size(
      Math.max(constraints.minWidth, Math.min(constraints.maxWidth, s.width)),
      Math.max(constraints.minHeight, Math.min(constraints.maxHeight, s.height)),
    );
  }

  protected override performLayout(): void {
    const child = this.child;
    const constraints = this.constraints;
    if (child === null) {
      this.size = constraints.biggest;
      return;
    }
    child.layout(new BoxConstraints(), { parentUsesSize: true });
    this.size = new Size(
      Math.max(constraints.minWidth, Math.min(constraints.maxWidth, child.size.width)),
      Math.max(constraints.minHeight, Math.min(constraints.maxHeight, child.size.height)),
    );
    this.alignChild();
  }
}

export interface FabOptions {
  kind: FabKind;
  child: RenderBox | null;
  /** The label of an extended FAB. */
  label?: RenderBox | null;
  tapTargetSize: MaterialTapTargetSize;
  textDirection: TextDirection;
}

/** `FloatingActionButton.build` (M3 defaults) around `RawMaterialButton`. */
export function buildFab(o: FabOptions): RenderBox {
  let constraints: BoxConstraints;
  let content: RenderBox | null = o.child;
  switch (o.kind) {
    case "small":
      constraints = BoxConstraints.tightFor({ width: 40, height: 40 });
      break;
    case "large":
      constraints = BoxConstraints.tightFor({ width: 96, height: 96 });
      break;
    case "extended": {
      constraints = BoxConstraints.tightFor({ height: 56 });
      const parts = [o.child, o.label ?? null].filter((c): c is RenderBox => c !== null);
      const spaced: RenderBox[] = [];
      for (const [i, part] of parts.entries()) {
        if (i > 0) {
          spaced.push(new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 8 }) }));
        }
        spaced.push(part);
      }
      content = new RenderChildOverflowBox(
        o.textDirection,
        new RenderPadding({
          padding: EdgeInsetsDirectional.only({ start: o.child !== null ? 16 : 20, end: 20 }),
          textDirection: o.textDirection,
          child: new RenderFlex({
            direction: Axis.horizontal,
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.center,
            textDirection: o.textDirection,
            children: spaced,
          }),
        }),
      );
      break;
    }
    default:
      constraints = BoxConstraints.tightFor({ width: 56, height: 56 });
  }
  const raw = new RenderConstrainedBox({
    additionalConstraints: VisualDensity.standard.effectiveConstraints(constraints),
    child: new RenderPadding({
      padding: EdgeInsetsDirectional.zero,
      textDirection: o.textDirection,
      child: new RenderPositionedBox({ alignment: Alignment.center, widthFactor: 1, heightFactor: 1, child: content }),
    }),
  });
  return new RenderInputPadding(tapTargetMinSize(o.tapTargetSize, VisualDensity.standard), raw);
}
