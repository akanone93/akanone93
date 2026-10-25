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

import { BoxConstraints } from "../src/flutter/constraints.ts";
import type { TextDirection } from "../src/flutter/geometry.ts";
import {
  appBarPreferredHeight,
  buildAppBar,
  kLeadingWidth,
  kMaxTitleTextScaleFactor,
  kMiddleSpacing,
} from "../src/flutter/material/app-bar.ts";
import { type ButtonKind, buildButton, buildFab, type FabKind } from "../src/flutter/material/buttons.ts";
import { buildChip } from "../src/flutter/material/chip.ts";
import { buildInputDecorator, decoratorIcon, RenderEditableLine } from "../src/flutter/material/input-decorator.ts";
import { buildListTile } from "../src/flutter/material/list-tile.ts";
import { type FabLocation, RenderScaffold } from "../src/flutter/material/scaffold.ts";
import {
  kToolbarHeight,
  MaterialTapTargetSize,
  m3TextStyle,
  type TextRole,
  VisualDensity,
} from "../src/flutter/material/theme.ts";
import { RenderConstrainedBox } from "../src/flutter/proxy-box.ts";
import type { RenderBox } from "../src/flutter/render-box.ts";
import type { TextStyle } from "../src/flutter/text-style.ts";
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
function syntheticText(node: SceneNode, name: string, text: string, ctx: MaterialContext, base: TextStyle): RenderBox {
  const box = ctx.build({ id: `${node.id}:${name}`, type: "Text", props: { text } }, base, {
    defaults: { overflow: "ellipsis" },
  });
  box.debugId = null;
  return box;
}

/** `TextField` with an `InputDecoration`: the input line is one line of `bodyLarge`; label and hint add no box. */
function buildTextField(node: SceneNode, ctx: MaterialContext): RenderBox {
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
  return buildInputDecorator({
    input: new RenderEditableLine(line),
    prefixIcon: prefix === null ? null : decoratorIcon(ctx.build(prefix), dens),
    suffixIcon: suffix === null ? null : decoratorIcon(ctx.build(suffix), dens),
    subtext: subtext === null ? null : syntheticText(node, "subtext", subtext, ctx, style("bodySmall")),
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

const builders: Record<string, Builder> = {
  Icon: (node) => {
    const given = prop(node.props, "size");
    const size = typeof given === "number" ? given : 24;
    return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: size, height: size }) });
  },
  ListTile: (node, ctx) => {
    const p = node.props;
    const dense = prop(p, "dense") === true;
    const withSize = (role: TextRole, size: number | null): TextStyle =>
      size === null ? style(role) : { ...style(role), fontSize: size };
    const sub = only(node, "subtitle");
    const lead = only(node, "leading");
    const trail = only(node, "trailing");
    return buildListTile({
      leading: lead === null ? null : ctx.build(lead, style("labelSmall")),
      title: ctx.build(required(node, "title"), withSize("bodyLarge", dense ? 13 : null)),
      subtitle: sub === null ? null : ctx.build(sub, withSize("bodyMedium", dense ? 12 : null)),
      trailing: trail === null ? null : ctx.build(trail, style("labelSmall")),
      isDense: dense,
      isThreeLine: prop(p, "isThreeLine") === true,
      visualDensity: density(p),
      textDirection: ctx.direction,
    });
  },
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
  Chip: buildChipNode,
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

/** The node types this module builds. */
export const materialTypes: ReadonlySet<string> = new Set(Object.keys(builders));

/** Builds `node`, a Material composite. */
export function buildMaterial(node: SceneNode, ctx: MaterialContext): RenderBox {
  const builder = builders[node.type];
  if (builder === undefined) {
    throw new Error(`node "${node.id}": ${node.type} is not a Material composite`);
  }
  return builder(node, ctx);
}
