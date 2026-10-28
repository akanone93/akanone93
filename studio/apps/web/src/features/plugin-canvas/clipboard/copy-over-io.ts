// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { Client } from "@connectrpc/connect";
import type { DocumentService } from "@plux/api";
import type { JsonValue } from "../../editor/json-patch.ts";
import type { CopyOverIO } from "./copy-over.ts";
import type { ClipboardSource, DependencyKind } from "./envelope.ts";

type Documents = Pick<Client<typeof DocumentService>, "listDocuments">;

const decoder = new TextDecoder();

const parse = (bytes: Uint8Array): Record<string, JsonValue> | undefined => {
  try {
    const v = JSON.parse(decoder.decode(bytes)) as unknown;
    return typeof v === "object" && v !== null && !Array.isArray(v) ? (v as Record<string, JsonValue>) : undefined;
  } catch {
    return undefined;
  }
};

/**
 * Reads sources for a paste with the user's own access: the source plugin's documents and its app's. The server
 * decides what the user may read; a refusal, in another organisation too, reads as nothing found, and the item is
 * dropped and listed (F7).
 */
export function sourceReader(client: Documents, source: ClipboardSource): CopyOverIO["read"] {
  let cached: Promise<readonly Record<string, JsonValue>[]> | undefined;
  const load = () =>
    (cached ??= Promise.all(
      [source.plugin, ""].map((pluginId) =>
        client
          .listDocuments({ appId: source.app, pluginId, contentIncluded: true })
          .then((r) => r.documents.map((d) => parse(d.content)))
          .catch(() => []),
      ),
    ).then((lists) => lists.flat().filter((d): d is Record<string, JsonValue> => d !== undefined)));
  return async (kind: DependencyKind, id: string) => {
    const docs = await load();
    const found = docs.find((d) => d["id"] === id || d["key"] === id);
    if (found !== undefined) {
      return { content: found };
    }
    if (kind === "translations") {
      const hit = docs.find((d) => typeof d["messages"] === "object" && d["messages"] !== null && id in d["messages"]);
      const messages = hit?.["messages"] as Record<string, JsonValue> | undefined;
      return hit === undefined || messages === undefined
        ? undefined
        : { content: { ...hit, messages: { [id]: messages[id] as JsonValue } } };
    }
    return undefined;
  };
}
