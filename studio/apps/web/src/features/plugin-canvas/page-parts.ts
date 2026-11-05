// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * A page's `Scaffold` parts on the canvas (PC-12): which of its app bar, navigation bar, floating action button and
 * drawer it still lacks, the starter widget each one is added as, and which drawer the canvas shows open — the one
 * the user opened, or the one the selection is in. Showing a drawer open changes only what the canvas draws (the
 * shown copy's `x-studio.open`), never the document.
 */

import type { Node } from "./plugin-model.ts";
import { findNode } from "./select/edits.ts";
import { childNodes } from "./select/tree.ts";

/** The parts of a page's scaffold the canvas offers to add. */
export const pageParts = ["appBar", "bottomNavigationBar", "floatingActionButton", "drawer"] as const;
export type PagePart = (typeof pageParts)[number];

/** A scaffold's drawer, at its start or its end. */
export type DrawerSide = "drawer" | "endDrawer";

/** The texts the starter widgets show: the page's title, three destinations and two drawer entries. */
export interface PartTexts {
  readonly title: string;
  readonly home: string;
  readonly search: string;
  readonly profile: string;
  readonly settings: string;
}

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The slots a scaffold node has filled. */
function filled(node: Node): Set<string> {
  const slots = (node.slots ?? {}) as Readonly<Record<string, unknown>>;
  return new Set(Object.keys(slots).filter((k) => slots[k] !== undefined && slots[k] !== null));
}

/** The parts `scaffold` lacks, in the order the canvas offers them; none for a node that is not a `Scaffold`. */
export function missingParts(scaffold: Node | undefined): PagePart[] {
  if (scaffold?.type !== "Scaffold") {
    return [];
  }
  const has = filled(scaffold);
  return pageParts.filter((p) => !has.has(p));
}

/** The drawers `scaffold` has. */
export function drawersOf(scaffold: Node | undefined): DrawerSide[] {
  if (scaffold?.type !== "Scaffold") {
    return [];
  }
  const has = filled(scaffold);
  return (["drawer", "endDrawer"] as const).filter((d) => has.has(d));
}

const icon = (id: string, name: string): Obj => ({ id, type: "Icon", props: { icon: { name } } });
const text = (id: string, data: string): Obj => ({ id, type: "Text", props: { data } });

/** The starter widget a part is added as, with fresh ids from `newId`. */
export function partTemplate(part: PagePart, t: PartTexts, newId: () => string): Obj {
  switch (part) {
    case "appBar":
      return { id: newId(), type: "AppBar", slots: { title: text(newId(), t.title) } };
    case "bottomNavigationBar":
      return {
        id: newId(),
        type: "NavigationBar",
        props: {
          selectedIndex: 0,
          destinations: [
            { label: t.home, icon: { name: "home" } },
            { label: t.search, icon: { name: "search" } },
            { label: t.profile, icon: { name: "person" } },
          ],
        },
      };
    case "floatingActionButton":
      return {
        id: newId(),
        type: "FloatingActionButton",
        // A button without an action is drawn disabled, as on the device; a light tap gives it one to start from.
        events: { onPressed: { steps: [{ action: "haptic", id: "tap", input: { pattern: "light" } }] } },
        slots: { child: icon(newId(), "add") },
      };
    case "drawer":
      return {
        id: newId(),
        type: "Drawer",
        slots: {
          child: {
            id: newId(),
            type: "ListView",
            children: [
              {
                id: newId(),
                type: "ListTile",
                slots: { leading: icon(newId(), "home"), title: text(newId(), t.home) },
              },
              {
                id: newId(),
                type: "ListTile",
                slots: { leading: icon(newId(), "settings"), title: text(newId(), t.settings) },
              },
            ],
          },
        },
      };
    default:
      return { id: newId(), type: "SizedBox" };
  }
}

/** The edit recipe that puts `node` into `part` of the scaffold `scaffoldId` of a page root, unless it is taken. */
export function addPartRecipe(scaffoldId: string, part: PagePart, node: Obj): (root: unknown) => void {
  return (root) => {
    const found = findNode(root, scaffoldId);
    if (found === null) {
      return;
    }
    const target = found.node as Obj;
    const slots = isObj(target["slots"]) ? (target["slots"] as Obj) : {};
    if (slots[part] !== undefined) {
      return;
    }
    target["slots"] = { ...slots, [part]: node };
  };
}

/** Each scaffold whose drawer holds a node of `ids` (or is one), with that drawer: the selection opens it. */
export function drawersOfSelection(
  roots: readonly (Node | undefined)[],
  ids: readonly string[],
): Map<string, DrawerSide> {
  const out = new Map<string, DrawerSide>();
  if (ids.length === 0) {
    return out;
  }
  const wanted = new Set(ids);
  const holds = (node: Node): boolean => wanted.has(node.id) || childNodes(node).some(holds);
  const visit = (node: Node) => {
    if (node.type === "Scaffold") {
      for (const side of ["drawer", "endDrawer"] as const) {
        const drawer = (node.slots as Readonly<Record<string, Node | undefined>> | undefined)?.[side];
        if (drawer !== undefined && holds(drawer)) {
          out.set(node.id, side);
        }
      }
    }
    for (const c of childNodes(node)) {
      visit(c);
    }
  };
  for (const r of roots) {
    if (r !== undefined) {
      visit(r);
    }
  }
  return out;
}

/** A copy of `root` whose scaffolds in `open` are marked to be drawn with that drawer shown (`x-studio.open`). */
export function withOpenDrawers(root: Node, open: ReadonlyMap<string, DrawerSide>): Node {
  if (open.size === 0) {
    return root;
  }
  const copy = (node: Node): Node => {
    const side = open.get(node.id);
    const slots = node.slots === undefined ? undefined : mapSlots(node.slots, copy);
    const children = node.children?.map(copy);
    const next: Obj = { ...(node as unknown as Obj) };
    if (slots !== undefined) {
      next["slots"] = slots;
    }
    if (children !== undefined) {
      next["children"] = children;
    }
    if (side !== undefined) {
      const meta = isObj(next["x-studio"]) ? (next["x-studio"] as Obj) : {};
      next["x-studio"] = { ...meta, open: side };
    }
    return next as unknown as Node;
  };
  return copy(root);
}

function mapSlots(slots: Readonly<Record<string, unknown>>, f: (n: Node) => Node): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(slots)) {
    out[k] = Array.isArray(v)
      ? v.map((x) => (isObj(x) ? f(x as unknown as Node) : x))
      : isObj(v)
        ? f(v as unknown as Node)
        : v;
  }
  return out;
}

/** The scaffold a selection names: the one selected node when it is a `Scaffold` at a page's root. */
export function selectedPageScaffold(roots: readonly (Node | undefined)[], ids: readonly string[]): Node | undefined {
  if (ids.length !== 1) {
    return undefined;
  }
  const root = roots.find((r) => r?.id === ids[0]);
  if (root !== undefined) {
    return root.type === "Scaffold" ? root : undefined;
  }
  return undefined;
}

/** The node `id` in `roots`, if any. */
export function nodeIn(roots: readonly (Node | undefined)[], id: string): Node | undefined {
  for (const r of roots) {
    const found = r === undefined ? null : findNode(r, id);
    if (found !== null) {
      return found.node as Node;
    }
  }
  return undefined;
}

/**
 * The drawers the canvas shows: each one the user showed (`null` hides one), and each one the selection is in, unless
 * the user hid it.
 */
export function shownDrawers(
  toggles: ReadonlyMap<string, DrawerSide | null>,
  bySelection: ReadonlyMap<string, DrawerSide>,
): ReadonlyMap<string, DrawerSide> {
  const out = new Map<string, DrawerSide>();
  for (const [id, side] of bySelection) {
    if (!toggles.has(id)) {
      out.set(id, side);
    }
  }
  for (const [id, side] of toggles) {
    if (side !== null) {
      out.set(id, side);
    }
  }
  return out;
}
