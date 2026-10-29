// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Node } from "../plugin-model.ts";

/** Parent, children and depth of every node of a screen tree, by ID (F5: IDs, not positions). */
export interface NodeIndex {
  readonly parent: ReadonlyMap<string, string | null>;
  readonly children: ReadonlyMap<string, readonly string[]>;
  readonly depth: ReadonlyMap<string, number>;
  readonly type: ReadonlyMap<string, string>;
}

/** Children of a node in document order: slots (in key order) then `children`. */
export function childNodes(node: Node): readonly Node[] {
  return [...Object.values(node.slots ?? {}), ...(node.children ?? [])];
}

/** Indexes the trees rooted at `roots`. */
export function indexNodes(roots: readonly (Node | undefined)[]): NodeIndex {
  const parent = new Map<string, string | null>();
  const children = new Map<string, readonly string[]>();
  const depth = new Map<string, number>();
  const type = new Map<string, string>();
  const visit = (n: Node, p: string | null, d: number): void => {
    parent.set(n.id, p);
    depth.set(n.id, d);
    if (n.type !== undefined) {
      type.set(n.id, n.type);
    }
    const kids = childNodes(n);
    children.set(
      n.id,
      kids.map((k) => k.id),
    );
    for (const k of kids) {
      visit(k, n.id, d + 1);
    }
  };
  for (const r of roots) {
    if (r !== undefined) {
      visit(r, null, 0);
    }
  }
  return { parent, children, depth, type };
}

/** `id` and its ancestors, innermost first. */
export function lineage(index: NodeIndex, id: string): string[] {
  const out: string[] = [];
  let cur: string | null | undefined = id;
  while (cur !== null && cur !== undefined && index.parent.has(cur)) {
    out.push(cur);
    cur = index.parent.get(cur);
  }
  return out;
}

/** Hit and item IDs are `<screen>/<node>` (canvas `screens.ts`); the document node ID is the part after the slash. */
export const itemIdOf = (screen: string, node: string): string => `${screen}/${node}`;

export function nodeIdOfHit(hit: string): string {
  const i = hit.indexOf("/");
  return i < 0 ? hit : hit.slice(i + 1);
}
