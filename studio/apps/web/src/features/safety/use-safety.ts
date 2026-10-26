// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { AppService, PluginService } from "@plux/api";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { useApp } from "../../lib/use-app.tsx";
import type { EditSession } from "../editor/edit-session.ts";
import { flushed } from "../editor/editor-provider.tsx";
import type { LockController } from "../editor/lock-controller.ts";
import { uuidv7 } from "../plugin-canvas/screens.ts";
import { applyChangeSet, type ChangeSet } from "./change-set.ts";

/**
 * Whether any production environment of the app serves a release (STU-074): destructive actions on
 * such an app need the type-to-confirm dialog. Unknown while loading counts as no.
 */
export function useProductionRelease(appId: string): boolean {
  const { session } = useApp();
  const query = useQuery({
    queryKey: ["production-release", appId],
    enabled: appId !== "",
    queryFn: async () => {
      const apps = createClient(AppService, session.transport);
      const envs = (await apps.listEnvironments({ appId })).environments.filter((e) => e.production);
      const lists = await Promise.all(envs.map((e) => apps.listChannels({ environmentId: e.id })));
      return lists.some((l) => l.channels.some((c) => c.releaseSequence > 0n));
    },
  });
  return query.data === true;
}

/** The editor of the open plugin, whose lock session the change set names for that draft. */
export interface OpenPlugin {
  readonly pluginId: string;
  readonly lock: LockController;
  readonly edit: EditSession;
}

/**
 * Applies a change set (F3, §4.3 #15): pending edits are flushed first so every revision it tests is
 * current, the open plugin's lock session is used for its draft and every other draft's lock is taken
 * for the call and released after. Resolves false when the server refuses it, leaving nothing written.
 */
export function useChangeSets(appId: string, editorSessionId: string, open: OpenPlugin | undefined) {
  const { session } = useApp();
  const queryClient = useQueryClient();
  return useCallback(
    async (changeSet: ChangeSet): Promise<boolean> => {
      const plugins = createClient(PluginService, session.transport);
      const taken: { pluginId: string; session: string }[] = [];
      try {
        if (open !== undefined) {
          await flushed(open.edit);
        }
        await applyChangeSet(
          session.transport,
          { appId, editorSessionId, stepId: uuidv7() },
          changeSet,
          async (pluginId) => {
            if (open !== undefined && pluginId === open.pluginId) {
              await open.lock.ensure();
              return open.lock.session;
            }
            const lockSession = uuidv7();
            await plugins.acquireLock({ appId, pluginId, session: lockSession });
            taken.push({ pluginId, session: lockSession });
            return lockSession;
          },
        );
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: ["plugin-documents", appId] }),
          queryClient.invalidateQueries({ queryKey: ["project-documents", appId] }),
        ]);
        return true;
      } catch {
        return false;
      } finally {
        await Promise.all(
          taken.map((t) => plugins.releaseLock({ appId, pluginId: t.pluginId, session: t.session }).catch(() => null)),
        );
      }
    },
    [session.transport, queryClient, appId, editorSessionId, open],
  );
}
