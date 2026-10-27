// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The Material composites of the layout conformance scenes (STU-005, S4.3):
 * each node type maps to the port of the Flutter widget the golden writer
 * builds under a Material 3 theme in `NotoSans`. Children name the part of
 * the composite they fill with `slot` (default `child`), and a `Text` child
 * takes the style the composite gives that slot, as Flutter's
 * `DefaultTextStyle` does.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import type { TextDirection } from "../flutter/geometry.ts";
import { EdgeInsets, EdgeInsetsDirectional } from "../flutter/geometry.ts";
import {
  appBarPreferredHeight,
  buildAppBar,
  kLeadingWidth,
  kMaxTitleTextScaleFactor,
  kMiddleSpacing,
} from "../flutter/material/app-bar.ts";
import { buildModalBottomSheet } from "../flutter/material/bottom-sheet.ts";
import { type ButtonKind, buildButton, buildFab, type FabKind } from "../flutter/material/buttons.ts";
import { buildChip } from "../flutter/material/chip.ts";
import { buildAlertDialog } from "../flutter/material/dialog.ts";
import { buildInputDecorator, decoratorIcon, RenderEditableLine } from "../flutter/material/input-decorator.ts";
import { buildListTile } from "../flutter/material/list-tile.ts";
import {
  buildDestination,
  buildNavigationBar,
  destinationLabel,
  indicatorIcon,
  kIndicatorHeight,
  kIndicatorWidth,
  kMaxLabelTextScaleFactor as kMaxNavLabelScale,
  kNavigationBarHeight,
  type NavigationLabelBehavior,
} from "../flutter/material/navigation-bar.ts";
import {
  buildNavigationRail,
  buildRailDestination,
  kRailGroupAlignment,
  kRailMinExtendedWidth,
  kRailMinWidth,
  type RailLabelType,
} from "../flutter/material/navigation-rail.ts";
import { buildPopupMenu, buildPopupMenuDivider, buildPopupMenuItem } from "../flutter/material/popup-menu.ts";
import { type FabLocation, RenderScaffold } from "../flutter/material/scaffold.ts";
import { buildTab, buildTabBar, type TabAlignment, tabPreferredHeight } from "../flutter/material/tab-bar.ts";
import {
  kToolbarHeight,
  MaterialTapTargetSize,
  m3TextStyle,
  scaledPadding,
  type TextRole,
  VisualDensity,
} from "../flutter/material/theme.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPadding } from "../flutter/shifted-box.ts";
import type { TextStyle } from "../flutter/text-style.ts";
import { leafBuilders } from "./material-leaves.ts";
import { moreBuilders } from "./material-more.ts";
import type { SceneNode, SceneText, TextDefaults } from "./scene.ts";

/** The theme's font family, as `ThemeData(fontFamily:)` in the golden writer. */
export const materialFontFamily = "NotoSans";

/** Builds one child of a composite; `base` is the text style `Text` descendants inherit. */
export type BuildChild = (node: SceneNode, base?: TextStyle, options?: TextOptions) => RenderBox;

/** What a composite changes for the `Text` below it: `DefaultTextStyle` flags and a clamped text scale. */
export interface TextOptions {
  defaults?: TextDefaults;
  maxScale?: number;
}

/** Everything a composite builder needs. */
export interface MaterialContext {
  readonly direction: TextDirection;
  readonly text: SceneText | null;
  readonly build: BuildChild;
}

export const style = (role: TextRole): TextStyle => m3TextStyle(role, materialFontFamily);

/** The children of `node` filling `slot`. */
export function slot(node: SceneNode, name: string): SceneNode[] {
  return (node.children ?? []).filter((c) => (c.slot ?? "child") === name);
}

function only(node: SceneNode, name: string): SceneNode | null {
  const found = slot(node, name);
  if (found.length > 1) {
    throw new Error(`node "${node.id}": slot ${name} takes one child`);
  }
  return found[0] ?? null;
}

function required(node: SceneNode, name: string): SceneNode {
  const found = only(node, name);
  if (found === null) {
    throw new Error(`node "${node.id}" of type ${node.type} needs a ${name} child`);
  }
  return found;
}

function prop(props: Record<string, unknown>, name: string): unknown {
  return props[name];
}

function num(props: Record<string, unknown>, name: string, fallback: number): number {
  const value = props[name];
  return typeof value === "number" ? value : fallback;
}

export function tapTarget(props: Record<string, unknown>): MaterialTapTargetSize {
  return prop(props, "tapTargetSize") === "shrinkWrap"
    ? MaterialTapTargetSize.shrinkWrap
    : MaterialTapTargetSize.padded;
}

export function density(props: Record<string, unknown>): VisualDensity {
  return VisualDensity.named(prop(props, "density"));
}

const buttonKinds: Readonly<Record<string, ButtonKind>> = {
  ElevatedButton: "elevated",
  FilledButton: "filled",
  FilledTonalButton: "filledTonal",
  OutlinedButton: "outlined",
  TextButton: "text",
};

type Builder = (node: SceneNode, ctx: MaterialContext) => RenderBox;

/** `PreferredSizeWidget.preferredSize.height` of an app bar's bottom. */
function preferredHeight(node: SceneNode | null): number {
  if (node === null) {
    return 0;
  }
  return num(node.props, "height", 0);
}

/** `Chip`, `FilterChip` and `InputChip`: all build `RawChip`, with the label, an avatar and an optional delete icon. */
function buildChipNode(node: SceneNode, ctx: MaterialContext): RenderBox {
  const p = node.props;
  const avatar = only(node, "avatar");
  const deleteIcon = prop(p, "deletable") === true ? required(node, "deleteIcon") : null;
  return buildChip({
    label: ctx.build(required(node, "label"), style("labelLarge"), {
      defaults: { softWrap: false, overflow: "fade", maxLines: 1 },
    }),
    avatar: avatar === null ? null : ctx.build(avatar),
    deleteIcon: deleteIcon === null ? null : ctx.build(deleteIcon),
    selected: prop(p, "selected") === true,
    tapTargetSize: tapTarget(p),
    density: density(p),
    textScale: ctx.text?.textScaler.textScaleFactor ?? 1,
    textDirection: ctx.direction,
  });
}

/** A text the composite makes up itself: a scene node nothing keys, so no rect is reported for it. */
function syntheticText(
  node: SceneNode,
  name: string,
  text: string,
  ctx: MaterialContext,
  base: TextStyle,
  options: TextOptions = {},
): RenderBox {
  const box = ctx.build({ id: `${node.id}:${name}`, type: "Text", props: { text } }, base, options);
  box.debugId = null;
  return box;
}

/** `TextField` with an `InputDecoration`: the input line is one line of `bodyLarge`; label and hint add no box. */
function buildTextField(node: SceneNode, ctx: MaterialContext, trailingArrow = false): RenderBox {
  const p = node.props;
  const text = (name: string): string | null => {
    const value = prop(p, name);
    return typeof value === "string" ? value : null;
  };
  const dens = density(p);
  const prefix = only(node, "prefixIcon");
  const suffix = only(node, "suffixIcon");
  const subtext = text("errorText") ?? text("helperText");
  const withText = (text("text") ?? "") !== "";
  const line = syntheticText(node, "input", "A", ctx, style("bodyLarge"));
  const affix = (name: string): RenderBox | null => {
    const value = text(name);
    return value === null ? null : syntheticText(node, name, value, ctx, style("bodyLarge"));
  };
  // The hint is one line (`hintMaxLines` follows `maxLines`) and ellipsized.
  const hintText = text("hintText");
  const hint =
    hintText === null
      ? null
      : syntheticText(node, "hint", hintText, ctx, style("bodyLarge"), {
          defaults: { maxLines: 1, overflow: "ellipsis" },
        });
  const leading = only(node, "icon");
  // A `DropdownMenu`'s arrow: a 24-px icon button's icon, in the suffix.
  const arrow = trailingArrow ? ctx.build({ id: `${node.id}:arrow`, type: "Icon", props: {} }) : null;
  if (arrow !== null) {
    arrow.debugId = null;
  }
  return buildInputDecorator({
    input: new RenderEditableLine(line),
    // `InputDecoration.icon`: padded by 16 after it.
    icon:
      leading === null
        ? null
        : new RenderPadding({
            padding: EdgeInsetsDirectional.only({ end: 16 }),
            textDirection: ctx.direction,
            child: ctx.build({
              ...leading,
              props: { size: prop(p, "isDense") === true ? 18 : 24, ...leading.props },
            }),
          }),
    prefix: affix("prefixText"),
    suffix: affix("suffixText"),
    hint,
    isEmpty: !withText,
    counter:
      text("counterText") === null
        ? null
        : syntheticText(node, "counter", text("counterText") ?? "", ctx, style("bodySmall"), {
            defaults: { overflow: "ellipsis" },
          }),
    prefixIcon: prefix === null ? null : decoratorIcon(ctx.build(prefix), dens),
    suffixIcon: arrow !== null ? decoratorIcon(arrow, dens) : suffix === null ? null : decoratorIcon(ctx.build(suffix), dens),
    subtext:
      subtext === null
        ? null
        : syntheticText(node, "subtext", subtext, ctx, style("bodySmall"), { defaults: { overflow: "ellipsis" } }),
    outline: prop(p, "border") === "outline",
    filled: prop(p, "filled") === true,
    isDense: prop(p, "isDense") === true,
    hasLabel: text("labelText") !== null,
    floating: withText,
    floatingLabelHeight: 16 * (ctx.text?.textScaler.textScaleFactor ?? 1),
    density: dens,
    textDirection: ctx.direction,
  });
}

/** The width of a `DropdownMenu` whose `width` is not set: its widest entry, the trailing button and the prefix gap. */
function dropdownWidth(node: SceneNode, ctx: MaterialContext, trailing: boolean): number {
  const scale = ctx.text?.textScaler.textScaleFactor ?? 1;
  const [start, , end] = scaledPadding([12, 0, 12, 0], [8, 0, 8, 0], [4, 0, 4, 0], scale);
  let widest = 0;
  const entries = Array.isArray(node.props["dropdownMenuEntries"]) ? (node.props["dropdownMenuEntries"] as unknown[]) : [];
  for (const entry of entries) {
    const text = typeof entry === "object" && entry !== null ? (entry as Record<string, unknown>)["label"] : undefined;
    const box = syntheticText(node, "entry", typeof text === "string" ? text : "", ctx, style("labelLarge"));
    widest = Math.max(widest, box.getMaxIntrinsicWidth(Number.POSITIVE_INFINITY) + start + end);
  }
  return Math.max(112, widest + (trailing ? 56 : 0) + 16);
}

/** `DropdownMenu`: an outlined field as wide as its entries, with the trailing arrow; the menu is not open at design time. */
function buildDropdownMenu(node: SceneNode, ctx: MaterialContext): RenderBox {
  if (only(node, "leadingIcon") !== null || only(node, "label") !== null) {
    throw new Error(`node "${node.id}": a DropdownMenu with a leading icon or a label has no port`);
  }
  const trailing = prop(node.props, "showTrailingIcon") !== false;
  const field = buildTextField(
    {
      id: node.id,
      type: "TextField",
      props: { border: "outline", hintText: node.props["hintText"], helperText: node.props["helperText"] },
    },
    ctx,
    trailing,
  );
  const width = typeof node.props["width"] === "number" ? node.props["width"] : dropdownWidth(node, ctx, trailing);
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width }), child: field });
}

/** The icon a destination shows: its `selectedIcon` when selected (else its `icon`), its `icon` otherwise. */
function shownIcon(destination: SceneNode, selected: boolean): SceneNode {
  const icon = only(destination, "icon");
  const selectedIcon = only(destination, "selectedIcon");
  const shown = selected ? (selectedIcon ?? icon) : (icon ?? selectedIcon);
  if (shown === null) {
    throw new Error(`node "${destination.id}": a destination needs an icon`);
  }
  return shown;
}

/** `NavigationBar` with `NavigationDestination` children: an `icon` slot and a `label` prop each. */
function buildNavigationBarNode(node: SceneNode, ctx: MaterialContext): RenderBox {
  const p = node.props;
  const selectedIndex = num(p, "selectedIndex", 0);
  const behavior = (prop(p, "labelBehavior") ?? "alwaysShow") as NavigationLabelBehavior;
  const destinations = (node.children ?? []).map((destination, index) => {
    if (destination.type !== "NavigationDestination") {
      throw new Error(`node "${destination.id}": a NavigationBar holds NavigationDestination nodes`);
    }
    const label = prop(destination.props, "label");
    const text = syntheticText(
      destination,
      "label",
      typeof label === "string" ? label : "",
      ctx,
      style("labelMedium"),
      {
        maxScale: kMaxNavLabelScale,
      },
    );
    const shown = behavior === "alwaysShow" || (behavior === "onlyShowSelected" && index === selectedIndex) ? 1 : 0;
    const row = buildDestination(
      indicatorIcon(ctx.build(shownIcon(destination, index === selectedIndex)), kIndicatorWidth, kIndicatorHeight),
      destinationLabel(text),
      shown,
      ctx.direction,
    );
    row.debugId = destination.id;
    return row;
  });
  return buildNavigationBar({
    destinations,
    height: num(p, "height", kNavigationBarHeight),
    textDirection: ctx.direction,
  });
}

/** `NavigationRail` with `NavigationRailDestination` children: an `icon` slot and a `label` prop each. */
function buildNavigationRailNode(node: SceneNode, ctx: MaterialContext): RenderBox {
  const p = node.props;
  const selectedIndex = num(p, "selectedIndex", 0);
  const labelType = (prop(p, "labelType") ?? "none") as RailLabelType;
  const minWidth = num(p, "minWidth", kRailMinWidth);
  const destinations = slot(node, "child").map((destination, index) => {
    if (destination.type !== "NavigationRailDestination") {
      throw new Error(`node "${destination.id}": a NavigationRail holds NavigationRailDestination nodes`);
    }
    const label = prop(destination.props, "label");
    return buildRailDestination({
      icon: ctx.build(shownIcon(destination, index === selectedIndex)),
      label: syntheticText(destination, "label", typeof label === "string" ? label : "", ctx, style("labelMedium")),
      selected: index === selectedIndex,
      labelType,
      minWidth,
      extendedWidth: prop(p, "extended") === true ? num(p, "minExtendedWidth", kRailMinExtendedWidth) : null,
      textDirection: ctx.direction,
    });
  });
  const leading = only(node, "leading");
  const trailing = only(node, "trailing");
  return buildNavigationRail({
    destinations,
    leading: leading === null ? null : ctx.build(leading),
    trailing: trailing === null ? null : ctx.build(trailing),
    groupAlignment: num(p, "groupAlignment", kRailGroupAlignment),
    textDirection: ctx.direction,
  });
}

/** `TabBar` with `Tab` children: an optional `icon` slot and a `text` prop each. */
function buildTabBarNode(node: SceneNode, ctx: MaterialContext): RenderBox {
  const p = node.props;
  const isScrollable = prop(p, "isScrollable") === true;
  const tabs = (node.children ?? []).map((tab) => {
    if (tab.type !== "Tab") {
      throw new Error(`node "${tab.id}": a TabBar holds Tab nodes`);
    }
    const label = prop(tab.props, "text");
    const icon = only(tab, "icon");
    const height = typeof prop(tab.props, "height") === "number" ? num(tab.props, "height", 0) : null;
    const box = buildTab({
      icon: icon === null ? null : ctx.build(icon),
      text:
        typeof label === "string"
          ? syntheticText(tab, "label", label, ctx, style("titleSmall"), {
              defaults: { softWrap: false, overflow: "fade" },
            })
          : null,
      height,
    });
    box.debugId = tab.id;
    return {
      box,
      preferredHeight: tabPreferredHeight({ hasIcon: icon !== null, hasText: typeof label === "string", height }),
    };
  });
  return buildTabBar({
    tabs,
    isScrollable,
    tabAlignment: (prop(p, "tabAlignment") ?? (isScrollable ? "startOffset" : "fill")) as TabAlignment,
    textDirection: ctx.direction,
  });
}

/** `AlertDialog`: `title`, `content` and `actions` slots in the dialog panel's text styles. */
function buildAlertDialogNode(node: SceneNode, ctx: MaterialContext): RenderBox {
  const title = only(node, "title");
  const content = only(node, "content");
  const actions = slot(node, "actions");
  return buildAlertDialog({
    title: title === null ? null : ctx.build(title, style("headlineSmall")),
    content: content === null ? null : ctx.build(content, style("bodyMedium")),
    actions: actions.length === 0 ? null : actions.map((a) => ctx.build(a)),
    textScale: ctx.text?.textScaler.textScaleFactor ?? 1,
    textDirection: ctx.direction,
  });
}

/** A modal `BottomSheet` in its settled state: the `child` slot, with an optional drag handle. */
function buildBottomSheetNode(node: SceneNode, ctx: MaterialContext): RenderBox {
  return buildModalBottomSheet({
    child: ctx.build(required(node, "child")),
    showDragHandle: prop(node.props, "showDragHandle") === true,
    isScrollControlled: prop(node.props, "isScrollControlled") === true,
    textDirection: ctx.direction,
  });
}

/** A popup menu in its open state: `PopupMenuItem` (a `child` slot, optional `height`) and `PopupMenuDivider` children. */
function buildPopupMenuNode(node: SceneNode, ctx: MaterialContext): RenderBox {
  const optionalHeight = (entry: SceneNode): number | null =>
    typeof prop(entry.props, "height") === "number" ? num(entry.props, "height", 0) : null;
  const items = (node.children ?? []).map((entry) => {
    let box: RenderBox;
    if (entry.type === "PopupMenuDivider") {
      box = buildPopupMenuDivider(optionalHeight(entry));
    } else if (entry.type === "PopupMenuItem") {
      box = buildPopupMenuItem(
        ctx.build(required(entry, "child"), style("labelLarge")),
        optionalHeight(entry),
        ctx.direction,
      );
    } else {
      throw new Error(`node "${entry.id}": a PopupMenu holds PopupMenuItem and PopupMenuDivider nodes`);
    }
    box.debugId = entry.id;
    return box;
  });
  return buildPopupMenu(items, ctx.direction);
}

/** A control of a list tile, a box of the size the Flutter control takes inside the tile. */
function control(width: number, height: number): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width, height }) });
}

/** The `secondary` widget of a `CheckboxListTile`, `SwitchListTile` or `RadioListTile`, or null. */
function secondary(node: SceneNode, ctx: MaterialContext): RenderBox | null {
  const part = only(node, "secondary");
  return part === null ? null : ctx.build(part, style("labelSmall"));
}

/** A `ListTile`'s layout: `leading` and `trailing` already built, the title and subtitle from their slots. */
function buildTile(
  node: SceneNode,
  ctx: MaterialContext,
  leading: RenderBox | null,
  trailing: RenderBox | null,
): RenderBox {
  const p = node.props;
  const dense = prop(p, "dense") === true;
  const withSize = (role: TextRole, size: number | null): TextStyle =>
    size === null ? style(role) : { ...style(role), fontSize: size };
  const sub = only(node, "subtitle");
  return buildListTile({
    leading,
    title: ctx.build(required(node, "title"), withSize("bodyLarge", dense ? 13 : null)),
    subtitle: sub === null ? null : ctx.build(sub, withSize("bodyMedium", dense ? 12 : null)),
    trailing,
    isDense: dense,
    isThreeLine: prop(p, "isThreeLine") === true,
    visualDensity: density(p),
    textDirection: ctx.direction,
  });
}

const builders: Record<string, Builder> = {
  Icon: (node) => {
    const given = prop(node.props, "size");
    const size = typeof given === "number" ? given : 24;
    return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: size, height: size }) });
  },
  ListTile: (node, ctx) => {
    const lead = only(node, "leading");
    const trail = only(node, "trailing");
    return buildTile(
      node,
      ctx,
      lead === null ? null : ctx.build(lead, style("labelSmall")),
      trail === null ? null : ctx.build(trail, style("labelSmall")),
    );
  },
  // The tile and its 1-px border above and below; the trailing arrow is a 24-px icon.
  ExpansionTile: (node, ctx) => {
    const lead = only(node, "leading");
    return new RenderPadding({
      padding: EdgeInsets.symmetric({ vertical: 1 }),
      child: buildTile(
        node,
        ctx,
        lead === null ? null : ctx.build(lead, style("labelSmall")),
        prop(node.props, "showTrailingIcon") === false ? null : control(24, 24),
      ),
    });
  },
  CheckboxListTile: (node, ctx) => buildTile(node, ctx, secondary(node, ctx), control(40, 48)),
  SwitchListTile: (node, ctx) => buildTile(node, ctx, secondary(node, ctx), control(60, 48)),
  // The radio is the leading part, 40 wide; the secondary widget trails.
  RadioListTile: (node, ctx) => buildTile(node, ctx, control(40, 40), secondary(node, ctx)),
  PreferredSize: (node, ctx) => ctx.build(required(node, "child")),
  AppBar: (node, ctx) => {
    const p = node.props;
    const toolbarHeight = num(p, "toolbarHeight", kToolbarHeight);
    const lead = only(node, "leading");
    const title = only(node, "title");
    const bottom = only(node, "bottom");
    return buildAppBar({
      leading: lead === null ? null : ctx.build(lead),
      leadingIsIconButton: lead?.type === "IconButton",
      title:
        title === null
          ? null
          : ctx.build(title, style("titleLarge"), {
              defaults: { softWrap: false, overflow: "ellipsis" },
              maxScale: kMaxTitleTextScaleFactor,
            }),
      actions: slot(node, "actions").map((a) => ctx.build(a)),
      bottom: bottom === null ? null : ctx.build(bottom),
      toolbarHeight,
      centerTitle: prop(p, "centerTitle") === true,
      titleSpacing: num(p, "titleSpacing", kMiddleSpacing),
      leadingWidth: num(p, "leadingWidth", kLeadingWidth),
      textDirection: ctx.direction,
    });
  },
  Scaffold: (node, ctx) => {
    const part = (name: string): RenderBox | null => {
      const found = only(node, name);
      return found === null ? null : ctx.build(found);
    };
    const barNode = only(node, "appBar");
    const bottomNode = barNode === null ? null : only(barNode, "bottom");
    const toolbar =
      barNode === null || typeof prop(barNode.props, "toolbarHeight") !== "number"
        ? undefined
        : num(barNode.props, "toolbarHeight", kToolbarHeight);
    const bar = part("appBar");
    return new RenderScaffold({
      appBar:
        bar === null
          ? null
          : new RenderConstrainedBox({
              additionalConstraints: new BoxConstraints({
                maxHeight: appBarPreferredHeight(toolbar, preferredHeight(bottomNode)),
              }),
              child: bar,
            }),
      body: part("child"),
      bottomNavigationBar: part("bottomNavigationBar"),
      bottomSheet: part("bottomSheet"),
      floatingActionButton: part("floatingActionButton"),
      fabLocation: (prop(node.props, "floatingActionButtonLocation") ?? "endFloat") as FabLocation,
      textDirection: ctx.direction,
    });
  },
  TextField: buildTextField,
  // `Autocomplete`'s field is a `TextFormField` without a decoration: one underline field.
  Autocomplete: (node, ctx) =>
    buildTextField({ id: node.id, type: "TextField", props: { hintText: node.props["hintText"] } }, ctx),
  DropdownMenu: buildDropdownMenu,
  NavigationBar: buildNavigationBarNode,
  NavigationRail: buildNavigationRailNode,
  TabBar: buildTabBarNode,
  AlertDialog: buildAlertDialogNode,
  BottomSheet: buildBottomSheetNode,
  PopupMenu: buildPopupMenuNode,
  // `Drawer`: a `ConstrainedBox` expanded to the drawer width (304 by default) around its child.
  Drawer: (node, ctx) =>
    new RenderConstrainedBox({
      additionalConstraints: new BoxConstraints({
        minWidth: num(node.props, "width", 304),
        maxWidth: num(node.props, "width", 304),
        minHeight: Number.POSITIVE_INFINITY,
        maxHeight: Number.POSITIVE_INFINITY,
      }),
      child: ctx.build(required(node, "child")),
    }),
  Chip: buildChipNode,
  ActionChip: buildChipNode,
  ChoiceChip: buildChipNode,
  ...leafBuilders,
  ...moreBuilders,
  FilterChip: buildChipNode,
  InputChip: buildChipNode,
  IconButton: (node, ctx) =>
    buildButton({
      kind: "icon",
      label: ctx.build(required(node, "child")),
      tapTargetSize: tapTarget(node.props),
      density: density(node.props),
      textDirection: ctx.direction,
      textScale: ctx.text?.textScaler.textScaleFactor ?? 1,
    }),
  FloatingActionButton: (node, ctx) => {
    const label = only(node, "label");
    const child = only(node, "child");
    return buildFab({
      kind: (prop(node.props, "kind") ?? "regular") as FabKind,
      child: child === null ? null : ctx.build(child),
      label: label === null ? null : ctx.build(label, style("labelLarge")),
      tapTargetSize: tapTarget(node.props),
      textDirection: ctx.direction,
    });
  },
};

for (const type of Object.keys(buttonKinds)) {
  builders[type] = (node, ctx) => {
    const icon = only(node, "icon");
    return buildButton({
      kind: buttonKinds[type] as ButtonKind,
      label: ctx.build(required(node, "child"), style("labelLarge")),
      icon: icon === null ? null : ctx.build(icon),
      tapTargetSize: tapTarget(node.props),
      density: density(node.props),
      textDirection: ctx.direction,
      textScale: ctx.text?.textScaler.textScaleFactor ?? 1,
    });
  };
}

/** Node types only a composite builds, from its children. */
const partTypes: readonly string[] = [
  "NavigationDestination",
  "NavigationRailDestination",
  "Tab",
  "PopupMenuItem",
  "PopupMenuDivider",
];

/** The node types this module builds, the parts of composites included. */
export const materialTypes: ReadonlySet<string> = new Set([...Object.keys(builders), ...partTypes]);

/** Builds `node`, a Material composite. */
export function buildMaterial(node: SceneNode, ctx: MaterialContext): RenderBox {
  const builder = builders[node.type];
  if (builder === undefined) {
    throw new Error(`node "${node.id}": ${node.type} is not a Material composite`);
  }
  return builder(node, ctx);
}
