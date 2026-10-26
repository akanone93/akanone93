// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * `CupertinoAlertDialog` and `CupertinoActionSheet` of the layout conformance
 * scenes (STU-005, S4.4), as the static canvas states. A dialog's `title` and
 * `content` and a sheet's `title`, `message` and `cancelButton` are
 * `CupertinoSlot` children; the actions are `CupertinoDialogAction` and
 * `CupertinoActionSheetAction` children, whose label is their one child.
 */

import {
  actionSheetActionFontSize,
  cupertinoActionSheet,
  cupertinoActionSheetAction,
  cupertinoAlertDialog,
  cupertinoContextMenu,
  cupertinoContextMenuAction,
  cupertinoContextMenuActionFontSize,
  cupertinoDialogAction,
  cupertinoDialogTextStyles,
  isInAccessibilityMode,
  type RenderBox,
  Size,
} from "../index.ts";
import { type CupertinoContext, only, scaled, slotOf, withDefaultText } from "./cupertino-common.ts";
import type { SceneNode, SceneText } from "./scene.ts";

/** The node types of this file. */
export const cupertinoDialogTypes: ReadonlySet<string> = new Set([
  "CupertinoAlertDialog",
  "CupertinoDialogAction",
  "CupertinoActionSheet",
  "CupertinoActionSheetAction",
  "CupertinoContextMenu",
  "CupertinoContextMenuAction",
]);

/** Types that take several children. */
export const cupertinoDialogMultiChildTypes: ReadonlySet<string> = new Set([
  "CupertinoAlertDialog",
  "CupertinoActionSheet",
  "CupertinoContextMenu",
]);

type Style = { fontSize: number; fontWeight: number; height?: number; letterSpacing?: number };

function slotChild(
  node: SceneNode,
  name: string,
  ctx: CupertinoContext,
  text: SceneText | null,
  style: Style,
): RenderBox | null {
  const slot = slotOf(node, name);
  if (slot === undefined) {
    return null;
  }
  return ctx.build(withDefaultText(only(slot), { ...style, textAlign: "center" }), text);
}

function actionsOf(node: SceneNode, type: string, ctx: CupertinoContext, text: SceneText | null): RenderBox[] {
  return (node.children ?? []).filter((child) => child.type === type).map((child) => ctx.build(child, text));
}

function flag(node: SceneNode, name: string): boolean {
  return node.props[name] === true;
}

function dialogText(ctx: CupertinoContext): { text: SceneText | null; factor: number } {
  const text = scaled(ctx.text, (factor) => Math.max(1, factor));
  return { text, factor: text?.textScaler.textScaleFactor ?? 1 };
}

function buildAlertDialog(node: SceneNode, ctx: CupertinoContext): RenderBox {
  const { text, factor } = dialogText(ctx);
  const styles = cupertinoDialogTextStyles;
  return cupertinoAlertDialog({
    title: slotChild(node, "title", ctx, text, styles.title),
    content: slotChild(node, "content", ctx, text, styles.content),
    actions: actionsOf(node, "CupertinoDialogAction", ctx, text),
    textScaleFactor: factor,
    textDirection: ctx.textDirection,
  });
}

function buildDialogAction(node: SceneNode, ctx: CupertinoContext): RenderBox {
  const { text, factor } = dialogText(ctx);
  const { fontSize } = cupertinoDialogTextStyles.action;
  const regular = !isInAccessibilityMode(factor);
  const child = ctx.build(
    withDefaultText(only(node), {
      fontSize,
      fontWeight: flag(node, "isDefaultAction") ? 600 : 400,
      textAlign: "center",
      ...(regular ? { maxLines: 1, ellipsis: true } : {}),
    }),
    text,
  );
  return cupertinoDialogAction({ child, fontSize, textScaleFactor: factor, textDirection: ctx.textDirection });
}

function numberOr(node: SceneNode, name: string, fallback: number): number {
  const value = node.props[name];
  return typeof value === "number" ? value : fallback;
}

function buildActionSheet(node: SceneNode, ctx: CupertinoContext): RenderBox {
  const content = cupertinoDialogTextStyles.sheetContent;
  const hasMessage = slotOf(node, "message") !== undefined;
  const hasTitle = slotOf(node, "title") !== undefined;
  const cancel = slotOf(node, "cancelButton");
  const screen = ctx.text?.screen ?? { width: 360, height: 800 };
  return cupertinoActionSheet({
    title: slotChild(node, "title", ctx, ctx.text, { ...content, fontWeight: hasMessage ? 600 : 400 }),
    message: slotChild(node, "message", ctx, ctx.text, { ...content, fontWeight: hasTitle ? 400 : 600 }),
    actions: actionsOf(node, "CupertinoActionSheetAction", ctx, ctx.text),
    cancel: cancel === undefined ? null : ctx.build(only(cancel), ctx.text),
    screen: new Size(screen.width, screen.height),
    safeTop: numberOr(node, "safeTop", 0),
    safeBottom: numberOr(node, "safeBottom", 0),
    textDirection: ctx.textDirection,
  });
}

function buildActionSheetAction(node: SceneNode, ctx: CupertinoContext): RenderBox {
  const factor = ctx.text?.textScaler.textScaleFactor ?? 1;
  const child = ctx.build(
    withDefaultText(only(node), {
      fontSize: actionSheetActionFontSize(factor),
      fontWeight: flag(node, "isDefaultAction") ? 600 : 400,
      textAlign: "center",
    }),
    ctx.text,
  );
  return cupertinoActionSheetAction({ child, textScaleFactor: factor, textDirection: ctx.textDirection });
}

/** The closed menu is its child; the `actions` prop names the open sheet's rows, which take no part in the layout. */
function buildContextMenu(node: SceneNode, ctx: CupertinoContext): RenderBox {
  return cupertinoContextMenu(ctx.build(only(node), ctx.text));
}

function buildContextMenuAction(node: SceneNode, ctx: CupertinoContext): RenderBox {
  const child = ctx.build(
    withDefaultText(only(node), {
      fontSize: cupertinoContextMenuActionFontSize,
      fontWeight: flag(node, "isDefaultAction") ? 600 : 400,
    }),
    ctx.text,
  );
  return cupertinoContextMenuAction({ child, textDirection: ctx.textDirection });
}

/** Builds a dialog node; null when `node` is not one. */
export function buildCupertinoDialog(node: SceneNode, ctx: CupertinoContext): RenderBox | null {
  let box: RenderBox;
  switch (node.type) {
    case "CupertinoAlertDialog":
      box = buildAlertDialog(node, ctx);
      break;
    case "CupertinoDialogAction":
      box = buildDialogAction(node, ctx);
      break;
    case "CupertinoActionSheet":
      box = buildActionSheet(node, ctx);
      break;
    case "CupertinoActionSheetAction":
      box = buildActionSheetAction(node, ctx);
      break;
    case "CupertinoContextMenu":
      box = buildContextMenu(node, ctx);
      break;
    case "CupertinoContextMenuAction":
      box = buildContextMenuAction(node, ctx);
      break;
    default:
      return null;
  }
  box.debugId = node.id;
  return box;
}
