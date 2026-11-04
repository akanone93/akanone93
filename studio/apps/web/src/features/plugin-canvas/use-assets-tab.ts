// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { AssetService } from "@plux/api";
import { useQueryClient } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { useMemo } from "react";
import { useApp } from "../../lib/use-app.tsx";

/** What the left dock's Assets tab needs: the asset client of the signed-in session, the app and the editor session. */
export function useAssetsTab(editorSessionId: string) {
  const { session } = useApp();
  const { app } = useParams({ strict: false }) as { app?: string };
  const queries = useQueryClient();
  return useMemo(
    () => ({
      client: createClient(AssetService, session.transport),
      appId: app ?? "",
      session: editorSessionId,
      // The canvas's images and the image picker read the same list.
      onChanged: () => void queries.invalidateQueries({ queryKey: ["assets", app ?? ""] }),
    }),
    [session, app, editorSessionId, queries],
  );
}
