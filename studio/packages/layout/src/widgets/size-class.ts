// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Window size classes (WGT-010), as the runtime applies them to a screen of a given width: `compact` below 600 dp,
 * `medium` from 600, `expanded` from 840. A node's `responsive.medium` props apply from 600 and its
 * `responsive.expanded` props over them from 840 (the cascade of `PluxNode._activeOverrides`), and a `Responsive`
 * shows the variant of the size class, else the next narrower one. Studio marks a widget its visibility condition
 * shows only in some size classes with `onlyIn`, which is resolved here, where the width is known.
 */

import type { WidgetNode } from "./screen.ts";

export type SizeClass = "compact" | "medium" | "expanded";

export const SIZE_CLASSES: readonly SizeClass[] = ["compact", "medium", "expanded"];

/** The size class of a window `width` logical pixels wide (Material 3). */
export function sizeClassOf(width: number): SizeClass {
  return width >= 840 ? "expanded" : width >= 600 ? "medium" : "compact";
}

/** The `responsive` layers that apply in a size class, in the order they apply. */
export const ACTIVE_LAYERS: Readonly<Record<SizeClass, readonly ("medium" | "expanded")[]>> = {
  compact: [],
  medium: ["medium"],
  expanded: ["medium", "expanded"],
};

/** The variants a `Responsive` falls back through in a size class, widest first. */
export const RESPONSIVE_FALLBACK: Readonly<Record<SizeClass, readonly SizeClass[]>> = {
  compact: ["compact", "medium", "expanded"],
  medium: ["medium", "compact", "expanded"],
  expanded: ["expanded", "medium", "compact"],
};

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** Whether a node is drawn in `sc`: everywhere unless Studio marked the size classes it is shown in. */
function shownIn(node: unknown, sc: SizeClass): boolean {
  const only = isObj(node) ? node["onlyIn"] : undefined;
  return !Array.isArray(only) || only.includes(sc);
}

/** A node's props with its `responsive` layers for `sc` over them; the props themselves when none applies. */
function propsAt(node: Obj, sc: SizeClass): unknown {
  const responsive = node["responsive"];
  const layers = isObj(responsive) ? ACTIVE_LAYERS[sc].map((l) => responsive[l]).filter(isObj) : [];
  return layers.length === 0 ? node["props"] : Object.assign({}, isObj(node["props"]) ? node["props"] : {}, ...layers);
}

/** A node's children at `sc`: those shown there, each at `sc`; the same list when nothing changes. */
function childrenAt(kids: readonly unknown[], sc: SizeClass): readonly unknown[] {
  const next = kids.filter((c) => shownIn(c, sc)).map((c) => (isObj(c) ? nodeAt(c, sc) : c));
  return next.length === kids.length && next.every((c, i) => c === kids[i]) ? kids : next;
}

/** A node's slots at `sc`: a slot whose widget is not shown there is left empty; the same object when unchanged. */
function slotsAt(slots: Obj, sc: SizeClass): Obj {
  const next: Obj = {};
  for (const [k, v] of Object.entries(slots)) {
    if (shownIn(v, sc)) {
      next[k] = isObj(v) ? nodeAt(v, sc) : Array.isArray(v) ? childrenAt(v, sc) : v;
    }
  }
  const keys = Object.keys(slots);
  return keys.length === Object.keys(next).length && keys.every((k) => next[k] === slots[k]) ? slots : next;
}

/** A node as drawn at `sc`; the node itself when nothing about it depends on the size class. */
function nodeAt(node: Obj, sc: SizeClass): Obj {
  const props = propsAt(node, sc);
  const children = Array.isArray(node["children"]) ? childrenAt(node["children"], sc) : node["children"];
  const slots = isObj(node["slots"]) ? slotsAt(node["slots"], sc) : node["slots"];
  if (props === node["props"] && children === node["children"] && slots === node["slots"]) {
    return node;
  }
  return {
    ...node,
    ...(props === undefined ? {} : { props }),
    ...(children === undefined ? {} : { children }),
    ...(slots === undefined ? {} : { slots }),
  };
}

/** The tree as it is drawn at `width`: responsive props applied and nodes of other size classes left out. */
export function atSizeClass<T extends WidgetNode | null | undefined>(root: T, width: number): T {
  const sc = sizeClassOf(width);
  return (isObj(root) && shownIn(root, sc) ? nodeAt(root, sc) : root) as T;
}
