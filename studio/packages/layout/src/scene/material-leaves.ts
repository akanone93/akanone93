// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The Material leaf widgets of the layout conformance scenes (STU-005, S4.6):
 * selection controls, dividers, progress indicators, avatars, cards and
 * badges. Each is a box of the size its Flutter widget lays out to, which the
 * goldens of `material-leaves` and `material-chips-action-choice` check.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { EdgeInsets } from "../flutter/geometry.ts";
import { kMinInteractiveDimension, MaterialTapTargetSize, VisualDensity } from "../flutter/material/theme.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPadding } from "../flutter/shifted-box.ts";
import type { MaterialContext } from "./material-scene.ts";
import { padding, type SceneNode } from "./scene.ts";

type LeafBuilder = (node: SceneNode, ctx: MaterialContext) => RenderBox;

/** A prop that is a number, else `fallback`. */
function number(node: SceneNode, name: string, fallback: number): number {
  const value = node.props[name];
  return typeof value === "number" ? value : fallback;
}

function tight(width: number, height: number): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width, height }) });
}

/** The side of a selection control's tap target: 48 padded, 40 shrink-wrapped, less the density's adjustment. */
function targetSide(node: SceneNode): number {
  const shrink =
    node.props["tapTargetSize"] === "shrinkWrap" ? MaterialTapTargetSize.shrinkWrap : MaterialTapTargetSize.padded;
  const density = VisualDensity.named(node.props["density"]);
  const base = shrink === MaterialTapTargetSize.shrinkWrap ? kMinInteractiveDimension - 8 : kMinInteractiveDimension;
  return base + density.baseSizeAdjustmentX;
}

/** A box as wide (`axis` horizontal) or as high as its constraints allow and `thickness` across. */
function expanding(axis: "horizontal" | "vertical", across: number): RenderBox {
  return new RenderConstrainedBox({
    additionalConstraints:
      axis === "horizontal"
        ? new BoxConstraints({ minWidth: Number.POSITIVE_INFINITY, minHeight: across, maxHeight: across })
        : new BoxConstraints({ minHeight: Number.POSITIVE_INFINITY, minWidth: across, maxWidth: across }),
  });
}

const selection: LeafBuilder = (node) => {
  const side = targetSide(node);
  return tight(side, side);
};

/** `Switch`: 60 by 48 padded. */
const switchControl: LeafBuilder = (node) =>
  tight(
    60 + VisualDensity.named(node.props["density"]).baseSizeAdjustmentX,
    node.props["tapTargetSize"] === "shrinkWrap" ? 40 : 48,
  );

/** A `SearchBar`, and the bar that opens a `SearchAnchor`'s view. */
const searchBar: LeafBuilder = () =>
  new RenderConstrainedBox({
    additionalConstraints: new BoxConstraints({ minWidth: 360, maxWidth: 800, minHeight: 56 }),
    child: expanding("horizontal", 56),
  });

/** The leaf builders, merged into the Material composites. */
export const leafBuilders: Readonly<Record<string, LeafBuilder>> = {
  // `BackButton` and `CloseButton` are `IconButton`s: 48 by 48.
  BackButton: () => tight(kMinInteractiveDimension, kMinInteractiveDimension),
  CloseButton: () => tight(kMinInteractiveDimension, kMinInteractiveDimension),
  // M3 `SearchBar`: 56 high, 360 to 800 wide, as wide as its parent allows.
  SearchBar: searchBar,
  SearchAnchor: searchBar,
  // A `BottomNavigationBar` is 58 high: the bar's 56 and its 2-px border.
  BottomNavigationBar: () => expanding("horizontal", 58),
  // A slider is as wide as it may be and 48 high (its tap target).
  Slider: () => expanding("horizontal", kMinInteractiveDimension),
  RangeSlider: () => expanding("horizontal", kMinInteractiveDimension),
  Checkbox: selection,
  Radio: selection,
  Switch: switchControl,
  Divider: (node) => expanding("horizontal", number(node, "height", 16)),
  VerticalDivider: (node) => expanding("vertical", number(node, "width", 16)),
  CircleAvatar: (node) => {
    const side = 2 * number(node, "radius", 20);
    return tight(side, side);
  },
  CircularProgressIndicator: () =>
    new RenderConstrainedBox({ additionalConstraints: new BoxConstraints({ minWidth: 36, minHeight: 36 }) }),
  LinearProgressIndicator: (node) => expanding("horizontal", number(node, "minHeight", 4)),
  Card: (node, ctx) => {
    const child = node.children?.find((c) => (c.slot ?? "child") === "child");
    const margin = node.props["margin"];
    return new RenderPadding({
      padding: margin === undefined ? EdgeInsets.all(4) : padding(margin),
      textDirection: ctx.direction,
      child: child === undefined ? null : ctx.build(child),
    });
  },
  /** A badge's box is its child's: the label is positioned over it without taking space. */
  Badge: (node, ctx) => {
    const child = node.children?.find((c) => (c.slot ?? "child") === "child");
    if (child === undefined) {
      throw new Error(`node "${node.id}": a Badge needs a child`);
    }
    // A box of its own, so the badge and its child each keep their rect.
    return new RenderPadding({ padding: EdgeInsets.zero, textDirection: ctx.direction, child: ctx.build(child) });
  },
};
