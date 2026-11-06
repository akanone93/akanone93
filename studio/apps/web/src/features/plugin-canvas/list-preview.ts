// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Lists from data on the canvas (PC-24, WGT-012, DAT-011): a list, grid or page view with an item template is drawn
 * as the device builds it — one copy of the template per item of its `items`, each with `item` and `index` bound to
 * that item — and in the state its data is in: its `loading`, `error` or `empty` slot (a default view when it has
 * none), and a progress row at the end while it `hasMore` pages to load. The state comes from the list's `status`, from
 * the data source its items are bound to under the Preview's data state, or from the state picked for the list in the
 * inspector. Copies after the first carry ids `<template node>~item<n>`, so a click on any copy selects the template.
 */

import type { MockState } from "../preview/switches.ts";
import { type DesignScope, emptyScope, evaluateMockBinding, type SourceMocks } from "./mock-bindings.ts";
import type { Node } from "./plugin-model.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The widgets whose `item` slot is a template repeated per item. */
export const LIST_TYPES: ReadonlySet<string> = new Set([
  "ListView",
  "GridView",
  "PageView",
  "ReorderableListView",
  "SliverList",
  "SliverGrid",
  "InfiniteList",
]);

/** The state a list is drawn in: its items, or one of its data states, or the end of a paged list loading more. */
export type ListState = "items" | "empty" | "loading" | "error" | "more";
export const LIST_STATES: readonly ListState[] = ["items", "empty", "loading", "error", "more"];

/** How many copies of an item template the canvas draws at most; the device builds the rest lazily. */
export const MAX_DRAWN_ITEMS = 24;

/** The mark between a node id and the canvas-only suffix of a copy or a default view (`~item3`, `~loading`). */
export const COPY_MARK = "~";

interface Env {
  readonly mocks: SourceMocks;
  readonly states: Readonly<Record<string, MockState>>;
  readonly scope: DesignScope;
  /** The state picked for a list in the inspector, by list id. */
  readonly picked: ReadonlyMap<string, ListState>;
}

const exprOf = (v: unknown): string | undefined =>
  isObj(v) && Object.keys(v).length === 1 && typeof v["$expr"] === "string" ? v["$expr"] : undefined;

/** The data source an items expression reads (`data.orders.value…` → `orders`). */
export function sourceOfItems(items: unknown): string | undefined {
  const expr = exprOf(items);
  return expr === undefined ? undefined : /^\s*data\.(\w+)\.value\b/.exec(expr)?.[1];
}

/** The items a list draws: a literal list, or the value its binding has at design time; undefined if unknown. */
export function itemsOf(node: Node, env: Env): unknown[] | undefined {
  const items = (node.props as Obj | undefined)?.["items"];
  if (Array.isArray(items)) {
    return items;
  }
  const expr = exprOf(items);
  const value = expr === undefined ? undefined : evaluateMockBinding(expr, env.mocks, env.states, env.scope);
  return Array.isArray(value) ? value : value === null ? [] : undefined;
}

/** The state a list is in: picked in the inspector, else its `status`, else its items' source's preview state. */
export function listStateOf(node: Node, env: Env, items: readonly unknown[] | undefined): ListState {
  const picked = env.picked.get(node.id);
  if (picked !== undefined) {
    return picked;
  }
  const props = (node.props ?? {}) as Obj;
  if (props["status"] === "loading" || props["status"] === "error") {
    return props["status"];
  }
  const source = sourceOfItems(props["items"]);
  const mock = source === undefined ? undefined : env.states[source];
  if (mock === "loading" || mock === "error") {
    return mock;
  }
  if (items !== undefined && items.length === 0) {
    return "empty";
  }
  return props["hasMore"] === true ? "more" : "items";
}

/** A copy of `node` with every id suffixed by `suffix` (none for the first copy). */
function withIds(node: unknown, suffix: string): unknown {
  if (Array.isArray(node)) {
    return node.map((n) => withIds(n, suffix));
  }
  if (!isObj(node)) {
    return node;
  }
  const out: Obj = {};
  for (const [k, v] of Object.entries(node)) {
    out[k] = k === "id" && typeof v === "string" ? `${v}${suffix}` : k === "props" ? v : withIds(v, suffix);
  }
  return out;
}

/** `value` with each `{"$expr": "item…"}` or `{"$expr": "index"}` replaced by its value for one item. */
function bindItem(value: unknown, scope: DesignScope, env: Env): unknown {
  if (Array.isArray(value)) {
    return value.map((v) => bindItem(v, scope, env));
  }
  if (!isObj(value)) {
    return value;
  }
  const expr = exprOf(value);
  if (expr !== undefined && /^\s*(item|index)\b/.test(expr)) {
    const v = evaluateMockBinding(expr, env.mocks, env.states, scope);
    return v === undefined || v === null ? value : typeof v === "number" || typeof v === "boolean" ? String(v) : v;
  }
  const out: Obj = {};
  for (const [k, v] of Object.entries(value)) {
    out[k] = bindItem(v, scope, env);
  }
  return out;
}

/**
 * Copy `index` of an item template for `item` (PC-24, PC-25): ids suffixed after the first copy, `item…` and `index`
 * bound to the item's design-time values.
 */
export function templateCopy(
  template: unknown,
  item: unknown,
  index: number,
  base: {
    readonly mocks: SourceMocks;
    readonly states: Readonly<Record<string, MockState>>;
    readonly scope: DesignScope;
  },
): unknown {
  const env: Env = { ...base, picked: new Map() };
  return bindItem(
    withIds(template, index === 0 ? "" : `${COPY_MARK}item${index}`),
    { ...base.scope, item, index },
    env,
  );
}

const centred = (id: string, child: Obj): Obj => ({
  id: `${id}${COPY_MARK}pad`,
  type: "Padding",
  props: { padding: { all: 24 } },
  slots: { child: { id: `${id}${COPY_MARK}centre`, type: "Center", slots: { child } } },
});

/** The view a list shows in a data state it has no slot for: a progress indicator, or an icon over a muted line. */
function defaultView(id: string, state: "loading" | "error" | "empty" | "more", text: Readonly<StateTexts>): Obj {
  if (state === "loading" || state === "more") {
    return centred(id, { id: `${id}${COPY_MARK}${state}`, type: "CircularProgressIndicator" });
  }
  return centred(id, {
    id: `${id}${COPY_MARK}${state}`,
    type: "Column",
    props: { mainAxisSize: "min", spacing: 8 },
    children: [
      {
        id: `${id}${COPY_MARK}${state}-icon`,
        type: "Icon",
        props: { icon: { name: state === "error" ? "error_outline" : "inbox" }, size: 40 },
      },
      { id: `${id}${COPY_MARK}${state}-text`, type: "Text", props: { data: text[state] } },
    ],
  });
}

/** The words of the default views, in the previewed locale. */
export interface StateTexts {
  readonly empty: string;
  readonly error: string;
}

function drawnState(node: Node, state: "loading" | "error" | "empty", text: StateTexts): Node {
  const slots = (node.slots ?? {}) as Obj;
  const own = slots[state];
  const view = isObj(own) ? own : defaultView(node.id, state, text);
  const { item: _item, prototypeItem: _p, empty: _e, loading: _l, error: _x, ...rest } = slots;
  return { ...node, slots: rest, children: [view] } as unknown as Node;
}

function drawnItems(node: Node, items: readonly unknown[], more: boolean, env: Env, text: StateTexts): Node {
  const slots = (node.slots ?? {}) as Obj;
  const template = slots["item"];
  const copies = items.slice(0, MAX_DRAWN_ITEMS).map((item, index) => templateCopy(template, item, index, env));
  const { item: _item, prototypeItem: _p, empty: _e, loading: _l, error: _x, ...rest } = slots;
  const tail = more ? [defaultView(node.id, "more", text)] : [];
  return { ...node, slots: rest, children: [...copies, ...tail] } as unknown as Node;
}

/** One list as drawn: its copies or its state's view; a list whose items are unknown keeps its template, once. */
function drawnList(node: Node, env: Env, text: StateTexts): Node {
  const slots = (node.slots ?? {}) as Obj;
  if (!isObj(slots["item"])) {
    return node;
  }
  const items = itemsOf(node, env);
  const state = listStateOf(node, env, items);
  if (state === "loading" || state === "error" || state === "empty") {
    return drawnState(node, state, text);
  }
  if (items === undefined) {
    return state === "more" ? drawnItems(node, [undefined], true, env, text) : node;
  }
  return drawnItems(node, items, state === "more", env, text);
}

/**
 * The tree under `root` with every list drawn from its data (PC-24); `root` itself when it has no list. Lists inside
 * an item template are drawn per copy.
 */
export function expandLists(
  root: Node | undefined,
  mocks: SourceMocks,
  states: Readonly<Record<string, MockState>>,
  scope: DesignScope = emptyScope,
  picked: ReadonlyMap<string, ListState> = new Map(),
  text: StateTexts = { empty: "Nothing here yet", error: "Couldn't load" },
): Node | undefined {
  const env: Env = { mocks, states, scope, picked };
  let changed = false;
  const visit = (n: unknown): unknown => {
    if (Array.isArray(n)) {
      return n.map(visit);
    }
    if (!isObj(n)) {
      return n;
    }
    const node = LIST_TYPES.has(String(n["type"]))
      ? drawnList(n as unknown as Node, env, text)
      : (n as unknown as Node);
    changed ||= node !== (n as unknown);
    const out: Obj = { ...(node as unknown as Obj) };
    if (Array.isArray(out["children"])) {
      out["children"] = (out["children"] as unknown[]).map(visit);
    }
    if (isObj(out["slots"])) {
      out["slots"] = Object.fromEntries(Object.entries(out["slots"]).map(([k, v]) => [k, visit(v)]));
    }
    return out;
  };
  const drawn = root === undefined ? undefined : (visit(root) as Node);
  return changed ? drawn : root;
}

/** The template id a canvas copy or default view stands for (`<id>~item3` → `<id>`). */
export const templateIdOf = (id: string): string => {
  const i = id.indexOf(COPY_MARK);
  return i < 0 ? id : id.slice(0, i);
};
