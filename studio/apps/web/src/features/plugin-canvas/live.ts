// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The live surface (PC-15, STU-068): with the Interact tool, screens behave as on a device — a scroll view scrolls
 * inside its frame, a tab, destination or segment clicked is selected, the menu button opens the drawer, a link
 * follows to its screen — without changing the documents. Only the drawn copy carries the live values: a scroll view
 * its offset (`x-studio.scroll`), a switcher its picked item, a prop bound to page state the value a click set.
 * Actions that call networks or functions are not run; they run on a device.
 */

import type { Node } from "./plugin-model.ts";
import { childNodes } from "./select/tree.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

export interface LiveState {
  /** Each scroll view's offset, by node. */
  readonly scroll: ReadonlyMap<string, number>;
  /** A switcher's picked value (an index, or a segment's value), by node. */
  readonly picks: ReadonlyMap<string, unknown>;
  /** The value of page state a click set, by the PXL expression bound to it (`page.tab`). */
  readonly exprs: ReadonlyMap<string, unknown>;
  /** The interaction state the inspector previews on a widget (PC-21), by node: drawn as `x-studio.state`. */
  readonly states?: ReadonlyMap<string, string>;
}

export const emptyLive: LiveState = { scroll: new Map(), picks: new Map(), exprs: new Map() };

/** A widget a click switches: the prop it selects by, the list of its items, and whether it keeps a value list. */
interface Switcher {
  readonly prop: string;
  readonly items: string;
  /** The item's value, not its index, is selected (`value`), and as a one-item list (`list`). */
  readonly by: "index" | "value" | "list";
}

const SWITCHERS: Readonly<Record<string, Switcher>> = {
  TabBar: { prop: "selectedIndex", items: "tabs", by: "index" },
  NavigationBar: { prop: "selectedIndex", items: "destinations", by: "index" },
  BottomNavigationBar: { prop: "currentIndex", items: "items", by: "index" },
  CupertinoTabBar: { prop: "currentIndex", items: "items", by: "index" },
  SegmentedButton: { prop: "selected", items: "segments", by: "list" },
  CupertinoSlidingSegmentedControl: { prop: "groupValue", items: "values", by: "value" },
};

/** Whether a click on a widget of `type` switches it. */
export const isSwitcher = (type: string | undefined): boolean => type !== undefined && type in SWITCHERS;

/** What a click at `x` across a switcher's `box` picks: its prop, the value, and the expression it is bound to. */
export function switchAt(
  node: Node,
  box: { readonly l: number; readonly r: number },
  x: number,
  rtl: boolean,
): { readonly prop: string; readonly value: unknown; readonly expr?: string } | undefined {
  const s = SWITCHERS[node.type ?? ""];
  const props = (node.props ?? {}) as Obj;
  const items = s === undefined ? undefined : props[s.items];
  if (s === undefined || !Array.isArray(items) || items.length === 0 || box.r <= box.l) {
    return undefined;
  }
  const across = Math.min(0.999, Math.max(0, (x - box.l) / (box.r - box.l)));
  const index = Math.floor((rtl ? 1 - across : across) * items.length);
  const item = items[index];
  const itemValue = (it: unknown) => (isObj(it) && "value" in it ? it["value"] : it);
  const value = s.by === "index" ? index : s.by === "list" ? [itemValue(item)] : itemValue(item);
  const bound = props[s.prop];
  const expr = isObj(bound) && typeof bound["$expr"] === "string" ? bound["$expr"] : undefined;
  return { prop: s.prop, value, ...(expr === undefined ? {} : { expr }) };
}

/** `live` with a switcher's pick: a prop bound to page state sets that state for every prop bound to it. */
export function withPick(
  live: LiveState,
  node: string,
  pick: { readonly value: unknown; readonly expr?: string },
): LiveState {
  if (pick.expr !== undefined) {
    return { ...live, exprs: new Map(live.exprs).set(pick.expr, pick.value) };
  }
  return { ...live, picks: new Map(live.picks).set(node, pick.value) };
}

/** `live` with a scroll view moved by `delta`, kept between 0 and `max`. */
export function withScroll(live: LiveState, node: string, delta: number, max: number): LiveState {
  const next = Math.min(max, Math.max(0, (live.scroll.get(node) ?? 0) + delta));
  return next === (live.scroll.get(node) ?? 0) ? live : { ...live, scroll: new Map(live.scroll).set(node, next) };
}

/** A prop value with page state the clicks set in place of its expression. */
function liveValue(value: unknown, exprs: ReadonlyMap<string, unknown>): unknown {
  return isObj(value) && typeof value["$expr"] === "string" && exprs.has(value["$expr"])
    ? exprs.get(value["$expr"])
    : value;
}

function liveProps(node: Node, live: LiveState, tabIndex: number | undefined): Obj | undefined {
  const props = node.props as Obj | undefined;
  let next: Obj | undefined;
  const set = (name: string, value: unknown) => {
    next ??= { ...(props ?? {}) };
    next[name] = value;
  };
  if (props !== undefined && live.exprs.size > 0) {
    for (const [k, v] of Object.entries(props)) {
      const lv = liveValue(v, live.exprs);
      if (lv !== v) {
        set(k, lv);
      }
    }
  }
  const s = SWITCHERS[node.type ?? ""];
  if (s !== undefined && live.picks.has(node.id)) {
    set(s.prop, live.picks.get(node.id));
  }
  // A TabBarView follows the tab picked in its page, as one `DefaultTabController` drives both.
  if (node.type === "TabBarView" && tabIndex !== undefined) {
    set("index", tabIndex);
  }
  return next;
}

/** The first tab bar of `root` that a click picked, and its index. */
function pickedTab(root: Node, live: LiveState): number | undefined {
  let out: number | undefined;
  const visit = (n: Node) => {
    if (out === undefined && n.type === "TabBar" && typeof live.picks.get(n.id) === "number") {
      out = live.picks.get(n.id) as number;
    }
    childNodes(n).forEach(visit);
  };
  visit(root);
  return out;
}

/** Whether anything of `live` touches the tree under `root`. */
function touches(root: Node, live: LiveState): boolean {
  if (live.exprs.size > 0) {
    return true;
  }
  const visit = (n: Node): boolean =>
    live.scroll.has(n.id) || live.picks.has(n.id) || live.states?.has(n.id) === true || childNodes(n).some(visit);
  return visit(root);
}

/** Puts `node`'s children and slots, each through `copy`, on `out`. */
function copyKids(node: Node, out: Obj, copy: (n: Node) => Node): void {
  if (node.children !== undefined) {
    out["children"] = node.children.map(copy);
  }
  if (node.slots !== undefined) {
    const slots: Obj = {};
    for (const [k, v] of Object.entries(node.slots as Readonly<Record<string, unknown>>)) {
      slots[k] = Array.isArray(v) ? v.map((x) => copy(x as Node)) : copy(v as Node);
    }
    out["slots"] = slots;
  }
}

/** A copy of `root` as the live surface shows it; `root` itself when nothing live touches it. */
export function applyLive(root: Node, live: LiveState): Node {
  if (!touches(root, live)) {
    return root;
  }
  const tab = pickedTab(root, live);
  const copy = (node: Node): Node => {
    const out: Obj = { ...(node as unknown as Obj) };
    const props = liveProps(node, live, tab);
    if (props !== undefined) {
      out["props"] = props;
    }
    const offset = live.scroll.get(node.id);
    const state = live.states?.get(node.id);
    if (offset !== undefined || state !== undefined) {
      out["x-studio"] = {
        ...(isObj(out["x-studio"]) ? out["x-studio"] : {}),
        ...(offset === undefined ? {} : { scroll: offset }),
        ...(state === undefined ? {} : { state }),
      };
    }
    copyKids(node, out, copy);
    return out as unknown as Node;
  };
  return copy(root);
}

/** The button an app bar shows of itself, as Flutter's `automaticallyImplyLeading` does. */
export type ImpliedButton = "menu" | "back" | "close";

/** The suffix of the id of an implied button: no document node has it, so a click on it is the live surface's. */
export const IMPLIED = "~implied-";

/**
 * A page scaffold's app bar with the leading button it implies (PC-15): the menu when the scaffold has a drawer, else
 * back on a page another opens, or close on a full-screen dialog — unless it has a leading widget or turns the
 * implication off. The end drawer's menu goes into the actions when there are none.
 */
export function withImpliedButtons(
  root: Node,
  leading: "back" | "close" | undefined,
  platform: "material" | "cupertino",
): Node {
  const slots = root.slots as Readonly<Record<string, Node | undefined>> | undefined;
  const bar = slots?.["appBar"];
  if (root.type !== "Scaffold" || bar === undefined) {
    return root;
  }
  const barSlots = (bar.slots ?? {}) as Readonly<Record<string, unknown>>;
  const props = (bar.props ?? {}) as Obj;
  const implied: ImpliedButton | undefined = slots?.["drawer"] !== undefined ? "menu" : leading;
  const add: Obj = {};
  if (implied !== undefined && barSlots["leading"] === undefined && props["automaticallyImplyLeading"] !== false) {
    add["leading"] = impliedButton(bar.id, implied, platform);
  }
  if (slots?.["endDrawer"] !== undefined && barSlots["actions"] === undefined) {
    add["actions"] = [{ ...impliedButton(bar.id, "menu", platform), id: `${bar.id}${IMPLIED}endmenu` }];
  }
  if (Object.keys(add).length === 0) {
    return root;
  }
  return {
    ...root,
    slots: { ...(root.slots ?? {}), appBar: { ...bar, slots: { ...barSlots, ...add } } },
  } as unknown as Node;
}

function impliedButton(bar: string, kind: ImpliedButton, platform: "material" | "cupertino"): Obj {
  const icon =
    kind === "menu" ? "menu" : kind === "close" ? "close" : platform === "cupertino" ? "arrow_back_ios" : "arrow_back";
  return {
    id: `${bar}${IMPLIED}${kind}`,
    type: "IconButton",
    // An action, so the button is drawn enabled as on the device; the live surface handles the click itself.
    events: { onPressed: { steps: [{ action: "haptic", id: "implied", input: { pattern: "light" } }] } },
    slots: { icon: { id: `${bar}${IMPLIED}${kind}-icon`, type: "Icon", props: { icon: { name: icon } } } },
  };
}

/** The implied button a hit is on, if any. */
export function impliedOf(hit: string): ImpliedButton | "endmenu" | undefined {
  const at = hit.indexOf(IMPLIED);
  if (at < 0) {
    return undefined;
  }
  const rest = hit
    .slice(at + IMPLIED.length)
    .replace(/-icon.*$/, "")
    .replace(/#.*$/, "");
  return rest === "menu" || rest === "back" || rest === "close" || rest === "endmenu" ? rest : undefined;
}

/** Notes what one step does: the route it opens, or the action it runs (a haptic tap counts for neither). */
function noteStep(v: Obj, opens: string[], runs: Set<string>): void {
  const action = v["action"];
  const route = isObj(v["input"]) ? v["input"]["route"] : undefined;
  if ((action === "navigate" || action === "openDialog" || action === "openBottomSheet") && typeof route === "string") {
    opens.push(route);
  } else if (typeof action === "string" && action !== "haptic") {
    runs.add(action);
  }
}

/** The steps that open a page (`navigate`, `openDialog`, `openBottomSheet`) and the other actions a widget's events run. */
export function eventActions(node: Node): { readonly opens: readonly string[]; readonly runs: readonly string[] } {
  const opens: string[] = [];
  const runs = new Set<string>();
  const walk = (v: unknown) => {
    if (Array.isArray(v)) {
      v.forEach(walk);
    } else if (isObj(v)) {
      noteStep(v, opens, runs);
      Object.values(v).forEach(walk);
    }
  };
  walk(node.events);
  return { opens, runs: [...runs] };
}
