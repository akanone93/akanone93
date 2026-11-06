// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The layout editors' model (PC-22, WGT-010): how a widget lays out its children — a row, column or flex as an auto
 * layout with a direction, an alignment pad, a distribution, a gap and a main-axis size; a wrap with its runs — and how
 * its parent lays it out: in a row or column it hugs its content, fills the free space (`Expanded`, by its flex factor)
 * or has a fixed size; in a stack it is pinned to edges (`Positioned`). Every change is a recipe over a page's root,
 * written as the Flutter widgets a developer would write.
 */

import { widget } from "@plux/schema";
import { convertRecipe, replaceAt, wrapperOf } from "../plugin-canvas/context-actions.ts";
import { findNode } from "../plugin-canvas/select/edits.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const num = (v: unknown): number | undefined => (typeof v === "number" && Number.isFinite(v) ? v : undefined);

export type Axis = "horizontal" | "vertical";

const FLEXES: ReadonlySet<string> = new Set(["Row", "Column", "Flex"]);
const WRAPPERS: ReadonlySet<string> = new Set(["Expanded", "Flexible"]);

/** A box that lays out its children: a row, column or flex along its axis, or a wrap of runs. */
export interface LayoutBox {
  readonly kind: "flex" | "wrap";
  readonly type: string;
  readonly axis: Axis;
  readonly props: Readonly<Obj>;
}

/** How the parent lays a widget out: along a flex's axis (maybe in an `Expanded`), or in a stack (maybe pinned). */
export type LayoutItem =
  | {
      readonly kind: "flex";
      readonly axis: Axis;
      readonly parent: string;
      /** The `Expanded` or `Flexible` the widget sits in, with its flex factor. */
      readonly wrapper?: { readonly id: string; readonly type: string; readonly flex: number };
    }
  | {
      readonly kind: "stack";
      readonly parent: string;
      /** The `Positioned` the widget sits in, with its pins. */
      readonly positioned?: { readonly id: string; readonly props: Readonly<Obj> };
    };

export interface LayoutContext {
  readonly box?: LayoutBox;
  readonly item?: LayoutItem;
}

const axisOf = (node: Obj): Axis =>
  node["type"] === "Column" || (isObj(node["props"]) && node["props"]["direction"] === "vertical")
    ? "vertical"
    : "horizontal";

function boxOf(node: Obj): LayoutBox | undefined {
  const type = String(node["type"]);
  const props = isObj(node["props"]) ? node["props"] : {};
  if (FLEXES.has(type)) {
    return { kind: "flex", type, axis: axisOf(node), props };
  }
  return type === "Wrap" ? { kind: "wrap", type, axis: axisOf(node), props } : undefined;
}

function itemOf(root: unknown, parent: Obj | null): LayoutItem | undefined {
  if (parent === null) {
    return undefined;
  }
  const ptype = String(parent["type"]);
  const pid = String(parent["id"]);
  if (FLEXES.has(ptype)) {
    return { kind: "flex", axis: axisOf(parent), parent: pid };
  }
  if (ptype === "Stack") {
    return { kind: "stack", parent: pid };
  }
  const outer = findNode(root, pid)?.parent ?? null;
  if (WRAPPERS.has(ptype) && outer !== null && FLEXES.has(String(outer["type"]))) {
    const flex = num(isObj(parent["props"]) ? parent["props"]["flex"] : undefined) ?? 1;
    return { kind: "flex", axis: axisOf(outer), parent: String(outer["id"]), wrapper: { id: pid, type: ptype, flex } };
  }
  if (ptype === "Positioned" && outer?.["type"] === "Stack") {
    const props = isObj(parent["props"]) ? parent["props"] : {};
    return { kind: "stack", parent: String(outer["id"]), positioned: { id: pid, props } };
  }
  return undefined;
}

/** The layout of node `id` in the tree under `root`: the box it is and the item its parent makes of it. */
export function layoutContextOf(root: unknown, id: string): LayoutContext {
  const at = findNode(root, id);
  if (at === null) {
    return {};
  }
  const node = at.node as Obj;
  const box = boxOf(node);
  const item = itemOf(root, at.parent);
  return { ...(box === undefined ? {} : { box }), ...(item === undefined ? {} : { item }) };
}

/** The props of a flex for a point of its alignment pad: the main and cross alignments along its axis. */
export type Align3 = "start" | "center" | "end";

/**
 * The alignment pad's point of a flex: its main and cross alignment as start, centre or end — unset, Flutter's
 * defaults, start along the axis and centre across it; undefined for a distribution or a stretch.
 */
export function padOf(props: Readonly<Obj>): { readonly main: Align3 | undefined; readonly cross: Align3 | undefined } {
  const three = (v: unknown, unset: Align3): Align3 | undefined =>
    v === "start" || v === "center" || v === "end" ? v : v === undefined ? unset : undefined;
  return { main: three(props["mainAxisAlignment"], "start"), cross: three(props["crossAxisAlignment"], "center") };
}

/** Sets props of node `id` (removing those set to undefined); removes `props` when it ends empty. */
export function setPropsRecipe(id: string, values: Readonly<Obj>): (root: unknown) => void {
  return (root) => {
    const at = findNode(root, id);
    if (at === null) {
      return;
    }
    const node = at.node as Obj;
    const props: Obj = isObj(node["props"]) ? node["props"] : {};
    for (const [k, v] of Object.entries(values)) {
      if (v === undefined) {
        delete props[k];
      } else {
        props[k] = v;
      }
    }
    if (Object.keys(props).length === 0) {
      delete node["props"];
    } else {
      node["props"] = props;
    }
  };
}

/** Turns a flex the other way: a row into a column and back (keeping its props), a flex or wrap by its direction. */
export function flipRecipe(id: string, box: LayoutBox): (root: unknown) => void {
  if (box.type === "Row" || box.type === "Column") {
    return convertRecipe(id, box.type === "Row" ? "Column" : "Row");
  }
  return setPropsRecipe(id, { direction: box.axis === "horizontal" ? "vertical" : "horizontal" });
}

/** How a widget is sized along an axis in a row or column: its content, the free space, or a number. */
export type Sizing = "hug" | "fill" | "fixed";

const sizeProp = (axis: Axis): "width" | "height" => (axis === "horizontal" ? "width" : "height");
const declares = (type: unknown, prop: string): boolean =>
  widget(String(type))?.props.some((p) => p.name === prop) === true;

/** How node `id` is sized along `axis`: filling (its `Expanded`), fixed (its own width or height), else hugging. */
export function sizingOf(node: Obj, item: LayoutItem | undefined, axis: Axis): Sizing {
  if (item?.kind === "flex" && item.axis === axis && item.wrapper !== undefined) {
    return "fill";
  }
  const props = isObj(node["props"]) ? node["props"] : {};
  return props[sizeProp(axis)] === undefined ? "hug" : "fixed";
}

/** Replaces a one-child wrapper by its child, in its parent's list or slot. */
function unwrapOne(root: unknown, wrapperId: string): void {
  replaceAt(root, wrapperId, (w) => {
    const slots = isObj(w["slots"]) ? Object.values(w["slots"]) : [];
    const kids = Array.isArray(w["children"]) ? (w["children"] as unknown[]) : [];
    const [only] = [...slots, ...kids];
    return isObj(only) ? only : w;
  });
}

/**
 * Sizes node `id` along `axis` (PC-22): fill wraps it in an `Expanded` (id `wrapperId`) and drops its own size; hug
 * takes it out of its `Expanded` and drops its own size; fixed takes it out and gives it `size` — as its own width or
 * height when its widget has one, else in a `SizedBox` (id `wrapperId`).
 */
export function sizingRecipe(
  id: string,
  axis: Axis,
  sizing: Sizing,
  wrapperId: string,
  size: number,
): (root: unknown) => void {
  return (root) => {
    const ctx = layoutContextOf(root, id);
    const along = ctx.item?.kind === "flex" && ctx.item.axis === axis;
    const wrapper = ctx.item?.kind === "flex" && along ? ctx.item.wrapper : undefined;
    setPropsRecipe(id, { [sizeProp(axis)]: undefined })(root);
    if (sizing === "fill") {
      if (wrapper === undefined && along) {
        replaceAt(root, id, (node) => wrapperOf("Expanded", wrapperId, node));
      }
      return;
    }
    if (wrapper !== undefined) {
      unwrapOne(root, wrapper.id);
    }
    if (sizing === "fixed") {
      fixSize(root, id, sizeProp(axis), wrapperId, size);
    }
  };
}

/** Gives node `id` a fixed `prop` of `size`: its own when its widget has one, else a `SizedBox` around it. */
function fixSize(root: unknown, id: string, prop: "width" | "height", wrapperId: string, size: number): void {
  const at = findNode(root, id);
  if (at !== null && declares((at.node as Obj)["type"], prop)) {
    setPropsRecipe(id, { [prop]: size })(root);
  } else {
    replaceAt(root, id, (node) => ({ ...wrapperOf("SizedBox", wrapperId, node), props: { [prop]: size } }));
  }
}

/** The edges and sizes a `Positioned` pins a widget by. */
export const PINS = ["left", "top", "right", "bottom", "width", "height"] as const;
export type Pin = (typeof PINS)[number];

/**
 * Pins or unpins node `id` in its stack (PC-22): a first pin wraps it in a `Positioned` (id `wrapperId`); the last pin
 * taken off unwraps it. `value` undefined takes the pin off.
 */
export function pinRecipe(id: string, pin: Pin, value: number | undefined, wrapperId: string): (root: unknown) => void {
  return (root) => {
    const ctx = layoutContextOf(root, id);
    if (ctx.item?.kind !== "stack") {
      return;
    }
    const positioned = ctx.item.positioned;
    if (positioned === undefined) {
      if (value !== undefined) {
        replaceAt(root, id, (node) => ({ ...wrapperOf("Positioned", wrapperId, node), props: { [pin]: value } }));
      }
      return;
    }
    setPropsRecipe(positioned.id, { [pin]: value })(root);
    const after = findNode(root, positioned.id)?.node as Obj | undefined;
    if (after !== undefined && !isObj(after["props"])) {
      unwrapOne(root, positioned.id);
    }
  };
}

/** The layout props a "Reset layout" clears: every one of the widget's that the inspector files under Layout. */
export function resetLayoutRecipe(id: string, names: readonly string[]): (root: unknown) => void {
  return setPropsRecipe(id, Object.fromEntries(names.map((n) => [n, undefined])));
}

/** The size classes node `id` overrides, each with the props it sets (WGT-010). */
export function overridesOf(node: Obj | undefined): { readonly medium: string[]; readonly expanded: string[] } {
  const r = isObj(node?.["responsive"]) ? (node["responsive"] as Obj) : {};
  const keys = (bp: string) => (isObj(r[bp]) ? Object.keys(r[bp] as Obj) : []);
  return { medium: keys("medium"), expanded: keys("expanded") };
}

/** Clears node `id`'s overrides for one size class (WGT-010); `responsive` goes when nothing is left. */
export function clearOverridesRecipe(id: string, bp: "medium" | "expanded"): (root: unknown) => void {
  return (root) => {
    const at = findNode(root, id);
    const node = at?.node as Obj | undefined;
    if (node === undefined || !isObj(node["responsive"])) {
      return;
    }
    const r = node["responsive"] as Obj;
    delete r[bp];
    if (Object.keys(r).length === 0) {
      delete node["responsive"];
    }
  };
}
