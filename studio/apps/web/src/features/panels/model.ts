// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { WidgetDescriptor } from "@plux/schema";
import { widget } from "@plux/schema";
import type { Node } from "../plugin-canvas/plugin-model.ts";
import { type IdSource, uuidv7 } from "../plugin-canvas/screens.ts";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import { findNode, reparentRecipe, takeOutRecipe } from "../plugin-canvas/select/edits.ts";
import { containersOf, type GestureEnv } from "../plugin-canvas/select/gesture.ts";
import { type DropTarget, resolveDrop } from "../plugin-canvas/select/reparent.ts";
import type { Selection } from "../plugin-canvas/select/selection.ts";
import type { NodeIndex } from "../plugin-canvas/select/tree.ts";
import { buildNode, type StarterTexts } from "./catalogue.ts";

/** Where a new node goes: into `parent`'s `children` at `index`, or into its empty single `slot`. */
export interface Place {
  readonly parent: string;
  readonly index: number;
  readonly slot: string | null;
  /** Set for a Stack parent: the node goes into a `Positioned` at these Stack coordinates (F5). */
  readonly at?: { readonly left: number; readonly top: number };
  /**
   * Set when the node is a screen's first content: it goes into a `SafeArea`, a 16 px `Padding` and a `Column`, so it
   * is not drawn under the status bar or against the edges, and the next widget stacks below it rather than looking
   * for another slot.
   */
  readonly screenContent?: boolean;
}

/** The page trees by node id, with the `children` arrays kept apart from slots (the tree index mixes both). */
export interface PanelModel {
  readonly nodes: ReadonlyMap<string, Node>;
  readonly listKids: ReadonlyMap<string, readonly string[]>;
}

export function modelOf(roots: readonly (Node | undefined)[]): PanelModel {
  const nodes = new Map<string, Node>();
  const listKids = new Map<string, readonly string[]>();
  const visit = (n: Node): void => {
    nodes.set(n.id, n);
    listKids.set(
      n.id,
      (n.children ?? []).map((c) => c.id),
    );
    for (const c of [...Object.values(n.slots ?? {}), ...(n.children ?? [])]) {
      visit(c);
    }
  };
  for (const r of roots) {
    if (r !== undefined) {
      visit(r);
    }
  }
  return { nodes, listKids };
}

const holdsChildren = (type: string | undefined): boolean => type !== undefined && widget(type)?.children !== undefined;

/** The slot that holds a widget's content, filled first when free: a page's `body`, a wrapper's `child`. */
const contentSlots = ["body", "child"];

/**
 * The slots that hold a screen's chrome rather than its content, and the widgets each is for: an app bar never takes a
 * button just because it is the first empty slot of a Scaffold.
 */
const chromeSlots: Readonly<Record<string, readonly string[]>> = {
  appBar: ["AppBar"],
  floatingActionButton: ["FloatingActionButton"],
  bottomNavigationBar: ["NavigationBar", "BottomNavigationBar", "BottomAppBar", "TabBar"],
  drawer: ["Drawer", "NavigationDrawer"],
  endDrawer: ["Drawer", "NavigationDrawer"],
  bottomSheet: ["BottomSheet"],
};

/** Widgets that are themselves a screen's layout, which go into a screen's body as they are. */
const layoutRoots = new Set([
  "SafeArea",
  "Column",
  "ListView",
  "CustomScrollView",
  "SingleChildScrollView",
  "Stack",
  "Center",
  "Padding",
  "GridView",
  "PageView",
  "TabBarView",
]);

/**
 * The empty single (non-list) slot a new node of `type` goes to: a chrome slot made for it (an AppBar's `appBar`),
 * else the content slot (a Scaffold's `body`, a wrapper's `child`), else another slot that is not chrome. A widget of
 * no known type takes the content slot or the first empty one.
 */
export function freeSlot(model: PanelModel, id: string, type?: string): string | null {
  const node = model.nodes.get(id);
  const slots = (node?.type === undefined ? [] : (widget(node.type)?.slots ?? [])).filter(
    (s) => !s.list && node?.slots?.[s.name] === undefined,
  );
  const made = type === undefined ? undefined : slots.find((s) => chromeSlots[s.name]?.includes(type) === true);
  const content = slots.find((s) => contentSlots.includes(s.name));
  const other = slots.find((s) => type === undefined || chromeSlots[s.name] === undefined);
  return (made ?? content ?? other)?.name ?? null;
}

/** The node in `id`'s content slot, when that slot is taken. */
function contentOf(model: PanelModel, id: string): string | undefined {
  const slots = model.nodes.get(id)?.slots as Record<string, unknown> | undefined;
  for (const name of contentSlots) {
    const n = slots?.[name] as { id?: unknown } | undefined;
    if (typeof n?.id === "string") {
      return n.id;
    }
  }
  return undefined;
}

/**
 * Whether `id` can take one more node of `type`, and where: a container appends it; a widget with slots takes it in
 * the slot `freeSlot` picks; a widget whose content slot is taken passes it on to that content (a Scaffold's body
 * Column takes the next widget at its end), at most a few wrappers deep.
 */
export function placeIn(model: PanelModel, id: string, type?: string, depth = 0): Place | null {
  const node = model.nodes.get(id);
  if (holdsChildren(node?.type)) {
    return { parent: id, index: model.listKids.get(id)?.length ?? 0, slot: null };
  }
  const slot = freeSlot(model, id, type);
  if (slot !== null) {
    const screenContent = node?.type === "Scaffold" && slot === "body" && type !== undefined && !layoutRoots.has(type);
    return { parent: id, index: 0, slot, ...(screenContent ? { screenContent } : {}) };
  }
  const inner = contentOf(model, id);
  return inner === undefined || depth >= 4 ? null : placeIn(model, inner, type, depth + 1);
}

/** Keyboard insert: into the anchor, else the nearest ancestor that can take it, else the first screen root. */
export function insertionPoint(
  model: PanelModel,
  index: NodeIndex,
  selection: Selection,
  fallbackRoot: string | undefined,
  type?: string,
): Place | null {
  let cur: string | null | undefined = selection.anchor ?? fallbackRoot;
  while (cur !== null && cur !== undefined) {
    const p = placeIn(model, cur, type);
    if (p !== null) {
      return p;
    }
    cur = index.parent.get(cur);
  }
  return null;
}

export function insertRecipe(node: object, place: Place): (draft: unknown) => void {
  return (draft) => {
    const target = findNode(draft, place.parent);
    if (target === null) {
      return;
    }
    if (place.slot !== null) {
      const slots = (target.node.slots ?? {}) as Record<string, unknown>;
      slots[place.slot] = node;
      target.node.slots = slots;
      return;
    }
    if (!Array.isArray(target.node.children)) {
      target.node.children = [];
    }
    const kids = target.node.children as unknown[];
    kids.splice(Math.min(place.index, kids.length), 0, node);
  };
}

/** Builds the node and writes it; returns its id, or null when nothing was written. */
export function insertWidget(
  d: WidgetDescriptor,
  place: Place,
  edit: EditFn,
  docOf: (id: string) => string | undefined,
  canEdit: boolean,
  newId?: IdSource,
  props?: Readonly<Record<string, unknown>>,
  starter?: StarterTexts,
): string | null {
  const doc = docOf(place.parent);
  if (!canEdit || doc === undefined) {
    return null;
  }
  const ids = newId ?? uuidv7;
  const built = buildNode(d, ids, starter);
  const node = props === undefined ? built : { ...built, props: { ...(built.props ?? {}), ...props } };
  const positioned =
    place.at === undefined
      ? node
      : { id: ids(), type: "Positioned", props: { left: place.at.left, top: place.at.top }, slots: { child: node } };
  const placed =
    place.screenContent === true
      ? {
          id: ids(),
          type: "SafeArea",
          slots: {
            child: {
              id: ids(),
              type: "Padding",
              props: { padding: { all: 16 } },
              slots: {
                child: { id: ids(), type: "Column", props: { crossAxisAlignment: "start" }, children: [positioned] },
              },
            },
          },
        }
      : positioned;
  edit(doc, insertRecipe(placed, place), { group: "insert" });
  return node.id;
}

/** The drop target for a widget dragged over the canvas at world point (x, y): the S2.5 resolution and indicator. */
export function dropTarget(env: GestureEnv, x: number, y: number): DropTarget | null {
  return resolveDrop(env.index, containersOf(env), [], x, y);
}

/** Where a layer dropped `before`/`after` the row `target` lands: parent and index in the parent's children after removal. */
export function layerDrop(
  model: PanelModel,
  index: NodeIndex,
  dragged: string,
  target: string,
  half: "before" | "after",
): { parent: string; index: number } | null {
  if (dragged === target) {
    return null;
  }
  const into =
    half === "after" && holdsChildren(model.nodes.get(target)?.type) && (model.listKids.get(target)?.length ?? 0) > 0;
  const parent = into ? target : index.parent.get(target);
  if (parent === null || parent === undefined || !holdsChildren(model.nodes.get(parent)?.type)) {
    return null;
  }
  const sibs = (model.listKids.get(parent) ?? []).filter((k) => k !== dragged);
  if (into) {
    return { parent, index: sibs.length };
  }
  const at = sibs.indexOf(target);
  return at < 0 ? null : { parent, index: half === "before" ? at : at + 1 };
}

/**
 * Moves a layer into `parent`'s slot `slot` (PC-20): a list slot takes it last, a single slot only while it is empty,
 * so a drop never replaces what a slot holds. Refused across documents, into a slot the widget does not declare, or
 * into the moved layer itself.
 */
export function moveIntoSlot(
  index: NodeIndex,
  dragged: string,
  parent: string,
  slot: string,
  edit: EditFn,
  docOf: (id: string) => string | undefined,
  canEdit: boolean,
  model: PanelModel,
): boolean {
  const doc = docOf(dragged);
  const type = model.nodes.get(parent)?.type;
  const declared = type === undefined ? undefined : widget(type)?.slots.find((d) => d.name === slot);
  if (!canEdit || doc === undefined || doc !== docOf(parent) || declared === undefined || dragged === parent) {
    return false;
  }
  for (let p = index.parent.get(parent); p !== null && p !== undefined; p = index.parent.get(p)) {
    if (p === dragged) {
      return false;
    }
  }
  const held = (model.nodes.get(parent)?.slots ?? {}) as Readonly<Record<string, unknown>>;
  if (!declared.list && held[slot] !== undefined) {
    return false;
  }
  edit(
    doc,
    (draft) => {
      let taken: unknown;
      takeOutRecipe(dragged, (node) => {
        taken = node;
      })(draft);
      const target = findNode(draft, parent);
      if (taken === undefined || target === null) {
        return;
      }
      const slots = (target.node.slots ?? {}) as Record<string, unknown>;
      if (declared.list) {
        const list = Array.isArray(slots[slot]) ? (slots[slot] as unknown[]) : [];
        list.push(taken);
        slots[slot] = list;
      } else {
        slots[slot] = taken;
      }
      target.node.slots = slots;
    },
    { group: "reorder" },
  );
  return true;
}

export function moveLayer(
  model: PanelModel,
  index: NodeIndex,
  dragged: string,
  target: string,
  half: "before" | "after",
  edit: EditFn,
  docOf: (id: string) => string | undefined,
  canEdit: boolean,
): boolean {
  const to = layerDrop(model, index, dragged, target, half);
  const doc = docOf(dragged);
  if (!canEdit || to === null || doc === undefined || doc !== docOf(to.parent) || index.parent.get(dragged) === null) {
    return false;
  }
  edit(doc, reparentRecipe([dragged], to.parent, to.index), { group: "reorder" });
  return true;
}
