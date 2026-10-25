// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/** Document edits of canvas interactions (STU-067) as recipes for the injected `edit` function. */

export type EditFn = (docId: string, recipe: (draft: unknown) => void, options?: { group?: string }) => void;

type Obj = Record<string, unknown>;

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

/** Nudge / move: stores the offset of the nodes in `props.offset`, summed with any earlier offset. */
export function nudgeRecipe(ids: readonly string[], dx: number, dy: number): (draft: unknown) => void {
  return (draft) => {
    for (const id of ids) {
      const p = findNode(draft, id);
      if (p === null) {
        continue;
      }
      const props = propsOf(p.node);
      const o = isObj(props.offset) ? props.offset : {};
      props.offset = { x: Number(o.x ?? 0) + dx, y: Number(o.y ?? 0) + dy };
    }
  };
}

/** Resize: stores the size in `props.size`. */
export function resizeRecipe(id: string, width: number, height: number): (draft: unknown) => void {
  return (draft) => {
    const p = findNode(draft, id);
    if (p === null) {
      return;
    }
    propsOf(p.node).size = { width, height };
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
