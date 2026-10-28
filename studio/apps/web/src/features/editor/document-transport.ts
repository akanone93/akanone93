// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError, createClient, type Transport } from "@connectrpc/connect";
import { type Document, DocumentService, keepaliveHeader } from "@plux/api";
import type { DocumentTransport, SaveFailure, ServerDocument } from "./edit-session.ts";
import type { JsonPatchOp, JsonValue } from "./json-patch.ts";
import type { SessionEntry } from "./undo-stack.ts";

const encoder = new TextEncoder();
const decoder = new TextDecoder();

/** The pipeline's view of a stored document: its canonical JSON parsed. */
export function toServerDocument(doc: Document): ServerDocument {
  return { path: doc.path, revision: doc.revision, content: JSON.parse(decoder.decode(doc.content)) as JsonValue };
}

/** Revision conflicts are Aborted (SRV-030); outages and sign-outs keep the edits for later (F4). */
export function classifySaveError(error: unknown): SaveFailure {
  const code = ConnectError.from(error).code;
  if (code === Code.Aborted) {
    return "conflict";
  }
  if (
    code === Code.Unavailable ||
    code === Code.DeadlineExceeded ||
    code === Code.ResourceExhausted ||
    code === Code.Unauthenticated ||
    code === Code.Canceled ||
    code === Code.Unknown
  ) {
    return "transient";
  }
  return "rejected";
}

/** Binds the pipeline to DocumentService for one plugin; `confirmed` receives each stored document.
 * Writes carry the tab's editor session, which tags them for the undo rebuild (STU-069). */
export function documentTransport(
  transport: Transport,
  ids: { readonly appId: string; readonly pluginId: string; readonly editorSessionId?: string },
  confirmed: (doc: Document) => void,
  /** The lock session a save is sent under; it waits for the lock (F1.1). */
  lockSession?: () => Promise<string>,
): DocumentTransport {
  const client = createClient(DocumentService, transport);
  return {
    async patch(path, ops, ifRevision, { keepalive, step }) {
      const session = lockSession === undefined ? "" : await lockSession();
      const res = await client.patchDocument(
        {
          ...ids,
          session,
          path,
          ifRevision,
          stepId: step ?? "",
          patch: ops.map((o) => ({
            op: o.op,
            path: o.path,
            value: o.value === undefined ? new Uint8Array() : encoder.encode(JSON.stringify(o.value)),
          })),
        },
        keepalive ? { headers: { [keepaliveHeader]: "1" } } : {},
      );
      if (res.document === undefined) {
        throw new ConnectError("the server returned no document", Code.Internal);
      }
      confirmed(res.document);
      return toServerDocument(res.document);
    },
    async get(path) {
      const res = await client.getDocument({ ...ids, path });
      if (res.document === undefined) {
        throw new ConnectError("the server returned no document", Code.NotFound);
      }
      confirmed(res.document);
      return toServerDocument(res.document);
    },
    async put(path, content, options) {
      const res = await client.putDocument({
        ...ids,
        path,
        ifRevision: 0n,
        stepId: options?.step ?? "",
        content: encoder.encode(JSON.stringify(content)),
      });
      if (res.document === undefined) {
        throw new ConnectError("the server returned no document", Code.Internal);
      }
      confirmed(res.document);
      return toServerDocument(res.document);
    },
    async remove(path, ifRevision) {
      await client.deleteDocument({ ...ids, path, ifRevision });
    },
    classify: classifySaveError,
  };
}

/**
 * Reads an editor session's entries for one plugin (or the app's documents, pluginId ""), newest
 * first, at most `depth`, to rebuild its undo stack after a reload (STU-069).
 */
export async function loadSessionEntries(
  transport: Transport,
  ids: { readonly appId: string; readonly pluginId: string; readonly editorSessionId: string },
  depth: number,
): Promise<SessionEntry[]> {
  const client = createClient(DocumentService, transport);
  const out: SessionEntry[] = [];
  let pageToken = "";
  do {
    const res = await client.listEditorSessionEntries({ ...ids, page: { pageSize: Math.min(depth, 100), pageToken } });
    for (const e of res.entries) {
      out.push({
        path: e.path,
        step: e.stepId,
        inverse: e.inverse.map(
          (o): JsonPatchOp =>
            ({
              op: o.op,
              path: o.path,
              ...(o.value.length > 0 ? { value: JSON.parse(decoder.decode(o.value)) as JsonValue } : {}),
            }) as JsonPatchOp,
        ),
      });
    }
    pageToken = res.page?.nextPageToken ?? "";
  } while (pageToken !== "" && out.length < depth);
  return out.slice(0, depth);
}

/** The editor session of this tab: kept in sessionStorage so it survives a reload, not other tabs. */
export function tabEditorSession(storage: Pick<Storage, "getItem" | "setItem"> | undefined): string {
  const key = "plux.editorSession";
  try {
    const known = storage?.getItem(key);
    if (known) {
      return known;
    }
  } catch {
    // Storage is blocked: the session lasts until the reload.
  }
  const id = globalThis.crypto.randomUUID();
  try {
    storage?.setItem(key, id);
  } catch {
    // As above.
  }
  return id;
}
