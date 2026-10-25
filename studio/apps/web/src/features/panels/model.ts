// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { WidgetDescriptor } from "@plux/schema";
import { widget } from "@plux/schema";
import type { Node } from "../plugin-canvas/plugin-model.ts";
import type { IdSource } from "../plugin-canvas/screens.ts";
import type { EditFn } from "../plugin-canvas/select/edits.ts";
import { findNode, reparentRecipe } from "../plugin-canvas/select/edits.ts";
import { containersOf, type GestureEnv } from "../plugin-canvas/select/gesture.ts";
import { type DropTarget, resolveDrop } from "../plugin-canvas/select/reparent.ts";
import type { Selection } from "../plugin-canvas/select/selection.ts";
import type { NodeIndex } from "../plugin-canvas/select/tree.ts";
import { buildNode } from "./catalogue.ts";

/** Where a new node goes: into `parent`'s `children` at `index`, or into its empty single `slot`. */
export interface Place {
  readonly parent: string;
  readonly index: number;
  readonly slot: string | null;
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

/** The first empty single (non-list) slot of a node, if its widget has one. */
function freeSlot(model: PanelModel, id: string): string | null {
  const node = model.nodes.get(id);
  const slots = node?.type === undefined ? [] : (widget(node.type)?.slots ?? []);
  return slots.find((s) => !s.list && node?.slots?.[s.name] === undefined)?.name ?? null;
}

/** Whether `id` can take one more node, and where. */
export function placeIn(model: PanelModel, id: string): Place | null {
  const type = model.nodes.get(id)?.type;
  if (holdsChildren(type)) {
    return { parent: id, index: model.listKids.get(id)?.length ?? 0, slot: null };
  }
  const slot = freeSlot(model, id);
  return slot === null ? null : { parent: id, index: 0, slot };
}

/** Keyboard insert: into the anchor, else the nearest ancestor that can take it, else the first screen root. */
export function insertionPoint(
  model: PanelModel,
  index: NodeIndex,
  selection: Selection,
  fallbackRoot: string | undefined,
): Place | null {
  let cur: string | null | undefined = selection.anchor ?? fallbackRoot;
  while (cur !== null && cur !== undefined) {
    const p = placeIn(model, cur);
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
): string | null {
  const doc = docOf(place.parent);
  if (!canEdit || doc === undefined) {
    return null;
  }
  const node = buildNode(d, newId);
  edit(doc, insertRecipe(node, place), { group: "insert" });
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
