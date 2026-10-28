// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { DocumentDraft, JsonValue } from "../editor/index.ts";
import { referencedKeys, type TranslationKey } from "./l10n-model.ts";
import type { Entries } from "./translation-formats.ts";

/** The review state of a message (`translationState`, I18N-008), or `missing` where the locale has no message. */
export type CellStatus = "missing" | "new" | "translated" | "reviewed" | "approved";

export const STATUSES: readonly CellStatus[] = ["missing", "new", "translated", "reviewed", "approved"];

const states = new Set<string>(["new", "translated", "reviewed", "approved"]);

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The review states of a `translations` document by key identifier. */
export function readStates(doc: unknown): Record<string, CellStatus> {
  const raw = isObj(doc) ? doc["states"] : undefined;
  const out: Record<string, CellStatus> = {};
  if (isObj(raw)) {
    for (const [id, s] of Object.entries(raw)) {
      if (typeof s === "string" && states.has(s)) {
        out[id] = s as CellStatus;
      }
    }
  }
  return out;
}

/** The status of one message; a message without a recorded state counts as translated. */
export function statusOf(
  id: string,
  messages: Readonly<Record<string, string>> | undefined,
  recorded: Readonly<Record<string, CellStatus>> | undefined,
): CellStatus {
  return messages?.[id] === undefined || messages[id] === "" ? "missing" : (recorded?.[id] ?? "translated");
}

export interface GridFilter {
  readonly status: CellStatus | "all";
  readonly query: string;
  /** Keys a plugin uses; undefined shows every key. */
  readonly used: ReadonlySet<string> | undefined;
}

/** The keys that pass the filter: the plugin's keys, the status in the chosen locale, and the text in the key, its description or any message. */
export function filterKeys(
  keys: readonly TranslationKey[],
  f: GridFilter,
  cell: (id: string) => CellStatus,
  texts: (id: string) => readonly string[],
): TranslationKey[] {
  const q = f.query.trim().toLowerCase();
  return keys.filter(
    (k) =>
      (f.used === undefined || f.used.has(k.id)) &&
      (f.status === "all" || cell(k.id) === f.status) &&
      (q === "" ||
        k.key.toLowerCase().includes(q) ||
        k.description.toLowerCase().includes(q) ||
        texts(k.id).some((t) => t.toLowerCase().includes(q))),
  );
}

/** The screens (page names) of a plugin's documents that reference each key identifier. */
export function screensByKey(
  docs: readonly { readonly path: string; readonly json: unknown }[],
): Map<string, readonly string[]> {
  const out = new Map<string, string[]>();
  for (const d of docs) {
    const m = /(?:^|\/)([^/]+)\.page\.json$/.exec(d.path);
    if (m?.[1] === undefined) {
      continue;
    }
    for (const r of referencedKeys(d.json)) {
      const list = out.get(r.id) ?? [];
      if (!list.includes(m[1])) {
        list.push(m[1]);
      }
      out.set(r.id, list);
    }
  }
  return out;
}

/** Per plugin, how many of the keys it uses a locale lacks. */
export function missingByPlugin(
  used: ReadonlyMap<string, ReadonlySet<string>>,
  locales: readonly string[],
  messages: ReadonlyMap<string, Readonly<Record<string, string>>>,
): { plugin: string; total: number; missing: Record<string, number> }[] {
  return [...used].map(([plugin, ids]) => ({
    plugin,
    total: ids.size,
    missing: Object.fromEntries(
      locales.map((l) => [l, [...ids].filter((id) => (messages.get(l)?.[id] ?? "") === "").length]),
    ),
  }));
}

/** The recipe that gives messages a state, and optionally a new text, in a `translations` document. */
export function writeMessagesRecipe(texts: Entries, state: CellStatus): (d: DocumentDraft) => void {
  return (d) => {
    const messages = isObj(d["messages"]) ? d["messages"] : {};
    const recorded = isObj(d["states"]) ? d["states"] : {};
    for (const [id, text] of Object.entries(texts)) {
      messages[id] = text;
      recorded[id] = state;
    }
    d["messages"] = messages;
    d["states"] = recorded;
  };
}

/** The recipe that sets the state of messages that exist. */
export function setStatesRecipe(ids: readonly string[], state: CellStatus): (d: DocumentDraft) => void {
  return (d) => {
    const messages = isObj(d["messages"]) ? d["messages"] : {};
    const recorded = isObj(d["states"]) ? d["states"] : {};
    for (const id of ids) {
      if (typeof messages[id] === "string") {
        recorded[id] = state;
      }
    }
    d["states"] = recorded;
  };
}

/** The recipe that makes a locale supported by the app. */
export function addLocaleRecipe(locale: string): (d: DocumentDraft) => void {
  return (d) => {
    const list = Array.isArray(d["supportedLocales"]) ? (d["supportedLocales"] as unknown[]) : [];
    if (!list.includes(locale)) {
      d["supportedLocales"] = [...list, locale];
    }
  };
}

/** A new `translations` document with messages and their states. */
export function localeDocument(
  schemaVersion: JsonValue,
  id: string,
  locale: string,
  texts: Entries,
  state: CellStatus,
) {
  return {
    schemaVersion,
    kind: "translations",
    id,
    locale,
    messages: { ...texts },
    states: Object.fromEntries(Object.keys(texts).map((k) => [k, state])),
  } as JsonValue;
}

/** Maps an import's key names to key identifiers; names that are no key of the app are returned apart. */
export function matchKeys(entries: Entries, keys: readonly TranslationKey[]): { matched: Entries; unknown: string[] } {
  const byName = new Map(keys.map((k) => [k.key, k.id]));
  const matched: Entries = {};
  const unknown: string[] = [];
  for (const [name, text] of Object.entries(entries)) {
    const id = byName.get(name);
    if (id === undefined) {
      unknown.push(name);
    } else {
      matched[id] = text;
    }
  }
  return { matched, unknown };
}
