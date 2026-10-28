// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The screen builders of the widget types the layout engine ports and the
 * conformance suite verifies (STU-005): boxes, flex and stack layouts, scroll
 * views and slivers, and the Material composites. A screen node is turned into
 * a scene node (`../scene/scene.ts`) and built by the same `buildNode` the
 * conformance scenes use, so the canvas and the goldens share one code path.
 * A document spells some props differently from a scene; `sceneProps` and
 * `sceneChildren` are the only places that translate.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { TextDirection } from "../flutter/geometry.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import { RenderIntrinsicWidth } from "../flutter/proxy-boxes.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { TextScaler, type TextStyle } from "../flutter/text-style.ts";
import { buildNode, hostType, type SceneNode, type SceneText, supportedTypes } from "../scene/scene.ts";
import type { ParagraphEngine } from "../text/paragraph-engine.ts";
import type { WidgetBuilder, WidgetContext, WidgetNode } from "./screen.ts";
import { placeholder } from "./structural.ts";

/** Types built from a scene node wherever they appear in a screen. */
const boxTypes: readonly string[] = [
  "ConstrainedBox",
  "LimitedBox",
  "FittedBox",
  "AspectRatio",
  "FractionallySizedBox",
  "Baseline",
  "ClipOval",
  "ClipRRect",
  "Opacity",
  "Transform",
  "DecoratedBox",
  "AnimatedContainer",
  "AnimatedPadding",
  "AnimatedAlign",
  "AnimatedOpacity",
  "AnimatedPositioned",
  "Flex",
  "Stack",
  "IndexedStack",
  "Positioned",
  "Spacer",
  "Wrap",
  "Table",
];

/** Scroll views, built as shrink-wrapping viewports: a screen has no scroll offset to show. */
const scrollTypes: readonly string[] = [
  "GridView",
  "PageView",
  "CustomScrollView",
  "NestedScrollView",
  "ReorderableListView",
];

/** Slivers: a viewport lays them out, so one on its own is wrapped in a viewport. */
const sliverTypes: readonly string[] = [
  "SliverToBoxAdapter",
  "SliverList",
  "SliverGrid",
  "SliverFillRemaining",
  "SliverPadding",
  "SliverPersistentHeader",
  "SliverAppBar",
];

/** Material composites: everything below them is built by the scene, so their text takes the Material styles. */
const compositeTypes: readonly string[] = [
  "AppBar",
  "Drawer",
  "NavigationBar",
  "NavigationRail",
  "Icon",
  "ActionChip",
  "ChoiceChip",
  "Card",
  "Badge",
  "Checkbox",
  "Radio",
  "Switch",
  "Divider",
  "VerticalDivider",
  "CircleAvatar",
  "CircularProgressIndicator",
  "LinearProgressIndicator",
  "BackButton",
  "CloseButton",
  "Slider",
  "RangeSlider",
  "CheckboxListTile",
  "SwitchListTile",
  "RadioListTile",
  "SearchBar",
  "SearchAnchor",
  "BottomNavigationBar",
  "ExpansionTile",
  "Autocomplete",
  "BottomAppBar",
  "CalendarDatePicker",
  "DataTable",
  "DropdownMenu",
  "MaterialBanner",
  "MenuAnchor",
  "NavigationDrawer",
  "PopupMenuButton",
  "SegmentedButton",
  "Stepper",
];

/** The slot of a document node whose widgets are a list, and so arrive as `children`; scenes name the part the same. */
const listSlots: Readonly<Record<string, string>> = { AppBar: "actions", MaterialBanner: "actions" };

/** Slots of the item templates of a list, grid or page view; the other states have no design-time view. */
const templateSlots: ReadonlySet<string> = new Set(["item"]);

/** Document slots a scene has no part for. */
const unusedSlots: ReadonlySet<string> = new Set(["empty", "loading", "error", "header", "footer", "flexibleSpace"]);

/** What a node being converted needs: the screen's context and the render objects it built for the scene. */
interface Bridge {
  readonly ctx: WidgetContext;
  readonly hosts: Map<string, RenderBox>;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** An implicit animation's props: the document holds the end state, which is the scene's `to`. */
function animated(props: Record<string, unknown>): Record<string, unknown> {
  return { from: {}, to: props };
}

/** `Transform`: the document's `offset`, `angle` and `scale`/`scaleX`/`scaleY`, as the scene's translate, rotate and scale. */
function transformProps(props: Record<string, unknown>): Record<string, unknown> {
  const offset = isRecord(props["offset"]) ? props["offset"] : {};
  const scale = props["scale"] ?? 1;
  const out: Record<string, unknown> = {
    translate: [offset["dx"] ?? 0, offset["dy"] ?? 0],
    rotate: props["angle"] ?? 0,
    scale: [(props["scaleX"] ?? scale) as number, (props["scaleY"] ?? scale) as number],
  };
  if (props["alignment"] !== undefined) {
    out["alignment"] = props["alignment"];
  }
  return out;
}

/** The props of a document node as its scene node reads them. */
function sceneProps(type: string, props: Record<string, unknown>): Record<string, unknown> {
  switch (type) {
    case "AnimatedContainer":
    case "AnimatedPadding":
    case "AnimatedAlign":
    case "AnimatedOpacity":
    case "AnimatedPositioned":
      return animated(props);
    case "Transform":
      return transformProps(props);
    case "ConstrainedBox":
      return isRecord(props["constraints"]) ? { ...props["constraints"] } : {};
    case "DecoratedBox":
      return {};
    case "GridView":
    case "PageView":
    case "CustomScrollView":
    case "NestedScrollView":
    case "ReorderableListView":
      return { ...props, shrinkWrap: true };
    default:
      return props;
  }
}

/** Whether `type` is built from a scene node, given whether its parent is a composite. */
function converts(type: string | undefined, deep: boolean): boolean {
  if (type === undefined) {
    return false;
  }
  return bridged.has(type) || (deep && (type === "Text" || supportedTypes().has(type)) && !hostOnly.has(type));
}

const bridged: ReadonlySet<string> = new Set([...boxTypes, ...scrollTypes, ...sliverTypes, ...compositeTypes]);
/** Types the screen builds itself even below a composite: the scene would build them at golden metrics. */
const hostOnly: ReadonlySet<string> = new Set(["Row", "Column", "Scaffold", "ListView"]);

/** A scene node for a widget the screen builds itself: its box is built now and the scene takes it by id. */
function host(node: WidgetNode, bridge: Bridge): SceneNode {
  bridge.hosts.set(node.id, bridge.ctx.build(node));
  return { id: node.id, type: hostType, props: {} };
}

/** The destinations a `NavigationBar` or `NavigationRail` holds, as the scene's destination nodes. */
function destinations(node: WidgetNode): SceneNode[] {
  const list = (node.props as Record<string, unknown> | undefined)?.["destinations"];
  const type = node.type === "NavigationRail" ? "NavigationRailDestination" : "NavigationDestination";
  if (!Array.isArray(list)) {
    return [];
  }
  return list.map((entry, index): SceneNode => {
    const destination = isRecord(entry) ? entry : {};
    const icon = (name: string): SceneNode | null =>
      destination[name] === undefined
        ? null
        : { id: `${node.id}.${index}.${name}`, type: "Icon", slot: name, props: {} };
    return {
      id: `${node.id}.${index}`,
      type,
      props: { label: destination["label"] ?? "" },
      children: [
        icon("icon") ?? { id: `${node.id}.${index}.icon`, type: "Icon", slot: "icon", props: {} },
        icon("selectedIcon"),
      ].filter((child): child is SceneNode => child !== null),
    };
  });
}

/** A slot's widgets as scene children: the part it fills is the node's `slot`. */
function slotChildren(node: WidgetNode, bridge: Bridge, deep: boolean): SceneNode[] {
  const out: SceneNode[] = [];
  for (const [name, child] of Object.entries(node.slots ?? {})) {
    if (unusedSlots.has(name) && node.type !== "NavigationDrawer") {
      continue;
    }
    const converted = convert(child, bridge, deep);
    out.push(name === "child" || templateSlots.has(name) ? converted : { ...converted, slot: name });
  }
  return out;
}

/** The scene children of `node`: its destinations, list-slot children and slots, in the order a scene reads them. */
function sceneChildren(node: WidgetNode, bridge: Bridge, deep: boolean): SceneNode[] {
  const type = node.type ?? "";
  if (type === "NavigationBar" || type === "NavigationRail") {
    return destinations(node);
  }
  const listed = listSlots[type];
  const children = (node.children ?? []).map((child) => {
    const converted = convert(child, bridge, deep);
    return listed === undefined ? converted : { ...converted, slot: listed };
  });
  const slots = slotChildren(node, bridge, deep);
  // A NestedScrollView's body is its last child; the header slivers come before it.
  return type === "NestedScrollView"
    ? [...children, ...slots.filter((s) => s.slot === undefined)]
    : [...children, ...slots];
}

/** Types that need a bounded height, which a screen does not give: they are drawn in a box of the theme's viewport height. */
const boundedHeight: ReadonlySet<string> = new Set(["PageView", "Drawer", "VerticalDivider"]);

/** `scene` in a box of the viewport height; the box takes the node's id and the node an id of its own. */
function inViewportHeight(scene: SceneNode, bridge: Bridge): SceneNode {
  return {
    id: scene.id,
    type: "SizedBox",
    props: { height: bridge.ctx.theme.viewportHeight },
    children: [{ ...scene, id: `${scene.id}.view` }],
  };
}

/** The scene node of `node`, its descendants converted or built by the screen. */
function convert(node: WidgetNode, bridge: Bridge, deep: boolean): SceneNode {
  const type = node.type;
  if (!converts(type, deep) || type === undefined) {
    return host(node, bridge);
  }
  const inner = deep || compositeTypes.includes(type);
  const props = sceneProps(type, (node.props ?? {}) as Record<string, unknown>);
  if (type === "Text") {
    bridge.ctx.mark(node.id, "text", typeof props["data"] === "string" ? props["data"] : "");
  }
  const children = sceneChildren(node, bridge, inner);
  const scene: SceneNode = { id: node.id, type, props, ...(children.length === 0 ? {} : { children }) };
  return boundedHeight.has(type) ? inViewportHeight(scene, bridge) : scene;
}

/** The family the scene's Material styles name, mapped to the first family of the screen theme (the canvas has no "NotoSans"). */
function themedEngine(engine: ParagraphEngine, family: string | undefined): ParagraphEngine {
  const map = (name: string | undefined): string | undefined => (name === "NotoSans" ? family : name);
  return {
    buildParagraph: (style, runs) =>
      engine.buildParagraph(
        { ...style, fontFamily: map(style.fontFamily) },
        runs.map((run) =>
          run.style === null ? run : { ...run, style: { ...run.style, fontFamily: map(run.style.fontFamily) } },
        ),
      ),
  };
}

/** Builds `node` from its scene node; a node the scene cannot build is drawn as its labelled placeholder. */
function sceneBuilder(root: (node: WidgetNode) => WidgetNode): WidgetBuilder {
  return (node, ctx) => {
    const bridge: Bridge = { ctx, hosts: new Map() };
    const [family, ...fallback] = ctx.theme.fontFamilies;
    const base: TextStyle = { fontSize: ctx.theme.fontSize, fontFamily: family, fontFamilyFallback: fallback };
    const text: SceneText = {
      engine: themedEngine(ctx.engine, family),
      textScaler: TextScaler.linear(ctx.textScale ?? 1),
      base,
      hosts: bridge.hosts,
      screen: { width: ctx.available(), height: 0 },
    };
    try {
      const box = buildNode(convert(root(node), bridge, false), ctx.textDirection ?? TextDirection.ltr, text);
      ctx.mark(node.id, "box");
      return box;
    } catch {
      return placeholder(node, ctx, node.type ?? "");
    }
  };
}

/**
 * `NavigationRail`: a rail is as high as its parent, which a screen does not give, and as wide as its destinations
 * wherever its parent is bounded; it is drawn at the theme's viewport height (as a `Drawer` is) and at its intrinsic
 * width. The rail itself is built by the scene, so its destinations are the ones the conformance goldens verify.
 */
const navigationRail: WidgetBuilder = (node, ctx) => {
  const rail = sceneBuilder((n) => n)(node, ctx);
  rail.debugId = `${node.id}.rail`;
  const box = new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({ height: ctx.theme.viewportHeight }),
    child: new RenderIntrinsicWidth({ child: rail }),
  });
  box.debugId = node.id;
  return box;
};

/** A sliver on its own lies in a viewport that shrinks to it. */
function inViewport(node: WidgetNode): WidgetNode {
  return { id: node.id, type: "CustomScrollView", children: [{ ...node, id: `${node.id}.sliver` }] };
}

/** The builders of the types the scene builder builds; merged into the default registry. */
export const sceneWidgetBuilders: ReadonlyArray<readonly [string, WidgetBuilder]> = [
  ...[...boxTypes, ...scrollTypes, ...compositeTypes].map((type): readonly [string, WidgetBuilder] => [
    type,
    sceneBuilder((node) => node),
  ]),
  ...sliverTypes.map((type): readonly [string, WidgetBuilder] => [type, sceneBuilder(inViewport)]),
  ["NavigationRail", navigationRail],
];
