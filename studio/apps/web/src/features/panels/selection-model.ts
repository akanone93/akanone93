// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { TreeNode } from "@plux/ui";
import type { PlacedScreen } from "../plugin-canvas/scene.ts";
import type { Selection } from "../plugin-canvas/select/selection.ts";
import type { NodeIndex } from "../plugin-canvas/select/tree.ts";

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

/** The layer tree data: one synthetic root over the screen roots; rows are named by widget type. */
export function treeNodes(
  screens: readonly PlacedScreen[],
  index: NodeIndex,
  rootName: string,
): Record<string, TreeNode> {
  const out: Record<string, TreeNode> = {
    plugin: {
      id: "plugin",
      name: rootName,
      type: "",
      children: screens.flatMap((s) => (s.root === undefined ? [] : [s.root.id])),
    },
  };
  for (const [id, kids] of index.children) {
    const type = index.type.get(id) ?? "?";
    out[id] = { id, name: type, type, children: kids };
  }
  return out;
}
