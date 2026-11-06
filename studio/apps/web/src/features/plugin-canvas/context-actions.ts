// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * What the context toolbar does to the selected widget (PC-19, STU-067): wrap it in a container of a chosen type,
 * convert it to a related widget, add a sibling after it, align it in its row or column. Each is a recipe on the page
 * root; the descriptors decide where a child goes (a list of children, or the `child` slot) and which props a
 * converted widget keeps.
 */

import { widget } from "@plux/schema";
import { findNode } from "./select/edits.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The containers a widget can be wrapped in, most used first. */
export const wrapTypes = [
  "Column",
  "Row",
  "Container",
  "Padding",
  "Center",
  "Card",
  "SizedBox",
  "Expanded",
  "Stack",
  "Align",
] as const;

/** Widgets that stand in for one another: a convert keeps the props both declare, and the children. */
const families: readonly (readonly string[])[] = [
  ["Column", "Row", "Wrap", "ListView"],
  ["FilledButton", "ElevatedButton", "OutlinedButton", "TextButton"],
  ["Padding", "Center", "Align", "SizedBox", "Container", "Card", "Expanded", "Flexible", "DecoratedBox", "ColoredBox"],
  ["Switch", "Checkbox"],
  ["TextField", "TextFormField"],
  ["NavigationBar", "BottomNavigationBar"],
];

/** The widgets `type` can be converted to. */
export function convertTargets(type: string | undefined): string[] {
  const family = families.find((f) => type !== undefined && f.includes(type));
  return family === undefined ? [] : family.filter((t) => t !== type && widget(t) !== undefined);
}

/** The starting props a wrapper gets: a padding for `Padding`, the centre for `Align`. */
const wrapperProps: Readonly<Record<string, Obj>> = {
  Padding: { padding: { all: 16 } },
  Align: { alignment: "center" },
};

/** Puts `child` into a fresh `type` node: as its list of children, or in its `child` slot. */
export function wrapperOf(type: string, id: string, child: unknown): Obj {
  const d = widget(type);
  const props = wrapperProps[type];
  const base: Obj = { id, type, ...(props === undefined ? {} : { props }) };
  if (d?.children !== undefined || d === undefined) {
    return { ...base, children: [child] };
  }
  const slot = d.slots.find((s) => s.name === "child") ?? d.slots[0];
  return { ...base, slots: { [slot?.name ?? "child"]: child } };
}

/** Puts `node` where `id` stands (a list or a slot). */
export function replaceAt(draft: unknown, id: string, make: (node: Obj) => Obj): void {
  const p = findNode(draft, id);
  if (p === null || p.parent === null) {
    return;
  }
  const next = make(p.node as Obj);
  if (p.list !== null) {
    p.list[p.index] = next;
  } else if (p.slot !== null && isObj(p.parent.slots)) {
    p.parent.slots[p.slot] = next;
  }
}

/** The recipe that wraps node `id` in a new `type` with id `wrapperId`. */
export function wrapInRecipe(id: string, wrapperId: string, type: string): (draft: unknown) => void {
  return (draft) => replaceAt(draft, id, (node) => wrapperOf(type, wrapperId, node));
}

/** The one child of a node, from its children or its slots. */
function onlyChild(node: Obj): unknown {
  const kids = Array.isArray(node["children"]) ? (node["children"] as unknown[]) : [];
  const slots = isObj(node["slots"]) ? Object.values(node["slots"]) : [];
  return [...kids, ...slots].length === 1 ? [...kids, ...slots][0] : undefined;
}

/**
 * The recipe that turns node `id` into a `to`, keeping its id, its events, the props `to` declares too, and its
 * children: as a list when `to` takes one, else the only child in its `child` slot (a widget with several children
 * cannot become one that takes a single child).
 */
export function convertRecipe(id: string, to: string): (draft: unknown) => void {
  return (draft) =>
    replaceAt(draft, id, (node) => {
      const d = widget(to);
      return d === undefined ? node : converted(node, to, d);
    });
}

/** A node as a `to`: its props `to` declares too, its children in the form `to` takes them. */
function converted(node: Obj, to: string, d: NonNullable<ReturnType<typeof widget>>): Obj {
  const names = new Set(d.props.map((p) => p.name));
  const props = isObj(node["props"])
    ? Object.fromEntries(Object.entries(node["props"]).filter(([k]) => names.has(k)))
    : {};
  const out: Obj = { ...node, type: to };
  delete out["children"];
  delete out["slots"];
  delete out["props"];
  if (Object.keys(props).length > 0) {
    out["props"] = props;
  }
  return { ...out, ...childrenFor(node, d) };
}

/** A converted node's children: a list when it takes one, else its slots, the only child in the first. */
function childrenFor(node: Obj, d: NonNullable<ReturnType<typeof widget>>): Obj {
  const kids = Array.isArray(node["children"]) ? (node["children"] as unknown[]) : [];
  const slots = isObj(node["slots"]) ? (node["slots"] as Obj) : {};
  if (d.children !== undefined) {
    const all = [...kids, ...Object.values(slots)];
    return all.length > 0 ? { children: all } : {};
  }
  if (d.slots.length === 0) {
    return {};
  }
  const kept = Object.fromEntries(Object.entries(slots).filter(([k]) => d.slots.some((s) => s.name === k)));
  const child = Object.keys(kept).length > 0 ? undefined : onlyChild(node);
  const merged = child === undefined ? kept : { ...kept, [d.slots[0]?.name ?? "child"]: child };
  return Object.keys(merged).length > 0 ? { slots: merged } : {};
}

/** Whether a node of `type` can be converted to `to` without losing children: both take a list, or one child fits. */
export function convertKeepsChildren(childCount: number, to: string): boolean {
  const d = widget(to);
  return d === undefined ? false : d.children !== undefined || childCount <= 1;
}

/** The quick widgets a sibling can be: a text, a button, an icon, a gap, a divider, an image, a container. */
export const siblingTypes = ["Text", "FilledButton", "Icon", "SizedBox", "Divider", "Image", "Container"] as const;
export type SiblingType = (typeof siblingTypes)[number];

/** A new sibling of `type` with fresh ids from `newId`; `label` is a button's and a text's starting text. */
export function siblingOf(type: SiblingType, label: string, newId: () => string): Obj {
  switch (type) {
    case "Text":
      return { id: newId(), type, props: { data: label } };
    case "FilledButton":
      return {
        id: newId(),
        type,
        events: { onPressed: { steps: [{ action: "haptic", id: "tap", input: { pattern: "light" } }] } },
        slots: { child: { id: newId(), type: "Text", props: { data: label } } },
      };
    case "Icon":
      return { id: newId(), type, props: { icon: { name: "star" } } };
    case "SizedBox":
      return { id: newId(), type, props: { height: 16, width: 16 } };
    case "Container":
      return { id: newId(), type, props: { height: 80 } };
    default:
      return { id: newId(), type };
  }
}

/** The recipe that adds `node` right after node `id` in its parent's children; nothing where `id` fills a slot. */
export function addAfterRecipe(id: string, node: Obj): (draft: unknown) => void {
  return (draft) => {
    const p = findNode(draft, id);
    if (p?.list == null) {
      return;
    }
    p.list.splice(p.index + 1, 0, node);
  };
}

/** The cross-axis alignments of a row's or a column's children. */
export const crossAlignments = ["start", "center", "end", "stretch"] as const;
export type CrossAlignment = (typeof crossAlignments)[number];

/** The flex types whose children the toolbar aligns. */
export const isFlex = (type: string | undefined): boolean =>
  type === "Column" || type === "Row" || type === "Flex" || type === "Wrap";

/** The recipe that sets the cross-axis alignment of flex node `id`'s children. */
export function crossAlignRecipe(id: string, value: CrossAlignment): (draft: unknown) => void {
  return (draft) => {
    const p = findNode(draft, id);
    if (p === null) {
      return;
    }
    const node = p.node as Obj;
    const props = isObj(node["props"]) ? (node["props"] as Obj) : {};
    node["props"] = { ...props, crossAxisAlignment: value };
  };
}

/** The cross-axis alignment a flex node's children have now; `center` when unset (Flutter's default). */
export function crossAlignmentOf(node: { readonly props?: unknown } | undefined): CrossAlignment {
  const v = isObj(node?.props) ? (node.props as Obj)["crossAxisAlignment"] : undefined;
  return crossAlignments.find((a) => a === v) ?? "center";
}
