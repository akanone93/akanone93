// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { DocumentDraft, JsonValue } from "../editor/index.ts";

type Obj = Record<string, unknown>;

const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);

/** The app document, the translation keys and the messages of one locale sit at fixed paths (document model §1). */
export const KEYS_PATH = "translations/keys.json";
export const APP_PATH = "app.json";
export const localePath = (locale: string): string => `translations/${locale}.json`;
/** The lock scope of one locale's messages (I18N-008). */
export const localeScope = (locale: string): string => `translations/${locale}`;

/** The translation key pattern of the schema (`translation-keys.schema.json`). */
const keyPattern = /^[a-z][a-z0-9]*([.-][a-z0-9]+)*$/;
const keyMaxLength = 128;

export interface TranslationKey {
  readonly id: string;
  readonly key: string;
  readonly description: string;
}

/** The keys of a `translationKeys` document; anything malformed is skipped. */
export function readKeys(doc: unknown): TranslationKey[] {
  const keys = isObj(doc) && Array.isArray(doc["keys"]) ? doc["keys"] : [];
  return keys.flatMap((k) =>
    isObj(k) && typeof k["id"] === "string" && typeof k["key"] === "string"
      ? [{ id: k["id"], key: k["key"], description: typeof k["description"] === "string" ? k["description"] : "" }]
      : [],
  );
}

/** The messages of a `translations` document by key identifier. */
export function readMessages(doc: unknown): Record<string, string> {
  const messages = isObj(doc) ? doc["messages"] : undefined;
  const out: Record<string, string> = {};
  if (isObj(messages)) {
    for (const [id, text] of Object.entries(messages)) {
      if (typeof text === "string") {
        out[id] = text;
      }
    }
  }
  return out;
}

export interface AppLocales {
  readonly defaultLocale: string;
  readonly supported: readonly string[];
}

/** The app's default and supported locales, the default first. */
export function readLocales(app: unknown): AppLocales | undefined {
  if (!isObj(app) || typeof app["defaultLocale"] !== "string") {
    return undefined;
  }
  const supported = Array.isArray(app["supportedLocales"])
    ? app["supportedLocales"].filter((l): l is string => typeof l === "string")
    : [];
  const rest = supported.filter((l) => l !== app["defaultLocale"]);
  return { defaultLocale: app["defaultLocale"], supported: [app["defaultLocale"], ...rest] };
}

/** The node id a value belongs to: its own when it is a node (it has an `id` and a `type`), else its parent's. */
function nodeIdOf(v: Obj, parent: string): string {
  return typeof v["id"] === "string" && "type" in v ? v["id"] : parent;
}

/** Every translation key identifier a document references with `{"$t": id}`, with the node id it sits in. */
export function referencedKeys(doc: unknown): { id: string; nodeId: string }[] {
  const out: { id: string; nodeId: string }[] = [];
  const walk = (v: unknown, nodeId: string): void => {
    const children = Array.isArray(v) ? v : isObj(v) ? Object.values(v) : [];
    if (isObj(v) && typeof v["$t"] === "string") {
      out.push({ id: v["$t"], nodeId });
    }
    const here = isObj(v) ? nodeIdOf(v, nodeId) : nodeId;
    for (const x of children) {
      walk(x, here);
    }
  };
  walk(doc, "");
  return out;
}

/** A key name proposed for a literal text: lower-case words joined by dots under `prefix`, unique among `taken`. */
export function proposeKey(text: string, prefix: string, taken: ReadonlySet<string>): string {
  const words = text
    .normalize("NFKD")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, " ")
    .trim()
    .split(" ")
    .filter((w) => w !== "")
    .slice(0, 4);
  const head = prefix.toLowerCase().replace(/[^a-z0-9]+/g, "") || "text";
  const base = [/^[a-z]/.test(head) ? head : `t${head}`, ...(words.length === 0 ? ["text"] : words)].join(".");
  const clipped = base.slice(0, keyMaxLength - 6).replace(/[.-]+$/, "");
  let candidate = clipped;
  for (let n = 2; taken.has(candidate); n++) {
    candidate = `${clipped}.${n}`;
  }
  return candidate;
}

export type KeyProblem = "empty" | "invalid" | "taken";

/** Why `key` cannot be a new translation key, if it cannot. */
export function keyProblem(key: string, taken: ReadonlySet<string>): KeyProblem | undefined {
  if (key === "") {
    return "empty";
  }
  if (!keyPattern.test(key) || key.length > keyMaxLength) {
    return "invalid";
  }
  return taken.has(key) ? "taken" : undefined;
}

/** The recipe that appends a key to a `translationKeys` document. */
export function addKeyRecipe(entry: { id: string; key: string; description?: string }): (d: DocumentDraft) => void {
  return (d) => {
    const keys = Array.isArray(d["keys"]) ? (d["keys"] as unknown[]) : [];
    const added: Obj = { id: entry.id, key: entry.key };
    if (entry.description !== undefined && entry.description !== "") {
      added["description"] = entry.description;
    }
    d["keys"] = [...keys, added];
  };
}

/** The recipe that sets (or, for an empty text, removes) one message of a `translations` document. */
export function setMessageRecipe(id: string, text: string): (d: DocumentDraft) => void {
  return (d) => {
    const messages = isObj(d["messages"]) ? d["messages"] : {};
    if (text === "") {
      delete messages[id];
    } else {
      messages[id] = text;
    }
    d["messages"] = messages;
  };
}

/** A new `translationKeys` document for an app without one. */
export function newKeysDocument(schemaVersion: JsonValue, id: string, entry: TranslationKey): JsonValue {
  return {
    schemaVersion,
    kind: "translationKeys",
    id,
    keys: [{ id: entry.id, key: entry.key }],
  };
}

/** A new `translations` document for one locale. */
export function newLocaleDocument(schemaVersion: JsonValue, id: string, locale: string, messages: Obj): JsonValue {
  return { schemaVersion, kind: "translations", id, locale, messages: messages as JsonValue };
}
