// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { DocumentDraft, JsonValue, WholeWrite } from "../editor/index.ts";
import {
  addKeyRecipe,
  KEYS_PATH,
  keyProblem,
  localePath,
  newKeysDocument,
  newLocaleDocument,
  readKeys,
  setMessageRecipe,
} from "./l10n-model.ts";

/** The part of an edit session the extraction writes through; `EditSession` satisfies it. */
export interface DocWriter {
  readonly store: { getState(): { readonly docs: Readonly<Record<string, { readonly content: JsonValue }>> } };
  edit(
    path: string,
    recipe: (draft: DocumentDraft) => void,
    options?: { readonly group?: string; readonly step?: string },
  ): unknown;
  create(path: string, content: JsonValue): Promise<WholeWrite>;
}

export interface ExtractRequest {
  /** The session of the app's draft (keys document) and of the source locale's messages. */
  readonly keys: DocWriter;
  readonly source: DocWriter;
  readonly sourceLocale: string;
  readonly schemaVersion: JsonValue;
  readonly newId: () => string;
  /** The new key and the literal text it takes over as the source-locale message. */
  readonly key: string;
  readonly text: string;
  /** Replaces the prop with the key reference; one edit in the page's document. */
  readonly replaceProp: (reference: { $t: string }, step: string) => void;
}

export type ExtractResult =
  | { readonly ok: true; readonly id: string }
  | { readonly ok: false; readonly reason: "invalid" | "taken" | "write" };

const done: WholeWrite = { ok: true };

/** Adds the key to `translations/keys.json`, creating the document for an app that has none. */
async function writeKeys(
  req: ExtractRequest,
  existing: JsonValue | undefined,
  id: string,
  step: string,
): Promise<WholeWrite> {
  if (existing === undefined) {
    return req.keys.create(
      KEYS_PATH,
      newKeysDocument(req.schemaVersion, req.newId(), { id, key: req.key, description: "" }),
    );
  }
  req.keys.edit(KEYS_PATH, addKeyRecipe({ id, key: req.key }), { step });
  return done;
}

/** Sets the text as the source locale's message, creating that locale's document when it has none. */
async function writeMessage(req: ExtractRequest, id: string, step: string): Promise<WholeWrite> {
  const path = localePath(req.sourceLocale);
  if (req.source.store.getState().docs[path]?.content === undefined) {
    return req.source.create(
      path,
      newLocaleDocument(req.schemaVersion, req.newId(), req.sourceLocale, { [id]: req.text }),
    );
  }
  req.source.edit(path, setMessageRecipe(id, req.text), { step });
  return done;
}

/**
 * Extracts literal text to a translation key (I18N-007): the key joins `translations/keys.json`, the text becomes
 * its message in the source locale, and the prop holds `{"$t": id}` instead. Every write carries the same step
 * identifier, so the server regroups them into one undo step (STU-069, F3).
 */
export async function extractKey(req: ExtractRequest): Promise<ExtractResult> {
  const existing = req.keys.store.getState().docs[KEYS_PATH]?.content;
  const taken = new Set(readKeys(existing).map((k) => k.key));
  const problem = keyProblem(req.key, taken);
  if (problem === "taken") {
    return { ok: false, reason: "taken" };
  }
  if (problem !== undefined) {
    return { ok: false, reason: "invalid" };
  }
  const id = req.newId();
  const step = req.newId();
  const keysWrite = await writeKeys(req, existing, id, step);
  if (!keysWrite.ok) {
    return { ok: false, reason: "write" };
  }
  const messageWrite = await writeMessage(req, id, step);
  if (!messageWrite.ok) {
    return { ok: false, reason: "write" };
  }
  req.replaceProp({ $t: id }, step);
  return { ok: true, id };
}
