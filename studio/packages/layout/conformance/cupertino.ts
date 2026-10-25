// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The Cupertino composites of the layout conformance scenes (STU-005, S4.4).
 * Each type maps to the render tree its Flutter widget builds
 * (`../src/flutter/cupertino.ts`); the Flutter side builds the real widget
 * under a `CupertinoTheme` whose text styles use the harness fonts (SF Pro is
 * not available), so the text styles below are the ones that theme defines.
 */

import {
  type CupertinoButtonSize,
  type CupertinoListType,
  cupertinoButton,
  cupertinoListSection,
  cupertinoListTile,
  cupertinoNavigationBar,
  cupertinoPageScaffold,
  cupertinoPicker,
  cupertinoSegmentedControl,
  cupertinoSlider,
  cupertinoSwitch,
  cupertinoTabBar,
  cupertinoTabScaffold,
  cupertinoTextField,
  cupertinoTextTheme,
  type EdgeInsetsGeometry,
  navBarClampedScale,
  navBarEdge,
  navBarLargeTitleScale,
  navBarPreferredHeight,
  type RenderBox,
  type TextDirection,
  TextScaler,
} from "../src/index.ts";
import type { SceneNode, SceneText } from "./scene.ts";

/** The node types of this file; `CupertinoTabItem` and `CupertinoNavSlot` are wrappers without a render object. */
export const cupertinoTypes: ReadonlySet<string> = new Set([
  "CupertinoButton",
  "CupertinoSwitch",
  "CupertinoSlider",
  "CupertinoTabBar",
  "CupertinoTabItem",
  "CupertinoNavigationBar",
  "CupertinoNavSlot",
  "CupertinoPageScaffold",
  "CupertinoSlot",
  "CupertinoTabScaffold",
  "CupertinoPicker",
  "CupertinoListTile",
  "CupertinoListSection",
  "CupertinoTextField",
  "CupertinoSegmentedControl",
]);

/** Types that take several children. */
export const cupertinoMultiChildTypes: ReadonlySet<string> = new Set([
  "CupertinoTabBar",
  "CupertinoNavigationBar",
  "CupertinoPageScaffold",
  "CupertinoTabScaffold",
  "CupertinoListTile",
  "CupertinoListSection",
  "CupertinoSegmentedControl",
]);

/** Types that wrap their child without a render object: their id is their child's rect. */
export const cupertinoAliasTypes: ReadonlySet<string> = new Set([
  "CupertinoTabItem",
  "CupertinoNavSlot",
  "CupertinoSlot",
]);

/** What the builder borrows from `scene.ts`. */
export interface CupertinoContext {
  textDirection: TextDirection;
  text: SceneText | null;
  build(node: SceneNode, text: SceneText | null): RenderBox;
  padding(value: unknown): EdgeInsetsGeometry;
}

const family = "NotoSans";

type Defaults = { fontSize?: number; fontWeight?: number; maxLines?: number; ellipsis?: boolean };

/** `DefaultTextStyle`: fills the style and line limits a `Text` descendant leaves unset. */
function withDefaultText(node: SceneNode, defaults: Defaults): SceneNode {
  const children = node.children?.map((child) => withDefaultText(child, defaults));
  if (node.type !== "Text") {
    return children === undefined ? node : { ...node, children };
  }
  const style = { fontFamily: family, ...(node.props["style"] as Record<string, unknown> | undefined) } as Record<
    string,
    unknown
  >;
  for (const name of ["fontSize", "fontWeight"] as const) {
    if (style[name] === undefined && defaults[name] !== undefined) {
      style[name] = defaults[name];
    }
  }
  const props: Record<string, unknown> = { ...node.props, style };
  if (props["maxLines"] === undefined && defaults.maxLines !== undefined) {
    props["maxLines"] = defaults.maxLines;
  }
  if (props["overflow"] === undefined && defaults.ellipsis === true) {
    props["overflow"] = "ellipsis";
  }
  return { ...node, props, children: children ?? [] };
}

function scaled(text: SceneText | null, factor: (base: number) => number): SceneText | null {
  return text === null ? null : { ...text, textScaler: TextScaler.linear(factor(text.textScaler.textScaleFactor)) };
}

function str(node: SceneNode, name: string, fallback: string): string {
  const value = node.props[name];
  return typeof value === "string" ? value : fallback;
}

function numberProp(node: SceneNode, name: string, fallback: number): number {
  const value = node.props[name];
  return typeof value === "number" ? value : fallback;
}

function only(node: SceneNode): SceneNode {
  const child = node.children?.[0];
  if (child === undefined) {
    throw new Error(`node "${node.id}" of type ${node.type} needs a child`);
  }
  return child;
}

function label(ctx: CupertinoContext, text: string, defaults: Defaults): RenderBox {
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

function navSlots(node: SceneNode): Map<string, SceneNode> {
  const slots = new Map<string, SceneNode>();
  for (const child of node.children ?? []) {
    if (child.type === "CupertinoNavSlot") {
      slots.set(str(child, "slot", "middle"), child);
    }
  }
  return slots;
}

function buildNavigationBar(node: SceneNode, ctx: CupertinoContext): RenderBox {
  const slots = navSlots(node);
  const side = (name: string, edge: "start" | "end"): RenderBox | null => {
    const slot = slots.get(name);
    if (slot === undefined) {
      return null;
    }
    const text = scaled(ctx.text, navBarClampedScale);
    const child = ctx.build(withDefaultText(slot, cupertinoTextTheme.textStyle), text);
    return navBarEdge(child, edge, ctx.textDirection);
  };
  const middleSlot = slots.get("middle");
  const largeSlot = slots.get("largeTitle");
  const middle =
    middleSlot === undefined
      ? null
      : ctx.build(
          withDefaultText(middleSlot, cupertinoTextTheme.navTitleTextStyle),
          scaled(ctx.text, navBarClampedScale),
        );
  const largeTitle =
    largeSlot === undefined
      ? null
      : ctx.build(
          withDefaultText(largeSlot, { ...cupertinoTextTheme.navLargeTitleTextStyle, maxLines: 1, ellipsis: true }),
          scaled(ctx.text, (factor) => navBarLargeTitleScale(factor)),
        );
  const box = cupertinoNavigationBar({
    leading: side("leading", "start"),
    middle,
    trailing: side("trailing", "end"),
    largeTitle,
    safeTop: numberProp(node, "safeTop", 0),
    textDirection: ctx.textDirection,
  });
  return box;
}

function slotOf(node: SceneNode, name: string): SceneNode | undefined {
  return (node.children ?? []).find((child) => child.type === "CupertinoSlot" && child.props["slot"] === name);
}

function buildListTile(node: SceneNode, ctx: CupertinoContext): RenderBox {
  const type = str(node, "type", "base") as CupertinoListType;
  const base = type === "base";
  const leading = slotOf(node, "leading");
  const subtitle = slotOf(node, "subtitle");
  const title = slotOf(node, "title");
  if (title === undefined) {
    throw new Error(`node "${node.id}" of type CupertinoListTile needs a title slot`);
  }
  const titleDefaults: Defaults = {
    fontSize: !base && subtitle !== undefined && leading === undefined ? 16 : 17,
    ...(!base && subtitle !== undefined ? { fontWeight: 600 } : {}),
    maxLines: 1,
    ellipsis: true,
  };
  const build = (slot: SceneNode | undefined, defaults: Defaults | null): RenderBox | null =>
    slot === undefined ? null : ctx.build(defaults === null ? slot : withDefaultText(slot, defaults), ctx.text);
  return cupertinoListTile({
    type,
    title: build(title, titleDefaults) as RenderBox,
    subtitle: build(subtitle, { fontSize: base ? 12 : 14, maxLines: 1, ellipsis: true }),
    additionalInfo: build(slotOf(node, "additionalInfo"), { fontSize: 17, maxLines: 1 }),
    leading: build(leading, null),
    trailing: build(slotOf(node, "trailing"), null),
    padding: node.props["padding"] === undefined ? null : ctx.padding(node.props["padding"]),
    textDirection: ctx.textDirection,
  });
}

function buildListSection(node: SceneNode, ctx: CupertinoContext): RenderBox {
  const type = str(node, "type", "base") as CupertinoListType;
  const base = type === "base";
  const header = slotOf(node, "header");
  const footer = slotOf(node, "footer");
  const rows = (node.children ?? []).filter((child) => child.type === "CupertinoListTile");
  const build = (slot: SceneNode | undefined, defaults: Defaults | null): RenderBox | null =>
    slot === undefined ? null : ctx.build(defaults === null ? slot : withDefaultText(slot, defaults), ctx.text);
  return cupertinoListSection({
    type,
    header: build(header, base ? { fontSize: 13 } : { fontSize: 20, fontWeight: 700 }),
    footer: build(footer, base ? { fontSize: 13 } : { fontSize: 17 }),
    rows: rows.map((row) => ctx.build(row, ctx.text)),
    hasLeading: node.props["hasLeading"] !== false,
    topMargin: typeof node.props["topMargin"] === "number" ? node.props["topMargin"] : null,
    devicePixelRatio: ctx.text?.devicePixelRatio ?? 1,
    textDirection: ctx.textDirection,
  });
}

function buildGroup1(node: SceneNode, ctx: CupertinoContext): RenderBox | null {
  let box: RenderBox;
  switch (node.type) {
    case "CupertinoButton": {
      const size = str(node, "size", "large") as CupertinoButtonSize;
      const child = ctx.build(
        withDefaultText(
          only(node),
          size === "small" ? cupertinoTextTheme.actionSmallTextStyle : cupertinoTextTheme.actionTextStyle,
        ),
        ctx.text,
      );
      box = cupertinoButton({
        size,
        padding: node.props["padding"] === undefined ? null : ctx.padding(node.props["padding"]),
        minWidth: typeof node.props["minWidth"] === "number" ? node.props["minWidth"] : null,
        minHeight: typeof node.props["minHeight"] === "number" ? node.props["minHeight"] : null,
        textDirection: ctx.textDirection,
        child,
      });
      break;
    }
    case "CupertinoListTile":
      box = buildListTile(node, ctx);
      break;
    default:
      return null;
  }
  box.debugId = node.id;
  return box;
}

function buildGroup2(node: SceneNode, ctx: CupertinoContext): RenderBox | null {
  let box: RenderBox;
  switch (node.type) {
    case "CupertinoTabItem":
      return ctx.build(only(node), ctx.text);
    case "CupertinoSegmentedControl":
      box = cupertinoSegmentedControl({
        segments: (node.children ?? []).map((child) => ctx.build(child, ctx.text)),
        padding: node.props["padding"] === undefined ? null : ctx.padding(node.props["padding"]),
        textDirection: ctx.textDirection,
      });
      break;
    default:
      return null;
  }
  box.debugId = node.id;
  return box;
}

function buildTabScaffold(node: SceneNode, ctx: CupertinoContext): RenderBox {
  const bar = (node.children ?? []).find((child) => child.type === "CupertinoTabBar");
  const tab = (node.children ?? []).find((child) => child.type !== "CupertinoTabBar");
  if (bar === undefined || tab === undefined) {
    throw new Error(`node "${node.id}" of type CupertinoTabScaffold needs a tab bar and a tab`);
  }
  const safeBottom = numberProp(node, "safeBottom", 0);
  const noScaling = ctx.text === null ? null : { ...ctx.text, textScaler: TextScaler.noScaling };
  return cupertinoTabScaffold({
    tab: ctx.build(tab, ctx.text),
    tabBar: ctx.build({ ...bar, props: { safeBottom, ...bar.props } }, noScaling),
    tabBarHeight: numberProp(bar, "height", 50),
    safeBottom,
    opaque: bar.props["opaque"] === true,
    textDirection: ctx.textDirection,
  });
}

function buildPageScaffold(node: SceneNode, ctx: CupertinoContext): RenderBox {
  const bar = (node.children ?? []).find((child) => child.type === "CupertinoNavigationBar");
  const content = (node.children ?? []).find((child) => child.type !== "CupertinoNavigationBar");
  if (content === undefined) {
    throw new Error(`node "${node.id}" of type CupertinoPageScaffold needs a content child`);
  }
  const noScaling = ctx.text === null ? null : { ...ctx.text, textScaler: TextScaler.noScaling };
  // The scaffold's insets reach the bar through MediaQuery.
  const inset = numberProp(node, "safeTop", 0);
  const barWithInset = bar === undefined ? undefined : { ...bar, props: { safeTop: inset, ...bar.props } };
  const navigationBar = barWithInset === undefined ? null : ctx.build(barWithInset, noScaling);
  return cupertinoPageScaffold({
    content: ctx.build(content, ctx.text),
    navigationBar,
    navigationBarHeight: bar === undefined ? 0 : navBarPreferredHeight(navSlots(bar).has("largeTitle")),
    obstructing: bar?.props["opaque"] === true,
    safeTop: numberProp(node, "safeTop", 0),
    textDirection: ctx.textDirection,
  });
}

function buildGroup3(node: SceneNode, ctx: CupertinoContext): RenderBox | null {
  let box: RenderBox;
  switch (node.type) {
    case "CupertinoTabScaffold":
      box = buildTabScaffold(node, ctx);
      break;
    case "CupertinoPageScaffold":
      box = buildPageScaffold(node, ctx);
      break;
    default:
      return null;
  }
  box.debugId = node.id;
  return box;
}

function buildGroup4(node: SceneNode, ctx: CupertinoContext): RenderBox | null {
  let box: RenderBox;
  switch (node.type) {
    case "CupertinoTextField": {
      const text = str(node, "text", "");
      const shown = text !== "" ? text : str(node, "placeholder", "");
      const line = label(ctx, shown === "" ? "A" : shown, { ...cupertinoTextTheme.textStyle, maxLines: 1 });
      box = cupertinoTextField({
        line,
        padding: node.props["padding"] === undefined ? null : ctx.padding(node.props["padding"]),
        textDirection: ctx.textDirection,
      });
      break;
    }
    case "CupertinoSlider":
      box = cupertinoSlider();
      break;
    default:
      return null;
  }
  box.debugId = node.id;
  return box;
}

function buildGroup5(node: SceneNode, ctx: CupertinoContext): RenderBox | null {
  let box: RenderBox;
  switch (node.type) {
    case "CupertinoNavigationBar":
      box = buildNavigationBar(node, ctx);
      break;
    case "CupertinoPicker":
      box = cupertinoPicker(ctx.textDirection);
      break;
    default:
      return null;
  }
  box.debugId = node.id;
  return box;
}

function buildGroup6(node: SceneNode, ctx: CupertinoContext): RenderBox | null {
  let box: RenderBox;
  switch (node.type) {
    case "CupertinoSwitch":
      box = cupertinoSwitch();
      break;
    case "CupertinoListSection":
      box = buildListSection(node, ctx);
      break;
    default:
      return null;
  }
  box.debugId = node.id;
  return box;
}

function buildGroup7(node: SceneNode, ctx: CupertinoContext): RenderBox | null {
  let box: RenderBox;
  switch (node.type) {
    case "CupertinoNavSlot":
    case "CupertinoSlot":
      return ctx.build(only(node), ctx.text);
    case "CupertinoTabBar": {
      const items = (node.children ?? []).map((item) => {
        const text = item.props["label"];
        return {
          icon: ctx.build(item, ctx.text),
          label: typeof text === "string" ? label(ctx, text, cupertinoTextTheme.tabLabelTextStyle) : null,
        };
      });
      box = cupertinoTabBar({
        height: numberProp(node, "height", 50),
        bottomPadding: numberProp(node, "safeBottom", 0),
        items,
        textDirection: ctx.textDirection,
      });
      break;
    }
    default:
      return null;
  }
  box.debugId = node.id;
  return box;
}

const builders = [buildGroup1, buildGroup2, buildGroup3, buildGroup4, buildGroup5, buildGroup6, buildGroup7] as const;

/** Builds a Cupertino node; wrappers return their child's box, which keeps the child's id. */
export function buildCupertino(node: SceneNode, ctx: CupertinoContext): RenderBox {
  for (const build of builders) {
    const box = build(node, ctx);
    if (box !== null) {
      return box;
    }
  }
  throw new Error(`node "${node.id}": unknown Cupertino type "${node.type}"`);
}
