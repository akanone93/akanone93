// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { type Client, createClient } from "@connectrpc/connect";
import { AssetService } from "@plux/api";
import { useQueryClient } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { useMemo, useRef } from "react";
import { useApp } from "../../lib/use-app.tsx";
import { type LockController, LockUnavailable } from "../editor/lock-controller.ts";
import { useAppSession } from "../localisation/index.ts";

type AssetClient = Client<typeof AssetService>;

/**
 * The asset client with its writes sent under the app's editing lock. Assets are the app's documents
 * (`assets/index.json`), so the server takes an upload or a delete only from the session that holds the app's lock
 * (SRV-040, SRV-060), not from the tab's editor session that tags edits for undo. The lock is taken when it is free,
 * as by the first edit; a write is refused when another tab or person holds it, or before the app session is ready.
 */
export function underAppLock(client: AssetClient, lock: () => LockController | undefined): AssetClient {
  const session = async (): Promise<string> => {
    const l = lock();
    if (l === undefined) {
      throw new LockUnavailable();
    }
    return l.whenHeld();
  };
  return {
    ...client,
    beginAssetUpload: async (req, options) => client.beginAssetUpload({ ...req, session: await session() }, options),
    deleteAsset: async (req, options) => client.deleteAsset({ ...req, session: await session() }, options),
  };
}

/** What the left dock's Assets tab needs: the asset client of the signed-in session, the app and the editor session. */
export function useAssetsTab(editorSessionId: string) {
  const { session } = useApp();
  const { app } = useParams({ strict: false }) as { app?: string };
  const queries = useQueryClient();
  // The app session arrives after the canvas; the client reads the latest one at each write.
  const appLock = useRef<LockController | undefined>(undefined);
  appLock.current = useAppSession()?.lock;
  return useMemo(
    () => ({
      client: underAppLock(createClient(AssetService, session.transport), () => appLock.current),
      appId: app ?? "",
      session: editorSessionId,
      // The canvas's images and the image picker read the same list.
      onChanged: () => void queries.invalidateQueries({ queryKey: ["assets", app ?? ""] }),
    }),
    [session, app, editorSessionId, queries],
  );
}
