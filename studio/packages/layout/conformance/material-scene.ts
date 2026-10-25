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
import { RenderConstrainedBox } from "../src/flutter/proxy-box.ts";
import type { RenderBox } from "../src/flutter/render-box.ts";
import type { TextStyle } from "../src/flutter/text-style.ts";
import { buildButton, buildFab, type ButtonKind, type FabKind } from "../src/flutter/material/buttons.ts";
import { buildListTile } from "../src/flutter/material/list-tile.ts";
import { m3TextStyle, MaterialTapTargetSize, type TextRole, VisualDensity } from "../src/flutter/material/theme.ts";
import type { SceneNode, SceneText } from "./scene.ts";

/** The theme's font family, as `ThemeData(fontFamily:)` in the golden writer. */
export const materialFontFamily = "NotoSans";

/** Builds one child of a composite; `base` is the text style `Text` descendants inherit. */
export type BuildChild = (node: SceneNode, base?: TextStyle) => RenderBox;

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

export function tapTarget(props: Record<string, unknown>): MaterialTapTargetSize {
  return props["tapTargetSize"] === "shrinkWrap" ? MaterialTapTargetSize.shrinkWrap : MaterialTapTargetSize.padded;
}

export function density(props: Record<string, unknown>): VisualDensity {
  return VisualDensity.named(props["density"]);
}

const buttonKinds: Readonly<Record<string, ButtonKind>> = {
  ElevatedButton: "elevated",
  FilledButton: "filled",
  FilledTonalButton: "filledTonal",
  OutlinedButton: "outlined",
  TextButton: "text",
};

type Builder = (node: SceneNode, ctx: MaterialContext) => RenderBox;

const builders: Record<string, Builder> = {
  Icon: (node) => {
    const size = typeof node.props["size"] === "number" ? node.props["size"] : 24;
    return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: size, height: size }) });
  },
  ListTile: (node, ctx) => {
    const p = node.props;
    const dense = p["dense"] === true;
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
      isThreeLine: p["isThreeLine"] === true,
      visualDensity: density(p),
      textDirection: ctx.direction,
    });
  },
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
      kind: (node.props["kind"] ?? "regular") as FabKind,
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
