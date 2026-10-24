// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { CaseDefinition } from "./case.ts";
import { Tree, type TreeNode } from "./tree.tsx";

export const treeNodes: Readonly<Record<string, TreeNode>> = {
  root: { id: "root", name: "Root", type: "Root", children: ["page", "footer"] },
  page: { id: "page", name: "Home", type: "Page", children: ["title", "list"] },
  title: { id: "title", name: "Title", type: "Txt" },
  list: { id: "list", name: "Items", type: "Col", children: ["item"] },
  item: { id: "item", name: "Item", type: "Txt" },
  footer: { id: "footer", name: "Footer", type: "Row" },
};

export const cases: readonly CaseDefinition[] = [
  { name: "ltr", element: <Tree label="Layers" nodes={treeNodes} rootId="root" /> },
  { name: "rtl", element: <Tree label="Layers (rtl)" nodes={treeNodes} rootId="root" dir="rtl" /> },
];
