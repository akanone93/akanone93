// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Code, ConnectError, createClient, type Transport } from "@connectrpc/connect";
import { type Document, DocumentService, keepaliveHeader } from "@plux/api";
import type { DocumentTransport, SaveFailure, ServerDocument } from "./edit-session.ts";
import type { JsonValue } from "./json-patch.ts";

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

/** Binds the pipeline to DocumentService for one plugin; `confirmed` receives each stored document. */
export function documentTransport(
  transport: Transport,
  ids: { readonly appId: string; readonly pluginId: string },
  confirmed: (doc: Document) => void,
): DocumentTransport {
  const client = createClient(DocumentService, transport);
  return {
    async patch(path, ops, ifRevision, { keepalive }) {
      const res = await client.patchDocument(
        {
          ...ids,
          path,
          ifRevision,
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
    classify: classifySaveError,
  };
}
