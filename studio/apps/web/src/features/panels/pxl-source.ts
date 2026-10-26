// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { scopeAt } from "@plux/validator";
import type { PxlLanguage } from "../problems/use-validation.ts";
import { type Breakpoint, pointerOf, setPropRecipe } from "./properties-model.ts";
import type { PxlEditorSource } from "./pxl-editor.tsx";

/** How long a field reuses the scope it asked the worker for; a new state variable shows within this. */
export const PXL_SCOPE_TTL_MS = 2000;

/** Scopes asked of the worker, by field; one per Properties panel so a re-render keeps them. */
export type PxlScopeCache = Map<string, { readonly at: number; readonly scope: Promise<string | undefined> }>;

/**
 * Where the language service gets the scope of one prop's expression: the page with an expression
 * at the prop, whose site the validator reports with what it can read (PXL-007). The answer is
 * kept for `PXL_SCOPE_TTL_MS` so a keystroke's completion, check and hover share one page check.
 */
export function pxlSource(
  language: PxlLanguage | undefined,
  file: string | undefined,
  content: unknown,
  id: string,
  bp: Breakpoint,
  name: string,
  cache: PxlScopeCache,
  now: () => number = Date.now,
): PxlEditorSource | undefined {
  if (language === undefined || file === undefined || content === undefined) {
    return undefined;
  }
  const where = bp === "compact" ? "props" : `responsive/${bp}`;
  const ask = async (): Promise<string | undefined> => {
    const page = structuredClone(content) as { root?: unknown };
    setPropRecipe(id, bp, name, { $expr: "null" })(page.root);
    const ptr = pointerOf(page, id);
    if (ptr === null) {
      return undefined;
    }
    return scopeAt(await language.sites(file, JSON.stringify(page)), `${ptr}/${where}/${name}`);
  };
  const key = `${file}|${id}|${bp}|${name}`;
  return {
    service: language.service,
    scope: () => {
      const cached = cache.get(key);
      if (cached !== undefined && now() - cached.at <= PXL_SCOPE_TTL_MS) {
        return cached.scope;
      }
      const scope = ask().catch(() => {
        cache.delete(key);
        return undefined;
      });
      cache.set(key, { at: now(), scope });
      return scope;
    },
  };
}
