// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Per-locale overrides (PC-32, I18N-012): a node's `localeOverrides` — props it sets differently in a locale, such as a
 * longer label's width or an image with text — applied over its props for the previewed locale as the runtime layers
 * them: the language's (`de`) first, the exact locale's (`de-AT`) over it; and the recipe the inspector edits them with.
 */

import { findNode } from "./select/edits.ts";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The overrides of node `node` for `locale`: its language's, with the exact locale's over them. */
export function overridesFor(node: unknown, locale: string): Obj | undefined {
  const all = isObj(node) && isObj(node["localeOverrides"]) ? node["localeOverrides"] : {};
  const language = locale.split("-")[0] ?? locale;
  const layers = [language === locale ? undefined : all[language], all[locale]].filter(isObj);
  return layers.length === 0 ? undefined : Object.assign({}, ...layers);
}

/** The overrides node `node` sets for exactly `locale`, which the inspector edits. */
export function ownOverrides(node: unknown, locale: string): Obj {
  const all = isObj(node) && isObj(node["localeOverrides"]) ? node["localeOverrides"] : {};
  return isObj(all[locale]) ? all[locale] : {};
}

/** A node's props with its overrides for `locale` over them; the node itself when it has none. */
export function withLocaleOverrides(node: Obj, locale: string): Obj {
  const own = overridesFor(node, locale);
  return own === undefined ? node : { ...node, props: { ...(isObj(node["props"]) ? node["props"] : {}), ...own } };
}

/** The recipe that sets (or, with undefined, removes) node `id`'s override of `prop` for `locale`. */
export function setOverrideRecipe(id: string, locale: string, prop: string, value: unknown): (root: unknown) => void {
  return (root) => {
    const node = findNode(root, id)?.node as Obj | undefined;
    if (node === undefined) {
      return;
    }
    const all: Obj = isObj(node["localeOverrides"]) ? node["localeOverrides"] : {};
    const own: Obj = isObj(all[locale]) ? (all[locale] as Obj) : {};
    if (value === undefined) {
      delete own[prop];
    } else {
      own[prop] = value;
    }
    if (Object.keys(own).length === 0) {
      delete all[locale];
    } else {
      all[locale] = own;
    }
    if (Object.keys(all).length === 0) {
      delete node["localeOverrides"];
    } else {
      node["localeOverrides"] = all;
    }
  };
}
