// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { hotkeysCoreFeature, selectionFeature, syncDataLoaderFeature } from "@headless-tree/core";
import { useTree } from "@headless-tree/react";
import { type ReactElement, useRef } from "react";
import { cx, focusRing } from "./cx.ts";
import { Icon } from "./icon.tsx";
import { TypeTag } from "./type-tag.tsx";

export interface TreeNode {
  readonly id: string;
  readonly name: string;
  /** Node type shown in the type tag column. */
  readonly type: string;
  readonly children?: readonly string[];
}

/** A row that left the data between two renders: shown as nothing until the rows are rebuilt. */
export function goneNode(id: string): TreeNode {
  return { id, name: "", type: "", children: [] };
}

/**
 * Rebuilds a Headless Tree's rows when its data changes. Headless Tree rebuilds them only when the open rows change,
 * so a node removed from the data (a widget deleted) was still listed and read: "sync dataLoader returned undefined";
 * a node added stayed out of the tree. The rebuild is scheduled before the rows are read, in the same render.
 */
export function useRebuildOnData(tree: { scheduleRebuildTree(): void }, data: unknown): void {
  const built = useRef(data);
  if (built.current !== data) {
    built.current = data;
    tree.scheduleRebuildTree();
  }
}

export interface TreeProps {
  /** Nodes by id; the root is `rootId` and is not shown. */
  readonly nodes: Readonly<Record<string, TreeNode>>;
  readonly rootId: string;
  /** Accessible name of the tree, from the message catalogue. */
  readonly label: string;
  /** Reading direction: Right expands in LTR, Left expands in RTL (WAI-ARIA tree, mirrored). */
  readonly dir?: "ltr" | "rtl";
}

/** Layer tree on Headless Tree: WAI-ARIA tree keyboard, arrows follow the reading direction (U11 "Tree row"). */
export function Tree({ nodes, rootId, label, dir = "ltr" }: TreeProps): ReactElement {
  const rtl = dir === "rtl";
  const tree = useTree<TreeNode>({
    rootItemId: rootId,
    getItemName: (item) => item.getItemData().name,
    isItemFolder: (item) => (item.getItemData().children?.length ?? 0) > 0,
    dataLoader: {
      getItem: (id) => nodes[id] ?? goneNode(id),
      getChildren: (id) => [...(nodes[id]?.children ?? [])],
    },
    indent: 0,
    hotkeys: {
      expandOrDown: { hotkey: rtl ? "ArrowLeft" : "ArrowRight" },
      collapseOrUp: { hotkey: rtl ? "ArrowRight" : "ArrowLeft" },
    },
    features: [syncDataLoaderFeature, selectionFeature, hotkeysCoreFeature],
  });
  useRebuildOnData(tree, nodes);

  return (
    <div {...tree.getContainerProps(label)} dir={dir} className="flex flex-col outline-none">
      {tree.getItems().map((item) => {
        const folder = item.isFolder();
        const { key: _key, ...props } = item.getProps();
        return (
          <div
            key={item.getId()}
            {...props}
            className={cx(
              "flex h-(--density-tree-row) items-center gap-2 rounded-md px-2 text-start text-body",
              focusRing,
              item.isSelected() ? "bg-raised text-primary shadow-e1" : "text-foreground hover:bg-hover",
              item.isFocused() && "outline-2 outline-focus",
            )}
            style={{ paddingInlineStart: `calc(var(--spacing-4) * ${item.getItemMeta().level})` }}
          >
            {folder ? (
              <Icon name={item.isExpanded() ? "caret-down" : "caret-right"} size="pill" />
            ) : (
              <span className="size-icon-sm" aria-hidden="true" />
            )}
            <TypeTag>{item.getItemData().type}</TypeTag>
            <span className="truncate">{item.getItemName()}</span>
          </div>
        );
      })}
    </div>
  );
}
