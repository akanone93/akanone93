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
import { cupertinoSlidingSegment, cupertinoSlidingSegmentedControl } from "../flutter/cupertino-sliding.ts";
import { TextDirection } from "../flutter/geometry.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
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
  ["CupertinoSwitch", (node, ctx) => marked(node, ctx, cupertinoSwitch())],
  ["CupertinoSlider", (node, ctx) => marked(node, ctx, cupertinoSlider())],
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
      const rows = (node.children ?? []).map((child) => ctx.paragraph(labelOf(child) ?? ""));
      const picker = cupertinoPicker({ rows, itemExtent, textDirection: direction });
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
