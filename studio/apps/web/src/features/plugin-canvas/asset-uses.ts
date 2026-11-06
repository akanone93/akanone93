// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Where the app's assets are used, and what they weigh (PC-36, AST-001, AST-003): the widgets of the plugin's pages that
 * name each asset (`{"$asset": id}`), the recipe that points them at another asset when one is replaced, and the bytes
 * the plugin's used assets add up to against `plugin.assetBytes`, the limit publish enforces — with `asset.fileSize`,
 * the most one file may weigh, checked before an upload starts.
 */

import { limit } from "@plux/schema";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** One use of an asset: the widget that names it and the page it is on. */
export interface AssetUse {
  readonly node: string;
  readonly page: string;
}

/** Whether a value names `id` (or, with no id, any asset) anywhere inside it. */
function namesAsset(v: unknown, found: (asset: string) => void): void {
  if (Array.isArray(v)) {
    for (const x of v) {
      namesAsset(x, found);
    }
    return;
  }
  if (!isObj(v)) {
    return;
  }
  if (typeof v["$asset"] === "string") {
    found(v["$asset"]);
  }
  for (const x of Object.values(v)) {
    namesAsset(x, found);
  }
}

const kidsOf = (n: Obj): unknown[] => [
  ...(isObj(n["slots"]) ? Object.values(n["slots"]).flatMap((v) => (Array.isArray(v) ? v : [v])) : []),
  ...(Array.isArray(n["children"]) ? n["children"] : []),
];

/** Every asset the pages' widgets name, with each widget and page that names it, in document order. */
export function assetUses(
  pages: readonly { readonly id: string; readonly root?: unknown }[],
): ReadonlyMap<string, readonly AssetUse[]> {
  const out = new Map<string, AssetUse[]>();
  for (const page of pages) {
    const visit = (n: unknown) => {
      if (!isObj(n)) {
        return;
      }
      if (typeof n["id"] === "string") {
        const node = n["id"];
        const seen = new Set<string>();
        namesAsset(n["props"], (asset) => {
          if (!seen.has(asset)) {
            seen.add(asset);
            out.set(asset, [...(out.get(asset) ?? []), { node, page: page.id }]);
          }
        });
      }
      kidsOf(n).forEach(visit);
    };
    visit(page.root);
  }
  return out;
}

/** The recipe that points every `{"$asset": from}` under a page's root at `to`, as a replaced asset's uses move on. */
export function replaceAssetRecipe(from: string, to: string): (root: unknown) => void {
  const visit = (v: unknown) => {
    if (Array.isArray(v)) {
      v.forEach(visit);
      return;
    }
    if (!isObj(v)) {
      return;
    }
    if (v["$asset"] === from) {
      v["$asset"] = to;
    }
    Object.values(v).forEach(visit);
  };
  return visit;
}

/** The most one asset file may weigh (`asset.fileSize`), checked before an upload. */
export const assetFileLimit = (): number => limit("asset.fileSize").default;

export type BudgetState = "within" | "warning" | "over";

/** What the plugin's used assets weigh against `plugin.assetBytes`; a warning from 80 % of it, as publish warns (LIM-003). */
export function assetBudget(
  assets: readonly { readonly id: string; readonly size: number }[],
  used: ReadonlySet<string>,
): { readonly total: number; readonly limit: number; readonly state: BudgetState } {
  const max = limit("plugin.assetBytes");
  const total = assets.filter((a) => used.has(a.id)).reduce((sum, a) => sum + a.size, 0);
  const warning = max.warning > 0 ? max.warning : Math.floor(max.default * 0.8);
  return { total, limit: max.default, state: total > max.default ? "over" : total > warning ? "warning" : "within" };
}
