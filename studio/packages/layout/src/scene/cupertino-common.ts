// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** What the Cupertino scene builders share: the context, `DefaultTextStyle` and the node-prop readers (STU-005, S4.4). */

import { type EdgeInsetsGeometry, type RenderBox, type TextDirection, TextScaler } from "../index.ts";
import type { SceneNode, SceneText } from "./scene.ts";

/** What the builder borrows from `scene.ts`. */
export interface CupertinoContext {
  textDirection: TextDirection;
  text: SceneText | null;
  build(node: SceneNode, text: SceneText | null): RenderBox;
  padding(value: unknown): EdgeInsetsGeometry;
}

export const family = "NotoSans";

export type Defaults = {
  fontSize?: number;
  fontWeight?: number;
  height?: number;
  letterSpacing?: number;
  textAlign?: string;
  maxLines?: number;
  ellipsis?: boolean;
};

/** `DefaultTextStyle`: fills the style and line limits a `Text` descendant leaves unset. */
export function withDefaultText(node: SceneNode, defaults: Defaults): SceneNode {
  const children = node.children?.map((child) => withDefaultText(child, defaults));
  if (node.type !== "Text") {
    return children === undefined ? node : { ...node, children };
  }
  const style = { fontFamily: family, ...(node.props["style"] as Record<string, unknown> | undefined) } as Record<
    string,
    unknown
  >;
  for (const name of ["fontSize", "fontWeight", "height", "letterSpacing"] as const) {
    if (style[name] === undefined && defaults[name] !== undefined) {
      style[name] = defaults[name];
    }
  }
  const props: Record<string, unknown> = { ...node.props, style };
  if (props["textAlign"] === undefined && defaults.textAlign !== undefined) {
    props["textAlign"] = defaults.textAlign;
  }
  if (props["maxLines"] === undefined && defaults.maxLines !== undefined) {
    props["maxLines"] = defaults.maxLines;
  }
  if (props["overflow"] === undefined && defaults.ellipsis === true) {
    props["overflow"] = "ellipsis";
  }
  return { ...node, props, children: children ?? [] };
}

export function scaled(text: SceneText | null, factor: (base: number) => number): SceneText | null {
  return text === null ? null : { ...text, textScaler: TextScaler.linear(factor(text.textScaler.textScaleFactor)) };
}

export function str(node: SceneNode, name: string, fallback: string): string {
  const value = node.props[name];
  return typeof value === "string" ? value : fallback;
}

export function numberProp(node: SceneNode, name: string, fallback: number): number {
  const value = node.props[name];
  return typeof value === "number" ? value : fallback;
}

export function only(node: SceneNode): SceneNode {
  const child = node.children?.[0];
  if (child === undefined) {
    throw new Error(`node "${node.id}" of type ${node.type} needs a child`);
  }
  return child;
}

export function label(ctx: CupertinoContext, text: string, defaults: Defaults): RenderBox {
  const { maxLines, ...style } = defaults;
  const node: SceneNode = {
    id: "",
    type: "Text",
    props: { text, style: { fontFamily: family, ...style }, ...(maxLines === undefined ? {} : { maxLines }) },
  };
  const box = ctx.build(node, ctx.text);
  box.debugId = null;
  return box;
}

export function slotOf(node: SceneNode, name: string): SceneNode | undefined {
  return (node.children ?? []).find((child) => child.type === "CupertinoSlot" && child.props["slot"] === name);
}
