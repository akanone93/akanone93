// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type Client, createClient } from "@connectrpc/connect";
import { DocumentService, ReleaseService } from "@plux/api";
import { useCallback } from "react";
import { useApp } from "../../lib/use-app.tsx";

/** Reads the document at `path` as of the plugin's latest published version, or null when none was published. */
export type LoadPublished = (path: string) => Promise<string | null>;

/** The snapshot of the highest published version, or undefined when the plugin was never published. */
async function latestSnapshot(releases: Client<typeof ReleaseService>, pluginId: string): Promise<string | undefined> {
  let latest: { version: bigint; snapshot: string } | undefined;
  let token = "";
  do {
    const page = await releases.listPluginVersions({ pluginId, page: { pageSize: 100, pageToken: token } });
    for (const v of page.versions) {
      if (v.sourceSnapshotId !== "" && (latest === undefined || v.version > latest.version)) {
        latest = { version: v.version, snapshot: v.sourceSnapshotId };
      }
    }
    token = page.page?.nextPageToken ?? "";
  } while (token !== "");
  return latest?.snapshot;
}

/** The document's text in the last published version of the plugin, formatted as the editor shows it. */
export function usePublishedLoader(pluginId: string): LoadPublished {
  const { session } = useApp();
  return useCallback(
    async (path) => {
      const snapshot = await latestSnapshot(createClient(ReleaseService, session.transport), pluginId);
      if (snapshot === undefined) {
        return null;
      }
      const res = await createClient(DocumentService, session.transport).getSnapshot({ id: snapshot, paths: [path] });
      const doc = res.documents.find((d) => d.path === path);
      return doc === undefined
        ? null
        : `${JSON.stringify(JSON.parse(new TextDecoder().decode(doc.content)), null, 2)}\n`;
    },
    [session, pluginId],
  );
}
