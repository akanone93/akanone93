// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { lineage, type NodeIndex } from "./tree.ts";

/** F5: an ordered set of node IDs, the anchor (last clicked) and the scope (the container you are inside). */
export interface Selection {
  readonly ids: readonly string[];
  readonly anchor: string | null;
  readonly scope: string | null;
}

export const emptySelection: Selection = { ids: [], anchor: null, scope: null };

/** Modifiers of a click or marquee; `deep` is Ctrl/Cmd (F5), `toggle` is Shift. */
export interface Modifiers {
  readonly toggle?: boolean;
  readonly deep?: boolean;
}

/**
 * The node a hit selects in `scope`: the innermost hit when `deep`, else the ancestor-or-self that is a direct
 * child of the scope. A scope the hit is not inside falls back to the top level (the screen root's children).
 */
export function resolveHit(
  index: NodeIndex,
  hits: readonly string[],
  scope: string | null,
  deep: boolean,
): string | null {
  const hit = hits.find((h) => index.parent.has(h));
  if (hit === undefined) {
    return null;
  }
  if (deep) {
    return hit;
  }
  const chain = lineage(index, hit);
  const inScope = scope !== null && chain.includes(scope) && scope !== hit;
  const directChild = inScope ? chain.find((c) => index.parent.get(c) === scope) : undefined;
  if (directChild !== undefined) {
    return directChild;
  }
  const top = chain.find((c) => (index.depth.get(c) ?? 0) === 1);
  return top ?? chain[chain.length - 1] ?? null;
}

/** Click (STU-067): replaces the selection, or toggles with Shift. */
export function click(index: NodeIndex, s: Selection, hits: readonly string[], mods: Modifiers = {}): Selection {
  const target = resolveHit(index, hits, s.scope, mods.deep === true);
  if (target === null) {
    return mods.toggle === true ? s : emptySelection;
  }
  const scope = index.parent.get(target) ?? null;
  if (mods.toggle !== true) {
    return { ids: [target], anchor: target, scope };
  }
  const has = s.ids.includes(target);
  const ids = has ? s.ids.filter((i) => i !== target) : [...s.ids, target];
  return { ids, anchor: has ? (ids[ids.length - 1] ?? null) : target, scope: has ? s.scope : scope };
}

/** Marquee: nodes touching (or, with `contain`, inside) the rectangle that are direct children of the scope. */
export function marquee(index: NodeIndex, s: Selection, hits: readonly string[], mods: Modifiers = {}): Selection {
  const level = s.scope;
  const inLevel = (h: string) => (level === null ? index.depth.get(h) === 1 : index.parent.get(h) === level);
  const picked = hits.filter((h) => index.parent.has(h) && inLevel(h));
  const unique = [...new Set(picked)];
  const ids =
    mods.toggle === true
      ? [...s.ids.filter((i) => !unique.includes(i)), ...unique.filter((i) => !s.ids.includes(i))]
      : unique;
  return { ids, anchor: ids[ids.length - 1] ?? null, scope: level };
}

/** Esc: select the parent of the anchor; at the top level, clear. */
export function toParent(index: NodeIndex, s: Selection): Selection {
  if (s.anchor === null) {
    return emptySelection;
  }
  const p = index.parent.get(s.anchor) ?? null;
  if (p === null || (index.depth.get(p) ?? 0) === 0) {
    return emptySelection;
  }
  return { ids: [p], anchor: p, scope: index.parent.get(p) ?? null };
}

/** Enter: select the first child of the anchor and go into it. */
export function enter(index: NodeIndex, s: Selection): Selection {
  const first = s.anchor === null ? undefined : index.children.get(s.anchor)?.[0];
  return first === undefined || s.anchor === null ? s : { ids: [first], anchor: first, scope: s.anchor };
}

/** Tab / Shift+Tab: the next or previous sibling of the anchor, wrapping. */
export function sibling(index: NodeIndex, s: Selection, step: 1 | -1): Selection {
  const p = s.anchor === null ? undefined : index.parent.get(s.anchor);
  const kids = p === null || p === undefined ? undefined : index.children.get(p);
  if (kids === undefined || s.anchor === null) {
    return s;
  }
  const next = kids[(kids.indexOf(s.anchor) + step + kids.length) % kids.length];
  return next === undefined ? s : { ids: [next], anchor: next, scope: p ?? null };
}

/** Ctrl/Cmd+A: all siblings of the anchor. */
export function selectSiblings(index: NodeIndex, s: Selection): Selection {
  const p = s.anchor === null ? undefined : index.parent.get(s.anchor);
  const kids = p === null || p === undefined ? undefined : index.children.get(p);
  return kids === undefined ? s : { ids: kids, anchor: s.anchor, scope: p ?? null };
}

/** Deleted nodes drop out of the selection (F5). */
export function prune(index: NodeIndex, s: Selection): Selection {
  const ids = s.ids.filter((i) => index.parent.has(i));
  if (ids.length === s.ids.length) {
    return s;
  }
  const anchor = s.anchor !== null && ids.includes(s.anchor) ? s.anchor : (ids[ids.length - 1] ?? null);
  return { ids, anchor, scope: s.scope !== null && index.parent.has(s.scope) ? s.scope : null };
}
