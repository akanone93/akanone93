// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Node } from "../plugin-canvas/plugin-model.ts";
import { findNode } from "../plugin-canvas/select/edits.ts";
import { childNodes } from "../plugin-canvas/select/tree.ts";

/**
 * A layer's editor-only settings (STU-061): its name in the layer tree, whether the canvas hides it, and whether it is
 * locked against edits. They live in the node's `x-studio` object, which the schema keeps and the compiler ignores
 * (SCH-004), so they never reach a bundle or a device; the device's own switch is the node's `visible`.
 */
export interface StudioMeta {
  readonly name?: string;
  readonly hidden?: boolean;
  readonly locked?: boolean;
}

export const STUDIO_KEY = "x-studio";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The editor-only settings of a node; empty when it has none. */
export function metaOf(node: unknown): StudioMeta {
  const raw = isObj(node) ? node[STUDIO_KEY] : undefined;
  if (!isObj(raw)) {
    return {};
  }
  const name = typeof raw["name"] === "string" && raw["name"].trim() !== "" ? raw["name"] : undefined;
  return {
    ...(name === undefined ? {} : { name }),
    ...(raw["hidden"] === true ? { hidden: true } : {}),
    ...(raw["locked"] === true ? { locked: true } : {}),
  };
}

/** A change to a layer's settings: a new name (`null` clears it), hidden or locked. */
export interface MetaChange {
  readonly name?: string | null;
  readonly hidden?: boolean;
  readonly locked?: boolean;
}

/** `meta` with `change` applied: a blank name and a cleared flag are removed rather than stored empty. */
function applyMeta(meta: Obj, change: MetaChange): Obj {
  if (change.name !== undefined) {
    const name = change.name?.trim() ?? "";
    if (name === "") {
      delete meta["name"];
    } else {
      meta["name"] = name;
    }
  }
  for (const flag of ["hidden", "locked"] as const) {
    if (change[flag] === true) {
      meta[flag] = true;
    } else if (change[flag] === false) {
      delete meta[flag];
    }
  }
  return meta;
}

/** The edit recipe that applies `change` to node `id` of a page root; an empty `x-studio` is removed. */
export function metaRecipe(id: string, change: MetaChange): (root: unknown) => void {
  return (root) => {
    const found = findNode(root, id);
    if (found === null) {
      return;
    }
    const node = found.node as Obj;
    const next = applyMeta(isObj(node[STUDIO_KEY]) ? { ...(node[STUDIO_KEY] as Obj) } : {}, change);
    if (Object.keys(next).length === 0) {
      delete node[STUDIO_KEY];
    } else {
      node[STUDIO_KEY] = next;
    }
  };
}

/** The ids of locked nodes and of every node inside one: none of them is picked or edited on the canvas. */
export function lockedIds(roots: readonly (Node | undefined)[]): Set<string> {
  const out = new Set<string>();
  const walk = (node: Node, locked: boolean) => {
    const here = locked || metaOf(node).locked === true;
    if (here) {
      out.add(node.id);
    }
    for (const c of childNodes(node)) {
      walk(c, here);
    }
  };
  for (const r of roots) {
    if (r !== undefined) {
      walk(r, false);
    }
  }
  return out;
}

/** The tree the canvas draws: hidden nodes left out with everything inside them; untouched trees are returned as they are. */
export function withoutHidden(root: Node | undefined): Node | undefined {
  if (root === undefined || metaOf(root).hidden === true) {
    return undefined;
  }
  let changed = false;
  const children = root.children?.flatMap((c) => {
    const kept = withoutHidden(c);
    changed ||= kept !== c;
    return kept === undefined ? [] : [kept];
  });
  const slots =
    root.slots === undefined
      ? undefined
      : Object.fromEntries(
          Object.entries(root.slots).flatMap(([k, c]) => {
            const kept = withoutHidden(c);
            changed ||= kept !== c;
            return kept === undefined ? [] : [[k, kept]];
          }),
        );
  if (!changed) {
    return root;
  }
  return {
    ...root,
    ...(children === undefined ? {} : { children }),
    ...(slots === undefined ? {} : { slots }),
  };
}
