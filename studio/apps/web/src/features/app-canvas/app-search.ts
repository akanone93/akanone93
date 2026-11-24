// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { referencedKeys } from "../localisation/l10n-model.ts";
import type { LoadedDoc, Use, WhereUsedIndex } from "../safety/index.ts";

/**
 * One search across the app (#265): its plugins, its screens and their routes, and the app-level things a screen uses —
 * translation keys, components, state entries, assets and functions — each with where it is used, so "where is this
 * used?" has one answer on the app canvas instead of one per page (an asset's in the Assets tab, a key's in the
 * Localisation table, a state entry's only inside a delete check).
 */

/** A place a thing is used: a screen, opened at it, or another document of a plugin or of the app. */
export interface Place {
  readonly pluginId: string;
  /** The page, when the place is a screen. */
  readonly pageId?: string;
  readonly title: string;
}

export type Hit =
  | { readonly kind: "plugin"; readonly id: string; readonly title: string; readonly detail: string }
  | {
      readonly kind: "screen";
      readonly pluginId: string;
      readonly pageId: string;
      readonly title: string;
      readonly detail: string;
    }
  | {
      readonly kind: "key" | "component" | "state" | "asset" | "function";
      readonly id: string;
      readonly title: string;
      readonly detail: string;
      readonly uses: readonly Place[];
    };

export type HitKind = Hit["kind"];

/** The order the groups are listed in. */
export const HIT_KINDS: readonly HitKind[] = ["plugin", "screen", "key", "component", "state", "asset", "function"];

type Json = Record<string, unknown>;
const isObj = (v: unknown): v is Json => typeof v === "object" && v !== null && !Array.isArray(v);
const str = (v: unknown): string | undefined => (typeof v === "string" ? v : undefined);

/** What the search reads: the loaded documents, their where-used index, the plugins and what each calls. */
export interface SearchInput {
  readonly docs: readonly LoadedDoc[];
  readonly index: WhereUsedIndex;
  readonly plugins: readonly {
    readonly id: string;
    readonly key: string;
    readonly name: string;
    readonly functionsUsed: readonly string[];
  }[];
}

/** The screen a use sits in, or its document. */
function placeOf(u: Use, index: WhereUsedIndex): Place {
  const page = index.pages.get(u.fromId);
  return page === undefined
    ? { pluginId: u.pluginId, title: u.fromTitle === "" ? u.path : u.fromTitle }
    : { pluginId: page.pluginId, pageId: page.id, title: page.title };
}

/** The places of `uses`, each once. */
function placesOf(uses: readonly Use[], index: WhereUsedIndex): Place[] {
  const seen = new Map<string, Place>();
  for (const u of uses) {
    const p = placeOf(u, index);
    seen.set(`${p.pluginId}|${p.pageId ?? p.title}`, p);
  }
  return [...seen.values()];
}

/** Every translation key of the app with the screens that show it (`{"$t": id}`). */
function keyHits(input: SearchInput): Hit[] {
  const pageByPath = new Map([...input.index.pages.values()].map((p) => [p.path, p]));
  const usesOf = new Map<string, Place[]>();
  for (const d of input.docs) {
    for (const r of referencedKeys(d.json)) {
      const page = pageByPath.get(d.path);
      const place: Place =
        page === undefined
          ? { pluginId: d.pluginId, title: d.path }
          : { pluginId: page.pluginId, pageId: page.id, title: page.title };
      const list = usesOf.get(r.id) ?? [];
      if (!list.some((p) => p.pluginId === place.pluginId && p.pageId === place.pageId && p.title === place.title)) {
        list.push(place);
      }
      usesOf.set(r.id, list);
    }
  }
  return input.docs
    .filter((d) => d.kind === "translationKeys" && isObj(d.json) && Array.isArray(d.json["keys"]))
    .flatMap((d) => (d.json as { keys: unknown[] }).keys)
    .flatMap((k) => {
      const id = isObj(k) ? str(k["id"]) : undefined;
      const key = isObj(k) ? str(k["key"]) : undefined;
      return id === undefined || key === undefined
        ? []
        : [{ kind: "key" as const, id, title: key, detail: "", uses: usesOf.get(id) ?? [] }];
    });
}

/** The app's components, each with the screens that use it. */
function componentHits(input: SearchInput): Hit[] {
  const { docs, index } = input;
  return docs.flatMap((d) => {
    const json = isObj(d.json) ? d.json : undefined;
    const id = d.kind === "component" ? str(json?.["id"]) : undefined;
    return id === undefined
      ? []
      : [
          {
            kind: "component" as const,
            id,
            title: str(json?.["name"]) ?? str(json?.["key"]) ?? id,
            detail: d.path,
            uses: placesOf(index.usesOf("component", id), index),
          },
        ];
  });
}

/** The state entries a document declares, by name. */
function stateEntriesOf(d: LoadedDoc): string[] {
  if ((d.kind !== "app" && d.kind !== "plugin") || !isObj(d.json) || !Array.isArray(d.json["state"])) {
    return [];
  }
  return d.json["state"].flatMap((e) => (isObj(e) && typeof e["name"] === "string" ? [e["name"]] : []));
}

/** The state entries of the app and of each plugin, each with the screens that read or write it. */
function stateHits(input: SearchInput): Hit[] {
  const { docs, index } = input;
  const declared = new Map<string, string>();
  for (const d of docs) {
    for (const name of stateEntriesOf(d)) {
      declared.set(name, d.path);
    }
  }
  return [...declared].map(([name, path]) => ({
    kind: "state" as const,
    id: name,
    title: name,
    detail: path,
    uses: placesOf(
      index.uses.filter((u) => u.kind === "usesState" && u.target.id === name),
      index,
    ),
  }));
}

/** The assets screens use, each with them. */
function assetHits(input: SearchInput): Hit[] {
  const { index } = input;
  const ids = new Set(index.uses.filter((u) => u.kind === "usesAsset").map((u) => u.target.id));
  return [...ids].map((id) => ({
    kind: "asset" as const,
    id,
    title: id,
    detail: "",
    uses: placesOf(index.usesOf("asset", id), index),
  }));
}

/** Each function the plugins call, with its callers as places. */
function functionHits(input: SearchInput): Hit[] {
  const callers = new Map<string, Place[]>();
  for (const p of input.plugins) {
    for (const fn of new Set(p.functionsUsed)) {
      const list = callers.get(fn) ?? [];
      list.push({ pluginId: p.id, title: p.name });
      callers.set(fn, list);
    }
  }
  return [...callers].map(([name, uses]) => ({ kind: "function" as const, id: name, title: name, detail: "", uses }));
}

/** Everything the search can find, before a query. */
export function searchables(input: SearchInput): Hit[] {
  const plugins: Hit[] = input.plugins.map((p) => ({ kind: "plugin", id: p.id, title: p.name, detail: p.key }));
  const screens: Hit[] = [...input.index.pages.values()].map((p) => ({
    kind: "screen",
    pluginId: p.pluginId,
    pageId: p.id,
    title: p.title,
    detail: p.route,
  }));
  return [
    ...plugins,
    ...screens,
    ...keyHits(input),
    ...componentHits(input),
    ...stateHits(input),
    ...assetHits(input),
    ...functionHits(input),
  ];
}

/** What a query is matched against: a plugin's name and key, a screen's title and route, anything else's name. */
function wordsOf(h: Hit): string {
  return (h.kind === "plugin" || h.kind === "screen" ? `${h.title} ${h.detail}` : h.title).toLowerCase();
}

/** The hits a query keeps, by name or detail, grouped by kind in `HIT_KINDS` order; nothing for an empty query. */
export function searchApp(all: readonly Hit[], query: string): Map<HitKind, Hit[]> {
  const q = query.trim().toLowerCase();
  const out = new Map<HitKind, Hit[]>();
  if (q === "") {
    return out;
  }
  for (const kind of HIT_KINDS) {
    const found = all
      .filter((h) => h.kind === kind && wordsOf(h).includes(q))
      .sort((a, b) => a.title.localeCompare(b.title));
    if (found.length > 0) {
      out.set(kind, found);
    }
  }
  return out;
}
