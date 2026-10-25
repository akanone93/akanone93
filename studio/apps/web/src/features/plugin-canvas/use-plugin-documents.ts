// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { DocumentService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";

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
      return res.documents;
    },
  });
}
