// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { widget } from "@plux/schema";
import { type ArrowKey, nudgeDelta } from "./geometry.ts";

/** Document edits of canvas interactions (STU-067) as recipes for the injected `edit` function. */

export type EditFn = (
  docId: string,
  recipe: (draft: unknown) => void,
  options?: { group?: string; step?: string },
) => void;

interface Obj {
  type?: unknown;
  left?: unknown;
  top?: unknown;
  width?: unknown;
  height?: unknown;
  id?: unknown;
  props?: unknown;
  children?: unknown;
  slots?: unknown;
  offset?: unknown;
  size?: unknown;
  x?: unknown;
  y?: unknown;
  [key: string]: unknown;
}

const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** Where a node sits in its parent: the array and index, or the slot name. */
export interface Place {
  readonly node: Obj;
  readonly parent: Obj | null;
  readonly list: unknown[] | null;
  readonly index: number;
  readonly slot: string | null;
}

function searchIn(owner: Obj, id: string): Place | null {
  const slots = isObj(owner.slots) ? owner.slots : {};
  for (const [name, child] of Object.entries(slots)) {
    if (isObj(child) && child.id === id) {
      return { node: child, parent: owner, list: null, index: -1, slot: name };
    }
    const deeper = findNode(child, id, owner);
    if (deeper !== null) {
      return deeper;
    }
  }
  const kids = Array.isArray(owner.children) ? owner.children : [];
  for (const [i, child] of kids.entries()) {
    if (isObj(child) && child.id === id) {
      return { node: child, parent: owner, list: kids, index: i, slot: null };
    }
    const deeper = findNode(child, id, owner);
    if (deeper !== null) {
      return deeper;
    }
  }
  return null;
}

/** Finds the node `id` below `root` (a draft). */
export function findNode(root: unknown, id: string, parent: Obj | null = null): Place | null {
  if (!isObj(root)) {
    return null;
  }
  return root.id === id ? { node: root, parent, list: null, index: -1, slot: null } : searchIn(root, id);
}

function propsOf(node: Obj): Obj {
  if (!isObj(node.props)) {
    node.props = {};
  }
  return node.props as Obj;
}

const FLOW = new Set(["Row", "Column", "Wrap", "Flex"]);

/** The `Positioned` that places \`node\` in a \`Stack\`: the node itself, or its parent when the node is its child. */
function positionedOf(draft: unknown, place: Place): Obj | null {
  const candidate =
    place.node.type === "Positioned" ? place.node : place.parent?.type === "Positioned" ? place.parent : null;
  if (candidate === null || typeof candidate.id !== "string") {
    return null;
  }
  const outer = findNode(draft, candidate.id);
  return outer?.parent?.type === "Stack" ? candidate : null;
}

const num = (v: unknown): number => (typeof v === "number" ? v : 0);

/** Which sibling step an arrow means in a flow parent (F5: arrows step one sibling), or 0. */
function flowStep(parentType: unknown, key: ArrowKey): number {
  const back = key === "ArrowUp" || key === "ArrowLeft";
  const column = parentType === "Column";
  const row = parentType === "Row";
  const horizontal = key === "ArrowLeft" || key === "ArrowRight";
  if ((column && horizontal) || (row && !horizontal)) {
    return 0;
  }
  return back ? -1 : 1;
}

/**
 * Nudge / move by an arrow key, following the parent (F5): in a \`Stack\` the child's \`Positioned\` left/top change
 * by 1 px (Shift 10 px); in Row, Column or Wrap the node steps one sibling; anywhere else nothing changes.
 */
export function nudgeRecipe(id: string, key: ArrowKey, shift: boolean): (draft: unknown) => void {
  return (draft) => {
    const p = findNode(draft, id);
    if (p === null) {
      return;
    }
    const positioned = positionedOf(draft, p);
    if (positioned !== null) {
      const d = nudgeDelta(key, shift);
      const props = propsOf(positioned);
      props.left = num(props.left) + d.x;
      props.top = num(props.top) + d.y;
      return;
    }
    const step =
      p.parent !== null && p.list !== null && FLOW.has(String(p.parent.type)) ? flowStep(p.parent.type, key) : 0;
    const to = p.index + step;
    if (step !== 0 && p.list !== null && to >= 0 && to < p.list.length) {
      const [moved] = p.list.splice(p.index, 1);
      p.list.splice(to, 0, moved);
    }
  };
}

/** Move by a drag: in a \`Stack\` the \`Positioned\` left/top change by (dx, dy) rounded to whole pixels; else nothing. */
export function moveRecipe(id: string, dx: number, dy: number): (draft: unknown) => void {
  return (draft) => {
    const p = findNode(draft, id);
    const positioned = p === null ? null : positionedOf(draft, p);
    if (positioned !== null) {
      const props = propsOf(positioned);
      props.left = num(props.left) + Math.round(dx);
      props.top = num(props.top) + Math.round(dy);
    }
  };
}

/** Resize: sets width/height only where the node's descriptor has those size props. */
export function resizeRecipe(
  id: string,
  width: number,
  height: number,
  propsOfType: (type: string) => readonly string[] = (t) => widget(t)?.props.map((p) => p.name) ?? [],
): (draft: unknown) => void {
  return (draft) => {
    const p = findNode(draft, id);
    const names = typeof p?.node.type === "string" ? propsOfType(p.node.type) : [];
    if (p === null || !names.includes("width") || !names.includes("height")) {
      return;
    }
    const props = propsOf(p.node);
    props.width = width;
    props.height = height;
  };
}

/** Reparent: moves `ids` into `parent` at `index`; refuses to move a node into itself. */
export function reparentRecipe(ids: readonly string[], parent: string, index: number): (draft: unknown) => void {
  return (draft) => {
    const target = findNode(draft, parent);
    if (target === null || ids.some((id) => findNode(findNode(draft, id)?.node, parent) !== null)) {
      return;
    }
    const moved: unknown[] = [];
    for (const id of ids) {
      const p = findNode(draft, id);
      if (p?.list != null) {
        moved.push(p.list.splice(p.index, 1)[0]);
      }
    }
    if (!Array.isArray(target.node.children)) {
      target.node.children = [];
    }
    const kids = target.node.children as unknown[];
    kids.splice(Math.min(index, kids.length), 0, ...moved);
  };
}

/** Removes a node from its parent's list or slot; false when it has neither. */
function detach(p: Place): boolean {
  if (p.list !== null) {
    p.list.splice(p.index, 1);
    return true;
  }
  if (p.slot !== null && p.parent !== null && isObj(p.parent.slots)) {
    delete p.parent.slots[p.slot];
    return true;
  }
  return false;
}

/**
 * Drops `id` into the Stack `stack` at `position`: the node is lifted out of its parent (with its own `Positioned`
 * when it has one) and placed in a `Positioned` whose left/top are the drop point (F5). Only schema fields are written.
 */
export function stackDropRecipe(
  id: string,
  stack: string,
  position: { readonly left: number; readonly top: number },
  fresh: () => string,
): (draft: unknown) => void {
  return (draft) => {
    const target = findNode(draft, stack);
    const p = findNode(draft, id);
    if (target === null || p === null || p.parent === null || findNode(p.node, stack) !== null) {
      return;
    }
    const outer = positionedOf(draft, p);
    const lifted = outer === null ? p : findNode(draft, String(outer.id));
    if (lifted === null || lifted.parent === null) {
      return;
    }
    if (!detach(lifted)) {
      return;
    }
    const wrapper = outer ?? { id: fresh(), type: "Positioned", slots: { child: p.node } };
    propsOf(wrapper).left = position.left;
    propsOf(wrapper).top = position.top;
    if (!Array.isArray(target.node.children)) {
      target.node.children = [];
    }
    (target.node.children as unknown[]).push(wrapper);
  };
}

/** Wrap: replaces `id` by a new `wrapper` node (given ID and type) that contains it. */
export function wrapRecipe(id: string, wrapperId: string, wrapperType: string): (draft: unknown) => void {
  return (draft) => {
    const p = findNode(draft, id);
    if (p === null || p.parent === null) {
      return;
    }
    const wrapper = { id: wrapperId, type: wrapperType, children: [p.node] };
    if (p.list !== null) {
      p.list[p.index] = wrapper;
    } else if (p.slot !== null && isObj(p.parent.slots)) {
      p.parent.slots[p.slot] = wrapper;
    }
  };
}

/** Unwrap: replaces a container by its children. */
export function unwrapRecipe(id: string): (draft: unknown) => void {
  return (draft) => {
    const p = findNode(draft, id);
    if (p?.list == null) {
      return;
    }
    const kids = Array.isArray(p.node.children) ? p.node.children : [];
    p.list.splice(p.index, 1, ...kids);
  };
}

/** Clones `node` with every ID replaced by `fresh(oldId)`; also returns the old-to-new map. */
export function cloneWithFreshIds(
  node: unknown,
  fresh: (oldId: string) => string,
): { node: unknown; map: Map<string, string> } {
  const map = new Map<string, string>();
  const walk = (n: unknown): unknown => {
    if (Array.isArray(n)) {
      return n.map(walk);
    }
    if (!isObj(n)) {
      return n;
    }
    const out: Obj = {};
    for (const [k, v] of Object.entries(n)) {
      if (k === "id" && typeof v === "string") {
        const next = fresh(v);
        map.set(v, next);
        out[k] = next;
      } else {
        out[k] = walk(v);
      }
    }
    return out;
  };
  return { node: walk(node), map };
}

/** Duplicate: inserts a copy with fresh IDs right after `id` (children lists only). */
export function duplicateRecipe(id: string, fresh: (oldId: string) => string): (draft: unknown) => void {
  return (draft) => {
    const p = findNode(draft, id);
    if (p?.list == null) {
      return;
    }
    p.list.splice(p.index + 1, 0, cloneWithFreshIds(JSON.parse(JSON.stringify(p.node)), fresh).node);
  };
}
