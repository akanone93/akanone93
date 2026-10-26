// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Builders for the Material composites of a screen document beyond the basic
 * controls (STU-005, S4.3): tab bars, dialogs, sheets and menus. Each lays out
 * with the metrics of the Flutter widget (`../flutter/material/`, checked
 * against Flutter's goldens by the layout conformance suite) and the canvas
 * fonts.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { TextDirection } from "../flutter/geometry.ts";
import { buildBottomSheet } from "../flutter/material/bottom-sheet.ts";
import { buildAlertDialog } from "../flutter/material/dialog.ts";
import { buildPopupMenu, buildPopupMenuDivider, buildPopupMenuItem } from "../flutter/material/popup-menu.ts";
import { buildTab, buildTabBar, type TabAlignment, tabPreferredHeight } from "../flutter/material/tab-bar.ts";
import { m3TextStyle, type TextRole } from "../flutter/material/theme.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import type { TextStyle } from "../flutter/text-style.ts";
import type { WidgetBuilder, WidgetContext, WidgetNode } from "./screen.ts";

const direction = TextDirection.ltr;

function prop(node: WidgetNode, name: string): unknown {
  return (node.props as Record<string, unknown> | undefined)?.[name];
}

function textOf(node: WidgetNode): string | undefined {
  if (node.type === "Text" && typeof node.props?.data === "string") {
    return node.props.data;
  }
  for (const child of [...(node.children ?? []), ...Object.values(node.slots ?? {})]) {
    const label = textOf(child);
    if (label !== undefined) {
      return label;
    }
  }
  return undefined;
}

function str(node: WidgetNode, name: string): string | null {
  const value = prop(node, name);
  return typeof value === "string" ? value : null;
}

/** The theme's Material 3 text style for `role`, in the screen theme's font families. */
function role(ctx: WidgetContext, name: TextRole): TextStyle {
  const [family, ...fallback] = ctx.theme.fontFamilies;
  return { ...m3TextStyle(name, family), fontFamilyFallback: fallback };
}

function named(node: WidgetNode, ctx: WidgetContext, box: RenderBox, label?: string): RenderBox {
  ctx.mark(node.id, "box", label);
  box.debugId = node.id;
  return box;
}

/** The 24 by 24 square an icon takes. */
const iconBox = (): RenderBox =>
  new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 24, height: 24 }) });

/** A `TabBar` of `Tab` children: `props.text` and an `icon` slot each. */
const tabBar: WidgetBuilder = (node, ctx) => {
  const isScrollable = prop(node, "isScrollable") === true;
  const tabs = (node.children ?? []).map((tab) => {
    const label = str(tab, "text");
    const hasIcon = tab.slots?.["icon"] !== undefined;
    const given = prop(tab, "height");
    const height = typeof given === "number" ? given : null;
    const box = buildTab({
      icon: hasIcon ? iconBox() : null,
      text: label === null ? null : ctx.paragraph(label, role(ctx, "titleSmall")),
      height,
    });
    box.debugId = tab.id;
    ctx.mark(tab.id, "box", label ?? undefined);
    return { box, preferredHeight: tabPreferredHeight({ hasIcon, hasText: label !== null, height }) };
  });
  const alignment = str(node, "tabAlignment") ?? (isScrollable ? "startOffset" : "fill");
  return named(
    node,
    ctx,
    buildTabBar({ tabs, isScrollable, tabAlignment: alignment as TabAlignment, textDirection: direction }),
  );
};

/** The text of the widget in `node`'s slot `name`, drawn in the role's style; null when the slot is empty. */
function slotText(node: WidgetNode, ctx: WidgetContext, name: string, style: TextRole): RenderBox | null {
  const found = node.slots?.[name];
  const label = found === undefined ? undefined : textOf(found);
  return label === undefined ? null : ctx.paragraph(label, role(ctx, style));
}

/** An `AlertDialog` static state: `title` and `content` slots, its children the actions. */
const alertDialog: WidgetBuilder = (node, ctx) => {
  const actions = (node.children ?? []).map((a) => ctx.build(a));
  return named(
    node,
    ctx,
    buildAlertDialog({
      title: slotText(node, ctx, "title", "headlineSmall"),
      content: slotText(node, ctx, "content", "bodyMedium"),
      actions: actions.length === 0 ? null : actions,
      textScale: 1,
      textDirection: direction,
    }),
  );
};

/** A `BottomSheet` in a screen: as wide as the screen, its `child` slot below the optional drag handle. */
const bottomSheet: WidgetBuilder = (node, ctx) => {
  const child = node.slots?.["child"];
  return named(
    node,
    ctx,
    new RenderConstrainedBox({
      additionalConstraints: BoxConstraints.tightFor({ width: ctx.available() }),
      child: buildBottomSheet({
        child:
          child === undefined
            ? new RenderConstrainedBox({ additionalConstraints: new BoxConstraints() })
            : ctx.build(child),
        showDragHandle: prop(node, "showDragHandle") === true,
        isScrollControlled: false,
        textDirection: direction,
      }),
    }),
  );
};

/** A popup menu in its open state: `PopupMenuItem` children with a label each, and `PopupMenuDivider`s. */
const popupMenu: WidgetBuilder = (node, ctx) => {
  const items = (node.children ?? []).map((entry) => {
    const given = prop(entry, "height");
    const height = typeof given === "number" ? given : null;
    if (entry.type === "PopupMenuDivider") {
      return buildPopupMenuDivider(height);
    }
    const label = textOf(entry) ?? "";
    const box = buildPopupMenuItem(ctx.paragraph(label, role(ctx, "labelLarge")), height, direction);
    box.debugId = entry.id;
    ctx.mark(entry.id, "box", label);
    return box;
  });
  return named(node, ctx, buildPopupMenu(items, direction));
};

/** The builders of the Material composite types; merged into the default registry. */
export const materialWidgetBuilders: ReadonlyArray<readonly [string, WidgetBuilder]> = [
  ["TabBar", tabBar],
  ["AlertDialog", alertDialog],
  ["BottomSheet", bottomSheet],
  ["PopupMenu", popupMenu],
];
