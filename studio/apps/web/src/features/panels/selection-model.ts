// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { LayerNode } from "@plux/ui";
import type { Node } from "../plugin-canvas/plugin-model.ts";
import type { PlacedScreen } from "../plugin-canvas/scene.ts";
import type { Selection } from "../plugin-canvas/select/selection.ts";
import { childNodes, type NodeIndex } from "../plugin-canvas/select/tree.ts";
import { typeIcon } from "./catalogue.ts";
import { metaOf } from "./studio-meta.ts";

/** Selecting a node from a panel (F5): replaces the selection, or with `toggle` adds or removes it. Scope is its container. */
export function selectIds(index: NodeIndex, current: Selection, id: string, toggle: boolean): Selection {
  if (!index.parent.has(id)) {
    return current;
  }
  const scope = index.parent.get(id) ?? null;
  if (!toggle) {
    return { ids: [id], anchor: id, scope };
  }
  const ids = current.ids.includes(id) ? current.ids.filter((x) => x !== id) : [...current.ids, id];
  return { ids, anchor: ids.length === 0 ? null : id, scope };
}

/** A translation key's message as a row shows it (PC-32); undefined when the key has none. */
export type TextOf = (key: string) => string | undefined;

/** Text a prop shows: a string, or a translation reference's message (its key when it has none). */
function shownText(v: unknown, textOf: TextOf | undefined): string | undefined {
  if (typeof v === "string" && v.trim() !== "") {
    return v.trim();
  }
  if (typeof v === "object" && v !== null && typeof (v as Record<string, unknown>)["$t"] === "string") {
    const key = String((v as Record<string, unknown>)["$t"]);
    return textOf?.(key) ?? key;
  }
  return undefined;
}

/** The first text under a node, translated texts included. */
function firstShown(node: Node, textOf: TextOf | undefined): string | undefined {
  if (node.type === "Text") {
    const t = shownText(node.props?.data, textOf);
    if (t !== undefined) {
      return t;
    }
  }
  for (const child of childNodes(node)) {
    const t = firstShown(child, textOf);
    if (t !== undefined) {
      return t;
    }
  }
  return undefined;
}

const PREVIEW_PROPS = [
  "data",
  "label",
  "text",
  "title",
  "hintText",
  "labelText",
  "semanticLabel",
  "icon",
  "src",
  "name",
];

/** What a node shows, for its layer row: a Text's words, a field's label, a button's text, an image's source. */
export function nodePreview(node: Node, textOf?: TextOf): string | undefined {
  const props = (node.props ?? {}) as Readonly<Record<string, unknown>>;
  const clip = (t: string) => (t.length > 48 ? `${t.slice(0, 47)}…` : t);
  for (const k of PREVIEW_PROPS) {
    const t = shownText(props[k], textOf);
    if (t !== undefined) {
      return clip(t);
    }
  }
  if (/Button|Chip|Tab$/.test(node.type ?? "")) {
    const t = firstShown(node, textOf);
    return t === undefined ? undefined : clip(t);
  }
  return undefined;
}

function walk(node: Node, visit: (n: Node) => void): void {
  visit(node);
  for (const c of childNodes(node)) {
    walk(c, visit);
  }
}

/**
 * The layer tree data: one synthetic root over the screen roots. A screen's root reads as its screen (its title, the
 * phone icon, the entry mark); every other row is its widget's type with the widget's icon and what it shows.
 */
export function treeNodes(
  screens: readonly PlacedScreen[],
  index: NodeIndex,
  rootName: string,
  entry?: { readonly page: string | undefined; readonly label: string },
  textOf?: TextOf,
  instanceName?: (id: string) => string | undefined,
): Record<string, LayerNode> {
  const out: Record<string, LayerNode> = {
    plugin: {
      id: "plugin",
      name: rootName,
      type: "",
      children: screens.flatMap((s) => (s.root === undefined ? [] : [s.root.id])),
    },
  };
  for (const [id, kids] of index.children) {
    // A component instance has no widget type: its row is named after its component (PC-37).
    const type = index.type.get(id) ?? instanceName?.(id) ?? "?";
    out[id] = { id, name: type, type, icon: typeIcon(type), children: kids };
  }
  for (const s of screens) {
    if (s.root === undefined) {
      continue;
    }
    walk(s.root, (n) => {
      const row = out[n.id];
      if (row === undefined) {
        return;
      }
      const preview = nodePreview(n, textOf);
      const meta = metaOf(n);
      out[n.id] = {
        ...row,
        ...(meta.name === undefined ? {} : { name: meta.name }),
        ...(preview === undefined ? {} : { preview }),
        ...(meta.hidden === true ? { hidden: true } : {}),
        ...(meta.locked === true ? { locked: true } : {}),
      };
    });
    const root = out[s.root.id];
    if (root !== undefined) {
      out[root.id] = {
        ...root,
        name: metaOf(s.root).name ?? (s.title === "" ? root.name : s.title),
        icon: "device-mobile",
        preview: root.type,
        ...(entry !== undefined && s.id === entry.page ? { badge: entry.label } : {}),
      };
    }
  }
  return out;
}
