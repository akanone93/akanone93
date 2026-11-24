// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { createClient } from "@connectrpc/connect";
import { PluginService } from "@plux/api";
import { useQuery } from "@tanstack/react-query";
import { useApp } from "../../lib/use-app.tsx";

/** How often the app lock's holder is read again while the app canvas is open: the lock's heartbeat. */
const APP_LOCK_REFRESH_MS = 30_000;

/**
 * Who holds the app's own lock — app.json, the theme, the translation keys (SRV-042) — for the app canvas's Overview
 * (#267); empty while nobody does or it cannot be read.
 */
export function useAppLockHolder(appId: string): string {
  const { session } = useApp();
  const lock = useQuery({
    queryKey: ["app-lock", appId],
    enabled: appId !== "",
    refetchInterval: APP_LOCK_REFRESH_MS,
    queryFn: async () => (await createClient(PluginService, session.transport).getLock({ appId, pluginId: "" })).lock,
  });
  const holder = lock.data?.holder;
  return holder === undefined ? "" : holder.display === "" ? holder.id : holder.display;
}
