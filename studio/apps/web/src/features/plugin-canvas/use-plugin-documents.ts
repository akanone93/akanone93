// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { DocumentService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";
import { parseDocument } from "./plugin-model.ts";

/**
 * Each document with its content parsed once, as it is loaded (#214): every reader takes `parsed` rather than parsing
 * the bytes again, which every render of the plugin canvas did for the whole app. It lives in the query cache with the
 * listing and is replaced with it; it is shared, so a reader never changes it (it clones first).
 */
function withParsed<D extends { readonly content: Uint8Array }>(
  documents: readonly D[],
): (D & { readonly parsed: unknown })[] {
  return documents.map((d) => ({ ...d, parsed: parseDocument(d.content) }));
}

/** ListDocuments of one plugin with content (read-only), through the shared query cache. */
export function usePluginDocuments(appId: string, pluginId: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["plugin-documents", appId, pluginId],
    queryFn: async () => {
      const res = await createClient(DocumentService, session.transport).listDocuments({
        appId,
        pluginId,
        contentIncluded: true,
      });
      return withParsed(res.documents);
    },
  });
}

/** Every page of a listing, following `page` tokens until the last. */
async function allPages<T>(
  fetchPage: (pageToken: string) => Promise<{ items: T[]; next: string | undefined }>,
): Promise<T[]> {
  const out: T[] = [];
  let token = "";
  do {
    const { items, next } = await fetchPage(token);
    out.push(...items);
    token = next ?? "";
  } while (token !== "");
  return out;
}

/**
 * The whole project with content, as the validator loads it: the app-level documents (app.json) and the documents of
 * every plugin of the app, in one listing followed to its last page. One listing, not one per plugin: an app's 17
 * plugins took 18 calls on every page that validates, and an app at the plugin limit would take 201 (#195).
 */
export function useAppDocuments(appId: string) {
  const { session } = useApp();
  return useQuery({
    queryKey: ["project-documents", appId],
    queryFn: async () => {
      const documents = createClient(DocumentService, session.transport);
      return withParsed(
        await allPages((pageToken) =>
          documents
            .listDocuments({ appId, allPlugins: true, contentIncluded: true, page: { pageSize: 100, pageToken } })
            .then((r) => ({ items: r.documents, next: r.page?.nextPageToken })),
        ),
      );
    },
  });
}
