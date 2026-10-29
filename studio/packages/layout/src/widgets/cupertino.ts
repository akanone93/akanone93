// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Builders for the Cupertino widget types of a screen document (STU-005,
 * S4.4): each one lays out with the metrics of the Flutter widget
 * (`../flutter/cupertino.ts`, checked against Flutter's goldens by the layout
 * conformance suite) and the canvas fonts; SF Pro is not available, so the
 * device preview stays authoritative for text.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import {
  type CupertinoButtonSize,
  cupertinoActivityIndicator,
  cupertinoButton,
  cupertinoListTile,
  cupertinoNavigationBar,
  cupertinoPicker,
  cupertinoSegmentedControl,
  cupertinoSlider,
  cupertinoSwitch,
  cupertinoTabBar,
  cupertinoTextField,
  navBarEdge,
} from "../flutter/cupertino.ts";
import {
  cupertinoContextMenu,
  cupertinoContextMenuAction,
  cupertinoContextMenuActionFontSize,
} from "../flutter/cupertino-context-menu.ts";
import { cupertinoDatePicker, kDatePickerItemExtent } from "../flutter/cupertino-date-picker.ts";
import {
  actionSheetActionFontSize,
  cupertinoActionSheet,
  cupertinoActionSheetAction,
  cupertinoAlertDialog,
  cupertinoDialogAction,
  cupertinoDialogTextStyles,
} from "../flutter/cupertino-dialog.ts";
import { cupertinoSlidingSegment, cupertinoSlidingSegmentedControl } from "../flutter/cupertino-sliding.ts";
import { Size, TextDirection } from "../flutter/geometry.ts";
import { listRows } from "../flutter/list-wheel.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import type { TextStyle } from "../flutter/text-style.ts";
import { TextAlign } from "../text/paragraph-engine.ts";
import type { WidgetBuilder, WidgetContext, WidgetNode } from "./screen.ts";

const direction = TextDirection.ltr;

function labelOf(node: WidgetNode): string | undefined {
  if (node.type === "Text" && typeof node.props?.data === "string") {
    return node.props.data;
  }
  for (const child of node.children ?? []) {
    const text = labelOf(child);
    if (text !== undefined) {
      return text;
    }
  }
  return undefined;
}

function marked(node: WidgetNode, ctx: WidgetContext, box: RenderBox, label?: string): RenderBox {
  ctx.mark(node.id, "box", label);
  box.debugId = node.id;
  return box;
}

function wide(ctx: WidgetContext, child: RenderBox): RenderBox {
  return new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({ width: ctx.available() }),
    child,
  });
}

function data(node: WidgetNode): Record<string, unknown> {
  const value = node.props?.data;
  return typeof value === "object" && value !== null ? (value as Record<string, unknown>) : {};
}

function paragraph(ctx: WidgetContext, text: string | undefined, fallback: string): RenderBox {
  return ctx.paragraph(text ?? fallback);
}

/** `CupertinoDatePicker` and `CupertinoPicker` are 216 pt tall in Flutter's own examples and HIG. */
const kPickerHeight = 216;

function labelsOf(node: WidgetNode, type: string): string[] {
  return (node.children ?? []).filter((child) => child.type === type).map((child) => labelOf(child) ?? "");
}

function dataText(node: WidgetNode, name: string): string | undefined {
  const value = data(node)[name];
  return typeof value === "string" && value !== "" ? value : undefined;
}

/** `CupertinoAlertDialog` as a static state: the dialog's own metrics at the regular text size. */
function alertDialog(node: WidgetNode, ctx: WidgetContext): RenderBox {
  const styles = cupertinoDialogTextStyles;
  const part = (name: string, style: TextStyle): RenderBox | null => {
    const label = dataText(node, name);
    if (label === undefined) {
      return null;
    }
    // CupertinoAlertDialog sets `DefaultTextStyle(textAlign: TextAlign.center)` over its title and content.
    const text = ctx.paragraph(label, style);
    text.textAlign = TextAlign.center;
    return text;
  };
  const actions = labelsOf(node, "CupertinoDialogAction").map((label) =>
    cupertinoDialogAction({
      child: ctx.paragraph(label, styles.action),
      fontSize: styles.action.fontSize,
      textScaleFactor: 1,
      textDirection: direction,
    }),
  );
  return wide(
    ctx,
    cupertinoAlertDialog({
      title: part("title", styles.title),
      content: part("content", styles.content),
      actions,
      textScaleFactor: 1,
      textDirection: direction,
    }),
  );
}

/** `CupertinoActionSheet` as a static state: the sheet across the screen width, with its cancel button. */
function actionSheet(node: WidgetNode, ctx: WidgetContext): RenderBox {
  const content = cupertinoDialogTextStyles.sheetContent;
  const actionStyle = { fontSize: actionSheetActionFontSize(1) };
  const action = (label: string): RenderBox =>
    cupertinoActionSheetAction({
      child: ctx.paragraph(label, actionStyle),
      textScaleFactor: 1,
      textDirection: direction,
    });
  const title = dataText(node, "title");
  const message = dataText(node, "message");
  const cancel = dataText(node, "cancel");
  return wide(
    ctx,
    cupertinoActionSheet({
      title:
        title === undefined
          ? null
          : ctx.paragraph(title, { ...content, fontWeight: message === undefined ? 400 : 600 }),
      message:
        message === undefined
          ? null
          : ctx.paragraph(message, { ...content, fontWeight: title === undefined ? 600 : 400 }),
      actions: labelsOf(node, "CupertinoActionSheetAction").map(action),
      cancel: cancel === undefined ? null : action(cancel),
      screen: new Size(ctx.available(), Number.POSITIVE_INFINITY),
      safeTop: 0,
      safeBottom: 0,
      textDirection: direction,
    }),
  );
}

/** The sample rows of each mode's columns: the selected value of 9 April 2018 10:30, as a screen shows the picker at rest. */
const datePickerColumns: Readonly<Record<string, readonly string[]>> = {
  time: ["10", "30", "AM"],
  dateAndTime: ["Mon Apr 9", "10", "30", "AM"],
  date: ["April", "9", "2018"],
};

/** `CupertinoDatePicker`: a 216-pt wheel per column across the available width. */
function datePicker(node: WidgetNode, ctx: WidgetContext): RenderBox {
  const mode = String(data(node)["mode"] ?? "dateAndTime");
  const texts = datePickerColumns[mode] ?? datePickerColumns["dateAndTime"] ?? [];
  const rows = texts.map((text) => ctx.paragraph(text));
  const picker = cupertinoDatePicker({
    kind: mode === "date" ? "date" : "dateTime",
    pickers: rows.map((row) => ({ exists: (index: number) => index === 0, build: () => row, selectedItem: 0 })),
    columnWidths: rows.map((row) => row.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY)),
    itemExtent: kDatePickerItemExtent,
    textDirection: direction,
  });
  return new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({ width: ctx.available(), height: kPickerHeight }),
    child: picker,
  });
}

const sizes = new Set(["small", "medium", "large"]);

/** The Cupertino builders; [defaultWidgetBuilders] includes them. */
export const cupertinoWidgetBuilders: ReadonlyMap<string, WidgetBuilder> = new Map<string, WidgetBuilder>([
  [
    "CupertinoButton",
    (node, ctx) => {
      const label = labelOf(node);
      const size = String(data(node)["sizeStyle"] ?? "large");
      return marked(
        node,
        ctx,
        cupertinoButton({
          size: (sizes.has(size) ? size : "large") as CupertinoButtonSize,
          textDirection: direction,
          child: paragraph(ctx, label, "Button"),
        }),
        label,
      );
    },
  ],
  ["CupertinoAlertDialog", (node, ctx) => marked(node, ctx, alertDialog(node, ctx))],
  ["CupertinoActionSheet", (node, ctx) => marked(node, ctx, actionSheet(node, ctx))],
  ["CupertinoDatePicker", (node, ctx) => marked(node, ctx, datePicker(node, ctx))],
  [
    "CupertinoContextMenu",
    // The static state is the closed menu: its child; the actions only show in the open sheet.
    (node, ctx) => {
      const child = (node.children ?? []).find((c) => c.type !== "CupertinoContextMenuAction");
      const label = labelOf(node);
      return marked(
        node,
        ctx,
        cupertinoContextMenu(child === undefined ? paragraph(ctx, label, "Menu") : ctx.build(child)),
        label,
      );
    },
  ],
  [
    "CupertinoContextMenuAction",
    (node, ctx) => {
      const label = labelOf(node);
      const row = cupertinoContextMenuAction({
        child: ctx.paragraph(label ?? "Action", { fontSize: cupertinoContextMenuActionFontSize }),
        textDirection: direction,
      });
      return marked(node, ctx, wide(ctx, row), label);
    },
  ],
  ["CupertinoSwitch", (node, ctx) => marked(node, ctx, cupertinoSwitch())],
  ["CupertinoSlider", (node, ctx) => marked(node, ctx, cupertinoSlider())],
  ["CupertinoActivityIndicator", (node, ctx) => marked(node, ctx, cupertinoActivityIndicator())],
  [
    "CupertinoTextField",
    (node, ctx) => {
      const label = labelOf(node) ?? String(data(node)["placeholder"] ?? "");
      return marked(
        node,
        ctx,
        wide(
          ctx,
          cupertinoTextField({ line: paragraph(ctx, label === "" ? undefined : label, "A"), textDirection: direction }),
        ),
        label,
      );
    },
  ],
  [
    "CupertinoNavigationBar",
    (node, ctx) => {
      const title = labelOf(node) ?? String(data(node)["title"] ?? "");
      const leading = typeof data(node)["leading"] === "string" ? String(data(node)["leading"]) : null;
      const trailing = typeof data(node)["trailing"] === "string" ? String(data(node)["trailing"]) : null;
      const bar = cupertinoNavigationBar({
        leading: leading === null ? null : navBarEdge(ctx.paragraph(leading), "start", direction),
        middle: title === "" ? null : ctx.paragraph(title),
        trailing: trailing === null ? null : navBarEdge(ctx.paragraph(trailing), "end", direction),
        largeTitle: null,
        safeTop: 0,
        textDirection: direction,
      });
      return marked(node, ctx, wide(ctx, bar), title);
    },
  ],
  [
    "CupertinoTabBar",
    (node, ctx) => {
      const items = (node.children ?? []).map((child) => ({
        icon: new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 28, height: 28 }) }),
        label: ctx.paragraph(labelOf(child) ?? ""),
      }));
      return marked(
        node,
        ctx,
        wide(ctx, cupertinoTabBar({ height: 50, bottomPadding: 0, items, textDirection: direction })),
      );
    },
  ],
  [
    "CupertinoListTile",
    (node, ctx) => {
      const label = labelOf(node);
      return marked(
        node,
        ctx,
        wide(
          ctx,
          cupertinoListTile({
            type: "base",
            title: paragraph(ctx, label, "Title"),
            subtitle: null,
            additionalInfo: null,
            leading: null,
            trailing: null,
            textDirection: direction,
          }),
        ),
        label,
      );
    },
  ],
  [
    "CupertinoSegmentedControl",
    (node, ctx) => {
      const segments = (node.children ?? []).map((child) => ctx.paragraph(labelOf(child) ?? ""));
      return marked(node, ctx, wide(ctx, cupertinoSegmentedControl({ segments, textDirection: direction })));
    },
  ],
  [
    "CupertinoPicker",
    (node, ctx) => {
      const props = data(node);
      const itemExtent = typeof props["itemExtent"] === "number" ? props["itemExtent"] : 32;
      const given = data(node)["items"];
      const items =
        (node.children ?? []).length > 0
          ? (node.children ?? []).map((child) => labelOf(child) ?? "")
          : Array.isArray(given)
            ? given.map(String)
            : [];
      const selected = data(node)["selectedItem"];
      const picker = cupertinoPicker({
        rows: listRows(items.map((item) => ctx.paragraph(item))),
        selectedItem:
          typeof selected === "number" && items.length > 0
            ? Math.min(Math.max(Math.trunc(selected), 0), items.length - 1)
            : 0,
        itemExtent,
        textDirection: direction,
      });
      return marked(
        node,
        ctx,
        new RenderConstrainedBox({
          additionalConstraints: BoxConstraints.tightFor({ width: ctx.available(), height: kPickerHeight }),
          child: picker,
        }),
      );
    },
  ],
  [
    "CupertinoSlidingSegmentedControl",
    (node, ctx) => {
      const segments = (node.children ?? []).map((child) =>
        cupertinoSlidingSegment({
          content: ctx.paragraph(labelOf(child) ?? ""),
          highlightedContent: ctx.paragraph(labelOf(child) ?? ""),
          textDirection: direction,
        }),
      );
      const proportionalWidth = data(node)["proportionalWidth"] === true;
      return marked(
        node,
        ctx,
        wide(ctx, cupertinoSlidingSegmentedControl({ segments, proportionalWidth, textDirection: direction })),
      );
    },
  ],
  [
    "CupertinoSliverNavigationBar",
    (node, ctx) => {
      // A screen is a column of boxes: the bar is shown expanded, as the static large-title bar.
      const title = labelOf(node) ?? String(data(node)["title"] ?? "");
      const bar = cupertinoNavigationBar({
        leading: null,
        middle: null,
        trailing: null,
        largeTitle: ctx.paragraph(title === "" ? "Title" : title),
        safeTop: 0,
        textDirection: direction,
      });
      return marked(node, ctx, wide(ctx, bar), title);
    },
  ],
]);
