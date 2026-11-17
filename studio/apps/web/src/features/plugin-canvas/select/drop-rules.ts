// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type SlotDescriptor, widget } from "@plux/schema";
import type { Box } from "./geometry.ts";
import type { DropTarget } from "./reparent.ts";
import { lineage, type NodeIndex } from "./tree.ts";

/**
 * Where a dragged widget can go (PC-20). A drop lands in the innermost widget under the pointer that can take it; one
 * that Flutter would refuse at run time (a sliver outside a scroll view, an `Expanded` outside a flex, a `Positioned`
 * outside a stack) is never written, and the person is told why in a few words.
 */

/** Why a drop is refused; each is one short message. */
export type Refusal = "needsScrollView" | "needsFlex" | "needsStack" | "full" | "nowhere" | "offScreen";

/** The chrome slots of a screen and the widgets each is made for: an app bar is never filled with a button. */
export const chromeSlots: Readonly<Record<string, readonly string[]>> = {
  appBar: ["AppBar"],
  floatingActionButton: ["FloatingActionButton"],
  bottomNavigationBar: ["NavigationBar", "BottomNavigationBar", "BottomAppBar", "TabBar"],
  drawer: ["Drawer", "NavigationDrawer"],
  endDrawer: ["Drawer", "NavigationDrawer"],
  bottomSheet: ["BottomSheet"],
  bottom: ["TabBar"],
  persistentFooterButtons: [],
};

/**
 * Slots that show a state of their widget or a part drawn only now and then (an empty list's view, a dismissed row's
 * background): a drop never fills them by itself. The Layers tree fills them, by their own rows.
 */
const stateSlots = new Set([
  "prototypeItem",
  "empty",
  "loading",
  "error",
  "fallback",
  "childWhenDragging",
  "feedback",
  "background",
  "secondaryBackground",
  "selectedIcon",
  "selectedTrailingIcon",
  "deleteIcon",
  "otherwise",
  "else",
]);

/** The slots that hold a widget's content, filled first, in this order. */
const contentSlots = ["body", "child", "item", "success", "content", "then", "sliver", "children", "slivers"];

/** Slots whose list runs across: an app bar's actions, a footer's buttons, a segmented control's segments. */
const rowSlots = new Set(["actions", "persistentFooterButtons", "segments"]);

/** Slots that take slivers only. */
const sliverSlots = new Set(["slivers", "headerSlivers", "sliver"]);

const flexParents = new Set(["Row", "Column", "Flex"]);
const stackParents = new Set(["Stack", "IndexedStack"]);
const flexChildren = new Set(["Expanded", "Flexible", "Spacer"]);
const stackChildren = new Set(["Positioned", "AnimatedPositioned"]);

/** Slivers: the registry names every one `Sliver…`, as the compiler's parent-data rule does. */
export const isSliver = (type: string): boolean => type.startsWith("Sliver");

/**
 * What a widget of type `child` needs to go into `parent`'s `slot` (its `children` when null): nothing, a wrapper the
 * drop adds (a box among slivers goes in a `SliverToBoxAdapter`, a sliver in a screen's empty body in a
 * `CustomScrollView`), or the reason it cannot go there at all.
 */
export function placement(parent: string, slot: string | null, child: string): { readonly wrap?: string } | Refusal {
  if (flexChildren.has(child)) {
    return slot === null && flexParents.has(parent) ? {} : "needsFlex";
  }
  if (stackChildren.has(child)) {
    return slot === null && stackParents.has(parent) ? {} : "needsStack";
  }
  return sliverPlacement(parent, slot, child);
}

/** The sliver rule: slivers among slivers only; a box there, or a sliver on an empty screen, goes in a wrapper. */
function sliverPlacement(parent: string, slot: string | null, child: string): { readonly wrap?: string } | Refusal {
  const sliverSlot = slot !== null && sliverSlots.has(slot);
  if (sliverSlot === isSliver(child)) {
    return {};
  }
  if (sliverSlot) {
    return { wrap: "SliverToBoxAdapter" };
  }
  return parent === "Scaffold" && slot === "body" ? { wrap: "CustomScrollView" } : "needsScrollView";
}

/** What the drop resolution reads of the canvas. */
export interface DropEnv {
  readonly index: NodeIndex;
  readonly boxOf: (id: string) => Box | undefined;
  readonly typeOf: (id: string) => string | undefined;
  /** Layers that take nothing in (locked). */
  readonly locked?: (id: string) => boolean;
  /** Whether the canvas previews a right-to-left locale: rows run from the right, a tile's leading side is there. */
  readonly rtl?: boolean;
}

/** What is dragged: a widget type, and the nodes moved (none for a new widget). */
export interface Dragged {
  readonly type: string;
  readonly ids: readonly string[];
}

export type DropPlan =
  | { readonly ok: true; readonly target: DropTarget }
  | {
      readonly ok: false;
      readonly reason: Refusal;
      /** The widget the refusal is about (outlined while the drag is over it), or null off the screens. */
      readonly at: string | null;
    };

const inside = (b: Box | undefined, x: number, y: number): b is Box =>
  b !== undefined && x >= b.l && x <= b.r && y >= b.t && y <= b.b;
const area = (b: Box): number => (b.r - b.l) * (b.b - b.t);

/** How a list lays out its items, for the insertion index and indicator. */
type Flow = "row" | "column" | "reading" | "end" | "stack";

function flowOf(parent: string, slot: string | null): Flow {
  if (slot !== null) {
    return rowSlots.has(slot) ? "row" : "column";
  }
  if (parent === "Row") {
    return "row";
  }
  if (parent === "Stack") {
    return "stack";
  }
  if (parent === "Wrap" || parent === "Table") {
    return "reading";
  }
  return parent === "IndexedStack" || parent === "TabBarView" ? "end" : "column";
}

/** The items of `parent`'s list `slot` (its `children` when null), in order. */
const itemsOf = (index: NodeIndex, parent: string, slot: string | null): readonly string[] =>
  (index.children.get(parent) ?? []).filter((k) => (index.slot.get(k) ?? null) === slot);

/** Whether point (x, y) comes before box `k` along `flow`. */
function before(flow: Flow, k: Box, x: number, y: number, rtl: boolean): boolean {
  const midX = (k.l + k.r) / 2;
  const ahead = rtl ? x > midX : x < midX;
  if (flow === "row") {
    return ahead;
  }
  if (flow === "reading") {
    return y < k.t || (y <= k.b && ahead);
  }
  return y < (k.t + k.b) / 2;
}

/** The line between items at the insertion point, or the list's box when the items overlap. */
function indicatorOf(flow: Flow, box: Box, prev: Box | undefined, next: Box | undefined, rtl: boolean): Box {
  if (flow === "end") {
    return box;
  }
  if (flow === "column") {
    const py = next !== undefined ? next.t : prev !== undefined ? prev.b : box.t;
    return { l: box.l, t: py, r: box.r, b: py };
  }
  const start = (b: Box) => (rtl ? b.r : b.l);
  const end = (b: Box) => (rtl ? b.l : b.r);
  const px = next !== undefined ? start(next) : prev !== undefined ? end(prev) : start(box);
  const span = flow === "reading" ? (next ?? prev ?? box) : box;
  return { l: px, t: span.t, r: px, b: span.b };
}

/** A drop into the list `slot` of `parent` (its `children` when null) at (x, y). */
function listTarget(
  env: DropEnv,
  parent: string,
  box: Box,
  slot: string | null,
  dragged: Dragged,
  x: number,
  y: number,
  wrap: string | undefined,
): DropTarget {
  const type = env.typeOf(parent) ?? "";
  const flow = flowOf(type, slot);
  const own = slot === null ? {} : { slot, list: true };
  const extra = wrap === undefined ? own : { ...own, wrap };
  const items = itemsOf(env.index, parent, slot).filter((k) => !dragged.ids.includes(k));
  if (flow === "stack") {
    const [px, py] = [Math.round(x), Math.round(y)];
    return {
      parent,
      index: items.length,
      indicator: { l: px, t: py, r: px, b: py },
      position: { left: px - Math.round(box.l), top: py - Math.round(box.t) },
    };
  }
  const rtl = env.rtl === true && (flow === "row" || flow === "reading");
  let at = items.length;
  if (flow !== "end") {
    for (const [i, k] of items.entries()) {
      const b = env.boxOf(k);
      if (b !== undefined && before(flow, b, x, y, rtl)) {
        at = i;
        break;
      }
    }
  }
  const boxAt = (i: number): Box | undefined => {
    const k = items[i];
    return k === undefined ? undefined : env.boxOf(k);
  };
  return { parent, index: at, indicator: indicatorOf(flow, box, boxAt(at - 1), boxAt(at), rtl), ...extra };
}

/** The slots a drop at (x, y) may fill, in the order tried. */
function slotOrder(d: { readonly slots: readonly SlotDescriptor[] }, type: string, box: Box, x: number, rtl: boolean) {
  const named = (names: readonly string[]) =>
    names.flatMap((n) => {
      const s = d.slots.find((x) => x.name === n);
      return s === undefined ? [] : [s];
    });
  const madeFor = d.slots.filter((s) => chromeSlots[s.name]?.includes(type) === true);
  const has = (n: string) => d.slots.some((s) => s.name === n);
  // A tile or a bar: its leading part at the start, its trailing part or actions at the end, its title between.
  if (has("leading") && (has("trailing") || has("actions"))) {
    const w = box.r - box.l;
    const fromStart = rtl ? box.r - x : x - box.l;
    const zone = fromStart < Math.min(w * 0.25, 72) ? "start" : fromStart > w - Math.min(w * 0.3, 120) ? "end" : "mid";
    const parts =
      zone === "start"
        ? ["leading"]
        : zone === "end"
          ? ["trailing", "actions"]
          : ["title", "middle", "subtitle", "content", "children", "flexibleSpace"];
    return [...madeFor, ...named(parts)];
  }
  const free = (s: SlotDescriptor) => chromeSlots[s.name] === undefined && !stateSlots.has(s.name);
  const content = named(contentSlots).filter(free);
  const rest = d.slots.filter((s) => free(s) && !content.includes(s));
  return [...madeFor, ...content, ...rest.filter((s) => !s.list), ...rest.filter((s) => s.list)];
}

/** A drop into the `children` of list holder `n` (a Row, a Column, a Stack…), or why its rules refuse it. */
function childrenTarget(env: DropEnv, n: string, type: string, box: Box, dragged: Dragged, x: number, y: number) {
  const p = placement(type, null, dragged.type);
  if (typeof p === "string") {
    return { ok: false, reason: p, at: n } as const;
  }
  const max = widget(type)?.children?.max;
  const items = itemsOf(env.index, n, null).filter((k) => !dragged.ids.includes(k));
  if (max !== undefined && items.length >= max) {
    return { ok: false, reason: "full", at: n } as const;
  }
  return { ok: true, target: listTarget(env, n, box, null, dragged, x, y, undefined) } as const;
}

/** A drop into one of `n`'s slots, in the order `slotOrder` gives; or why its rules refuse it; or null. */
function slotTarget(env: DropEnv, n: string, type: string, box: Box, dragged: Dragged, x: number, y: number) {
  const d = widget(type);
  let refused: DropPlan | null = null;
  for (const s of d === undefined ? [] : slotOrder(d, dragged.type, box, x, env.rtl === true)) {
    const held = itemsOf(env.index, n, s.name).filter((k) => !dragged.ids.includes(k));
    if (!s.list && held.length > 0) {
      continue;
    }
    const p = placement(type, s.name, dragged.type);
    if (typeof p === "string") {
      refused ??= { ok: false, reason: p, at: n };
      continue;
    }
    const wrap = p.wrap === undefined ? {} : { wrap: p.wrap };
    const target: DropTarget = s.list
      ? listTarget(env, n, box, s.name, dragged, x, y, p.wrap)
      : { parent: n, index: 0, indicator: box, slot: s.name, ...wrap };
    return { ok: true, target } as const;
  }
  return refused;
}

/** Where node `n` takes the drop at (x, y) itself, or why it refuses it by rule; null when it simply has no room. */
function targetIn(env: DropEnv, n: string, dragged: Dragged, x: number, y: number): DropPlan | null {
  const type = env.typeOf(n);
  const d = type === undefined ? undefined : widget(type);
  const box = env.boxOf(n);
  if (type === undefined || d === undefined || box === undefined || env.locked?.(n) === true) {
    return null;
  }
  return d.children === undefined
    ? slotTarget(env, n, type, box, dragged, x, y)
    : childrenTarget(env, n, type, box, dragged, x, y);
}

/** Whether (x, y) is near the start or end edge of `box` along its list's flow: the drop goes beside it, not in. */
function nearEdge(flow: Flow, box: Box, x: number, y: number): boolean {
  if (flow === "end" || flow === "stack") {
    return false;
  }
  const across = flow === "column";
  const size = across ? box.b - box.t : box.r - box.l;
  const pos = across ? y - box.t : x - box.l;
  const band = Math.min(size / 4, 12);
  return pos < band || pos > size - band;
}

/** The list node `n` is in: its parent and slot (null for `children`), when the parent holds a list there. */
function listOf(env: DropEnv, n: string): { parent: string; type: string; slot: string | null; box: Box } | undefined {
  const parent = env.index.parent.get(n) ?? null;
  const type = parent === null ? undefined : env.typeOf(parent);
  const d = type === undefined ? undefined : widget(type);
  const slot = env.index.slot.get(n) ?? null;
  const listed = slot === null ? d?.children !== undefined : d?.slots.find((s) => s.name === slot)?.list === true;
  const box = parent === null ? undefined : env.boxOf(parent);
  if (parent === null || type === undefined || box === undefined || !listed || env.locked?.(parent) === true) {
    return undefined;
  }
  return { parent, type, slot, box };
}

/** A drop near the edge of node `n` in a list: beside it in that list, when the list can take the dragged widget. */
function besideTarget(env: DropEnv, n: string, dragged: Dragged, x: number, y: number): DropPlan | null {
  const list = listOf(env, n);
  const box = env.boxOf(n);
  if (list === undefined || box === undefined || !nearEdge(flowOf(list.type, list.slot), box, x, y)) {
    return null;
  }
  const p = placement(list.type, list.slot, dragged.type);
  const max = list.slot === null ? widget(list.type)?.children?.max : undefined;
  const count = itemsOf(env.index, list.parent, list.slot).filter((k) => !dragged.ids.includes(k)).length;
  if (typeof p === "string" || (max !== undefined && count >= max)) {
    return null;
  }
  return { ok: true, target: listTarget(env, list.parent, list.box, list.slot, dragged, x, y, p.wrap) };
}

/** The innermost node under (x, y) that is not being dragged: the deepest, then the smallest, then the last drawn. */
function innermost(env: DropEnv, dragged: Dragged, x: number, y: number): string | undefined {
  let best: { id: string; depth: number; area: number } | undefined;
  for (const id of env.index.parent.keys()) {
    const b = env.boxOf(id);
    if (!inside(b, x, y) || lineage(env.index, id).some((a) => dragged.ids.includes(a))) {
      continue;
    }
    const depth = env.index.depth.get(id) ?? 0;
    const a = area(b);
    if (best === undefined || depth > best.depth || (depth === best.depth && a <= best.area)) {
      best = { id, depth, area: a };
    }
  }
  return best?.id;
}

/** A chrome widget (an app bar, a navigation bar, a floating button) goes to its screen's free chrome slot. */
function chromeTarget(env: DropEnv, from: string, dragged: Dragged): DropPlan | null {
  const slots = Object.entries(chromeSlots).filter(([, types]) => types.includes(dragged.type));
  if (slots.length === 0) {
    return null;
  }
  const scaffold = lineage(env.index, from).find((id) => env.typeOf(id) === "Scaffold");
  const box = scaffold === undefined ? undefined : env.boxOf(scaffold);
  if (scaffold === undefined || box === undefined || env.locked?.(scaffold) === true) {
    return null;
  }
  const free = slots.find(([name]) => itemsOf(env.index, scaffold, name).length === 0);
  return free === undefined
    ? null
    : { ok: true, target: { parent: scaffold, index: 0, indicator: box, slot: free[0] } };
}

/**
 * Resolves a drop at world point (x, y): the innermost widget under the pointer that can take the dragged widget, by
 * the rules above, walking out from there; near the edge of a widget in a list, the drop goes beside it in that list.
 * Refused, with the innermost reason found, when nothing under the pointer can take it.
 */
export function planDrop(env: DropEnv, dragged: Dragged, x: number, y: number): DropPlan {
  const inner = innermost(env, dragged, x, y);
  if (inner === undefined) {
    return { ok: false, reason: "offScreen", at: null };
  }
  const chrome = chromeTarget(env, inner, dragged);
  if (chrome !== null) {
    return chrome;
  }
  let refused: DropPlan | null = null;
  for (const n of lineage(env.index, inner)) {
    const own = targetIn(env, n, dragged, x, y);
    if (own?.ok === true) {
      return besideTarget(env, n, dragged, x, y) ?? own;
    }
    refused ??= own;
  }
  return refused ?? { ok: false, reason: "nowhere", at: inner };
}

/** The node `wrap` makes of `node`: the wrapper of type `wrap`, with id `id`, holding it in its slot. */
export function wrapNode(wrap: string, id: string, node: unknown): unknown {
  return wrap === "CustomScrollView"
    ? { id, type: wrap, slots: { slivers: [node] } }
    : { id, type: wrap, slots: { child: node } };
}
