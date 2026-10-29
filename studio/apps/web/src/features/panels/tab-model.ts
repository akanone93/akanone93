// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { limits, widget } from "@plux/schema";

type Obj = Record<string, unknown>;

const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** Every node of the subtree: the node, its slots and their children, in document order. */
export function subtreeOf(node: unknown): Obj[] {
  if (!isObj(node)) {
    return [];
  }
  const out: Obj[] = typeof node["id"] === "string" ? [node] : [];
  const slots = isObj(node["slots"]) ? Object.values(node["slots"]) : [];
  const children = Array.isArray(node["children"]) ? node["children"] : [];
  for (const child of [...slots, ...children]) {
    out.push(...subtreeOf(child));
  }
  return out;
}

/** The compact JSON of a selection: one line per node, so a long document stays scannable (STU-062). */
export function compactJson(nodes: readonly unknown[]): string {
  return nodes.map((n) => JSON.stringify(n)).join("\n");
}

/** The names of the functions an `invokeFunction` step inside `value` calls, sorted and unique. */
export function functionsIn(value: unknown): string[] {
  const found = new Set<string>();
  const walk = (v: unknown) => {
    if (Array.isArray(v)) {
      v.forEach(walk);
    } else if (isObj(v)) {
      const input = v["input"];
      if (v["action"] === "invokeFunction" && isObj(input) && typeof input["function"] === "string") {
        found.add(input["function"]);
      }
      Object.values(v).forEach(walk);
    }
  };
  walk(value);
  return [...found].sort();
}

export interface LocalString {
  readonly node: string;
  readonly prop: string;
  /** The translation key, or undefined for a literal. */
  readonly key: string | undefined;
  readonly text: string;
}

/** The strings the user sees in the subtree: translatable props that hold a literal or a `$t` key (I18N-007). */
export function stringsIn(nodes: readonly Obj[]): LocalString[] {
  return nodes.flatMap((n) => {
    const type = typeof n["type"] === "string" ? n["type"] : undefined;
    const props = isObj(n["props"]) ? n["props"] : {};
    const d = type === undefined ? undefined : widget(type);
    return (d?.props ?? []).flatMap((p): LocalString[] => {
      const value = props[p.name];
      if (p.translatable !== true || value === undefined) {
        return [];
      }
      const node = String(n["id"]);
      if (typeof value === "string") {
        return [{ node, prop: p.name, key: undefined, text: value }];
      }
      return isObj(value) && typeof value["$t"] === "string"
        ? [{ node, prop: p.name, key: value["$t"], text: value["$t"] }]
        : [];
    });
  });
}

export type BudgetStatus = "within" | "warning" | "over";

export interface NodeBudget {
  readonly warning: number;
  readonly limit: number;
}

/** The node budget of a page document, from the limits registry (LIM-001). */
export function nodeBudget(): NodeBudget {
  const found = limits.find((l) => l.key === "page.nodes");
  return { warning: found?.warning ?? 0, limit: found?.default ?? 0 };
}

export function budgetStatus(count: number, budget: NodeBudget): BudgetStatus {
  if (count > budget.limit) {
    return "over";
  }
  return count > budget.warning ? "warning" : "within";
}
