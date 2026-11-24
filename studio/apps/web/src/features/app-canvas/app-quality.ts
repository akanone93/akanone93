// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type BudgetStatus, budgetStatus, nodeBudget } from "../panels/tab-model.ts";
import { lowContrast } from "../plugin-canvas/a11y-marks.ts";
import type { DocLike } from "../plugin-canvas/look.ts";
import { paintThemeOf, themeTokens } from "../plugin-canvas/paint-theme.ts";
import { countNodes } from "../plugin-canvas/screen-costs.ts";
import type { PageInfo } from "../safety/index.ts";

/**
 * The quality of each plugin on the app canvas (#288): the screens with texts that do not stand out from what is
 * behind them, in light or dark (A11Y-003), and the screens near or over the page's widget budget (PC-34, LIM-001),
 * each opening its screen. Both are read from the documents, as the plugin canvas reads them: a component instance is
 * one widget here and its own texts are checked where the component is; touch targets need the laid-out boxes and are
 * marked inside a plugin's Accessibility view.
 */

export interface ScreenQuality {
  readonly pageId: string;
  readonly title: string;
  /** Texts with too little contrast, the most of the two modes, and the modes they are found in. */
  readonly lowContrast: number;
  readonly modes: readonly ("light" | "dark")[];
  readonly nodes: number;
  readonly limit: number;
  readonly status: BudgetStatus;
}

export interface PluginQuality {
  readonly pluginId: string;
  readonly screens: readonly ScreenQuality[];
}

type Json = Record<string, unknown>;
const isObj = (v: unknown): v is Json => typeof v === "object" && v !== null && !Array.isArray(v);

/** The screens that need a look, by plugin, in the order the pages come; a plugin with none is left out. */
export function appQuality(
  documents: readonly DocLike[],
  pages: Iterable<PageInfo>,
  rootOf: (page: PageInfo) => unknown,
): PluginQuality[] {
  const tokens = themeTokens(documents);
  const themes = { light: paintThemeOf(tokens, false, ""), dark: paintThemeOf(tokens, true, "") };
  const budget = nodeBudget();
  const byPlugin = new Map<string, ScreenQuality[]>();
  for (const page of pages) {
    const root = rootOf(page);
    if (!isObj(root)) {
      continue;
    }
    const light = lowContrast(root, themes.light).size;
    const dark = lowContrast(root, themes.dark).size;
    const nodes = countNodes(root);
    const status = budgetStatus(nodes, budget);
    if (light === 0 && dark === 0 && status === "within") {
      continue;
    }
    const modes = [...(light > 0 ? ["light" as const] : []), ...(dark > 0 ? ["dark" as const] : [])];
    const list = byPlugin.get(page.pluginId) ?? [];
    list.push({
      pageId: page.id,
      title: page.title,
      lowContrast: Math.max(light, dark),
      modes,
      nodes,
      limit: budget.limit,
      status,
    });
    byPlugin.set(page.pluginId, list);
  }
  return [...byPlugin].map(([pluginId, screens]) => ({ pluginId, screens }));
}
