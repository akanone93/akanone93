// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { WidgetDescriptor } from "@plux/schema";
import { widget } from "@plux/schema";
import type { Node } from "../plugin-canvas/plugin-model.ts";
import { type IdSource, uuidv7 } from "../plugin-canvas/screens.ts";
import {
  chromeSlots,
  type DropPlan,
  placement,
  planDrop,
  type Refusal,
  wrapNode,
} from "../plugin-canvas/select/drop-rules.ts";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import { moveToRecipe, putRecipe, reparentRecipe } from "../plugin-canvas/select/edits.ts";
import type { GestureEnv } from "../plugin-canvas/select/gesture.ts";
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
  /** Set when `slot` holds a list: the node goes into it at `index`. */
  readonly list?: boolean;
  /** A widget the node goes in first, as the place needs (a `SliverToBoxAdapter` among slivers). */
  readonly wrap?: string;
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
    // A list slot (an app bar's `actions`, a scroll view's `slivers`) holds an array of nodes.
    const slotted = Object.values(n.slots ?? {}) as readonly (Node | readonly Node[])[];
    for (const c of [...slotted.flat(), ...(n.children ?? [])]) {
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

/** Widgets that are themselves a screen's layout, which go into a screen's body as they are. */
export const layoutRoots: ReadonlySet<string> = new Set([
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
  return putRecipe(node, place);
}

/** Widgets that place themselves in a Stack: they take the drop point as their own `left` and `top`. */
const selfPlaced = new Set(["Positioned", "AnimatedPositioned"]);

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
  // In a Stack the node goes in a `Positioned` at the drop point, or is placed there itself when it is one.
  const at = place.at;
  const positioned =
    at === undefined
      ? node
      : selfPlaced.has(d.type)
        ? { ...node, props: { ...(node.props ?? {}), left: at.left, top: at.top } }
        : { id: ids(), type: "Positioned", props: { left: at.left, top: at.top }, slots: { child: node } };
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
      : place.wrap === undefined
        ? positioned
        : wrapNode(place.wrap, ids(), positioned);
  edit(doc, insertRecipe(placed as object, place), { group: "insert" });
  return node.id;
}

/** Where a new widget of `type` dragged over the canvas at world point (x, y) lands, by the drop rules (PC-20). */
export function dropTarget(env: GestureEnv, x: number, y: number, type: string): DropPlan {
  return planDrop(env, { type, ids: [] }, x, y);
}

/** Whether `parent`'s `slot` (its `children` when null) holds a list. */
function holdsList(model: PanelModel, parent: string, slot: string | null): boolean {
  const type = model.nodes.get(parent)?.type;
  if (slot === null) {
    return holdsChildren(type);
  }
  return type !== undefined && widget(type)?.slots.find((d) => d.name === slot)?.list === true;
}

/** Where a layer dropped `before`/`after` the row `target` lands: parent, slot and index in the list after removal. */
export function layerDrop(
  model: PanelModel,
  index: NodeIndex,
  dragged: string,
  target: string,
  half: "before" | "after",
): { parent: string; index: number; slot?: string } | null {
  if (dragged === target) {
    return null;
  }
  if (
    half === "after" &&
    holdsChildren(model.nodes.get(target)?.type) &&
    (model.listKids.get(target)?.length ?? 0) > 0
  ) {
    return { parent: target, index: (model.listKids.get(target) ?? []).filter((k) => k !== dragged).length };
  }
  // Beside a node of a list: its parent's `children`, or a list slot such as an app bar's `actions`.
  const parent = index.parent.get(target);
  const slot = index.slot.get(target) ?? null;
  if (parent === null || parent === undefined || !holdsList(model, parent, slot)) {
    return null;
  }
  const sibs = (index.children.get(parent) ?? []).filter((k) => k !== dragged && (index.slot.get(k) ?? null) === slot);
  const at = sibs.indexOf(target);
  return at < 0 ? null : { parent, index: half === "before" ? at : at + 1, ...(slot === null ? {} : { slot }) };
}

/** Whether `id` is `ancestor` or inside it. */
const within = (index: NodeIndex, id: string, ancestor: string): boolean => {
  for (let p: string | null | undefined = id; p !== null && p !== undefined; p = index.parent.get(p)) {
    if (p === ancestor) {
      return true;
    }
  }
  return false;
};

/** Why layer `dragged` cannot go into `parent`'s `slot` (its `children` when null), or the wrapper it goes in. */
function layerPlacement(
  model: PanelModel,
  index: NodeIndex,
  dragged: string,
  parent: string,
  slot: string | null,
): { readonly wrap?: string } | Refusal {
  const type = model.nodes.get(parent)?.type;
  const own = model.nodes.get(dragged)?.type;
  if (type === undefined || own === undefined || within(index, parent, dragged)) {
    return "nowhere";
  }
  return placement(type, slot, own);
}

/**
 * Why layer `dragged` cannot be dropped on the Layers row `target` (a layer, before or after it, or a slot's row when
 * `into` names it), or null when it can (PC-20).
 */
export function layerRefusal(
  model: PanelModel,
  index: NodeIndex,
  dragged: string,
  target: string,
  half: "before" | "after",
  into: { readonly parent: string; readonly slot: string } | null,
): Refusal | null {
  if (into !== null) {
    const declared = widget(model.nodes.get(into.parent)?.type ?? "")?.slots.find((d) => d.name === into.slot);
    const held = (model.nodes.get(into.parent)?.slots ?? {}) as Readonly<Record<string, unknown>>;
    if (declared === undefined) {
      return "nowhere";
    }
    if (!declared.list && held[into.slot] !== undefined) {
      return "full";
    }
    const p = layerPlacement(model, index, dragged, into.parent, into.slot);
    return typeof p === "string" ? p : null;
  }
  const to = layerDrop(model, index, dragged, target, half);
  if (to === null) {
    return dragged === target ? null : "nowhere";
  }
  const p = layerPlacement(model, index, dragged, to.parent, to.slot ?? null);
  return typeof p === "string" ? p : null;
}

/** The node `wrap` makes of a moved layer, with a fresh id, when it needs one. */
const layerWrap = (wrap: string | undefined, ids: IdSource) =>
  wrap === undefined ? undefined : (node: unknown) => wrapNode(wrap, ids(), node);

/**
 * Moves a layer into `parent`'s slot `slot` (PC-20): a list slot takes it last, a single slot only while it is empty,
 * so a drop never replaces what a slot holds. Refused across documents, into a slot the widget does not declare, into
 * the moved layer itself, or where Flutter would not build it (the reason is returned).
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
  newId: IdSource = uuidv7,
): boolean | Refusal {
  const doc = docOf(dragged);
  const type = model.nodes.get(parent)?.type;
  const declared = type === undefined ? undefined : widget(type)?.slots.find((d) => d.name === slot);
  if (!canEdit || doc === undefined || doc !== docOf(parent) || declared === undefined || dragged === parent) {
    return false;
  }
  // A taken slot or the layer's own subtree is no place for it (false); a Flutter rule says why (its reason).
  const refused = layerRefusal(model, index, dragged, parent, "after", { parent, slot });
  if (refused === "full" || refused === "nowhere") {
    return false;
  }
  if (refused !== null) {
    return refused;
  }
  const p = layerPlacement(model, index, dragged, parent, slot);
  const wrap = typeof p === "string" ? undefined : p.wrap;
  const count = (index.children.get(parent) ?? []).filter((k) => k !== dragged && index.slot.get(k) === slot).length;
  edit(doc, moveToRecipe(dragged, { parent, slot, list: declared.list, index: count }, layerWrap(wrap, newId)), {
    group: "reorder",
  });
  return true;
}

/** Moves a layer before or after another (PC-20); false when nothing moved, the reason when the rules refuse it. */
export function moveLayer(
  model: PanelModel,
  index: NodeIndex,
  dragged: string,
  target: string,
  half: "before" | "after",
  edit: EditFn,
  docOf: (id: string) => string | undefined,
  canEdit: boolean,
  newId: IdSource = uuidv7,
): boolean | Refusal {
  const to = layerDrop(model, index, dragged, target, half);
  const doc = docOf(dragged);
  if (!canEdit || to === null || doc === undefined || doc !== docOf(to.parent) || index.parent.get(dragged) === null) {
    return false;
  }
  const p = layerPlacement(model, index, dragged, to.parent, to.slot ?? null);
  if (typeof p === "string") {
    return p;
  }
  const recipe =
    to.slot === undefined && p.wrap === undefined
      ? reparentRecipe([dragged], to.parent, to.index)
      : moveToRecipe(
          dragged,
          { parent: to.parent, index: to.index, slot: to.slot ?? null, list: to.slot !== undefined },
          layerWrap(p.wrap, newId),
        );
  edit(doc, recipe, { group: "reorder" });
  return true;
}
