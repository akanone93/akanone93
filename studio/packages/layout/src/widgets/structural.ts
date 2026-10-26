// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Builders for the widgets that add no layout of their own on the canvas
 * (STU-005): behaviour wrappers (gestures, drag and drop, transitions and
 * implicit animations at their end values), the structural primitives
 * (`If`, `Match`, `ForEach`, `Slot`, `Responsive`) and the native slot
 * placeholder (WGT-031). Each lays out the one child it shows; the choice of
 * child is the design-time rule written beside it.
 */

import { BoxConstraints } from "../flutter/constraints.ts";
import { Alignment } from "../flutter/geometry.ts";
import { RenderConstrainedBox } from "../flutter/proxy-box.ts";
import type { RenderBox } from "../flutter/render-box.ts";
import { RenderPositionedBox } from "../flutter/shifted-box.ts";
import type { WidgetBuilder, WidgetContext, WidgetNode } from "./screen.ts";

/** A box that takes no space: what a widget with nothing to show lays out as. */
export function emptyBox(): RenderBox {
  return new RenderConstrainedBox({ additionalConstraints: BoxConstraints.tightFor({ width: 0, height: 0 }) });
}

function prop(node: WidgetNode, name: string): unknown {
  return (node.props as Record<string, unknown> | undefined)?.[name];
}

/** The widget in slot `name`, or the first of `children` when the slot is a list. */
function slot(node: WidgetNode, name: string): WidgetNode | undefined {
  return node.slots?.[name];
}

/** The first child or slot of `node`, for a wrapper whose slot is `child`. */
function firstChild(node: WidgetNode): WidgetNode | undefined {
  return slot(node, "child") ?? node.children?.[0] ?? Object.values(node.slots ?? {})[0];
}

/** Lays out `shown`, or nothing. The wrapper's own id has no box: its child's box carries the child's id. */
function show(shown: WidgetNode | undefined, ctx: WidgetContext): RenderBox {
  return shown === undefined ? emptyBox() : ctx.build(shown);
}

/** The slot of `names` that is filled first. */
function firstFilled(node: WidgetNode, names: readonly string[]): WidgetNode | undefined {
  for (const name of names) {
    const found = slot(node, name);
    if (found !== undefined) {
      return found;
    }
  }
  return undefined;
}

/** A widget that lays out its `child`: behaviour (taps, drags, hero flights, tooltips) and animation end values. */
const passThrough: WidgetBuilder = (node, ctx) => show(firstChild(node), ctx);

/** `If`: the `then` slot, as when its condition holds; the `else` slot has no design-time view. */
const ifBuilder: WidgetBuilder = (node, ctx) => show(slot(node, "then") ?? node.children?.[0], ctx);

/**
 * `Match`: the branch of the literal `value` when the document gives a literal
 * and it names a case, else the `otherwise` slot, else the first branch.
 */
const matchBuilder: WidgetBuilder = (node, ctx) => {
  const cases = prop(node, "cases");
  const value = prop(node, "value");
  const index = Array.isArray(cases) && typeof value === "string" ? cases.indexOf(value) : -1;
  const branches = node.children ?? [];
  return show((index >= 0 ? branches[index] : undefined) ?? slot(node, "otherwise") ?? branches[0], ctx);
};

/** `ForEach`: its `item` template once, as the canvas has no items to repeat it over. */
const forEachBuilder: WidgetBuilder = (node, ctx) => show(slot(node, "item") ?? node.children?.[0], ctx);

/** `Slot`: its `fallback`, the content an instance that fills nothing shows. */
const slotBuilder: WidgetBuilder = (node, ctx) => show(slot(node, "fallback"), ctx);

/** `Responsive`: the compact variant, the phone the canvas draws; else the next wider one. */
const responsive: WidgetBuilder = (node, ctx) => show(firstFilled(node, ["compact", "medium", "expanded"]), ctx);

/** `DataScope`: the loaded state, else the first state that has a widget. */
const dataScope: WidgetBuilder = (node, ctx) => show(firstFilled(node, ["success", "loading", "empty", "error"]), ctx);

/** `AnimatedCrossFade`: the child its `crossFadeState` ends on. */
const crossFade: WidgetBuilder = (node, ctx) =>
  show(slot(node, prop(node, "crossFadeState") === "showSecond" ? "secondChild" : "firstChild"), ctx);

/** `TabBarView`: the child at `index` (the first by default). */
const tabBarView: WidgetBuilder = (node, ctx) => {
  const index = prop(node, "index");
  const children = node.children ?? [];
  return show(children[typeof index === "number" ? index : 0] ?? children[0], ctx);
};

/** A labelled placeholder box: `width` and `height` when the node declares them, else the available width and the control height. */
export function placeholder(node: WidgetNode, ctx: WidgetContext, label: string): RenderBox {
  const declared = (name: string): number | null => {
    const value = prop(node, name);
    return typeof value === "number" ? value : null;
  };
  ctx.mark(node.id, "placeholder", label);
  const box = new RenderConstrainedBox({
    additionalConstraints: BoxConstraints.tightFor({
      width: declared("width") ?? ctx.available(),
      height: declared("height") ?? ctx.theme.controlHeight,
    }),
    child: new RenderPositionedBox({ alignment: Alignment.center, child: ctx.paragraph(label) }),
  });
  box.debugId = node.id;
  return box;
}

/** `NativeSlot`: the host app's widget cannot run in Studio, so the slot draws as its labelled placeholder (WGT-031). */
const nativeSlot: WidgetBuilder = (node, ctx) => {
  const name = prop(node, "name");
  return placeholder(node, ctx, typeof name === "string" ? name : "NativeSlot");
};

/** `SelectableText`: a `Text` of `data`, laid out as one paragraph. */
const selectableText: WidgetBuilder = (node, ctx) => {
  const data = prop(node, "data");
  const text = typeof data === "string" ? data : "";
  ctx.mark(node.id, "text", text);
  const box = ctx.paragraph(text);
  box.debugId = node.id;
  return box;
};

/** The characters of a `TextSpan` document value: its own text, then its children's. */
function spanText(span: unknown): string {
  if (typeof span !== "object" || span === null) {
    return "";
  }
  const record = span as Record<string, unknown>;
  const own = typeof record["text"] === "string" ? record["text"] : "";
  const children = Array.isArray(record["children"]) ? record["children"] : [];
  return own + children.map(spanText).join("");
}

/** `RichText`: the spans' characters as one paragraph in the screen's text style; span styles have no design-time effect. */
const richText: WidgetBuilder = (node, ctx) => {
  const text = spanText(prop(node, "textSpan"));
  ctx.mark(node.id, "text", text);
  const box = ctx.paragraph(text);
  box.debugId = node.id;
  return box;
};

/** `Image`: its declared `width` and `height`, labelled with the source's name (the canvas loads no pictures). */
const image: WidgetBuilder = (node, ctx) => {
  const source = prop(node, "source");
  const name = typeof source === "object" && source !== null ? (source as Record<string, unknown>)["name"] : undefined;
  return placeholder(node, ctx, typeof name === "string" ? name : "Image");
};

const wrapperTypes: readonly string[] = [
  "GestureDetector",
  "InkWell",
  "Hero",
  "Dismissible",
  "Draggable",
  "DragTarget",
  "Tooltip",
  "FormScope",
  "Scrollbar",
  "RefreshIndicator",
  "InteractiveViewer",
  "MenuAnchor",
  "BackdropFilter",
  "FadeTransition",
  "RotationTransition",
  "ScaleTransition",
  "SlideTransition",
  "AnimatedRotation",
  "AnimatedScale",
  "AnimatedSize",
  "AnimatedSlide",
  "AnimatedSwitcher",
  "Timeline",
];

/** The builders of the widgets that show one child as it is; merged into the default registry. */
export const structuralWidgetBuilders: ReadonlyArray<readonly [string, WidgetBuilder]> = [
  ...wrapperTypes.map((type): readonly [string, WidgetBuilder] => [type, passThrough]),
  ["If", ifBuilder],
  ["Match", matchBuilder],
  ["ForEach", forEachBuilder],
  ["Slot", slotBuilder],
  ["Responsive", responsive],
  ["DataScope", dataScope],
  ["AnimatedCrossFade", crossFade],
  ["TabBarView", tabBarView],
  ["NativeSlot", nativeSlot],
  ["SelectableText", selectableText],
  ["RichText", richText],
  ["Image", image],
];
