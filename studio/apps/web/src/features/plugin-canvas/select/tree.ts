// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { LABEL_SUFFIX } from "@plux/canvas";
import type { Node } from "../plugin-model.ts";

/** Where the part of a widget's item id begins (`LABEL_SUFFIX`, `PAINT_SUFFIX`). */
const PART_MARK = LABEL_SUFFIX[0] as string;

/** Parent, children and depth of every node of a screen tree, by ID (F5: IDs, not positions). */
export interface NodeIndex {
  readonly parent: ReadonlyMap<string, string | null>;
  readonly children: ReadonlyMap<string, readonly string[]>;
  readonly depth: ReadonlyMap<string, number>;
  readonly type: ReadonlyMap<string, string>;
}

/** Children of a node in document order: slots (in key order; a list slot such as an app bar's `actions` in its own order) then `children`. */
export function childNodes(node: Node): readonly Node[] {
  // A slot holds one widget or, in documents, a list of them; the `Node` type names only the first.
  const slots = Object.values(node.slots ?? {}) as readonly (Node | readonly Node[])[];
  return [...slots.flatMap((v) => (Array.isArray(v) ? v : [v as Node])), ...(node.children ?? [])];
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
  const id = i < 0 ? hit : hit.slice(i + 1);
  // A box's label and the boxes a widget paints are items of their own (`#label`, `#paint0`…); a hit on one is a hit on
  // the widget.
  const part = id.indexOf(PART_MARK);
  return part < 0 ? id : id.slice(0, part);
}

/** The screen whose device frame, title or overflow band item `hit` is (`screen:`, `title:`, `overflow:`), if any. */
export function screenOfItem(hit: string): string | undefined {
  for (const prefix of ["screen:", "title:", "overflow:"]) {
    if (hit.startsWith(prefix)) {
      return hit.slice(prefix.length);
    }
  }
  return undefined;
}

/**
 * The document nodes under the pointer, topmost first and once each: item ids (`<screen>/<node>`, a widget's parts)
 * become node ids, and a screen's frame, title or overflow band stands for the screen's root. Anything else (ghosts,
 * arrows) is left out.
 */
export function nodesOfHits(
  hits: readonly string[],
  index: NodeIndex,
  rootOf: (screen: string) => string | undefined,
): string[] {
  const out: string[] = [];
  for (const hit of hits) {
    const screen = screenOfItem(hit);
    const id = screen === undefined ? nodeIdOfHit(hit) : rootOf(screen);
    if (id !== undefined && index.parent.has(id) && !out.includes(id)) {
      out.push(id);
    }
  }
  return out;
}
