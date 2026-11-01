// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { limitDefault } from "@plux/schema";
import { useEffect, useRef, useState } from "react";

/** One organisation-wide hit: names and keys only (F15). */
export interface OrgHit {
  readonly kind: string;
  readonly id: string;
  readonly name: string;
  /** The app the hit belongs to, for every kind but an app. */
  readonly appId?: string;
  /** The key path of an app or plugin, or the document's path in the project. */
  readonly path?: string;
}

/**
 * The route a hit opens: an app opens its canvas and a plugin its own; any other hit inside an app opens that app's
 * canvas, where it lives. A hit without its app has no route.
 */
export function hitPath(org: string, hit: OrgHit): string | undefined {
  if (hit.kind === "app") {
    return `/o/${org}/apps/${hit.id}`;
  }
  if (hit.appId === undefined || hit.appId === "") {
    return undefined;
  }
  return hit.kind === "plugin" ? `/o/${org}/apps/${hit.appId}/plugins/${hit.id}` : `/o/${org}/apps/${hit.appId}`;
}

export type OrgSearchFn = (query: string, kinds: readonly string[], limit: number) => Promise<readonly OrgHit[]>;

export type PaletteScope = "all" | "commands" | "screens";

export const searchDebounceMs = 200;

/** `>` narrows to commands, `#` to screens; anything else searches everything. */
export function parseScope(raw: string): { readonly scope: PaletteScope; readonly text: string } {
  if (raw.startsWith(">")) {
    return { scope: "commands", text: raw.slice(1).trim() };
  }
  if (raw.startsWith("#")) {
    return { scope: "screens", text: raw.slice(1).trim() };
  }
  return { scope: "all", text: raw.trim() };
}

export interface OrgSearchState {
  readonly hits: readonly OrgHit[];
  readonly pending: boolean;
  readonly failed: boolean;
}

const idle: OrgSearchState = { hits: [], pending: false, failed: false };

/**
 * Debounced server search. Only the latest query may write results: a response that arrives after the
 * text changed is dropped, so a slow answer never overwrites a newer one.
 */
export function useOrgSearch(search: OrgSearchFn | undefined, scope: PaletteScope, text: string): OrgSearchState {
  const [state, setState] = useState<OrgSearchState>(idle);
  const latest = useRef(0);
  useEffect(() => {
    latest.current += 1;
    const ticket = latest.current;
    if (search === undefined || scope === "commands" || text === "") {
      setState(idle);
      return;
    }
    setState((s) => ({ ...s, pending: true, failed: false }));
    const timer = setTimeout(() => {
      const limit = limitDefault("search.resultsPerPage");
      search(text, scope === "screens" ? ["page"] : [], limit).then(
        (hits) => {
          if (latest.current === ticket) {
            setState({ hits: hits.slice(0, limit), pending: false, failed: false });
          }
        },
        () => {
          if (latest.current === ticket) {
            setState({ hits: [], pending: false, failed: true });
          }
        },
      );
    }, searchDebounceMs);
    return () => clearTimeout(timer);
  }, [search, scope, text]);
  return state;
}

const recentsKey = "plux.palette.recents";
const recentsMax = 5;

export function readRecents(): readonly string[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(recentsKey) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((v): v is string => typeof v === "string") : [];
  } catch {
    return [];
  }
}

export function pushRecent(id: string): void {
  try {
    const next = [id, ...readRecents().filter((r) => r !== id)].slice(0, recentsMax);
    localStorage.setItem(recentsKey, JSON.stringify(next));
  } catch {
    // Recents are a convenience; a blocked store just means none.
  }
}

/** Recently run items first, the rest in their given order. */
export function recentsFirst<T>(items: readonly T[], idOf: (item: T) => string, recents: readonly string[]): T[] {
  const rank = (item: T) => {
    const at = recents.indexOf(idOf(item));
    return at === -1 ? recents.length : at;
  };
  return items
    .map((item, i) => ({ item, i }))
    .sort((a, b) => rank(a.item) - rank(b.item) || a.i - b.i)
    .map((x) => x.item);
}
