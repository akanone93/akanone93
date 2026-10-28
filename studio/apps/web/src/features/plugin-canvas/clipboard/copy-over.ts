// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { limitDefault } from "@plux/schema";
import type { JsonValue } from "../../editor/json-patch.ts";
import { type PageDoc, remapIds, uuidv7 } from "../screens.ts";
import { choiceKey, type DependencyKind, type ReportItem, type Resolution } from "./envelope.ts";

/** Copy over (STU-067, F7): the sources of a paste are fetched with the user's own read access and written anew. */

/** A source document as the reader found it. */
export interface SourceDocument {
  readonly content: JsonValue;
}

export interface CopyOverIO {
  /** The source document of an item; undefined when it does not exist or the user cannot read it. */
  readonly read: (kind: DependencyKind, id: string) => Promise<SourceDocument | undefined>;
  /** Creates a document in the target; false when the server refused it. */
  readonly write: (path: string, content: JsonValue) => Promise<boolean>;
}

export type DropReason = "forbidden" | "limit" | "rejected";

export interface Dropped {
  readonly kind: DependencyKind;
  readonly id: string;
  readonly reason: DropReason;
}

export interface CopyOverResult {
  /** Source ID to new ID, for the references of the pasted nodes. */
  readonly renamed: ReadonlyMap<string, string>;
  readonly written: readonly string[];
  /** What was not copied, to be listed to the user; references to it are dropped from the pasted nodes. */
  readonly dropped: readonly Dropped[];
}

export interface CopyOverOptions {
  readonly prefix: string;
  /** Documents the change set may still take (registry `studio.changeSetDocuments` minus those in it). */
  readonly room?: number;
  readonly newId?: () => string;
}

const FOLDER: Readonly<Record<DependencyKind, string>> = {
  functions: "functions",
  translations: "translations",
  assets: "assets",
  dataSources: "data-sources",
};

/** The items the user chose to copy, in report order. */
export const copyItems = (items: readonly ReportItem[], choices: ReadonlyMap<string, Resolution>): ReportItem[] =>
  items.filter((i) => choices.get(choiceKey(i.kind, i.id))?.action === "copy");

async function copyOne(
  item: ReportItem,
  io: CopyOverIO,
  options: Required<Pick<CopyOverOptions, "prefix" | "newId">>,
): Promise<{ readonly id: string; readonly path: string } | DropReason> {
  let source: SourceDocument | undefined;
  try {
    source = await io.read(item.kind, item.id);
  } catch {
    source = undefined;
  }
  if (source === undefined) {
    return "forbidden";
  }
  const copy = remapIds(source.content as unknown as PageDoc, options.newId) as unknown as Record<string, JsonValue>;
  const id = options.newId();
  copy["id"] = id;
  const path = `${options.prefix}${FOLDER[item.kind]}/${id}.json`;
  return (await io.write(path, copy as JsonValue)) ? { id, path } : "rejected";
}

/**
 * Creates the chosen sources in the target with new IDs. At most `room` documents are written (the change-set
 * limit); the rest, and anything unreadable or refused, is returned as dropped, never silently.
 */
export async function copyOver(
  items: readonly ReportItem[],
  io: CopyOverIO,
  options: CopyOverOptions,
): Promise<CopyOverResult> {
  const room = options.room ?? limitDefault("studio.changeSetDocuments");
  const newId = options.newId ?? uuidv7;
  const renamed = new Map<string, string>();
  const written: string[] = [];
  const dropped: Dropped[] = [];
  for (const item of items) {
    if (written.length >= room) {
      dropped.push({ kind: item.kind, id: item.id, reason: "limit" });
      continue;
    }
    const done = await copyOne(item, io, { prefix: options.prefix, newId });
    if (typeof done === "string") {
      dropped.push({ kind: item.kind, id: item.id, reason: done });
      continue;
    }
    renamed.set(item.id, done.id);
    written.push(done.path);
  }
  return { renamed, written, dropped };
}

/** The choices with copies that did not happen turned into drops, and the others into mappings to the new IDs. */
export function resolveCopies(
  choices: ReadonlyMap<string, Resolution>,
  items: readonly ReportItem[],
  result: CopyOverResult,
): ReadonlyMap<string, Resolution> {
  const out = new Map(choices);
  for (const i of copyItems(items, choices)) {
    const to = result.renamed.get(i.id);
    out.set(choiceKey(i.kind, i.id), to === undefined ? { action: "drop" } : { action: "map", to });
  }
  return out;
}
