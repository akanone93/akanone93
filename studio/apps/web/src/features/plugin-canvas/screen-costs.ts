// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * What each screen costs (PC-34, LIM-001, ANI-008, SCH-040): its widgets against the page's node budget, the
 * timelines that run on their own (autoplay or a scroll driver) against the count the compiler warns above, and the
 * errors and warnings of its page document — those the canvas pins to a widget and those about the page itself.
 */

import { limit } from "@plux/schema";
import type { Problem } from "@plux/validator";
import { type BudgetStatus, budgetStatus, nodeBudget } from "../panels/tab-model.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

const kidsOf = (n: Obj): unknown[] => [
  ...(isObj(n["slots"]) ? Object.values(n["slots"]).flatMap((v) => (Array.isArray(v) ? v : [v])) : []),
  ...(Array.isArray(n["children"]) ? n["children"] : []),
];

/** The widgets under `root`, itself included. */
export function countNodes(root: unknown): number {
  return isObj(root) ? 1 + kidsOf(root).reduce<number>((sum, k) => sum + countNodes(k), 0) : 0;
}

/** The timelines of a page that run on their own: those that autoplay or follow a scroll driver (ANI-006). */
export function runningTimelines(page: unknown): number {
  const animations = isObj(page) && Array.isArray(page["animations"]) ? page["animations"] : [];
  return animations.filter((t) => isObj(t) && (t["autoplay"] === true || isObj(t["driver"]))).length;
}

export interface ScreenCost {
  readonly nodes: number;
  /** The page's node budget: the count the compiler reports above, and the limit. */
  readonly budget: { readonly warning: number; readonly limit: number };
  readonly status: BudgetStatus;
  readonly timelines: number;
  /** The running timelines above which the compiler warns (`anim.simultaneousTimelines`). */
  readonly timelineLimit: number;
  readonly errors: number;
  readonly warnings: number;
}

/** The cost of each screen by id, from its drawn root, its page document and the problems of its file. */
export function screenCosts(
  screens: readonly { readonly id: string; readonly root?: unknown }[],
  pageOf: (id: string) => unknown,
  fileOf: (id: string) => string | undefined,
  problems: readonly Problem[],
): Map<string, ScreenCost> {
  const budget = nodeBudget();
  const timelineLimit = limit("anim.simultaneousTimelines").default;
  const out = new Map<string, ScreenCost>();
  for (const s of screens) {
    const file = fileOf(s.id);
    const own = file === undefined ? [] : problems.filter((p) => p.file === file);
    const nodes = countNodes(s.root);
    out.set(s.id, {
      nodes,
      budget,
      status: budgetStatus(nodes, budget),
      timelines: runningTimelines(pageOf(s.id)),
      timelineLimit,
      errors: own.filter((p) => p.severity === "error").length,
      warnings: own.filter((p) => p.severity === "warning").length,
    });
  }
  return out;
}

/** The words of a screen's cost chip; a part that has nothing to say is left out. */
export interface CostWords {
  readonly nodes: string;
  readonly title: string;
  readonly timelines?: string;
  readonly errors?: string;
  readonly warnings?: string;
}

/** What the overlay draws of each screen's cost, by the screen root's id. */
export interface CostsOverlay {
  readonly byRoot: ReadonlyMap<string, ScreenCost>;
  readonly text: (cost: ScreenCost) => CostWords;
}

/** The words of the cost chips, from the canvas's messages. */
export interface CostTexts {
  readonly costNodes: (nodes: number, limit: number) => string;
  readonly costTitle: (warning: number, limit: number) => string;
  readonly costTimelines: (running: number, limit: number) => string;
  readonly costErrors: (n: number) => string;
  readonly costWarnings: (n: number) => string;
}

/** A screen's cost chip in words: its widgets, and only the parts that need attention. */
export function costWords(c: ScreenCost, m: CostTexts): CostWords {
  return {
    nodes: m.costNodes(c.nodes, c.budget.limit),
    title: m.costTitle(c.budget.warning, c.budget.limit),
    ...(c.timelines > c.timelineLimit ? { timelines: m.costTimelines(c.timelines, c.timelineLimit) } : {}),
    ...(c.errors > 0 ? { errors: m.costErrors(c.errors) } : {}),
    ...(c.warnings > 0 ? { warnings: m.costWarnings(c.warnings) } : {}),
  };
}
