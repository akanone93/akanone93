// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { hotkeysCoreFeature, syncDataLoaderFeature } from "@headless-tree/core";
import { useTree } from "@headless-tree/react";
import { type ReactElement, useEffect, useRef, useState } from "react";
import { cx, focusRing } from "./cx.ts";
import { Icon } from "./icon.tsx";
import type { TreeNode } from "./tree.tsx";
import { TypeTag } from "./type-tag.tsx";

export interface LayerTreeProps {
  readonly nodes: Readonly<Record<string, TreeNode>>;
  readonly rootId: string;
  readonly label: string;
  readonly dir?: "ltr" | "rtl";
  /** The shared selection (F5): the tree shows it, expands to it and scrolls to it. */
  readonly selected: readonly string[];
  readonly onSelect: (id: string, toggle: boolean) => void;
  /** A row was dropped on `target`: before it, or after it (inside it when it is a folder). */
  readonly onDrop?: ((dragged: string, target: string, half: "before" | "after") => void) | undefined;
}

const DRAG_TYPE = "application/x-plux-layer";

function ancestorsOf(nodes: LayerTreeProps["nodes"], id: string): string[] {
  const parentOf = new Map<string, string>();
  for (const [pid, n] of Object.entries(nodes)) {
    for (const c of n.children ?? []) {
      parentOf.set(c, pid);
    }
  }
  const out: string[] = [];
  for (let p = parentOf.get(id); p !== undefined; p = parentOf.get(p)) {
    out.push(p);
  }
  return out;
}

/** The layer tree on Headless Tree with the selection owned by the caller (WAI-ARIA tree, arrows follow reading direction). */
export function LayerTree(props: LayerTreeProps): ReactElement {
  const { nodes, rootId, label, dir = "ltr", selected, onSelect, onDrop } = props;
  const rtl = dir === "rtl";
  const [expanded, setExpanded] = useState<string[]>([]);
  const box = useRef<HTMLDivElement>(null);
  const tree = useTree<TreeNode>({
    rootItemId: rootId,
    getItemName: (item) => item.getItemData().name,
    isItemFolder: (item) => (item.getItemData().children?.length ?? 0) > 0,
    dataLoader: {
      getItem: (id) => nodes[id] as TreeNode,
      getChildren: (id) => [...(nodes[id]?.children ?? [])],
    },
    state: { expandedItems: expanded },
    setExpandedItems: setExpanded,
    indent: 0,
    hotkeys: {
      expandOrDown: { hotkey: rtl ? "ArrowLeft" : "ArrowRight" },
      collapseOrUp: { hotkey: rtl ? "ArrowRight" : "ArrowLeft" },
    },
    features: [syncDataLoaderFeature, hotkeysCoreFeature],
  });
  const anchor = selected[selected.length - 1];
  // biome-ignore lint/correctness/useExhaustiveDependencies: runs when the selection or expansion changes, reading the current nodes
  useEffect(() => {
    if (anchor === undefined) {
      return;
    }
    const open = ancestorsOf(nodes, anchor).filter((a) => a !== rootId && !expanded.includes(a));
    if (open.length > 0) {
      setExpanded((cur) => [...new Set([...cur, ...open])]);
      return;
    }
    const row = [...(box.current?.querySelectorAll<HTMLElement>("[data-item-id]") ?? [])].find(
      (e) => e.dataset["itemId"] === anchor,
    );
    row?.scrollIntoView?.({ block: "nearest" });
  }, [anchor, expanded]);

  return (
    <div {...tree.getContainerProps(label)} ref={box} dir={dir} className="flex flex-col outline-none">
      {tree.getItems().map((item) => {
        const folder = item.isFolder();
        const { key: _key, onClick: _click, ...rest } = item.getProps();
        const id = item.getId();
        const isSel = selected.includes(id);
        return (
          <div
            key={id}
            {...rest}
            data-item-id={id}
            role="treeitem"
            tabIndex={rest["tabIndex"] ?? -1}
            aria-selected={isSel}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onSelect(id, e.shiftKey || e.ctrlKey || e.metaKey);
              }
            }}
            draggable={onDrop !== undefined}
            onClick={(e) => onSelect(id, e.shiftKey || e.ctrlKey || e.metaKey)}
            onDragStart={(e) => e.dataTransfer.setData(DRAG_TYPE, id)}
            onDragOver={(e) => {
              if (onDrop !== undefined && e.dataTransfer.types.includes(DRAG_TYPE)) {
                e.preventDefault();
              }
            }}
            onDrop={(e) => {
              const dragged = e.dataTransfer.getData(DRAG_TYPE);
              if (onDrop === undefined || dragged === "") {
                return;
              }
              e.preventDefault();
              const r = e.currentTarget.getBoundingClientRect();
              onDrop(dragged, id, e.clientY < r.top + r.height / 2 ? "before" : "after");
            }}
            className={cx(
              "flex h-(--density-tree-row) items-center gap-2 rounded-md px-2 text-start text-body",
              focusRing,
              isSel ? "bg-raised text-accent shadow-e1" : "text-primary hover:bg-hover",
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
